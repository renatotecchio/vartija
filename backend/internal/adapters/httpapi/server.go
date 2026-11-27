// Package httpapi is the REST API (Gin) and serves the React build.
package httpapi

import (
	"context"
	"embed"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"net/netip"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/renatotecchio/vartija/backend/internal/app"
	"github.com/renatotecchio/vartija/backend/internal/domain"
	"github.com/renatotecchio/vartija/backend/internal/ports"
)

// dist holds the frontend build (frontend/dist, copied here by the
// Dockerfile and `make build`). Only a .gitkeep is committed.
//
//go:embed all:dist
var dist embed.FS

const (
	cookieName = "vartija_session"
	// csrfHeader must be on every state-changing request. A cross-site form
	// can't set a custom header, and a cross-site fetch with one needs a
	// CORS preflight this API never grants.
	csrfHeader = "X-Vartija"
)

// Deps are the use cases the API exposes.
type Deps struct {
	Auth    *app.AuthService
	Devices *app.DeviceService
	Subnets *app.SubnetService
	Scan    *app.ScanService
	Events  ports.EventRepository
	Ready   func(context.Context) error
	Log     *slog.Logger
	// SecureCookie sets the cookie's Secure flag (behind HTTPS).
	SecureCookie bool
	// TrustedProxies are the reverse proxies whose X-Forwarded-For is
	// believed (the login rate limit is per client IP).
	TrustedProxies []string
}

// New returns the HTTP handler.
func New(d Deps) (http.Handler, error) {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery(), requestLog(d.Log), securityHeaders())
	if err := r.SetTrustedProxies(d.TrustedProxies); err != nil {
		return nil, err
	}

	h := &handlers{Deps: d, limiter: newLimiter(10, 15*time.Minute)}

	r.GET("/healthz", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	r.GET("/readyz", h.ready)

	api := r.Group("/api")
	api.Use(csrf())
	api.POST("/login", h.login)
	api.POST("/logout", h.logout)

	auth := api.Group("")
	auth.Use(h.requireSession)
	auth.GET("/me", h.me)
	auth.GET("/summary", h.summary)
	auth.GET("/devices", h.listDevices)
	auth.PATCH("/devices/:id", h.updateDevice)
	auth.DELETE("/devices/:id", h.deleteDevice)
	auth.POST("/devices/acknowledge", h.acknowledge)
	auth.GET("/subnets", h.listSubnets)
	auth.POST("/subnets", h.createSubnet)
	auth.PATCH("/subnets/:id", h.updateSubnet)
	auth.DELETE("/subnets/:id", h.deleteSubnet)
	auth.POST("/subnets/:id/scan", h.scanSubnet)
	auth.POST("/scan", h.scanAll)
	auth.GET("/events", h.listEvents)

	static, err := fs.Sub(dist, "dist")
	if err != nil {
		return nil, err
	}
	r.NoRoute(spa(static))
	return r, nil
}

type handlers struct {
	Deps
	limiter *limiter
}

// --- middleware ---

func requestLog(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		if c.Request.URL.Path == "/healthz" || c.Request.URL.Path == "/readyz" {
			return
		}
		log.Info("http", "method", c.Request.Method, "path", c.Request.URL.Path,
			"status", c.Writer.Status(), "ip", c.ClientIP(), "took", time.Since(start).Round(time.Millisecond))
	}
}

func securityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		hd := c.Writer.Header()
		hd.Set("X-Content-Type-Options", "nosniff")
		hd.Set("X-Frame-Options", "DENY")
		hd.Set("Referrer-Policy", "same-origin")
		hd.Set("Content-Security-Policy",
			"default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		c.Next()
	}
}

func csrf() gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			if c.GetHeader(csrfHeader) == "" {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "missing " + csrfHeader + " header"})
				return
			}
		}
		c.Next()
	}
}

func (h *handlers) requireSession(c *gin.Context) {
	cookie, err := c.Cookie(cookieName)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "login required"})
		return
	}
	user, err := h.Auth.Verify(cookie)
	if err != nil {
		h.clearCookie(c)
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "login required"})
		return
	}
	c.Set("user", user)
	c.Next()
}

// --- auth ---

func (h *handlers) login(c *gin.Context) {
	var in struct {
		User     string `json:"user" binding:"required,max=100"`
		Password string `json:"password" binding:"required,max=200"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user and password are required"})
		return
	}
	ip := c.ClientIP()
	if !h.limiter.allow(ip) {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "too many failed logins, try again later"})
		return
	}
	token, exp, err := h.Auth.Login(in.User, in.Password)
	if err != nil {
		h.limiter.fail(ip)
		h.Log.Warn("login failed", "user", in.User, "ip", ip)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user or password"})
		return
	}
	h.limiter.reset(ip)
	http.SetCookie(c.Writer, &http.Cookie{
		Name: cookieName, Value: token, Path: "/", Expires: exp,
		HttpOnly: true, Secure: h.SecureCookie, SameSite: http.SameSiteLaxMode,
	})
	h.Log.Info("login", "user", in.User, "ip", ip)
	c.JSON(http.StatusOK, gin.H{"user": in.User})
}

func (h *handlers) logout(c *gin.Context) {
	h.clearCookie(c)
	c.Status(http.StatusNoContent)
}

func (h *handlers) clearCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name: cookieName, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: h.SecureCookie, SameSite: http.SameSiteLaxMode,
	})
}

func (h *handlers) me(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"user": c.GetString("user")})
}

func (h *handlers) ready(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	if err := h.Ready(ctx); err != nil {
		c.String(http.StatusServiceUnavailable, "database unavailable")
		return
	}
	c.String(http.StatusOK, "ok")
}

// --- devices ---

type deviceDTO struct {
	ID             uint       `json:"id"`
	MAC            string     `json:"mac"`
	Nickname       string     `json:"nickname"`
	Notes          string     `json:"notes"`
	Hostname       string     `json:"hostname"`
	HostnameSource string     `json:"hostname_source"`
	DisplayName    string     `json:"display_name"`
	IP             string     `json:"ip"`
	SubnetID       uint       `json:"subnet_id"`
	Status         string     `json:"status"`
	RandomMAC      bool       `json:"random_mac"`
	FirstSeen      time.Time  `json:"first_seen"`
	LastSeen       time.Time  `json:"last_seen"`
	AcknowledgedAt *time.Time `json:"acknowledged_at"`
}

func toDeviceDTO(d domain.Device) deviceDTO {
	return deviceDTO{
		ID: d.ID, MAC: d.MAC, Nickname: d.Nickname, Notes: d.Notes,
		Hostname: d.Hostname, HostnameSource: d.HostnameSource, DisplayName: d.DisplayName(),
		IP: d.IP, SubnetID: d.SubnetID, Status: string(d.Status), RandomMAC: d.RandomMAC,
		FirstSeen: d.FirstSeen, LastSeen: d.LastSeen, AcknowledgedAt: d.AcknowledgedAt,
	}
}

func (h *handlers) listDevices(c *gin.Context) {
	subnetID, _ := strconv.ParseUint(c.Query("subnet_id"), 10, 32)
	list, err := h.Devices.List(c.Request.Context(), ports.DeviceFilter{
		Status:   domain.DeviceStatus(c.Query("status")),
		SubnetID: uint(subnetID),
		Search:   c.Query("q"),
	})
	if err != nil {
		h.fail(c, err)
		return
	}
	out := make([]deviceDTO, len(list))
	for i, d := range list {
		out[i] = toDeviceDTO(d)
	}
	c.JSON(http.StatusOK, out)
}

func (h *handlers) updateDevice(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	var in struct {
		Nickname *string `json:"nickname"`
		Notes    *string `json:"notes"`
		Status   *string `json:"status"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON"})
		return
	}
	u := app.DeviceUpdate{Nickname: in.Nickname, Notes: in.Notes}
	if in.Status != nil {
		st := domain.DeviceStatus(*in.Status)
		u.Status = &st
	}
	d, err := h.Devices.Update(c.Request.Context(), id, u)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, toDeviceDTO(d))
}

func (h *handlers) deleteDevice(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	if err := h.Devices.Delete(c.Request.Context(), id); err != nil {
		h.fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *handlers) acknowledge(c *gin.Context) {
	var in struct {
		IDs []uint `json:"ids"`
		All bool   `json:"all"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON"})
		return
	}
	// An empty list only means "all" when asked for: a UI bug sending []
	// must not acknowledge every device
	if len(in.IDs) == 0 && !in.All {
		c.JSON(http.StatusBadRequest, gin.H{"error": "give ids or all=true"})
		return
	}
	if in.All {
		in.IDs = nil
	}
	n, err := h.Devices.Acknowledge(c.Request.Context(), in.IDs)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"acknowledged": n})
}

func (h *handlers) summary(c *gin.Context) {
	counts, err := h.Devices.Counts(c.Request.Context())
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"new":      counts[domain.StatusNew],
		"known":    counts[domain.StatusKnown],
		"scanning": h.Scan.Running(),
	})
}

// --- subnets ---

type subnetDTO struct {
	ID            uint       `json:"id"`
	CIDR          string     `json:"cidr"`
	Name          string     `json:"name"`
	Enabled       bool       `json:"enabled"`
	Baselined     bool       `json:"baselined"`
	LastScanAt    *time.Time `json:"last_scan_at"`
	LastScanError string     `json:"last_scan_error"`
	LastScanFound int        `json:"last_scan_found"`
	// Via is the discoverer that sweeps it now (arp, mikrotik) or "".
	Via string `json:"via"`
}

func (h *handlers) toSubnetDTO(ctx context.Context, s domain.Subnet) subnetDTO {
	dto := subnetDTO{
		ID: s.ID, CIDR: s.CIDR, Name: s.Name, Enabled: s.Enabled, Baselined: s.Baselined,
		LastScanAt: s.LastScanAt, LastScanError: s.LastScanError, LastScanFound: s.LastScanFound,
	}
	if p, err := netip.ParsePrefix(s.CIDR); err == nil {
		dto.Via = h.Scan.Coverage(ctx, p)
	}
	return dto
}

func (h *handlers) listSubnets(c *gin.Context) {
	list, err := h.Subnets.List(c.Request.Context())
	if err != nil {
		h.fail(c, err)
		return
	}
	out := make([]subnetDTO, len(list))
	for i, s := range list {
		out[i] = h.toSubnetDTO(c.Request.Context(), s)
	}
	c.JSON(http.StatusOK, out)
}

func (h *handlers) createSubnet(c *gin.Context) {
	var in struct {
		CIDR string `json:"cidr" binding:"required"`
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cidr is required"})
		return
	}
	s, err := h.Subnets.Create(c.Request.Context(), in.CIDR, in.Name)
	if err != nil {
		h.fail(c, err)
		return
	}
	// First sweep right away, in the background
	h.background(func(ctx context.Context) { _ = h.Scan.ScanSubnet(ctx, s.ID) })
	c.JSON(http.StatusCreated, h.toSubnetDTO(c.Request.Context(), s))
}

func (h *handlers) updateSubnet(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	var in struct {
		Name    *string `json:"name"`
		Enabled *bool   `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON"})
		return
	}
	s, err := h.Subnets.Update(c.Request.Context(), id, app.SubnetUpdate{Name: in.Name, Enabled: in.Enabled})
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, h.toSubnetDTO(c.Request.Context(), s))
}

func (h *handlers) deleteSubnet(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	if err := h.Subnets.Delete(c.Request.Context(), id); err != nil {
		h.fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *handlers) scanSubnet(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	if _, err := h.Subnets.Get(c.Request.Context(), id); err != nil {
		h.fail(c, err)
		return
	}
	if h.Scan.Running() {
		c.JSON(http.StatusConflict, gin.H{"error": app.ErrScanRunning.Error()})
		return
	}
	h.background(func(ctx context.Context) { _ = h.Scan.ScanSubnet(ctx, id) })
	c.JSON(http.StatusAccepted, gin.H{"status": "started"})
}

func (h *handlers) scanAll(c *gin.Context) {
	if h.Scan.Running() {
		c.JSON(http.StatusConflict, gin.H{"error": app.ErrScanRunning.Error()})
		return
	}
	h.background(func(ctx context.Context) { _ = h.Scan.TryScanAll(ctx) })
	c.JSON(http.StatusAccepted, gin.H{"status": "started"})
}

// background runs a scan detached from the request, with its own timeout.
func (h *handlers) background(fn func(context.Context)) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		fn(ctx)
	}()
}

// --- events ---

type eventDTO struct {
	ID       uint      `json:"id"`
	Type     string    `json:"type"`
	DeviceID *uint     `json:"device_id"`
	SubnetID *uint     `json:"subnet_id"`
	Message  string    `json:"message"`
	At       time.Time `json:"at"`
}

func (h *handlers) listEvents(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "200"))
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	list, err := h.Events.List(c.Request.Context(), limit)
	if err != nil {
		h.fail(c, err)
		return
	}
	out := make([]eventDTO, len(list))
	for i, e := range list {
		out[i] = eventDTO{ID: e.ID, Type: string(e.Type), DeviceID: e.DeviceID, SubnetID: e.SubnetID, Message: e.Message, At: e.At}
	}
	c.JSON(http.StatusOK, out)
}

// --- helpers ---

func idParam(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return 0, false
	}
	return uint(id), true
}

func (h *handlers) fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ports.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
	case errors.Is(err, ports.ErrConflict):
		c.JSON(http.StatusConflict, gin.H{"error": "already exists"})
	case errors.Is(err, app.ErrValidation):
		c.JSON(http.StatusBadRequest, gin.H{"error": strings.TrimPrefix(err.Error(), "validation: ")})
	default:
		h.Log.Error("request failed", "path", c.Request.URL.Path, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
	}
}

// spa serves the built files, and index.html for any other path (client
// side routes). /api/* never falls back to the page.
func spa(static fs.FS) gin.HandlerFunc {
	fileServer := http.FileServer(http.FS(static))
	return func(c *gin.Context) {
		p := c.Request.URL.Path
		if strings.HasPrefix(p, "/api/") || (c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		name := strings.TrimPrefix(path.Clean(p), "/")
		if name == "" {
			name = "index.html"
		}
		if _, err := fs.Stat(static, name); err != nil {
			name = "index.html"
		}
		if name == "index.html" {
			index, err := fs.ReadFile(static, "index.html")
			if err != nil {
				c.String(http.StatusNotFound, "frontend not built")
				return
			}
			c.Header("Cache-Control", "no-cache")
			c.Data(http.StatusOK, "text/html; charset=utf-8", index)
			return
		}
		if strings.HasPrefix(name, "assets/") {
			c.Header("Cache-Control", "public, max-age=31536000, immutable")
		}
		c.Request.URL.Path = "/" + name
		fileServer.ServeHTTP(c.Writer, c.Request)
	}
}

// --- login rate limit ---

// limiter blocks an IP after max failed logins within window.
type limiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	fails  map[string][]time.Time
}

func newLimiter(max int, window time.Duration) *limiter {
	return &limiter{max: max, window: window, fails: map[string][]time.Time{}}
}

func (l *limiter) recent(ip string) []time.Time {
	cut := time.Now().Add(-l.window)
	var keep []time.Time
	for _, t := range l.fails[ip] {
		if t.After(cut) {
			keep = append(keep, t)
		}
	}
	if len(keep) == 0 {
		delete(l.fails, ip)
	} else {
		l.fails[ip] = keep
	}
	return keep
}

func (l *limiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.recent(ip)) < l.max
}

func (l *limiter) fail(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.fails[ip] = append(l.recent(ip), time.Now())
}

func (l *limiter) reset(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.fails, ip)
}
