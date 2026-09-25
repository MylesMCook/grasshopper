package gomcp

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/MylesMCook/grasshopper/internal/gomemory"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type pairedTransport struct{ token string }

func (p pairedTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	copy := request.Clone(request.Context())
	copy.Header.Set("Authorization", "Bearer "+p.token)
	return http.DefaultTransport.RoundTrip(copy)
}

func TestViewerApprovedDevicePairingAndRevocation(t *testing.T) {
	server, store := testServer(t, true)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	request := func(method, path, bearer string, body any, origin bool) (int, []byte) {
		t.Helper()
		var input io.Reader
		if body != nil {
			encoded, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			input = bytes.NewReader(encoded)
		}
		req, err := http.NewRequest(method, server.URL+path, input)
		if err != nil {
			t.Fatal(err)
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		if origin {
			req.Header.Set("Origin", server.URL)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		return resp.StatusCode, data
	}
	token := "synthetic-per-device-token-0123456789-abcdef"
	hash := sha256.Sum256([]byte(token))
	if status, _ := request(http.MethodGet, "/visualizer/api/pairings", "", nil, true); status != http.StatusUnauthorized {
		t.Fatalf("anonymous viewer could list pairings: %d", status)
	}
	status, data := request(http.MethodPost, "/pair/start", "", map[string]any{"device": "work-hp", "token_hash": hex.EncodeToString(hash[:])}, false)
	if status != http.StatusCreated {
		t.Fatalf("start pairing: %d %s", status, data)
	}
	var started struct {
		RequestID string `json:"request_id"`
		Code      string `json:"code"`
	}
	if err := json.Unmarshal(data, &started); err != nil || started.RequestID == "" || started.Code == "" {
		t.Fatalf("bad pairing response: %s %v", data, err)
	}
	if status, _ := request(http.MethodGet, "/healthz", token, nil, false); status != http.StatusUnauthorized {
		t.Fatalf("unapproved device authenticated: %d", status)
	}
	if status, _ := request(http.MethodPost, "/visualizer/api/session", testToken, nil, true); status != http.StatusNoContent {
		t.Fatalf("owner login: %d", status)
	}
	status, data = request(http.MethodGet, "/visualizer/api/pairings", "", nil, true)
	if status != http.StatusOK || !bytes.Contains(data, []byte(started.Code)) {
		t.Fatalf("request absent from viewer: %d %s", status, data)
	}
	decision := map[string]any{"request_id": started.RequestID, "code": started.Code, "decision": "approve"}
	if status, _ := request(http.MethodPost, "/visualizer/api/pairings", "", decision, false); status != http.StatusForbidden {
		t.Fatalf("approval without origin accepted: %d", status)
	}
	decision["code"] = "wrong"
	if status, _ := request(http.MethodPost, "/visualizer/api/pairings", "", decision, true); status != http.StatusBadRequest {
		t.Fatalf("mismatched code accepted: %d", status)
	}
	decision["code"] = started.Code
	if status, data := request(http.MethodPost, "/visualizer/api/pairings", "", decision, true); status != http.StatusNoContent {
		t.Fatalf("approval failed: %d %s", status, data)
	}
	if status, data := request(http.MethodPost, "/pair/poll", "", map[string]any{"request_id": started.RequestID}, false); status != http.StatusOK || !bytes.Contains(data, []byte("approved")) {
		t.Fatalf("approval not acknowledged: %d %s", status, data)
	}
	if status, _ := request(http.MethodGet, "/healthz", token, nil, false); status != http.StatusOK {
		t.Fatalf("approved device denied: %d", status)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "paired-test", Version: "0.1"}, nil)
	mcpSession, err := mcpClient.Connect(t.Context(), &mcp.StreamableClientTransport{
		Endpoint: server.URL + "/mcp", HTTPClient: &http.Client{Transport: pairedTransport{token: token}, Timeout: 5 * time.Second},
		MaxRetries: -1, DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		t.Fatalf("paired device MCP connection: %v", err)
	}
	defer mcpSession.Close()
	page := decodeResult[gomemory.Page](t, call(t, mcpSession, "context", map[string]any{"scope": map[string]any{}}))
	if len(page.Records) != 1 {
		t.Fatalf("paired device did not read context: %+v", page)
	}
	current := page.Records[0]
	correction := gomemory.WriteInput{
		Scope: current.Scope, ID: &current.ID, ExpectedRevision: &current.Revision,
		Key: current.Key, Content: "Synthetic paired-device correction.", Purpose: current.Purpose,
		Confirmed: true, Provenance: gomemory.Provenance{Harness: "cursor", Device: "work-hp", Source: "synthetic pairing test"},
		RequestID: "paired-device-correction",
	}
	stored := decodeResult[storeOutput](t, call(t, mcpSession, "store", correction))
	if stored.ID != current.ID || stored.Revision != current.Revision+1 {
		t.Fatalf("paired correction receipt: %+v", stored)
	}
	latest := decodeResult[gomemory.Record](t, call(t, mcpSession, "get", map[string]any{"scope": current.Scope, "id": current.ID}))
	prior := decodeResult[gomemory.Record](t, call(t, mcpSession, "get", map[string]any{"scope": current.Scope, "id": current.ID, "revision": current.Revision}))
	if latest.Content != correction.Content || prior.Content != current.Content {
		t.Fatalf("paired correction or prior revision missing: latest=%+v prior=%+v", latest, prior)
	}
	if status, _ := request(http.MethodPost, "/visualizer/api/session", token, nil, true); status != http.StatusUnauthorized {
		t.Fatalf("device token created an owner session: %d", status)
	}
	withoutOwnerCookie, _ := http.NewRequest(http.MethodPost, server.URL+"/visualizer/api/pairings", bytes.NewReader([]byte(`{"decision":"approve"}`)))
	withoutOwnerCookie.Header.Set("Authorization", "Bearer "+token)
	withoutOwnerCookie.Header.Set("Origin", server.URL)
	response, err := http.DefaultClient.Do(withoutOwnerCookie)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("device token changed access: %d", response.StatusCode)
	}
	restartedHandler, err := NewHandler(Backend{Store: store, Visualizer: true}, testToken)
	if err != nil {
		t.Fatal(err)
	}
	restarted := httptest.NewServer(restartedHandler)
	defer restarted.Close()
	if got, _ := visualizerRequest(t, http.MethodGet, restarted.URL+"/healthz", token); got.StatusCode != http.StatusOK {
		t.Fatalf("device lost on restart: %d", got.StatusCode)
	}
	status, data = request(http.MethodGet, "/visualizer/api/devices", "", nil, true)
	var devices []struct {
		ID     int64  `json:"id"`
		Device string `json:"device"`
	}
	if status != http.StatusOK || json.Unmarshal(data, &devices) != nil || len(devices) != 1 || devices[0].Device != "work-hp" {
		t.Fatalf("device list: %d %s", status, data)
	}
	if status, _ := request(http.MethodDelete, "/visualizer/api/devices", "", map[string]any{"id": devices[0].ID}, true); status != http.StatusNoContent {
		t.Fatalf("revoke: %d", status)
	}
	if status, _ := request(http.MethodGet, "/healthz", token, nil, false); status != http.StatusUnauthorized {
		t.Fatalf("revoked token accepted: %d", status)
	}
	if got, _ := visualizerRequest(t, http.MethodGet, restarted.URL+"/healthz", token); got.StatusCode != http.StatusUnauthorized {
		t.Fatalf("restart cached revoked token: %d", got.StatusCode)
	}
	if status, _ := request(http.MethodGet, "/healthz", testToken, nil, false); status != http.StatusOK {
		t.Fatalf("master token broken: %d", status)
	}
	if listed, err := store.ListClientTokens(t.Context()); err != nil || len(listed) != 1 || listed[0].RevokedAt == nil {
		t.Fatalf("revocation missing: %+v %v", listed, err)
	}
	deniedToken := "synthetic-denied-device-token-0123456789"
	deniedHash := sha256.Sum256([]byte(deniedToken))
	status, data = request(http.MethodPost, "/pair/start", "", map[string]any{"device": "denied-device", "token_hash": hex.EncodeToString(deniedHash[:])}, false)
	if status != http.StatusCreated || json.Unmarshal(data, &started) != nil {
		t.Fatalf("denied request start: %d %s", status, data)
	}
	decision = map[string]any{"request_id": started.RequestID, "code": started.Code, "decision": "deny"}
	if status, _ := request(http.MethodPost, "/visualizer/api/pairings", "", decision, true); status != http.StatusNoContent {
		t.Fatalf("deny failed: %d", status)
	}
	if status, _ := request(http.MethodPost, "/pair/poll", "", map[string]any{"request_id": started.RequestID}, false); status != http.StatusForbidden {
		t.Fatalf("denied request accepted: %d", status)
	}
	if status, _ := request(http.MethodGet, "/healthz", deniedToken, nil, false); status != http.StatusUnauthorized {
		t.Fatalf("denied token authenticated: %d", status)
	}
}

func TestPairingExpiresAndPendingRequestsAreBounded(t *testing.T) {
	_, store := testServer(t, true)
	manager := newPairingManager(store)
	hash := sha256.Sum256([]byte("synthetic-pending-token-0123456789"))
	start := func() (int, string) {
		body, _ := json.Marshal(map[string]string{"device": "test", "token_hash": hex.EncodeToString(hash[:])})
		req := httptest.NewRequest(http.MethodPost, "/pair/start", bytes.NewReader(body))
		writer := httptest.NewRecorder()
		manager.start(writer, req, "http://example.test")
		return writer.Code, writer.Body.String()
	}
	var id string
	for i := 0; i < maxPendingPairings; i++ {
		status, body := start()
		if status != http.StatusCreated {
			t.Fatalf("pending request %d: %d %s", i, status, body)
		}
		if i == 0 {
			var item struct {
				RequestID string `json:"request_id"`
			}
			_ = json.Unmarshal([]byte(body), &item)
			id = item.RequestID
		}
	}
	if status, _ := start(); status != http.StatusTooManyRequests {
		t.Fatalf("unbounded pairing requests: %d", status)
	}
	manager.mu.Lock()
	manager.requests[id].Expires = time.Now().Add(-time.Second)
	manager.mu.Unlock()
	req := httptest.NewRequest(http.MethodPost, "/pair/poll", strings.NewReader(`{"request_id":"`+id+`"}`))
	writer := httptest.NewRecorder()
	manager.poll(writer, req, "http://example.test")
	if writer.Code != http.StatusGone {
		t.Fatalf("expired request stayed active: %d", writer.Code)
	}
	if status, _ := start(); status != http.StatusCreated {
		t.Fatalf("expired slot not freed: %d", status)
	}
}
