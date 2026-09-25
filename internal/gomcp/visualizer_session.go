package gomcp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const visualizerCookieName = "grasshopper_view"
const visualizerSessionLifetime = 30 * 24 * time.Hour

func visualizerOrigin(r *http.Request, allowedProxyHost string) string {
	scheme := "http"
	if allowedProxyHost != "" && r.Host == allowedProxyHost {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

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
	if err != nil || len(nonce) != 16 {
		return false
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[3])
	if err != nil || len(signature) != sha256.Size {
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

func visualizerSession(w http.ResponseWriter, r *http.Request, key []byte, bearerValid bool, allowedProxyHost string) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	secure := allowedProxyHost != "" && r.Host == allowedProxyHost
	if origin := r.Header.Get("Origin"); origin != "" && origin != visualizerOrigin(r, allowedProxyHost) {
		http.Error(w, "invalid Origin header", http.StatusForbidden)
		return
	}
	switch r.Method {
	case http.MethodGet:
		cookie, err := r.Cookie(visualizerCookieName)
		connected := err == nil && validVisualizerSession(cookie.Value, key, time.Now())
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if connected {
			_, _ = w.Write([]byte("{\"connected\":true}\n"))
		} else {
			_, _ = w.Write([]byte("{\"connected\":false}\n"))
		}
	case http.MethodPost:
		if !bearerValid {
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		value, err := newVisualizerSession(key, time.Now().Add(visualizerSessionLifetime))
		if err != nil {
			http.Error(w, "session unavailable", http.StatusInternalServerError)
			return
		}
		http.SetCookie(w, visualizerCookie(value, secure, int(visualizerSessionLifetime.Seconds())))
		w.WriteHeader(http.StatusNoContent)
	case http.MethodDelete:
		if r.Header.Get("Origin") != visualizerOrigin(r, allowedProxyHost) {
			http.Error(w, "invalid Origin header", http.StatusForbidden)
			return
		}
		http.SetCookie(w, visualizerCookie("", secure, -1))
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
