package client

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOversizedAncestorGuidanceStillDeliversTheHook(t *testing.T) {
	_, config, root := testClientServer(t)
	workspace := filepath.Join(root, "workspace")
	if err := os.Mkdir(workspace, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "AGENTS.md"), []byte(strings.Repeat("Long project rules.\n", 4000)), 0600); err != nil {
		t.Fatal(err)
	}
	output, err := Hook(config, "codex", map[string]any{"cwd": workspace, "hook_event_name": "SubagentStart"})
	if err != nil {
		t.Fatalf("oversized AGENTS.md broke the hook: %v", err)
	}
	text := output["hookSpecificOutput"].(map[string]any)["additionalContext"].(string)
	for _, expected := range []string{"Canonical memory policy marker.", "read the applicable AGENTS.md files from " + workspace} {
		if !strings.Contains(text, expected) {
			t.Fatalf("hook lost %q: %s", expected, text[:min(len(text), 400)])
		}
	}
}
