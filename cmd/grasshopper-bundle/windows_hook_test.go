//go:build windows

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The copied test binary checks what reaches the native Windows client.
func TestMain(m *testing.M) {
	if os.Getenv("GRASSHOPPER_TEST_HOOK_CLIENT") == "1" && filepath.Base(os.Args[0]) == "grasshopper.exe" {
		input, err := io.ReadAll(os.Stdin)
		if err != nil {
			os.Exit(1)
		}
		_ = json.NewEncoder(os.Stdout).Encode(struct {
			Args  []string
			Input string
		}{os.Args[1:], string(input)})
		os.Exit(7)
	}
	os.Exit(m.Run())
}

func TestWindowsHookPreservesInputAndExitStatus(t *testing.T) {
	for _, shell := range []string{"cmd", "powershell"} {
		for _, name := range []string{"plugin's path with spaces", "plugin & (test) %HOOK_LITERAL% !bang"} {
			t.Run(shell+"/"+name, func(t *testing.T) { testWindowsHookPath(t, name, shell) })
		}
	}
}

func testWindowsHookPath(t *testing.T, name, shell string) {
	root := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(filepath.Join(root, "bin"), 0700); err != nil {
		t.Fatal(err)
	}
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "bin", "grasshopper.exe"), data, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PLUGIN_ROOT", root)
	t.Setenv("HOOK_LITERAL", "must-not-replace-the-literal-path")
	t.Setenv("GRASSHOPPER_TEST_HOOK_CLIENT", "1")
	input := `{"hook_event_name":"SessionStart","cwd":"C:\\example with spaces"}`
	// This synthetic-client test checks transport and failure propagation, not
	// the hook's latency. The packaged eight-second deadline is asserted separately;
	// native Codex execution verifies the real startup and prompt timing.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "cmd.exe")
	// Match the outer quotes used by Codex's Windows command runner.
	command.SysProcAttr = &syscall.SysProcAttr{CmdLine: `cmd.exe /c "` + windowsHookCommand() + `"`}
	expectedExit := 7
	if shell == "powershell" {
		command = exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", windowsHookCommand())
		// PowerShell normalizes a failing native command to exit 1.
		expectedExit = 1
	}
	command.Stdin = strings.NewReader(input)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err = command.Run()
	if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != expectedExit {
		t.Fatalf("client exit status lost: %v; %s", err, stderr.String())
	}
	var reply struct {
		Args  []string
		Input string
	}
	if err := json.Unmarshal(stdout.Bytes(), &reply); err != nil {
		t.Fatalf("client JSON output lost: %v; %s", err, stdout.String())
	}
	if strings.Join(reply.Args, " ") != "hook --harness codex" || reply.Input != input {
		t.Fatalf("client arguments or stdin changed: %+v", reply)
	}
	if stderr.Len() != 0 {
		t.Fatalf("launcher emitted unexpected output: %s", stderr.String())
	}
}
