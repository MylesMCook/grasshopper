package gomcp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"github.com/MylesMCook/grasshopper/internal/gomemory"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const visualizerCookieName = "grasshopper_view"
const visualizerSessionLifetime = 7 * 24 * time.Hour

func visualizerOrigin(r *http.Request, allowedProxyHost string) string {
	scheme := "http"
	if proxyHostMatches(r.Host, allowedProxyHost) {
		scheme = "https"
	}
	return canonicalHTTPSOrigin(scheme + "://" + r.Host)
}

// Session signatures are keyed by the master token hash. Authentication also
// requires the cookie hash to exist in SQLite; signatures alone grant no access.
func newVisualizerSession(key []byte, expires time.Time) (string, error) {
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	payload := "v1." + strconv.FormatInt(expires.Unix(), 10) + "." + base64.RawURLEncoding.EncodeToString(nonce)
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte("grasshopper-visualizer-session\x00" + payload))
	return payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func validVisualizerSession(value string, key []byte, now time.Time) bool {
	parts := strings.Split(value, ".")
	if len(parts) != 4 || parts[0] != "v1" {
		return false
	}
	expires, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || expires <= now.Unix() || expires > now.Add(visualizerSessionLifetime).Unix() {
		return false
	}
	nonce, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(nonce) != 16 || base64.RawURLEncoding.EncodeToString(nonce) != parts[2] {
		return false
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[3])
	if err != nil || len(signature) != sha256.Size || base64.RawURLEncoding.EncodeToString(signature) != parts[3] {
		return false
	}
	payload := strings.Join(parts[:3], ".")
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte("grasshopper-visualizer-session\x00" + payload))
	return hmac.Equal(signature, mac.Sum(nil))
}

func visualizerCookie(value string, secure bool, age int) *http.Cookie {
	return &http.Cookie{
		Name: visualizerCookieName, Value: value, Path: "/visualizer/api/",
		MaxAge: age, HttpOnly: true, Secure: secure, SameSite: http.SameSiteStrictMode,
	}
}

func (sessions *ownerSessions) serve(w http.ResponseWriter, r *http.Request, key []byte, bearerValid bool, allowedProxyHost string) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	secure := proxyHostMatches(r.Host, allowedProxyHost)
	if origin := r.Header.Get("Origin"); origin != "" && !originsMatch(origin, visualizerOrigin(r, allowedProxyHost)) {
		http.Error(w, "invalid Origin header", http.StatusForbidden)
		return
	}
	switch r.Method {
	case http.MethodGet:
		cookie, err := r.Cookie(visualizerCookieName)
		connected := err == nil && sessions.valid(r, cookie.Value, key)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if connected {
			_, _ = w.Write([]byte("{\"connected\":true}\n"))
		} else {
			_, _ = w.Write([]byte("{\"connected\":false}\n"))
		}
	case http.MethodPost:
		if !originsMatch(r.Header.Get("Origin"), visualizerOrigin(r, allowedProxyHost)) {
			http.Error(w, "invalid Origin header", http.StatusForbidden)
			return
		}
		if !bearerValid && !sessions.allowLogin(r) {
			w.Header().Set("Retry-After", "300")
			http.Error(w, "too many sign-in attempts", http.StatusTooManyRequests)
			return
		}
		if !bearerValid {
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		sessions.resetLogin(r)
		expires := time.Now().Add(visualizerSessionLifetime)
		value, err := newVisualizerSession(key, expires)
		if err == nil {
			err = sessions.store.AddOwnerSession(r.Context(), value, expires)
		}
		if err != nil {
			http.Error(w, "session unavailable", http.StatusInternalServerError)
			return
		}
		sessions.audit.event("owner_session_created", r, auditFields{})
		http.SetCookie(w, visualizerCookie(value, secure, int(visualizerSessionLifetime.Seconds())))
		w.WriteHeader(http.StatusNoContent)
	case http.MethodDelete:
		if !originsMatch(r.Header.Get("Origin"), visualizerOrigin(r, allowedProxyHost)) {
			http.Error(w, "invalid Origin header", http.StatusForbidden)
			return
		}
		value := ""
		if cookie, err := r.Cookie(visualizerCookieName); err == nil {
			value = cookie.Value
		}
		if err := sessions.store.RevokeOwnerSession(r.Context(), value, false); err != nil {
			http.Error(w, "session unavailable", 503)
			return
		}
		sessions.audit.event("owner_session_ended", r, auditFields{})
		http.SetCookie(w, visualizerCookie("", secure, -1))
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// Login attempts use the actual peer IP, never spoofable forwarded headers.
// State is capped and expired windows are discarded; no request sleeps.
type loginWindow struct {
	start time.Time
	count int
}
type ownerSessions struct {
	store    *gomemory.Writer
	audit    *auditLog
	mu       sync.Mutex
	attempts map[string]loginWindow
}

func (sessions *ownerSessions) valid(r *http.Request, value string, key []byte) bool {
	if !validVisualizerSession(value, key, time.Now()) {
		return false
	}
	valid, err := sessions.store.OwnerSessionValid(r.Context(), value, time.Now())
	return err == nil && valid
}
func (sessions *ownerSessions) allowLogin(r *http.Request) bool {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	now := time.Now()
	sessions.mu.Lock()
	defer sessions.mu.Unlock()
	for peer, window := range sessions.attempts {
		if now.Sub(window.start) >= 5*time.Minute {
			delete(sessions.attempts, peer)
		}
	}
	window, exists := sessions.attempts[ip]
	if !exists {
		if len(sessions.attempts) >= 1024 {
			return false
		}
		window.start = now
	}
	if window.count >= 10 {
		return false
	}
	window.count++
	sessions.attempts[ip] = window
	return true
}
func (sessions *ownerSessions) revokeAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	if err := sessions.store.RevokeOwnerSession(r.Context(), "", true); err != nil {
		http.Error(w, "session unavailable", 503)
		return
	}
	sessions.audit.event("owner_session_ended", r, auditFields{everywhere: true})
	http.SetCookie(w, visualizerCookie("", r.TLS != nil || strings.HasPrefix(r.Header.Get("Origin"), "https://"), -1))
	w.WriteHeader(http.StatusNoContent)
}

func (sessions *ownerSessions) resetLogin(r *http.Request) {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	sessions.mu.Lock()
	defer sessions.mu.Unlock()
	delete(sessions.attempts, ip)
}
