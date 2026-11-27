package app

import (
	"bufio"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// ErrBadCredentials is returned for a wrong user or password.
var ErrBadCredentials = errors.New("invalid user or password")

// ErrBadSession is returned for a missing, tampered or expired session.
var ErrBadSession = errors.New("invalid session")

// AuthService checks logins against bcrypt hashes (an htpasswd file) and
// issues signed session tokens. There is no user table: Vartija is a local
// tool with one or a few operators, listed in the htpasswd file.
type AuthService struct {
	users  map[string][]byte // user -> bcrypt hash
	secret []byte
	ttl    time.Duration
	now    func() time.Time
	// A real bcrypt hash compared when the user doesn't exist, so a wrong
	// user takes as long as a wrong password.
	dummy []byte
}

// NewAuthService needs at least one user and a secret of 32 bytes or more.
func NewAuthService(users map[string][]byte, secret []byte, ttl time.Duration, now func() time.Time) (*AuthService, error) {
	if len(users) == 0 {
		return nil, errors.New("no users: set VARTIJA_HTPASSWD_FILE or VARTIJA_ADMIN_PASSWORD")
	}
	if len(secret) < 32 {
		return nil, errors.New("the session secret must have at least 32 bytes")
	}
	dummy, err := bcrypt.GenerateFromPassword([]byte("vartija-dummy"), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	return &AuthService{users: users, secret: secret, ttl: ttl, now: now, dummy: dummy}, nil
}

// Login checks the password and returns a session token.
func (a *AuthService) Login(user, password string) (string, time.Time, error) {
	hash, ok := a.users[user]
	if !ok {
		_ = bcrypt.CompareHashAndPassword(a.dummy, []byte(password))
		return "", time.Time{}, ErrBadCredentials
	}
	if bcrypt.CompareHashAndPassword(hash, []byte(password)) != nil {
		return "", time.Time{}, ErrBadCredentials
	}
	exp := a.now().Add(a.ttl)
	return a.sign(user, exp), exp, nil
}

// Verify returns the user of a valid session token.
func (a *AuthService) Verify(token string) (string, error) {
	payload, sig, ok := strings.Cut(token, ".")
	if !ok {
		return "", ErrBadSession
	}
	want := a.mac(payload)
	got, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil || subtle.ConstantTimeCompare(want, got) != 1 {
		return "", ErrBadSession
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return "", ErrBadSession
	}
	user, expStr, ok := strings.Cut(string(raw), "|")
	if !ok {
		return "", ErrBadSession
	}
	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil || a.now().Unix() >= exp {
		return "", ErrBadSession
	}
	// A user removed from the htpasswd file loses its sessions on restart
	if _, ok := a.users[user]; !ok {
		return "", ErrBadSession
	}
	return user, nil
}

func (a *AuthService) sign(user string, exp time.Time) string {
	payload := base64.RawURLEncoding.EncodeToString([]byte(user + "|" + strconv.FormatInt(exp.Unix(), 10)))
	return payload + "." + base64.RawURLEncoding.EncodeToString(a.mac(payload))
}

func (a *AuthService) mac(payload string) []byte {
	h := hmac.New(sha256.New, a.secret)
	h.Write([]byte("vartija-session|" + payload))
	return h.Sum(nil)
}

// ParseHtpasswd reads an htpasswd file (user:hash per line). Only bcrypt
// hashes are accepted ($2y$, $2a$, $2b$: htpasswd -B); MD5/SHA1/crypt lines
// are rejected with an error, so a weak file never half works.
func ParseHtpasswd(r io.Reader) (map[string][]byte, error) {
	users := map[string][]byte{}
	sc := bufio.NewScanner(r)
	line := 0
	for sc.Scan() {
		line++
		text := strings.TrimSpace(sc.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		user, hash, ok := strings.Cut(text, ":")
		if !ok || user == "" {
			return nil, fmt.Errorf("htpasswd line %d: expected user:hash", line)
		}
		if !strings.HasPrefix(hash, "$2y$") && !strings.HasPrefix(hash, "$2a$") && !strings.HasPrefix(hash, "$2b$") {
			return nil, fmt.Errorf("htpasswd line %d (%s): only bcrypt hashes are supported (htpasswd -B)", line, user)
		}
		if _, err := bcrypt.Cost([]byte(hash)); err != nil {
			return nil, fmt.Errorf("htpasswd line %d (%s): %v", line, user, err)
		}
		users[user] = []byte(hash)
	}
	return users, sc.Err()
}

// HashPassword returns a bcrypt hash, for a user given in plain text in the
// environment (VARTIJA_ADMIN_PASSWORD).
func HashPassword(password string) ([]byte, error) {
	if len(password) < 12 {
		return nil, errors.New("the admin password must have at least 12 characters")
	}
	return bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
}
