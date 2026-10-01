package client

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProductPolicyIsToolNeutralAndBoundsHandoffs(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "integrations", "policy", "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	policy := string(data)
	for _, name := range []string{"Linear", "Bitwarden", "Beelink", "Mac mini"} {
		if strings.Contains(policy, name) {
			t.Errorf("product policy names personal tool or host %q", name)
		}
	}
	if !strings.Contains(policy, "under 600 characters") {
		t.Error("handoff size guidance missing")
	}
}
