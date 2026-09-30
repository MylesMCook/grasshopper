package gomcp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func auditEntries(t *testing.T, buffer *bytes.Buffer) []map[string]any {
	t.Helper()
	entries := []map[string]any{}
	for _, line := range bytes.Split(bytes.TrimSpace(buffer.Bytes()), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal(line, &entry); err != nil {
			t.Fatal(err)
		}
		entries = append(entries, entry)
	}
	return entries
}

func TestAuditLogsOwnerActionsWithoutSecretsOrMemoryText(t *testing.T) {
	_, store := testServer(t, true)
	var logs bytes.Buffer
	handler, err := NewHandler(Backend{Store: store, Visualizer: true, Logger: slog.New(slog.NewJSONHandler(&logs, nil))}, testToken)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	secretContent := "synthetic private memory never present in operational logs"
	saved := agentSave(t, store, "audit-agent", "Private title", secretContent, "observation", false)
	call := func(method, path, token string, body any, cookie *http.Cookie) (int, []byte, *http.Cookie) {
		raw, _ := json.Marshal(body)
		request, _ := http.NewRequest(method, server.URL+path, bytes.NewReader(raw))
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("Origin", server.URL)
		request.Header.Set("X-Forwarded-For", "203.0.113.123")
		if cookie != nil {
			request.AddCookie(cookie)
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var data bytes.Buffer
		_, _ = data.ReadFrom(response.Body)
		var set *http.Cookie
		if len(response.Cookies()) > 0 {
			set = response.Cookies()[0]
		}
		return response.StatusCode, data.Bytes(), set
	}
	status, _, cookie := call(http.MethodPost, "/visualizer/api/session", testToken, nil, nil)
	if status != 204 || cookie == nil {
		t.Fatalf("login %d", status)
	}
	requestID := "synthetic-request-id-private-text"
	status, _, _ = call(http.MethodPost, "/visualizer/api/update", "", map[string]any{"action": "confirm", "id": saved.ID, "expected_revision": 1, "request_id": requestID}, cookie)
	if status != 200 {
		t.Fatalf("confirm %d", status)
	}
	_, _, _ = call(http.MethodDelete, "/visualizer/api/session", "", nil, cookie)
	_, _, cookie = call(http.MethodPost, "/visualizer/api/session", testToken, nil, nil)
	_, _, _ = call(http.MethodPost, "/visualizer/api/session/revoke-all", "", nil, cookie)
	for i := 0; i < 30; i++ {
		_, _, _ = call(http.MethodPost, "/visualizer/api/context?secret=query-secret", "wrong-secret-token", map[string]any{}, nil)
	}
	events := map[string]int{}
	foundWrite := false
	foundEverywhere := false
	for _, entry := range auditEntries(t, &logs) {
		event := entry["event"].(string)
		events[event]++
		if event == "owner_write" {
			hash := sha256.Sum256([]byte(requestID))
			if entry["action"] != "confirm" || entry["id"] != float64(saved.ID) || entry["revision"] != float64(2) || entry["request_id"] != hex.EncodeToString(hash[:]) {
				t.Fatalf("wrong write fields %v", entry)
			}
			foundWrite = true
		}
		if event == "owner_session_ended" && entry["everywhere"] == true {
			foundEverywhere = true
		}
		if event == "auth_failed" && entry["route"] != "/visualizer/api/context" {
			t.Fatalf("logged arbitrary URL %v", entry)
		}
	}
	if events["startup"] != 1 || events["owner_session_created"] != 2 || events["owner_session_ended"] != 2 || events["auth_failed"] != 1 || !foundWrite || !foundEverywhere {
		t.Fatalf("events %v", events)
	}
	for _, secret := range []string{testToken, "wrong-secret-token", secretContent, "Private title", requestID, "query-secret", cookie.Value, "203.0.113.123"} {
		if strings.Contains(logs.String(), secret) {
			t.Fatalf("log exposed private input %q", secret)
		}
	}
}

func TestAuditFailureBurstSuppressionAndBoundedPeers(t *testing.T) {
	var logs bytes.Buffer
	audit := newAuditLog(slog.New(slog.NewJSONHandler(&logs, nil)))
	now := time.Now()
	audit.now = func() time.Time { return now }
	request := httptest.NewRequest(http.MethodPost, "http://localhost/mcp?token=private", nil)
	request.RemoteAddr = "127.0.0.1:1234"
	request.Header.Set("X-Forwarded-For", "203.0.113.123")
	for i := 0; i < 100; i++ {
		audit.event("auth_failed", request, auditFields{status: 401})
	}
	if entries := auditEntries(t, &logs); len(entries) != 1 {
		t.Fatalf("burst emitted%d entries", len(entries))
	}
	now = now.Add(time.Minute + time.Second)
	audit.event("auth_failed", request, auditFields{status: 401})
	entries := auditEntries(t, &logs)
	if len(entries) != 2 || entries[1]["suppressed"] != float64(99) {
		t.Fatalf("missing suppression count %v", entries)
	}
	for i := 0; i < 1100; i++ {
		request.RemoteAddr = "10.0." + jsonNumber(int64(i/256)) + "." + jsonNumber(int64(i%256)) + ":1234"
		audit.event("auth_failed", request, auditFields{status: 401})
	}
	if len(audit.failures) > 1025 {
		t.Fatalf("unbounded failure state %d", len(audit.failures))
	}
}

func TestAuditPairingLifecycleAnd5xxWithoutCredentials(t *testing.T) {
	_, store := testServer(t, true)
	var logs bytes.Buffer
	audit := newAuditLog(slog.New(slog.NewJSONHandler(&logs, nil)))
	pairings := newPairingManager(store)
	pairings.audit = audit
	token := "synthetic-pairing-token-private-0123456789"
	hash := sha256.Sum256([]byte(token))
	start := func() (string, string) {
		raw, _ := json.Marshal(map[string]any{"device": "private-device-name", "token_hash": hex.EncodeToString(hash[:])})
		req := httptest.NewRequest(http.MethodPost, "http://localhost/pair/start", bytes.NewReader(raw))
		resp := httptest.NewRecorder()
		pairings.start(resp, req, "http://localhost")
		if resp.Code != 201 {
			t.Fatalf("start%d %s", resp.Code, resp.Body.String())
		}
		var out map[string]string
		_ = json.Unmarshal(resp.Body.Bytes(), &out)
		return out["request_id"], out["code"]
	}
	first, code := start()
	raw, _ := json.Marshal(map[string]any{"request_id": first, "code": code, "decision": "approve"})
	req := httptest.NewRequest(http.MethodPost, "http://localhost/visualizer/api/pairings", bytes.NewReader(raw))
	resp := httptest.NewRecorder()
	pairings.adminPairings(resp, req)
	if resp.Code != 204 {
		t.Fatalf("approve%d", resp.Code)
	}
	devices, err := store.ListClientTokens(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(map[string]any{"id": devices[0].ID})
	req = httptest.NewRequest(http.MethodDelete, "http://localhost/visualizer/api/devices", bytes.NewReader(raw))
	resp = httptest.NewRecorder()
	pairings.adminDevices(resp, req)
	if resp.Code != 204 {
		t.Fatalf("revoke%d", resp.Code)
	}
	second, secondCode := start()
	raw, _ = json.Marshal(map[string]any{"request_id": second, "code": secondCode, "decision": "deny"})
	pairings.adminPairings(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "http://localhost/visualizer/api/pairings", bytes.NewReader(raw)))
	third, _ := start()
	pairings.requests[third].Expires = time.Now().Add(-time.Second)
	pairings.expire(time.Now())
	pairings.expire(time.Now())
	failing := audit.wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "private error detail", 503) }))
	failing.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "http://localhost/visualizer/api/update", nil))
	events := map[string]int{}
	for _, entry := range auditEntries(t, &logs) {
		events[entry["event"].(string)]++
	}
	if events["pairing_started"] != 3 || events["pairing_approved"] != 1 || events["pairing_denied"] != 1 || events["pairing_expired"] != 1 || events["device_revoked"] != 1 || events["server_error"] != 1 {
		t.Fatalf("events%v", events)
	}
	for _, secret := range []string{token, hex.EncodeToString(hash[:]), first, second, third, code, secondCode, "private-device-name", "private error detail"} {
		if strings.Contains(logs.String(), secret) {
			t.Fatalf("log exposed %q", secret)
		}
	}
}

func TestAuditDoesNotLogSuccessfulOrDeniedPolling(t *testing.T) {
	var logs bytes.Buffer
	audit := newAuditLog(slog.New(slog.NewJSONHandler(&logs, nil)))
	handler := audit.wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/pair/poll" {
			w.WriteHeader(403)
		} else {
			w.WriteHeader(200)
		}
	}))
	for i := 0; i < 30; i++ {
		for _, path := range []string{"/pair/poll", "/visualizer/api/context", "/visualizer/api/devices", "/visualizer/api/session"} {
			handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "http://localhost"+path, nil))
		}
	}
	if logs.Len() != 0 {
		t.Fatalf("polling produced logs%s", logs.String())
	}
}
