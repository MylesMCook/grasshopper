package client

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// A copy of the test executable supplies a stalled Git process on every OS.
func TestMain(m *testing.M) {
	if os.Getenv("GRASSHOPPER_TEST_STALLED_GIT") == "1" && strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe") == "git" {
		time.Sleep(5 * time.Second)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func TestHookReturnsContextWhenGitStalls(t *testing.T) {
	_, config, root := testClientServer(t)
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	name := "git"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if err := os.WriteFile(filepath.Join(bin, name), data, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("GRASSHOPPER_TEST_STALLED_GIT", "1")
	for _, event := range []string{"SessionStart", "UserPromptSubmit"} {
		t.Run(event, func(t *testing.T) {
			start := time.Now()
			output, err := Hook(config, "codex", map[string]any{"cwd": root, "hook_event_name": event})
			elapsed := time.Since(start)
			if err != nil {
				t.Fatal(err)
			}
			if elapsed >= 4*time.Second {
				t.Fatalf("stalled Git consumed the hook budget: %s", elapsed)
			}
			text := output["hookSpecificOutput"].(map[string]any)["additionalContext"].(string)
			for _, expected := range []string{"Canonical memory policy marker.", "Grasshopper context loaded", "project_resolved=false"} {
				if !strings.Contains(text, expected) {
					t.Fatalf("stalled Git lost %q: %s", expected, text)
				}
			}
		})
	}
}
