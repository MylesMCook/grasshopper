package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestPairConnectsMarketplaceWithoutMasterToken(t *testing.T) {
	root, _, config, _ := setupFixture(t)
	tokenPath := filepath.Join(t.TempDir(), "device-token")
	var approvedHash string
	var starts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/pair/start":
			starts.Add(1)
			var body struct {
				TokenHash string `json:"token_hash"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			approvedHash = body.TokenHash
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"request_id":"` + strings.Repeat("a", 64) + `","code":"ABCD1234","expires_in":300}`))
		case "/pair/poll":
			_, _ = w.Write([]byte(`{"status":"approved"}`))
		case "/mcp":
			bearer := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			hash := sha256.Sum256([]byte(bearer))
			if bearer == "" || hex.EncodeToString(hash[:]) != approvedHash {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			var rpc struct {
				Method string `json:"method"`
			}
			_ = json.NewDecoder(r.Body).Decode(&rpc)
			if rpc.Method == "notifications/initialized" {
				w.WriteHeader(http.StatusAccepted)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			if rpc.Method == "initialize" {
				_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-03-26"}}`))
				return
			}
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":2,"result":{"structuredContent":{"records":[]}}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	run := func(string, ...string) ([]byte, error) { t.Fatal("native agent settings changed"); return nil, nil }
	if err := connectWithRoot(ctx, []string{"--url", server.URL + "/visualizer/", "--token-file", tokenPath, "--config", config}, root, run); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(tokenPath)
	if err != nil || string(data) == strings.Repeat("t", 40) || len(data) < 32 {
		t.Fatalf("device credential missing or copied master: %v", err)
	}
	if err := checkConnection([]string{"--config", config}); err != nil {
		t.Fatal(err)
	}
	if err := connectWithRoot(ctx, []string{"--config", config}, root, run); err != nil {
		t.Fatalf("repeat connect: %v", err)
	}
	if starts.Load() != 1 {
		t.Fatalf("repeat connection requested another approval: %d", starts.Load())
	}
	if err := os.Remove(config); err != nil {
		t.Fatal(err)
	}
	if err := connectWithRoot(ctx, []string{"--url", server.URL, "--token-file", tokenPath, "--config", config}, root, run); err != nil {
		t.Fatalf("resume after approved token but incomplete setup: %v", err)
	}
	if err := checkConnection([]string{"--config", config}); err != nil {
		t.Fatalf("resumed connection cannot read context: %v", err)
	}
	if starts.Load() != 1 {
		t.Fatalf("partial setup requested another approval: %d", starts.Load())
	}
	server.Close()
	if state, _ := connectionStatus(config); state != "unreachable_server" {
		t.Fatalf("offline status: %s", state)
	}
	if err := connectWithRoot(ctx, []string{"--config", config}, root, run); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("offline connection replaced credential: %v", err)
	}
	if starts.Load() != 1 {
		t.Fatalf("offline connection requested another approval: %d", starts.Load())
	}
}

func TestPairUnavailableLeavesNoCredentialOrConfig(t *testing.T) {
	root, _, config, _ := setupFixture(t)
	tokenPath := filepath.Join(t.TempDir(), "device-token")
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	run := func(string, ...string) ([]byte, error) { t.Fatal("native agent settings changed"); return nil, nil }
	if err := connectWithRoot(ctx, []string{"--url", "http://127.0.0.1:1/mcp", "--token-file", tokenPath, "--config", config}, root, run); err == nil {
		t.Fatal("unavailable backend accepted")
	}
	if _, err := os.Stat(config); !os.IsNotExist(err) {
		t.Fatal("failure wrote configuration")
	}
	if _, err := os.Stat(tokenPath); !os.IsNotExist(err) {
		t.Fatal("failure wrote credential")
	}
}

func TestJSONPairReturnsImmediatelyAndResumesWithoutDuplicateRequest(t *testing.T) {
	root, _, config, _ := setupFixture(t)
	tokenPath := filepath.Join(t.TempDir(), "device-token")
	var starts atomic.Int32
	var approved atomic.Bool
	var approvedHash string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/pair/start":
			starts.Add(1)
			var body struct {
				TokenHash string `json:"token_hash"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			approvedHash = body.TokenHash
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"request_id":"` + strings.Repeat("a", 64) + `","code":"ABCD1234","expires_in":300}`))
		case "/pair/poll":
			if !approved.Load() {
				w.WriteHeader(http.StatusAccepted)
				_, _ = w.Write([]byte(`{"status":"pending"}`))
				return
			}
			_, _ = w.Write([]byte(`{"status":"approved"}`))
		case "/mcp":
			bearer := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			hash := sha256.Sum256([]byte(bearer))
			if bearer == "" || hex.EncodeToString(hash[:]) != approvedHash {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			var rpc struct {
				Method string `json:"method"`
			}
			_ = json.NewDecoder(r.Body).Decode(&rpc)
			if rpc.Method == "notifications/initialized" {
				w.WriteHeader(http.StatusAccepted)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			if rpc.Method == "initialize" {
				_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-03-26"}}`))
			} else {
				_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":2,"result":{"structuredContent":{"records":[]}}}`))
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	run := func(string, ...string) ([]byte, error) { t.Fatal("native agent settings changed"); return nil, nil }
	firstArgs := []string{"--json", "--url", server.URL + "/visualizer/", "--token-file", tokenPath, "--config", config}
	start := time.Now()
	first := captureConnectJSON(t, func() error { return connectWithRoot(t.Context(), firstArgs, root, run) })
	// A three-second bound catches an accidental five-second poll without
	// making a loaded CI runner fail on routine scheduling delay.
	if first["status"] != "approval_pending" || first["code"] != "ABCD1234" || time.Since(start) > 3*time.Second {
		t.Fatalf("first call did not promptly return approval: %v", first)
	}
	if state, _ := connectionStatus(config); state != "approval_pending" {
		t.Fatalf("check did not report pending approval: %s", state)
	}
	info, err := os.Stat(pendingPath(config))
	if err != nil || (runtime.GOOS != "windows" && info.Mode().Perm() != 0600) {
		t.Fatalf("pending secret not private: %v", err)
	}
	for _, path := range []string{config, tokenPath} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("pending request wrote %s: %v", path, err)
		}
	}
	second := captureConnectJSON(t, func() error { return connectWithRoot(t.Context(), []string{"--json", "--config", config}, root, run) })
	if second["status"] != "approval_pending" || second["approval_url"] != first["approval_url"] || starts.Load() != 1 {
		t.Fatalf("repeat request changed pending approval: %v, starts=%d", second, starts.Load())
	}
	approved.Store(true)
	third := captureConnectJSON(t, func() error { return connectWithRoot(t.Context(), []string{"--json", "--config", config}, root, run) })
	if third["status"] != "connected" || starts.Load() != 1 {
		t.Fatalf("approved request not connected once: %v", third)
	}
	if _, err := os.Stat(pendingPath(config)); !os.IsNotExist(err) {
		t.Fatalf("pending secret not removed: %v", err)
	}
	if state, _ := connectionStatus(config); state != "connected" {
		t.Fatalf("saved connection: %s", state)
	}
}

func TestJSONPendingOfflineAndDenialDoNotCreateCredential(t *testing.T) {
	root, _, config, _ := setupFixture(t)
	tokenPath := filepath.Join(t.TempDir(), "device-token")
	var denied atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/pair/start":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"request_id":"` + strings.Repeat("b", 64) + `","code":"1234ABCD","expires_in":300}`))
		case "/pair/poll":
			if denied.Load() {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"status":"pending"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	run := func(string, ...string) ([]byte, error) { t.Fatal("agent settings changed"); return nil, nil }
	first := captureConnectJSON(t, func() error {
		return connectWithRoot(t.Context(), []string{"--json", "--url", server.URL, "--token-file", tokenPath, "--config", config}, root, run)
	})
	if first["status"] != "approval_pending" {
		t.Fatalf("start: %v", first)
	}
	denied.Store(true)
	err := connectWithRoot(t.Context(), []string{"--json", "--config", config}, root, run)
	if state, _ := connectErrorStatus(err); state != "approval_denied" {
		t.Fatalf("denial: %v", err)
	}
	for _, path := range []string{config, tokenPath, pendingPath(config)} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("denied request retained %s: %v", path, err)
		}
	}
	denied.Store(false)
	_ = captureConnectJSON(t, func() error {
		return connectWithRoot(t.Context(), []string{"--json", "--url", server.URL, "--token-file", tokenPath, "--config", config}, root, run)
	})
	server.Close()
	err = connectWithRoot(t.Context(), []string{"--json", "--config", config}, root, run)
	if state, _ := connectErrorStatus(err); state != "unreachable_server" {
		t.Fatalf("offline: %v", err)
	}
	if _, err := os.Stat(pendingPath(config)); err != nil {
		t.Fatalf("offline lost pending request: %v", err)
	}
	for _, path := range []string{config, tokenPath} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("offline wrote %s: %v", path, err)
		}
	}
}

func captureConnectJSON(t *testing.T, call func() error) map[string]string {
	t.Helper()
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Stdout
	os.Stdout = write
	callErr := call()
	os.Stdout = previous
	_ = write.Close()
	defer read.Close()
	if callErr != nil {
		t.Fatal(callErr)
	}
	var result map[string]string
	if err := json.NewDecoder(read).Decode(&result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestOldServerRequiresUpgradeWithoutSavingCredential(t *testing.T) {
	root, _, config, _ := setupFixture(t)
	tokenPath := filepath.Join(t.TempDir(), "device-token")
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	run := func(string, ...string) ([]byte, error) { t.Fatal("native agent settings changed"); return nil, nil }
	err := connectWithRoot(t.Context(), []string{"--url", server.URL + "/mcp", "--token-file", tokenPath, "--config", config}, root, run)
	if err == nil || !strings.Contains(err.Error(), "server needs a Grasshopper version with viewer pairing") {
		t.Fatalf("old server guidance: %v", err)
	}
	if _, err := os.Stat(config); !os.IsNotExist(err) {
		t.Fatal("old server wrote configuration")
	}
	if _, err := os.Stat(tokenPath); !os.IsNotExist(err) {
		t.Fatal("old server wrote credential")
	}
}

func TestRejectedOrConflictingConnectionKeepsWorkingFiles(t *testing.T) {
	root, token, config, _ := setupFixture(t)
	server := setupServer(t, false)
	if err := configureWithReport([]string{"--url", server.URL + "/mcp", "--token-file", token, "--device", "test", "--policy-file", filepath.Join(root, "policy", "AGENTS.md"), "--config", config}, false); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(config)
	credential, _ := os.ReadFile(token)
	if state, _ := connectionStatus(config); state != "authentication_rejected" {
		t.Fatalf("rejected credential status: %s", state)
	}
	run := func(string, ...string) ([]byte, error) { t.Fatal("agent settings changed"); return nil, nil }
	if err := connectWithRoot(t.Context(), []string{"--config", config}, root, run); err == nil || !strings.Contains(err.Error(), "rejected") {
		t.Fatalf("rejected credential caused pairing: %v", err)
	}
	if err := connectWithRoot(t.Context(), []string{"--config", config, "--url", "http://127.0.0.1:1"}, root, run); err == nil || !strings.Contains(err.Error(), "--switch-server") {
		t.Fatalf("server changed without confirmation: %v", err)
	}
	if err := connectWithRoot(t.Context(), []string{"--config", config, "--reconnect"}, root, run); err == nil || !strings.Contains(err.Error(), "viewer pairing") {
		t.Fatalf("reconnect to old server: %v", err)
	}
	after, _ := os.ReadFile(config)
	afterCredential, _ := os.ReadFile(token)
	if string(after) != string(before) || string(afterCredential) != string(credential) {
		t.Fatal("failed reconnect damaged saved configuration")
	}
}

func TestReconnectKeepsOldConnectionUntilNewCredentialWorks(t *testing.T) {
	root, token, config, _ := setupFixture(t)
	var approvedHash atomic.Value
	approvedHash.Store("")
	pending := make(chan struct{})
	release := make(chan struct{})
	var pollCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/pair/start":
			var input struct {
				TokenHash string `json:"token_hash"`
			}
			_ = json.NewDecoder(r.Body).Decode(&input)
			approvedHash.Store(input.TokenHash)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"request_id":"` + strings.Repeat("a", 64) + `","code":"ABCD1234","expires_in":300}`))
		case "/pair/poll":
			if pollCount.Add(1) == 1 {
				close(pending)
			}
			<-release
			_, _ = w.Write([]byte(`{"status":"approved"}`))
		case "/mcp":
			bearer := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			hash := sha256.Sum256([]byte(bearer))
			if bearer == "" || hex.EncodeToString(hash[:]) != approvedHash.Load().(string) {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			var rpc struct {
				Method string `json:"method"`
			}
			_ = json.NewDecoder(r.Body).Decode(&rpc)
			if rpc.Method == "notifications/initialized" {
				w.WriteHeader(http.StatusAccepted)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			if rpc.Method == "initialize" {
				_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-03-26"}}`))
				return
			}
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":2,"result":{"structuredContent":{"records":[]}}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	if err := configureWithReport([]string{"--url", server.URL + "/mcp", "--token-file", token, "--device", "test", "--policy-file", filepath.Join(root, "policy", "AGENTS.md"), "--config", config}, false); err != nil {
		t.Fatal(err)
	}
	oldConfig, _ := os.ReadFile(config)
	oldToken, _ := os.ReadFile(token)
	done := make(chan error, 1)
	go func() {
		done <- connectWithRoot(t.Context(), []string{"--config", config, "--reconnect"}, root, func(string, ...string) ([]byte, error) { return nil, nil })
	}()
	select {
	case <-pending:
	case <-time.After(3 * time.Second):
		t.Fatal("replacement approval did not start")
	}
	currentConfig, _ := os.ReadFile(config)
	currentToken, _ := os.ReadFile(token)
	if string(currentConfig) != string(oldConfig) || string(currentToken) != string(oldToken) {
		t.Fatal("pending replacement changed current connection")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if state, _ := connectionStatus(config); state != "connected" {
		t.Fatalf("replacement status: %s", state)
	}
	newConfig, _ := os.ReadFile(config)
	if string(newConfig) == string(oldConfig) {
		t.Fatal("approved replacement did not become active")
	}
	if saved, _ := os.ReadFile(token); string(saved) != string(oldToken) {
		t.Fatal("old credential was overwritten")
	}
}
