package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCheckTreatsPluginsOnTheSelfUpdatedClientAsCurrent(t *testing.T) {
	reports := []agentReport{
		{Agent: "Claude Code", Version: "2.11.0", State: agentBehind, Update: "claude plugin update"},
		{Agent: "Codex", Version: "2.10.5", State: agentBehind, Update: "codex plugin add"},
	}

	reports = withManagedClient(reports, "2.11.2", "2.11.2")

	if claude := reports[0]; claude.State != agentCurrent || claude.Update != "" || !strings.Contains(claude.Detail, "2.11.2") {
		t.Fatalf("a plugin with current wiring runs the managed client, got %+v", claude)
	}
	if codex := reports[1]; codex.State != agentBehind || codex.Update == "" {
		t.Fatalf("a plugin without hand-off still needs its own update, got %+v", codex)
	}
}

func TestCheckKeepsPluginsBehindWhenTheManagedClientIsBehind(t *testing.T) {
	reports := []agentReport{{Agent: "Claude Code", Version: "2.11.0", State: agentBehind, Update: "claude plugin update"}}
	if got := withManagedClient(reports, "2.11.1", "2.11.2")[0]; got.State != agentBehind {
		t.Fatalf("managed client behind the server should not hide the plugin, got %+v", got)
	}
}

func TestCheckWarnsWhenAnAgentRunsAClientNewerThanTheServer(t *testing.T) {
	reports := []agentReport{
		{Agent: "Claude Code", Version: "2.12.0", State: agentNewer},
		{Agent: "Codex", Version: "2.11.0", State: agentBehind, Update: "codex plugin update"},
		{Agent: "Cursor", Version: "2.11.0", State: agentBroken},
	}
	// A managed client ahead of the server makes agents that hand off to it
	// newer too.
	reports = withManagedClient(reports, "2.11.2", "2.11.1")
	if codex := reports[1]; codex.State != agentNewer || !strings.Contains(codex.Detail, "2.11.2") {
		t.Fatalf("Codex hands off to 2.11.2, got %+v", codex)
	}
	var out bytes.Buffer
	result := map[string]string{"status": "connected", "host_version": "2.11.1", "next_step": connectedNextStep}
	_ = renderCheck(&out, result, reports) // the broken Cursor fails check
	var warnings []string
	for _, line := range strings.Split(out.String(), "\n") {
		if strings.HasPrefix(line, "Warning:") {
			warnings = append(warnings, line)
		}
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "Claude Code and Codex") || strings.Contains(warnings[0], "Cursor") {
		t.Fatalf("want one warning naming Claude Code and Codex, got %q\n%s", warnings, out.String())
	}
}

// TestHookHandsOffToANewerManagedClient runs a real bundled client whose
// managed folder holds a newer "client" script, and checks the script got the
// agent's arguments, stdin and the bundled version.
func TestHookHandsOffToANewerManagedClient(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as the managed client")
	}
	root := t.TempDir()
	bundled := filepath.Join(root, "bundled")
	if output, err := exec.Command("go", "build", "-ldflags=-X main.clientVersion=2.11.0", "-o", bundled, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, output)
	}
	configPath := filepath.Join(root, "client.json")
	versionDir := filepath.Join(root, "client", "versions", "2.11.1")
	if err := os.MkdirAll(versionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\nread line\necho \"managed $GRASSHOPPER_HANDED_OFF $* $line\"\n"
	if err := os.WriteFile(filepath.Join(versionDir, "grasshopper"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "client", "current"), []byte("2.11.1\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	command := exec.Command(bundled, "hook", "--config", configPath, "--harness", "claude")
	command.Stdin = strings.NewReader("{}\n")
	command.Env = os.Environ()
	output, err := command.Output()
	if err != nil {
		t.Fatalf("hook: %v", err)
	}
	want := "managed 2.11.0 hook --config " + configPath + " --harness claude {}"
	if strings.TrimSpace(string(output)) != want {
		t.Fatalf("hand-off output = %q, want %q", output, want)
	}

	// A managed client never hands off again.
	command = exec.Command(bundled, "hook", "--config", configPath, "--harness", "claude")
	command.Stdin = strings.NewReader("{}\n")
	command.Env = append(os.Environ(), handedOffEnv+"=2.11.0")
	output, _ = command.Output()
	if strings.Contains(string(output), "managed") {
		t.Fatalf("handed-off client handed off again: %q", output)
	}
}
