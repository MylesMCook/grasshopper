package pi

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// The client fills in its configuration by replacing ConfigLine, so the
// extension must carry it exactly once.
func TestExtensionCarriesTheConfigLineOnce(t *testing.T) {
	if n := strings.Count(Extension, ConfigLine); n != 1 {
		t.Fatalf("extension has %d copies of %q, want 1", n, ConfigLine)
	}
}

// Pi shows a package changelog only when package.json's version rises, so it
// must follow VERSION, and the manifest must load this extension.
func TestPackageManifestFollowsVersion(t *testing.T) {
	data, err := os.ReadFile("../../package.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Version string `json:"version"`
		Pi      struct {
			Extensions    []string `json:"extensions"`
			ChangelogPath string   `json:"changelogPath"`
		} `json:"pi"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	version, err := os.ReadFile("../../VERSION")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Version != strings.TrimSpace(string(version)) {
		t.Fatalf("package.json version %q does not match VERSION %q", manifest.Version, strings.TrimSpace(string(version)))
	}
	if len(manifest.Pi.Extensions) != 1 || manifest.Pi.Extensions[0] != "./integrations/pi/extension.ts" {
		t.Fatalf("package.json must load integrations/pi/extension.ts, got %v", manifest.Pi.Extensions)
	}
	changelog, err := os.ReadFile("../../" + manifest.Pi.ChangelogPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(changelog), "## ["+manifest.Version+"]") {
		t.Fatalf("%s has no entry for %s", manifest.Pi.ChangelogPath, manifest.Version)
	}
}
