package goclient

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/MylesMCook/grasshopper/internal/gomcp"
	"github.com/MylesMCook/grasshopper/internal/gomemory"
)

const testToken = "synthetic-client-token-0123456789-abcdef"

func testClientServer(t *testing.T) (*httptest.Server, string, string) {
	t.Helper()
	source := filepath.Join("..", "..", "tests", "fixtures", "go-compat", "memory.db")
	store, err := gomemory.OpenWritableCopy(context.Background(), source, filepath.Join(t.TempDir(), "copy.db"))
	if err != nil {
		t.Fatal(err)
	}
	handler, err := gomcp.NewHandler(gomcp.Backend{Store: store}, testToken)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(func() { server.Close(); store.Close() })
	root := t.TempDir()
	policy := filepath.Join(root, "policy", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(policy), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(policy, []byte("Canonical memory policy marker."), 0600); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(root, "client.json")
	data, _ := json.Marshal(Config{URL: server.URL + "/mcp", TokenEnv: "GRASSHOPPER_TEST_TOKEN", PolicyPath: policy, Device: "synthetic-mac"})
	if err := os.WriteFile(config, data, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GRASSHOPPER_TEST_TOKEN", testToken)
	return server, config, root
}

func TestGoBridgeAndHookUseOneBackend(t *testing.T) {
	_, config, root := testClientServer(t)
	for _, args := range [][]string{{"init", "-q"}, {"remote", "add", "origin", "git@GitHub.com:Owner/Repo.git"}} {
		if output, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git fixture: %v: %s", err, output)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("Root marker."), 0600); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "nested")
	if err := os.Mkdir(nested, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "AGENTS.md"), []byte("Nested marker."), 0600); err != nil {
		t.Fatal(err)
	}
	pretool, err := Hook(config, "claude", map[string]any{"cwd": root, "hook_event_name": "PreToolUse", "tool_input": map[string]any{"file_path": "nested/note.txt"}})
	if err != nil {
		t.Fatal(err)
	}
	guidance := pretool["hookSpecificOutput"].(map[string]any)["additionalContext"].(string)
	for _, expected := range []string{"Canonical memory policy marker.", "Root marker.", "Nested marker."} {
		if !strings.Contains(guidance, expected) {
			t.Errorf("missing %q in nested guidance", expected)
		}
	}
	contextOutput, err := Hook(config, "cursor", map[string]any{"cwd": root})
	if err != nil {
		t.Fatal(err)
	}
	loaded := contextOutput["additional_context"].(string)
	for _, expected := range []string{"project_resolved=true", "git:github.com/Owner/Repo", "Canonical memory policy marker."} {
		if !strings.Contains(loaded, expected) {
			t.Errorf("missing %q in context", expected)
		}
	}
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"context","arguments":{"scope":{},"budget":12000}}}`,
	}, "\n") + "\n"
	var output bytes.Buffer
	if err := Bridge(context.Background(), config, strings.NewReader(input), &output); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 3 || !strings.Contains(lines[1], `"context"`) || !strings.Contains(lines[2], `"structuredContent"`) {
		t.Fatalf("unexpected bridge response: %s", output.String())
	}
}

func TestGoBridgeOutageIsBoundedAndDoesNotAcknowledgeWrite(t *testing.T) {
	server, config, root := testClientServer(t)
	server.Close()
	start := time.Now()
	var output bytes.Buffer
	request := `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"store","arguments":{}}}` + "\n"
	if err := Bridge(context.Background(), config, strings.NewReader(request), &output); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 6*time.Second || !strings.Contains(output.String(), "persistence not acknowledged") {
		t.Fatalf("outage was unbounded or falsely acknowledged: %s", output.String())
	}
	result, err := Hook(config, "codex", map[string]any{"cwd": root, "hook_event_name": "SessionStart"})
	if err != nil {
		t.Fatal(err)
	}
	text := result["hookSpecificOutput"].(map[string]any)["additionalContext"].(string)
	if !strings.Contains(text, "context unavailable") || !strings.Contains(text, "project_resolved=true") && strings.Contains(text, "context loaded") {
		t.Fatalf("offline hook claimed context: %s", text)
	}
}

func TestProjectIdentityAndUnresolvedScope(t *testing.T) {
	want := "git:github.com/Owner/Repo"
	for _, remote := range []string{"git@GitHub.com:Owner/Repo.git", "https://user:secret@github.com/Owner/Repo.git?secret=yes", "ssh://git@github.com:22/Owner/Repo.git"} {
		got, err := ProjectIdentity(remote)
		if err != nil || got != want {
			t.Fatalf("%q became %q: %v", remote, got, err)
		}
	}
	if first, _ := ProjectIdentity("https://github.com/Owner/Repo"); first == "git:github.com/owner/repo" {
		t.Fatal("repository path was lowercased")
	}
	for _, remote := range []string{"/local/path", "C:\\work", "file:///repo", ""} {
		if _, err := ProjectIdentity(remote); err == nil {
			t.Errorf("accepted local remote %q", remote)
		}
	}
	root := t.TempDir()
	if _, err := ResolveScope(root, "test-device"); err == nil {
		t.Fatal("unresolved folder became project scope")
	}
}

func TestPrivateTokenFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte(testToken+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	config := Config{URL: "http://127.0.0.1:8106/mcp", TokenFile: path, Device: "test"}
	if _, err := NewRemote(config); err != nil {
		t.Fatal(err)
	}
	config.TokenEnv = "ALSO_SET"
	if _, err := NewRemote(config); err == nil {
		t.Fatal("accepted two credential sources")
	}
	if runtime.GOOS != "windows" {
		config.TokenEnv = ""
		if err := os.Chmod(path, 0644); err != nil {
			t.Fatal(err)
		}
		if _, err := NewRemote(config); err == nil {
			t.Fatal("accepted public token file")
		}
	}
}
