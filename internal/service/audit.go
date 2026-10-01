package service

import (
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"
)

// auditLog accepts only typed, allowlisted operational metadata. It never sees
// request bodies, authorization headers, cookie values, memory text or errors.
type auditLog struct {
	logger   *slog.Logger
	mu       sync.Mutex
	failures map[string]auditWindow
	now      func() time.Time
}
type auditWindow struct {
	until      time.Time
	suppressed int
}
type auditFields struct {
	action       string
	id, revision int64
	requestID    string
	everywhere   bool
	status       int
}

func newAuditLog(logger *slog.Logger) *auditLog {
	return &auditLog{logger: logger, failures: map[string]auditWindow{}, now: time.Now}
}
func auditRoute(r *http.Request) string {
	switch r.URL.Path {
	case "/mcp", "/healthz", "/connection", "/pair/start", "/pair/poll", "/visualizer/api/session", "/visualizer/api/session/revoke-all", "/visualizer/api/context", "/visualizer/api/search", "/visualizer/api/record", "/visualizer/api/update", "/visualizer/api/archive", "/visualizer/api/move", "/visualizer/api/startup", "/visualizer/api/export", "/visualizer/api/pairings", "/visualizer/api/devices":
		return r.URL.Path
	}
	return "other"
}
func auditPeer(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.String()
	}
	return "unknown"
}
func (audit *auditLog) event(name string, r *http.Request, fields auditFields) {
	if audit == nil || audit.logger == nil {
		return
	}
	attrs := []any{"event", name}
	if r != nil {
		attrs = append(attrs, "route", auditRoute(r), "peer", auditPeer(r))
	}
	if fields.action != "" {
		attrs = append(attrs, "action", fields.action)
	}
	if fields.id > 0 {
		attrs = append(attrs, "id", fields.id)
	}
	if fields.revision > 0 {
		attrs = append(attrs, "revision", fields.revision)
	}
	// IDs are supplied by clients and can contain arbitrary text. A one-way
	// correlation hash retains retry matching without logging that text.
	if fields.requestID != "" {
		hash := sha256.Sum256([]byte(fields.requestID))
		attrs = append(attrs, "request_id", hex.EncodeToString(hash[:]), "request_id_hashed", true)
	}
	if name == "owner_session_ended" {
		attrs = append(attrs, "everywhere", fields.everywhere)
	}
	if fields.status > 0 {
		attrs = append(attrs, "status", fields.status)
	}
	if name == "auth_failed" || name == "server_error" {
		key := name + ":" + auditRoute(r) + ":" + auditPeer(r)
		now := audit.now()
		audit.mu.Lock()
		window, exists := audit.failures[key]
		if exists && now.Before(window.until) {
			window.suppressed++
			audit.failures[key] = window
			audit.mu.Unlock()
			return
		}
		// A global overflow bucket preserves bounded state for an attack using
		// many IPs. Expired buckets are pruned only when admitting new keys.
		if !exists {
			for other, entry := range audit.failures {
				if !now.Before(entry.until) {
					delete(audit.failures, other)
				}
			}
			if len(audit.failures) >= 1024 {
				key = "overflow"
				window, exists = audit.failures[key]
				if exists && now.Before(window.until) {
					window.suppressed++
					audit.failures[key] = window
					audit.mu.Unlock()
					return
				}
			}
		}
		audit.failures[key] = auditWindow{until: now.Add(time.Minute)}
		audit.mu.Unlock()
		attrs = append(attrs, "suppressed", window.suppressed)
	}
	audit.logger.Info(name, attrs...)
}

type auditResponse struct {
	http.ResponseWriter
	status int
}

func (w *auditResponse) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
func (w *auditResponse) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(200)
	}
	return w.ResponseWriter.Write(data)
}
func (w *auditResponse) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *auditResponse) Flush() {
	if w.status == 0 {
		w.WriteHeader(200)
	}
	_ = http.NewResponseController(w.ResponseWriter).Flush()
}
func (audit *auditLog) wrap(next http.Handler) http.Handler {
	if audit.logger == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := &auditResponse{ResponseWriter: w}
		next.ServeHTTP(response, r)
		if (response.status == 401 || response.status == 403 || (response.status == 429 && r.URL.Path == "/visualizer/api/session")) && r.URL.Path != "/pair/poll" {
			audit.event("auth_failed", r, auditFields{status: response.status})
		}
		if response.status >= 500 {
			audit.event("server_error", r, auditFields{status: response.status})
		}
	})
}
