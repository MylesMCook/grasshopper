package client

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStartupGuidesSanitizedTaskRecall(t *testing.T) {
	_, config, root := testClientServer(t)
	for _, harness := range []string{"codex", "cursor", "claude"} {
		output, err := Hook(config, harness, map[string]any{"cwd": root, "hook_event_name": "SessionStart"})
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(output)
		text := strings.ToLower(string(raw))
		if !strings.Contains(text, "sanitized") || !strings.Contains(text, "search") {
			t.Fatalf("%s lacks sanitized task recall guidance", harness)
		}
	}
}
