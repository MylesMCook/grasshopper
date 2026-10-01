//go:build windows

package main

import (
	"encoding/json"
	"github.com/MylesMCook/grasshopper/internal/client"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestWindowsConnectMigratesOnlyVerifiedLegacyDevice(t *testing.T) {
	for _, item := range []struct {
		role, device string
		migrate      bool
	}{{"device", "laptop", true}, {"owner", "", false}, {"device", "other", false}} {
		t.Run(item.role+item.device, func(t *testing.T) {
			root, token, _, _ := setupFixture(t)
			t.Setenv("USERPROFILE", t.TempDir())
			t.Setenv("AppData", t.TempDir())
			t.Setenv("GRASSHOPPER_CLIENT_CONFIG", "")
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/connection":
					json.NewEncoder(w).Encode(map[string]string{"role": item.role, "device": item.device, "version": "test"})
				case "/mcp":
					var request struct{ Method string }
					json.NewDecoder(r.Body).Decode(&request)
					if request.Method == "notifications/initialized" {
						w.WriteHeader(202)
					} else if request.Method == "initialize" {
						w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-03-26"}}`))
					} else {
						w.Write([]byte(`{"jsonrpc":"2.0","id":2,"result":{"structuredContent":{"records":[]}}}`))
					}
				default:
					t.Errorf("unexpected pairing request: %s", r.URL.Path)
					w.WriteHeader(400)
				}
			}))
			defer server.Close()
			legacy, _ := client.LegacyConfigPath()
			canonical, _ := client.DefaultConfigPath()
			config := client.Config{URL: server.URL + "/mcp", TokenFile: token, Device: "laptop", PolicyPath: filepath.Join(root, "policy", "AGENTS.md")}
			data, _ := json.Marshal(config)
			os.MkdirAll(filepath.Dir(legacy), 0700)
			os.WriteFile(legacy, data, 0600)
			err := connectWithRoot(t.Context(), []string{"--json"}, root, func(string, ...string) ([]byte, error) { t.Fatal("agent wiring changed"); return nil, nil })
			if item.migrate {
				if err != nil {
					t.Fatal(err)
				}
				shared, err := client.LoadConfig(canonical)
				if err != nil {
					t.Fatal(err)
				}
				if shared.Device != "laptop" || shared.TokenFile != filepath.Join(filepath.Dir(canonical), "device-token") {
					t.Fatal("shared device configuration differs")
				}
				path, _ := client.ConfigPath("")
				if path != canonical {
					t.Fatal("agent default still legacy")
				}
			} else {
				if err == nil {
					t.Fatal("unverified device migrated")
				}
				if _, err := os.Stat(canonical); !os.IsNotExist(err) {
					t.Fatal("unverified shared config written")
				}
			}
			retained, _ := os.ReadFile(legacy)
			if string(data) != string(retained) {
				t.Fatal("legacy recovery config changed")
			}
		})
	}
}
