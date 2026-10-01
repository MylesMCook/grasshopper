package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/MylesMCook/grasshopper/internal/memory"
	"github.com/MylesMCook/grasshopper/internal/service"
)

const testToken = "synthetic-client-token-0123456789-abcdef"

func TestHookUnavailableIsVisibleInEachHarness(t *testing.T) {
	for _, harness := range []string{"codex", "cursor", "claude"} {
		output := HookUnavailable(harness, map[string]any{"hook_event_name": "SessionStart"})
		encoded, err := json.Marshal(output)
		if err != nil {
			t.Fatal(err)
		}
		message := string(encoded)
		if !strings.Contains(message, "startup unavailable") || !strings.Contains(message, "no memory write was acknowledged") {
			t.Fatalf("%s did not disclose startup failure: %s", harness, message)
		}
		if harness == "cursor" && output["additional_context"] == nil || harness != "cursor" && output["hookSpecificOutput"] == nil {
			t.Fatalf("%s returned the wrong hook shape: %s", harness, message)
		}
	}
}

// Cursor imports Claude Code plugins and runs their SessionStart hooks, but
// reads startup context only from additional_context.
func TestClaudeSessionStartAlsoReachesCursorImports(t *testing.T) {
	_, config, root := testClientServer(t)
	startup, err := Hook(config, "claude", map[string]any{"cwd": root, "hook_event_name": "SessionStart"})
	if err != nil {
		t.Fatal(err)
	}
	claudeText, _ := startup["hookSpecificOutput"].(map[string]any)["additionalContext"].(string)
	if claudeText == "" || startup["additional_context"] != claudeText {
		t.Fatalf("Claude SessionStart must carry the same context for Cursor imports: %v", startup)
	}
	unavailable := HookUnavailable("claude", map[string]any{"hook_event_name": "SessionStart"})
	if unavailable["additional_context"] != unavailable["hookSpecificOutput"].(map[string]any)["additionalContext"] {
		t.Fatalf("Cursor imports must also see the unavailable notice: %v", unavailable)
	}
	subagent, err := Hook(config, "claude", map[string]any{"cwd": root, "hook_event_name": "SubagentStart"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := subagent["additional_context"]; ok {
		t.Fatalf("Cursor does not map SubagentStart; keep its output Claude-only: %v", subagent)
	}
}

// Cursor on Windows starts hook input with a UTF-8 byte order mark and names
// the imported Claude SessionStart event sessionStart. Both previously made the
// imported hook fail before loading context.
func TestCursorImportedSessionStartLoadsContext(t *testing.T) {
	input, err := ParseHookInput(strings.NewReader("\xef\xbb\xbf" + `{"hook_event_name":"sessionStart","cursor_version":"3.23.12"}`))
	if err != nil {
		t.Fatalf("hook input with a byte order mark must parse: %v", err)
	}
	_, config, root := testClientServer(t)
	input["cwd"] = root
	startup, err := Hook(config, "claude", input)
	if err != nil {
		t.Fatalf("Cursor's sessionStart event must load context: %v", err)
	}
	text, _ := startup["additional_context"].(string)
	if !strings.Contains(text, "Grasshopper context loaded") {
		t.Fatalf("Cursor import did not receive memory context: %v", startup)
	}
	if _, err := HookGlobalPart(1, input); err != nil {
		t.Fatalf("Claude global guidance hooks must not fail under Cursor: %v", err)
	}
	unavailable := HookUnavailable("claude", input)
	if text, _ := unavailable["additional_context"].(string); !strings.Contains(text, "startup unavailable") {
		t.Fatalf("Cursor imports must see the unavailable notice for sessionStart: %v", unavailable)
	}
}

type blockedTransport struct{ err error }

func (b blockedTransport) RoundTrip(*http.Request) (*http.Response, error) { return nil, b.err }

func TestRemoteDistinguishesSandboxNetworkDenial(t *testing.T) {
	for _, denied := range []error{syscall.EPERM, syscall.EACCES} {
		remote := &Remote{endpoint: "http://127.0.0.1:18119/mcp", client: &http.Client{Transport: blockedTransport{denied}}}
		_, _, err := remote.Request(t.Context(), []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize"}`))
		if !errors.Is(err, ErrNetworkRestricted) {
			t.Fatalf("%v became %v", denied, err)
		}
	}
}

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
	store, err := memory.OpenWritableCopy(context.Background(), source, filepath.Join(t.TempDir(), "copy.db"))
	if err != nil {
		t.Fatal(err)
	}
	handler, err := service.NewHandler(service.Backend{Store: store}, testToken)
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
	store, err := memory.OpenWritableCopy(context.Background(), source, filepath.Join(root, "shared.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	projectOnly := memory.Scope{Project: mac.Project}
	receipt, err := store.Write(context.Background(), memory.WriteInput{
		Scope: projectOnly, Content: "Synthetic same-project decision.", Purpose: "decision", Confirmed: true,
		Provenance: memory.Provenance{Harness: "codex", Device: "mac-device", Source: "synthetic cross-clone test"},
		RequestID:  "cross-clone-project-decision",
	}, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	linuxPlatform := "linux"
	seen, err := store.Context(context.Background(), memory.Scope{Project: linux.Project, Device: linux.Device, Platform: &linuxPlatform}, 16000)
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
	unrelated, err := store.Context(context.Background(), memory.Scope{Project: &other, Device: linux.Device, Platform: &linuxPlatform}, 16000)
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

func TestPromptFallbackTracksWorkspaceAndResume(t *testing.T) {
	_, config, root := testClientServer(t)
	t.Setenv("PLUGIN_DATA", t.TempDir())
	input := map[string]any{"cwd": root, "hook_event_name": "UserPromptSubmit", "session_id": "same-session"}
	if _, err := Hook(config, "codex", input); err != nil {
		t.Fatal(err)
	}
	input["cwd"] = t.TempDir()
	if got, err := Hook(config, "codex", input); err != nil || len(got) == 0 {
		t.Fatalf("different workspace lost context: %v %v", got, err)
	}
	input["hook_event_name"] = "SessionStart"
	input["source"] = "resume"
	if _, err := Hook(config, "codex", input); err != nil {
		t.Fatal(err)
	}
	input["hook_event_name"] = "UserPromptSubmit"
	if got, err := Hook(config, "codex", input); err != nil || len(got) == 0 {
		t.Fatalf("resume suppressed fallback: %v %v", got, err)
	}
	input["hook_event_name"] = "SubagentStart"
	if got, err := Hook(config, "codex", input); err != nil || len(got) == 0 {
		t.Fatalf("subagent lost context: %v %v", got, err)
	}
	_, otherConfig, _ := testClientServer(t)
	input["hook_event_name"] = "UserPromptSubmit"
	if got, err := Hook(otherConfig, "codex", input); err != nil || len(got) == 0 {
		t.Fatalf("different server lost context: %v %v", got, err)
	}
}

func TestCompactHookPreservesMemoryMeaning(t *testing.T) {
	_, config, root := testClientServer(t)
	got, err := Hook(config, "cursor", map[string]any{"cwd": root})
	if err != nil {
		t.Fatal(err)
	}
	text := got["additional_context"].(string)
	for _, empty := range []string{`"project":null`, `"archived":false`, `"tags":""`, `"omitted_ids":[]`} {
		if strings.Contains(text, empty) {
			t.Errorf("hook still includes redundant %s", empty)
		}
	}
	for _, required := range []string{`"revision":`, `"confirmed":true`, `"updated_at":`, `"provenance":`, "Only use this preference for the synthetic compatibility test."} {
		if !strings.Contains(text, required) {
			t.Errorf("hook lost %s", required)
		}
	}
}

func TestCompactContextKeepsFullRecordsAndOmissions(t *testing.T) {
	original := map[string]any{"records": []any{map[string]any{
		"id": 99, "revision": 7, "scope": map[string]any{"project": "git:example.com/Owner/Repo", "device": nil, "platform": "windows", "legacy": false},
		"confirmed": false, "purpose": "handoff", "title": "Continue", "content": strings.Repeat("é", 220) + " Qualification: verify current Git state.",
		"tags": "review", "archived": false, "created_at": "earlier", "updated_at": "current", "provenance": map[string]any{"harness": "cursor", "device": "test", "source": "commit abc"}, "future_field": "preserved",
	}}, "omitted": 1, "omitted_ids": []int{100}, "omitted_records": []any{map[string]any{"id": 100, "revision": 2, "scope": map[string]any{"project": "id:other"}}}}
	raw, _ := json.Marshal(original)
	compact, err := compactContext(raw)
	if err != nil {
		t.Fatal(err)
	}
	var before, after map[string]any
	_ = json.Unmarshal(raw, &before)
	_ = json.Unmarshal(compact, &after)
	b := before["records"].([]any)[0].(map[string]any)
	a := after["records"].([]any)[0].(map[string]any)
	for _, key := range []string{"id", "revision", "confirmed", "purpose", "title", "content", "tags", "updated_at", "provenance", "future_field"} {
		x, _ := json.Marshal(a[key])
		y, _ := json.Marshal(b[key])
		if !bytes.Equal(x, y) {
			t.Errorf("changed %s", key)
		}
	}
	if after["omitted"] != float64(1) || len(after["omitted_records"].([]any)) != 1 {
		t.Fatal("lost omissions")
	}
	if len(compact) >= len(raw) {
		t.Fatal("presentation is not smaller")
	}
}

func TestPromptFallbackWithoutSessionDoesNotSuppressContext(t *testing.T) {
	_, config, root := testClientServer(t)
	t.Setenv("PLUGIN_DATA", t.TempDir())
	for i := 0; i < 2; i++ {
		got, err := Hook(config, "codex", map[string]any{"cwd": root, "hook_event_name": "UserPromptSubmit"})
		if err != nil || len(got) == 0 {
			t.Fatalf("missing identity suppressed context: %v %v", got, err)
		}
	}
}

func TestIdentityRejectsMalformedResponsesAndNetworkDenial(t *testing.T) {
	for _, body := range []string{`{}`, `{"role":"owner","device":"fake","version":"test"}`, `{"role":"device","device":"","version":"test"}`, strings.Repeat("x", 2049)} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
		remote := &Remote{endpoint: server.URL + "/mcp", client: server.Client()}
		if _, err := remote.Identity(t.Context()); err == nil {
			t.Fatalf("invalid identity accepted: %.80s", body)
		}
		server.Close()
	}
	remote := &Remote{endpoint: "http://127.0.0.1:1/mcp", client: &http.Client{Transport: blockedTransport{syscall.EPERM}}}
	if _, err := remote.Identity(t.Context()); !errors.Is(err, ErrNetworkRestricted) {
		t.Fatal("identity lost network-permission failure")
	}
}
