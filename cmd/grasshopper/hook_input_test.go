package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// An oversized or malformed hook input must not fail the agent's hook.
func TestHookWithUnreadableInputAddsNothingAndSucceeds(t *testing.T) {
	config := filepath.Join(t.TempDir(), "client.json")
	if err := os.WriteFile(config, []byte(`{"url":"http://127.0.0.1:1/mcp","token_file":"/nonexistent","policy_path":"/nonexistent","device":"test"}`), 0600); err != nil {
		t.Fatal(err)
	}
	for name, input := range map[string]string{"oversized": `{"prompt":"` + strings.Repeat("x", 140000) + `"}`, "malformed": `{not json`} {
		t.Run(name, func(t *testing.T) {
			stdinR, stdinW, _ := os.Pipe()
			stdoutR, stdoutW, _ := os.Pipe()
			oldArgs, oldIn, oldOut := os.Args, os.Stdin, os.Stdout
			os.Args, os.Stdin, os.Stdout = []string{"grasshopper", "hook", "--harness", "codex", "--config", config}, stdinR, stdoutW
			go func() { _, _ = io.WriteString(stdinW, input); stdinW.Close() }()
			err := run()
			stdoutW.Close()
			os.Args, os.Stdin, os.Stdout = oldArgs, oldIn, oldOut
			out, _ := io.ReadAll(stdoutR)
			if err != nil {
				t.Fatalf("hook failed: %v", err)
			}
			if strings.TrimSpace(string(out)) != "{}" {
				t.Fatalf("want an empty hook answer, got %q", out)
			}
		})
	}
}

func TestConnectNamesAFullPairingQueue(t *testing.T) {
	root, _, config, _ := setupFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "too many pending connections", http.StatusTooManyRequests)
	}))
	defer server.Close()
	run := func(string, ...string) ([]byte, error) { return nil, nil }
	args := []string{"--json", "--url", server.URL + "/visualizer/", "--token-file", filepath.Join(t.TempDir(), "device-token"), "--config", config}
	err := connectWithRoot(t.Context(), args, root, run)
	if status, _ := connectErrorStatus(err); status != "pairing_queue_full" {
		t.Fatalf("a full queue was reported as %q: %v", status, err)
	}
}
