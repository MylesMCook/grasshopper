//go:build windows

package client

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Cursor on Windows passes no cwd and names the workspace as a URI-style root
// such as "/C:/Users/me/repo". The imported Claude hook previously failed on
// that root and returned only the startup-unavailable notice.
func TestCursorWindowsWorkspaceRootLoadsProjectContext(t *testing.T) {
	_, config, root := testClientServer(t)
	for _, args := range [][]string{{"init", "-q"}, {"remote", "add", "origin", "git@GitHub.com:Owner/Repo.git"}} {
		if output, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git fixture: %v: %s", err, output)
		}
	}
	input := map[string]any{
		"hook_event_name": "sessionStart",
		"cursor_version":  "3.23.12",
		"workspace_roots": []any{"/" + filepath.ToSlash(root)},
	}
	startup, err := Hook(config, "claude", input)
	if err != nil {
		t.Fatalf("Cursor's Windows workspace root must load context: %v", err)
	}
	text, _ := startup["additional_context"].(string)
	for _, expected := range []string{"project_resolved=true", "git:github.com/Owner/Repo"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("missing %q in Cursor startup context: %s", expected, text)
		}
	}
}

func TestWorkspaceRootPathOnWindows(t *testing.T) {
	for root, want := range map[string]string{
		"/C:/Users/me/repo": `C:\Users\me\repo`,
		"/d:/work":          `d:\work`,
		`C:\Users\me\repo`:  `C:\Users\me\repo`,
		"/12:/not-a-drive":  "/12:/not-a-drive",
		"":                  "",
	} {
		if got := workspaceRootPath(root); got != want {
			t.Errorf("workspaceRootPath(%q) = %q, want %q", root, got, want)
		}
	}
}
