package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/renatotecchio/vartija/backend/internal/app"
)

func newTestServer(t *testing.T) http.Handler {
	t.Helper()
	hash, err := app.HashPassword("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	auth, err := app.NewAuthService(map[string][]byte{"admin": hash}, []byte(strings.Repeat("k", 32)), time.Hour, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h, err := New(Deps{
		Auth:  auth,
		Ready: func(context.Context) error { return nil },
		Log:   log,
		Scan:  app.NewScanService(nil, nil, nil, nil, nil, nil, nil, log),
	})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func do(h http.Handler, method, path, body string, header map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range header {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestLoginFlow(t *testing.T) {
	h := newTestServer(t)
	csrf := map[string]string{csrfHeader: "1"}

	if w := do(h, "GET", "/api/me", "", nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("/api/me without session: %d", w.Code)
	}
	if w := do(h, "POST", "/api/login", `{"user":"admin","password":"correct horse battery"}`, nil); w.Code != http.StatusForbidden {
		t.Fatalf("login without the CSRF header: %d", w.Code)
	}
	if w := do(h, "POST", "/api/login", `{"user":"admin","password":"nope"}`, csrf); w.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password: %d", w.Code)
	}
	w := do(h, "POST", "/api/login", `{"user":"admin","password":"correct horse battery"}`, csrf)
	if w.Code != http.StatusOK {
		t.Fatalf("login: %d %s", w.Code, w.Body)
	}
	cookie := w.Result().Cookies()[0]
	if cookie.Name != cookieName || !cookie.HttpOnly {
		t.Fatalf("cookie: %+v", cookie)
	}
	w = do(h, "GET", "/api/me", "", map[string]string{"Cookie": cookie.Name + "=" + cookie.Value})
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "admin") {
		t.Fatalf("/api/me with session: %d %s", w.Code, w.Body)
	}
}

func TestLoginRateLimit(t *testing.T) {
	h := newTestServer(t)
	csrf := map[string]string{csrfHeader: "1"}
	for i := 0; i < 10; i++ {
		do(h, "POST", "/api/login", `{"user":"admin","password":"nope"}`, csrf)
	}
	w := do(h, "POST", "/api/login", `{"user":"admin","password":"correct horse battery"}`, csrf)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("want 429 after 10 failures, got %d", w.Code)
	}
}

func TestSPAFallbackAndAPI404(t *testing.T) {
	h := newTestServer(t)
	if w := do(h, "GET", "/api/nope", "", nil); w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "not found") {
		t.Fatalf("/api/nope: %d %s", w.Code, w.Body)
	}
	if w := do(h, "GET", "/healthz", "", nil); w.Code != http.StatusOK {
		t.Fatalf("/healthz: %d", w.Code)
	}
	w := do(h, "GET", "/devices", "", nil)
	if w.Header().Get("Content-Security-Policy") == "" {
		t.Error("missing CSP header")
	}
}
