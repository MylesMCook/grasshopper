package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	checkAgents = func(string) []agentReport { return nil }
	os.Exit(m.Run())
}

// fakeAgents answers agent CLI calls from a table keyed by the full command
// line. Missing keys behave like an agent CLI that is not installed.
func fakeAgents(responses map[string]string) commandRunner {
	return func(name string, args ...string) ([]byte, error) {
		key := strings.Join(append([]string{name}, args...), " ")
		if output, ok := responses[key]; ok {
			return []byte(output), nil
		}
		return nil, fmt.Errorf("%s: %w", key, exec.ErrNotFound)
	}
}

func claudePluginList(id, version string) string {
	return fmt.Sprintf(`[{"id":"other@market","version":"1.0.0"},{"id":%q,"version":%q,"enabled":true}]`, id, version)
}

func codexPluginList(id, version string) string {
	return fmt.Sprintf(`{"installed":[{"pluginId":"other@market","version":"1.0.0"},{"pluginId":%q,"version":%q,"enabled":true}]}`, id, version)
}

// cursorWired writes Cursor's direct MCP and hook wiring for binary.
func cursorWired(t *testing.T, cursorDir, binary string) {
	t.Helper()
	if err := os.MkdirAll(cursorDir, 0o700); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(t.TempDir(), "client.json")
	mcp := map[string]any{"mcpServers": map[string]any{"grasshopper": grasshopperMCPEntry(binary, config)}}
	hooks := map[string]any{"version": 1, "hooks": map[string]any{"sessionStart": []any{map[string]any{
		"command": shellQuoted(binary) + " hook --config " + shellQuoted(config) + " --harness cursor", "timeout": 8,
	}}}}
	for name, value := range map[string]any{"mcp.json": mcp, "hooks.json": hooks} {
		data, _ := json.Marshal(value)
		if err := os.WriteFile(filepath.Join(cursorDir, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func fakeBinary(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "bin", "grasshopper.exe")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func agentByName(t *testing.T, reports []agentReport, agent string) agentReport {
	t.Helper()
	var found []agentReport
	for _, report := range reports {
		if report.Agent == agent {
			found = append(found, report)
		}
	}
	if len(found) != 1 {
		t.Fatalf("want one %s report, got %+v", agent, reports)
	}
	return found[0]
}

func TestCheckListsAgentsMatchingServerAsCurrent(t *testing.T) {
	cursorDir := filepath.Join(t.TempDir(), ".cursor")
	binary := fakeBinary(t)
	cursorWired(t, cursorDir, binary)
	run := fakeAgents(map[string]string{
		"claude plugin list --json": claudePluginList("grasshopper-windows@grasshopper-marketplace", "2.9.1"),
		"codex plugin list --json":  codexPluginList("grasshopper-windows@grasshopper-marketplace", "2.9.1"),
		binary + " --version":       "grasshopper 2.9.1\n",
	})

	reports := inspectAgents(run, cursorDir, "2.9.1")

	if len(reports) != 3 {
		t.Fatalf("want Claude Code, Codex and Cursor, got %+v", reports)
	}
	for _, report := range reports {
		if report.State != "current" || report.Version != "2.9.1" || report.Update != "" {
			t.Fatalf("want current 2.9.1 without an update step, got %+v", report)
		}
	}
	if err := agentProblem(reports); err != nil {
		t.Fatalf("current agents must not fail check: %v", err)
	}
}

func TestCheckShowsUpdateForAgentBehindServer(t *testing.T) {
	cursorDir := filepath.Join(t.TempDir(), ".cursor")
	binary := fakeBinary(t)
	cursorWired(t, cursorDir, binary)
	run := fakeAgents(map[string]string{
		"claude plugin list --json": claudePluginList("grasshopper-windows@grasshopper-marketplace", "2.8.0"),
		"codex plugin list --json":  codexPluginList("grasshopper@grasshopper-local", "2.9.1"),
		binary + " --version":       "grasshopper 2.6.0\n",
	})

	reports := inspectAgents(run, cursorDir, "2.9.1")

	cursor := agentByName(t, reports, "Cursor")
	if cursor.State != "behind" || cursor.Version != "2.6.0" || !strings.Contains(cursor.Update, "setup --agents cursor --update") {
		t.Fatalf("Cursor should be behind with its update command, got %+v", cursor)
	}
	claude := agentByName(t, reports, "Claude Code")
	if claude.State != "behind" || !strings.Contains(claude.Update, "claude plugin update grasshopper-windows@grasshopper-marketplace") {
		t.Fatalf("Claude Code should be behind with its update command, got %+v", claude)
	}
	if codex := agentByName(t, reports, "Codex"); codex.State != "current" {
		t.Fatalf("Codex matches the server, got %+v", codex)
	}
	if err := agentProblem(reports); err != nil {
		t.Fatalf("an agent behind the server must not fail check: %v", err)
	}
}

func TestCheckFailsWhenAgentPointsAtMissingProgram(t *testing.T) {
	cursorDir := filepath.Join(t.TempDir(), ".cursor")
	missing := filepath.Join(t.TempDir(), "client", "2.6.0", "bin", "grasshopper.exe")
	cursorWired(t, cursorDir, missing)

	reports := inspectAgents(fakeAgents(nil), cursorDir, "2.9.1")

	cursor := agentByName(t, reports, "Cursor")
	if cursor.State != "broken" || !strings.Contains(cursor.Detail, missing) || cursor.Update == "" {
		t.Fatalf("Cursor should be broken, name the missing program and say how to fix it, got %+v", cursor)
	}
	err := agentProblem(reports)
	if err == nil || !strings.Contains(err.Error(), "Cursor") {
		t.Fatalf("a missing program must fail check and name the agent, got %v", err)
	}
}

func TestCheckFailsWhenOnlyCursorHookProgramIsMissing(t *testing.T) {
	cursorDir := filepath.Join(t.TempDir(), ".cursor")
	binary := fakeBinary(t)
	cursorWired(t, cursorDir, binary)
	missing := filepath.Join(t.TempDir(), "old", "grasshopper.exe")
	hooks := map[string]any{"version": 1, "hooks": map[string]any{"sessionStart": []any{map[string]any{
		"command": shellQuoted(missing) + " hook --config " + shellQuoted("client.json") + " --harness cursor",
	}}}}
	data, _ := json.Marshal(hooks)
	if err := os.WriteFile(filepath.Join(cursorDir, "hooks.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	run := fakeAgents(map[string]string{binary + " --version": "grasshopper 2.9.1\n"})

	cursor := agentByName(t, inspectAgents(run, cursorDir, "2.9.1"), "Cursor")
	if cursor.State != "broken" || !strings.Contains(cursor.Detail, missing) {
		t.Fatalf("a missing startup hook program is broken wiring, got %+v", cursor)
	}
}

func TestCheckOmitsAgentsThatAreNotInstalled(t *testing.T) {
	reports := inspectAgents(fakeAgents(nil), filepath.Join(t.TempDir(), ".cursor"), "2.9.1")

	if len(reports) != 0 {
		t.Fatalf("no agents are installed, got %+v", reports)
	}
	if err := agentProblem(reports); err != nil {
		t.Fatalf("missing agents must not fail check: %v", err)
	}
}

func TestCheckReportsAgentCLIFailureWithoutHidingIt(t *testing.T) {
	run := func(name string, args ...string) ([]byte, error) {
		if name == "codex" {
			return nil, errors.New("codex plugin list failed: exit status 2")
		}
		return nil, exec.ErrNotFound
	}

	codex := agentByName(t, inspectAgents(run, filepath.Join(t.TempDir(), ".cursor"), "2.9.1"), "Codex")
	if codex.State != "unknown" || codex.Detail == "" {
		t.Fatalf("an installed but failing Codex CLI should be reported as unknown, got %+v", codex)
	}
}

func TestCheckListsOldCursorMarketplacePlugin(t *testing.T) {
	cursorDir := filepath.Join(t.TempDir(), ".cursor")
	manifest := filepath.Join(cursorDir, "plugins", "cache", "grasshopper-marketplace", "grasshopper-windows", "5ad8fac", ".cursor-plugin", "plugin.json")
	if err := os.MkdirAll(filepath.Dir(manifest), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest, []byte(`{"name":"grasshopper-windows","version":"2.2.0"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	cursor := agentByName(t, inspectAgents(fakeAgents(nil), cursorDir, "2.9.1"), "Cursor")
	if cursor.State != "behind" || cursor.Version != "2.2.0" || !strings.Contains(cursor.Route, "grasshopper-windows") ||
		!strings.Contains(cursor.Update, "agent plugin marketplace remove grasshopper-marketplace") {
		t.Fatalf("an old Cursor marketplace plugin should be behind with its update steps, got %+v", cursor)
	}
}

func TestCheckListsAgentsWhenServerVersionIsUnknown(t *testing.T) {
	run := fakeAgents(map[string]string{
		"claude plugin list --json": claudePluginList("grasshopper-windows@grasshopper-marketplace", "2.9.1"),
	})

	claude := agentByName(t, inspectAgents(run, filepath.Join(t.TempDir(), ".cursor"), ""), "Claude Code")
	if claude.State != "unknown" || claude.Version != "2.9.1" {
		t.Fatalf("an unknown server version cannot be compared, got %+v", claude)
	}

	var out bytes.Buffer
	err := renderCheck(&out, map[string]string{"status": "unreachable_server", "next_step": "The private server is unavailable."}, []agentReport{claude})
	if err == nil {
		t.Fatal("an unreachable server must still fail check")
	}
	text := out.String()
	if !strings.Contains(text, "Server version unknown") || !strings.Contains(text, "Claude Code") || !strings.Contains(text, "2.9.1") {
		t.Fatalf("unreachable server output must still list agents, got:\n%s", text)
	}
}

func TestCheckRendersAgentsWithUpdateSteps(t *testing.T) {
	var out bytes.Buffer
	result := map[string]string{"status": "connected", "device": "laptop", "registration": "verified", "host_version": "2.9.1", "next_step": connectedNextStep}
	agents := []agentReport{
		{Agent: "Claude Code", Version: "2.9.1", Route: "plugin grasshopper-windows@grasshopper-marketplace", State: "current"},
		{Agent: "Cursor", Version: "2.6.0", Route: "Cursor settings", State: "behind", Update: "Run grasshopper setup --agents cursor --update from the new client archive."},
	}

	if err := renderCheck(&out, result, agents); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{"Grasshopper connected", "Server 2.9.1", "Claude Code", "current", "Cursor", "2.6.0", "behind server", "setup --agents cursor --update"} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
}
