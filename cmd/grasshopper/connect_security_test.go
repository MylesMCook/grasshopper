package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MylesMCook/grasshopper/internal/goclient"
)

func isolatedDeviceDefaults(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	for _, name := range []string{"HOME", "USERPROFILE", "APPDATA", "XDG_CONFIG_HOME"} {
		t.Setenv(name, home)
	}
}

func pairingOnlyTarget(t *testing.T) (*httptest.Server, *atomic.Int32, *atomic.Int32) {
	t.Helper()
	var authorized, starts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			authorized.Add(1)
		}
		if r.URL.Path == "/pair/start" {
			starts.Add(1)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"request_id": strings.Repeat("a", 64), "code": "ABCD1234", "expires_in": 300})
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(server.Close)
	return server, &authorized, &starts
}

func TestConnectNeverProbesUnboundDefaultCredentials(t *testing.T) {
	for _, occupied := range []string{"access-token", "device-token"} {
		t.Run(occupied, func(t *testing.T) {
			isolatedDeviceDefaults(t)
			root, _, config, _ := setupFixture(t)
			path, err := goclient.DefaultTokenPath()
			if err != nil {
				t.Fatal(err)
			}
			if filepath.Base(path) != "device-token" {
				t.Fatal("client default collides with owner token")
			}
			stray := filepath.Join(filepath.Dir(path), occupied)
			if err := os.MkdirAll(filepath.Dir(stray), 0700); err != nil {
				t.Fatal(err)
			}
			secret := []byte(strings.Repeat("synthetic-private-", 3))
			if err := os.WriteFile(stray, secret, 0600); err != nil {
				t.Fatal(err)
			}
			server, authorized, starts := pairingOnlyTarget(t)
			result := captureConnectJSON(t, func() error {
				_ = connectWithRoot(t.Context(), []string{"--json", "--url", server.URL, "--config", config}, root, nil)
				return nil
			})
			if result["status"] != "approval_pending" || starts.Load() != 1 || authorized.Load() != 0 {
				t.Fatalf("status=%s starts=%d authenticated=%d", result["status"], starts.Load(), authorized.Load())
			}
			pending, err := loadPendingConnection(config)
			if err != nil || pending.TokenPath == stray {
				t.Fatal("pairing reused an unbound credential path")
			}
			if after, _ := os.ReadFile(stray); string(after) != string(secret) {
				t.Fatal("existing credential changed")
			}
		})
	}
}

func TestConnectSwitchNeverSendsAnExistingCredentialToAnotherServer(t *testing.T) {
	for _, options := range [][]string{{"--switch-server"}, {"--switch-server", "--reconnect"}} {
		t.Run(strings.Join(options, "-"), func(t *testing.T) {
			root, token, config, _ := setupFixture(t)
			if err := configureWithReport([]string{"--url", "https://first.example.invalid/mcp", "--token-file", token, "--device", "test", "--policy-file", filepath.Join(root, "policy", "AGENTS.md"), "--config", config}, false); err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(config)
			credential, _ := os.ReadFile(token)
			// A leftover replacement must not become a credential source for B.
			if err := os.WriteFile(token+".replacement", credential, 0600); err != nil {
				t.Fatal(err)
			}
			server, authorized, starts := pairingOnlyTarget(t)
			args := append([]string{"--json", "--url", server.URL, "--token-file", token, "--config", config}, options...)
			result := captureConnectJSON(t, func() error { _ = connectWithRoot(t.Context(), args, root, nil); return nil })
			if result["status"] != "approval_pending" || starts.Load() != 1 || authorized.Load() != 0 {
				t.Fatalf("status=%s starts=%d authenticated=%d", result["status"], starts.Load(), authorized.Load())
			}
			if after, _ := os.ReadFile(config); string(after) != string(before) {
				t.Fatal("pending switch changed active config")
			}
			for _, path := range []string{token, token + ".replacement"} {
				if after, _ := os.ReadFile(path); string(after) != string(credential) {
					t.Fatal("pending switch changed old credential")
				}
			}
		})
	}
}

func TestConnectExplicitOrphanTokenIsAnOutputPathOnly(t *testing.T) {
	root, token, config, _ := setupFixture(t)
	before, _ := os.ReadFile(token)
	server, authorized, starts := pairingOnlyTarget(t)
	result := captureConnectJSON(t, func() error {
		_ = connectWithRoot(t.Context(), []string{"--json", "--url", server.URL, "--token-file", token, "--config", config}, root, nil)
		return nil
	})
	if result["status"] != "approval_pending" || starts.Load() != 1 || authorized.Load() != 0 {
		t.Fatalf("status=%s starts=%d authenticated=%d", result["status"], starts.Load(), authorized.Load())
	}
	if after, _ := os.ReadFile(token); string(after) != string(before) {
		t.Fatal("orphan token overwritten")
	}
}

func TestSameServerOriginCredentialBoundaries(t *testing.T) {
	for _, pair := range [][2]string{
		{"https://memory.example.com/mcp", "https://MEMORY.example.com:443/visualizer/"},
		{"http://localhost:80/mcp", "http://localhost"},
	} {
		if !sameServerOrigin(pair[0], pair[1]) {
			t.Fatal("same origin treated as different")
		}
	}
	for _, pair := range [][2]string{
		{"https://first.example.com/mcp", "https://second.example.com/mcp"},
		{"http://localhost:8199/mcp", "http://localhost:8200/mcp"},
		{"http://localhost/mcp", "https://localhost/mcp"},
	} {
		if sameServerOrigin(pair[0], pair[1]) {
			t.Fatal("credential origin boundary removed")
		}
	}
}

func TestConnectPendingCollisionUsesOnlyServerBoundPairingSecret(t *testing.T) {
	root, token, config, _ := setupFixture(t)
	original, _ := os.ReadFile(token)
	secret := strings.Repeat("synthetic-paired-", 3)
	var leaked, starts, polls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); auth != "" && auth != "Bearer "+secret {
			leaked.Add(1)
		}
		switch r.URL.Path {
		case "/pair/start":
			starts.Add(1)
			w.WriteHeader(http.StatusBadRequest)
		case "/pair/poll":
			polls.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "approved"})
		case "/mcp":
			var rpc struct{ Method string }
			_ = json.NewDecoder(r.Body).Decode(&rpc)
			w.Header().Set("Content-Type", "application/json")
			switch rpc.Method {
			case "initialize":
				_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-03-26"}}`))
			case "notifications/initialized":
				w.WriteHeader(http.StatusAccepted)
			default:
				_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":2,"result":{"structuredContent":{"records":[]}}}`))
			}
		default:
			t.Error("unexpected credential probe")
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	pending := pendingConnection{Address: server.URL + "/mcp", TokenPath: token, Device: "synthetic", Agents: "none", Secret: secret, RequestID: strings.Repeat("a", 64), Code: "ABCD1234", ApprovalURL: server.URL + "/visualizer/", ExpiresAt: time.Now().Add(time.Minute)}
	if err := savePendingConnection(config, pending); err != nil {
		t.Fatal(err)
	}
	result := captureConnectJSON(t, func() error { return connectWithRoot(t.Context(), []string{"--json", "--config", config}, root, nil) })
	if result["status"] != "connected" || starts.Load() != 0 || polls.Load() != 1 || leaked.Load() != 0 {
		t.Fatalf("status=%s starts=%d polls=%d leaked=%d", result["status"], starts.Load(), polls.Load(), leaked.Load())
	}
	active, err := goclient.LoadConfig(config)
	if err != nil || active.TokenFile == token {
		t.Fatal("pending flow reused occupied token")
	}
	if contents, _ := os.ReadFile(active.TokenFile); string(contents) != secret {
		t.Fatal("pairing secret not installed")
	}
	if after, _ := os.ReadFile(token); string(after) != string(original) {
		t.Fatal("occupied token overwritten")
	}
}
