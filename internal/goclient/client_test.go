package goclient

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
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

func TestInstalledClientUsesOneConfigAndPolicy(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "client.json")
	if err := os.WriteFile(configPath, []byte(`{"url":"https://example.invalid/mcp","token_env":"GRASSHOPPER_TEST_TOKEN","device":"test-device"}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GRASSHOPPER_CLIENT_CONFIG", configPath)
	path, err := ConfigPath("")
	if err != nil || path != configPath {
		t.Fatalf("plugin config path = %q: %v", path, err)
	}
	if path, err := ConfigPath("explicit.json"); err != nil || path != "explicit.json" {
		t.Fatalf("explicit config path = %q: %v", path, err)
	}
	config, err := LoadConfig(configPath)
	if err != nil || config.PolicyPath != filepath.Join(root, "AGENTS.md") {
		t.Fatalf("shared policy path = %q: %v", config.PolicyPath, err)
	}
}

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
	pretool, err := Hook(config, "claude", map[string]any{"cwd": nested, "hook_event_name": "SubagentStart"})
	if err != nil {
		t.Fatal(err)
	}
	guidance := pretool["hookSpecificOutput"].(map[string]any)["additionalContext"].(string)
	for _, expected := range []string{"Canonical memory policy marker.", "Root marker.", "Nested marker."} {
		if !strings.Contains(guidance, expected) {
			t.Errorf("missing %q in nested guidance", expected)
		}
	}
	startup, err := Hook(config, "claude", map[string]any{"cwd": nested, "hook_event_name": "SessionStart"})
	if err != nil {
		t.Fatal(err)
	}
	startupText := startup["hookSpecificOutput"].(map[string]any)["additionalContext"].(string)
	if !strings.Contains(startupText, "Root marker.") || !strings.Contains(startupText, "Nested marker.") {
		t.Fatal("Claude startup did not load applicable AGENTS.md guidance")
	}
	codexStartup, err := Hook(config, "codex", map[string]any{"cwd": nested, "hook_event_name": "SessionStart"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(codexStartup["hookSpecificOutput"].(map[string]any)["additionalContext"].(string), "Root marker.") {
		t.Fatal("Codex startup duplicated native AGENTS.md guidance")
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
	if strings.Contains(loaded, `"structuredContent"`) || strings.Contains(loaded, `"content":[`) {
		t.Fatal("hook repeated the MCP text and structured payload")
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

func TestCodexPromptFallbackLoadsContextOncePerSession(t *testing.T) {
	_, config, root := testClientServer(t)
	t.Setenv("PLUGIN_DATA", t.TempDir())
	input := map[string]any{"cwd": root, "hook_event_name": "UserPromptSubmit", "session_id": "synthetic-session"}
	first, err := Hook(config, "codex", input)
	if err != nil {
		t.Fatal(err)
	}
	output := first["hookSpecificOutput"].(map[string]any)
	if output["hookEventName"] != "UserPromptSubmit" || !strings.Contains(output["additionalContext"].(string), "Canonical memory policy marker.") {
		t.Fatalf("prompt hook did not deliver context: %v", first)
	}
	second, err := Hook(config, "codex", input)
	if err != nil || len(second) != 0 {
		t.Fatalf("prompt hook repeated context in one session: %v, %v", second, err)
	}
	input["session_id"] = "fresh-session"
	third, err := Hook(config, "codex", input)
	if err != nil || third["hookSpecificOutput"] == nil {
		t.Fatalf("fresh session lacked context: %v, %v", third, err)
	}
	input["session_id"] = "startup-session"
	input["hook_event_name"] = "SessionStart"
	if _, err := Hook(config, "codex", input); err != nil {
		t.Fatal(err)
	}
	input["hook_event_name"] = "UserPromptSubmit"
	if fallback, err := Hook(config, "codex", input); err != nil || fallback["hookSpecificOutput"] == nil {
		t.Fatalf("prompt fallback was suppressed by startup: %v, %v", fallback, err)
	}
}

func TestCodexPromptFallbackOutageDoesNotRetry(t *testing.T) {
	server, config, root := testClientServer(t)
	server.Close()
	t.Setenv("PLUGIN_DATA", t.TempDir())
	input := map[string]any{"cwd": root, "hook_event_name": "UserPromptSubmit", "session_id": "offline-session"}
	first, err := Hook(config, "codex", input)
	if err != nil {
		t.Fatal(err)
	}
	text := first["hookSpecificOutput"].(map[string]any)["additionalContext"].(string)
	if !strings.Contains(text, "context unavailable") || strings.Contains(text, "context loaded") {
		t.Fatalf("offline prompt hook claimed memory: %s", text)
	}
	second, err := Hook(config, "codex", input)
	if err != nil || len(second) != 0 {
		t.Fatalf("offline prompt hook retried automatically: %v, %v", second, err)
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

func TestHookSlowBackendDoesNotHoldUpStartup(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		<-release
	}))
	defer server.Close()
	defer close(release)
	root := t.TempDir()
	policy := filepath.Join(root, "AGENTS.md")
	if err := os.WriteFile(policy, []byte("Canonical memory policy marker."), 0600); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(root, "client.json")
	data, _ := json.Marshal(Config{URL: server.URL + "/mcp", TokenEnv: "GRASSHOPPER_TEST_TOKEN", PolicyPath: policy, Device: "synthetic-mac"})
	if err := os.WriteFile(config, data, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GRASSHOPPER_TEST_TOKEN", testToken)
	start := time.Now()
	result, err := Hook(config, "codex", map[string]any{"cwd": root, "hook_event_name": "SessionStart"})
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 4500*time.Millisecond {
		t.Fatalf("slow backend held startup for %s", elapsed)
	}
	text := result["hookSpecificOutput"].(map[string]any)["additionalContext"].(string)
	if !strings.Contains(text, "context unavailable") || strings.Contains(text, "context loaded") {
		t.Fatalf("slow backend was reported as loaded: %s", text)
	}
}

func TestClaudeGlobalGuidanceParts(t *testing.T) {
	_, config, root := testClientServer(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	global := filepath.Join(home, ".claude", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(global), 0700); err != nil {
		t.Fatal(err)
	}
	content := "GLOBAL-TAIL:" + strings.Repeat("x", 8487) + "é" + strings.Repeat("y", 3800)
	if err := os.WriteFile(global, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("Root marker."), 0600); err != nil {
		t.Fatal(err)
	}
	input := map[string]any{"cwd": root, "hook_event_name": "SessionStart"}
	result, err := Hook(config, "claude", input)
	if err != nil {
		t.Fatal(err)
	}
	output := result["hookSpecificOutput"].(map[string]any)["additionalContext"].(string)
	if len(output) > 9000 || !strings.Contains(output, "Root marker.") || strings.Contains(output, "GLOBAL-TAIL:") {
		t.Fatalf("main hook repeated or lost guidance: %d bytes", len(output))
	}
	var joined string
	for part := 1; part <= 2; part++ {
		result, err := HookGlobalPart(part, input)
		if err != nil {
			t.Fatal(err)
		}
		chunk := result["hookSpecificOutput"].(map[string]any)["additionalContext"].(string)
		if len(chunk) > 10000 {
			t.Fatalf("global part %d exceeded Claude's hook limit", part)
		}
		_, text, ok := strings.Cut(chunk, ":\n")
		if !ok {
			t.Fatalf("global part %d lacks label", part)
		}
		joined += text
	}
	if joined != content {
		t.Fatal("global AGENTS.md parts lost or repeated content")
	}
	if err := os.WriteFile(global, []byte(strings.Repeat("z", 17001)), 0600); err != nil {
		t.Fatal(err)
	}
	tooLarge, err := HookGlobalPart(1, input)
	if err != nil {
		t.Fatal(err)
	}
	warning := tooLarge["hookSpecificOutput"].(map[string]any)["additionalContext"].(string)
	if !strings.Contains(warning, "full guidance was not loaded") || strings.Contains(warning, strings.Repeat("z", 100)) {
		t.Fatal("oversized guidance was silently truncated or emitted")
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

func TestSameOriginAcrossClonePathsSharesProjectMemory(t *testing.T) {
	root := t.TempDir()
	makeClone := func(name, remote string) string {
		t.Helper()
		path := filepath.Join(root, name, "checkout")
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
		for _, args := range [][]string{{"init", "-q"}, {"remote", "add", "origin", remote}} {
			if output, err := exec.Command("git", append([]string{"-C", path}, args...)...).CombinedOutput(); err != nil {
				t.Fatalf("git fixture: %v: %s", err, output)
			}
		}
		return path
	}
	macPath := makeClone("mac", "https://user:credential@github.com/Owner/SameRepo.git")
	linuxPath := makeClone("linux", "git@github.com:Owner/SameRepo.git")
	mac, err := ResolveScope(macPath, "mac-device")
	if err != nil {
		t.Fatal(err)
	}
	linux, err := ResolveScope(linuxPath, "linux-device")
	if err != nil {
		t.Fatal(err)
	}
	if *mac.Project != "git:github.com/Owner/SameRepo" || *mac.Project != *linux.Project {
		t.Fatalf("clone paths or remote syntax split one project: mac=%v linux=%v", mac.Project, linux.Project)
	}

	source := filepath.Join("..", "..", "tests", "fixtures", "go-compat", "memory.db")
	store, err := gomemory.OpenWritableCopy(context.Background(), source, filepath.Join(root, "shared.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	projectOnly := gomemory.Scope{Project: mac.Project}
	receipt, err := store.Write(context.Background(), gomemory.WriteInput{
		Scope: projectOnly, Content: "Synthetic same-project decision.", Purpose: "decision", Confirmed: true,
		Provenance: gomemory.Provenance{Harness: "codex", Device: "mac-device", Source: "synthetic cross-clone test"},
		RequestID:  "cross-clone-project-decision",
	}, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	linuxPlatform := "linux"
	seen, err := store.Context(context.Background(), gomemory.Scope{Project: linux.Project, Device: linux.Device, Platform: &linuxPlatform}, 16000)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, record := range seen.Records {
		found = found || record.ID == receipt.ID
	}
	if !found {
		t.Fatal("second clone did not receive project-only memory")
	}
	other := "git:github.com/Owner/OtherRepo"
	unrelated, err := store.Context(context.Background(), gomemory.Scope{Project: &other, Device: linux.Device, Platform: &linuxPlatform}, 16000)
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range unrelated.Records {
		if record.ID == receipt.ID {
			t.Fatal("project memory leaked to unrelated repository")
		}
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
