package main

import (
	"encoding/json"
	"github.com/MylesMCook/grasshopper/internal/goclient"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCheckPendingReflectsHostWithoutWrites(t *testing.T) {
	for _, item := range []struct {
		code            int
		response, state string
	}{
		{202, "pending", "approval_pending"}, {200, "approved", "approval_ready"}, {403, "", "approval_denied"}, {410, "", "approval_expired"}, {404, "", "approval_expired"}, {503, "", "unreachable_server"},
	} {
		t.Run(item.state, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/pair/poll" {
					t.Errorf("unexpected write/request: %s", r.URL.Path)
				}
				w.WriteHeader(item.code)
				json.NewEncoder(w).Encode(map[string]string{"status": item.response})
			}))
			defer server.Close()
			config := filepath.Join(t.TempDir(), "client.json")
			pending := pendingConnection{Address: server.URL + "/mcp", TokenPath: filepath.Join(t.TempDir(), "token"), Device: "laptop", Agents: "none", Secret: strings.Repeat("s", 40), RequestID: strings.Repeat("a", 64), Code: "ABCD1234", ExpiresAt: time.Now().Add(time.Minute)}
			if err := savePendingConnection(config, pending); err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(pendingPath(config))
			state, _ := connectionStatus(config)
			if state != item.state {
				t.Fatalf("got %s, want %s", state, item.state)
			}
			after, _ := os.ReadFile(pendingPath(config))
			if string(before) != string(after) {
				t.Fatal("check changed pending state")
			}
			for _, path := range []string{config, pending.TokenPath} {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatal("check wrote connection")
				}
			}
		})
	}
}

func TestCheckDoesNotInferDeviceFromOwnerAccess(t *testing.T) {
	for _, item := range []struct{ role, device, state string }{{"owner", "", "owner_credential"}, {"device", "laptop", "connected"}, {"device", "different", "device_mismatch"}} {
		t.Run(item.state, func(t *testing.T) {
			root, token, path, _ := setupFixture(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/connection" {
					t.Errorf("unexpected %s", r.URL.Path)
				}
				json.NewEncoder(w).Encode(map[string]string{"role": item.role, "device": item.device, "version": "test"})
			}))
			defer server.Close()
			config := goclient.Config{URL: server.URL + "/mcp", TokenFile: token, Device: "laptop", PolicyPath: filepath.Join(root, "policy", "AGENTS.md")}
			data, _ := json.Marshal(config)
			os.MkdirAll(filepath.Dir(path), 0700)
			os.WriteFile(path, data, 0600)
			state, _ := connectionStatus(path)
			if state != item.state {
				t.Fatalf("got %s, want %s", state, item.state)
			}
			after, _ := os.ReadFile(path)
			if string(after) != string(data) {
				t.Fatal("check changed config")
			}
		})
	}
}

func TestLegacyMigrationPreservesOriginalAndRejectsConflict(t *testing.T) {
	root, token, _, _ := setupFixture(t)
	canonical := filepath.Join(t.TempDir(), "shared", "client.json")
	original, _ := os.ReadFile(token)
	config := goclient.Config{URL: "https://example.invalid/mcp", TokenFile: token, Device: "laptop", PolicyPath: filepath.Join(root, "policy", "AGENTS.md")}
	target, err := migrateDeviceCredential(config, canonical)
	if err != nil {
		t.Fatal(err)
	}
	copied, _ := os.ReadFile(target)
	retained, _ := os.ReadFile(token)
	if string(copied) != string(original) || string(retained) != string(original) {
		t.Fatal("migration altered credential")
	}
	// A retry reuses the exact credential after an interrupted setup.
	if _, err := migrateDeviceCredential(config, canonical); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(target, []byte(strings.Repeat("z", 40)), 0600)
	if _, err := migrateDeviceCredential(config, canonical); err == nil {
		t.Fatal("conflicting shared token accepted")
	}
	retained, _ = os.ReadFile(token)
	if string(retained) != string(original) {
		t.Fatal("conflict changed legacy access")
	}
}

func TestConnectPreservesApprovalOnHostFailure(t *testing.T) {
	root, _, config, _ := setupFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer server.Close()
	pending := pendingConnection{Address: server.URL + "/mcp", TokenPath: filepath.Join(t.TempDir(), "token"), Device: "laptop", Agents: "none", Secret: strings.Repeat("s", 40), RequestID: strings.Repeat("a", 64), Code: "12345678", ExpiresAt: time.Now().Add(time.Minute)}
	if err := savePendingConnection(config, pending); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(pendingPath(config))
	err := connectWithRoot(t.Context(), []string{"--json", "--config", config}, root, func(string, ...string) ([]byte, error) { t.Fatal("wiring changed"); return nil, nil })
	if state, _ := connectErrorStatus(err); state != "unreachable_server" {
		t.Fatalf("outage: %v", err)
	}
	after, _ := os.ReadFile(pendingPath(config))
	if string(before) != string(after) {
		t.Fatal("outage discarded approval")
	}
}
