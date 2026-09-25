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
	"strings"
	"testing"
	"time"
)

func TestPairConnectsMarketplaceWithoutMasterToken(t *testing.T) {
	root, _, config, _ := setupFixture(t)
	tokenPath := filepath.Join(t.TempDir(), "device-token")
	var approvedHash string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/pair/start":
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
	if err := connectWithRoot(ctx, []string{"--url", server.URL + "/mcp", "--token-file", tokenPath, "--config", config}, root, run); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(tokenPath)
	if err != nil || string(data) == strings.Repeat("t", 40) || len(data) < 32 {
		t.Fatalf("device credential missing or copied master: %v", err)
	}
	if err := checkConnection([]string{"--config", config}); err != nil {
		t.Fatal(err)
	}
	if err := connectWithRoot(ctx, []string{"--url", server.URL + "/mcp", "--token-file", tokenPath, "--config", config}, root, run); err != nil {
		t.Fatalf("repeat connect: %v", err)
	}
	if err := os.Remove(config); err != nil {
		t.Fatal(err)
	}
	if err := connectWithRoot(ctx, []string{"--url", server.URL + "/mcp", "--token-file", tokenPath, "--config", config}, root, run); err != nil {
		t.Fatalf("resume after approved token but incomplete setup: %v", err)
	}
	if err := checkConnection([]string{"--config", config}); err != nil {
		t.Fatalf("resumed connection cannot read context: %v", err)
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
