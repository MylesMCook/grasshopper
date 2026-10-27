package client

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// pluginWiringDigest fingerprints the hook and MCP templates that installed
// plugins carry. Self-update replaces the client, not this wiring, so a
// change here reaches agents only through a plugin update.
const pluginWiringDigest = "e257ab3c1c1be841338629b28eff41fd4bad147ad1150274f706a06e4a9f1eee"

// TestPluginWiringChangesRaiseMinimumPluginVersion fails when plugin
// templates change. Set MinimumPluginVersion to the release that will ship
// the change, so check asks for plugin updates, then update the digest.
func TestPluginWiringChangesRaiseMinimumPluginVersion(t *testing.T) {
	root := filepath.Join("..", "..", "integrations", "plugins")
	var names []string
	for _, harness := range []string{"claude", "codex", "cursor"} {
		err := filepath.WalkDir(filepath.Join(root, harness), func(path string, entry fs.DirEntry, err error) error {
			if err == nil && !entry.IsDir() {
				names = append(names, path)
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	sort.Strings(names)
	hash := sha256.New()
	for _, name := range names {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		relative, _ := filepath.Rel(root, name)
		hash.Write([]byte(filepath.ToSlash(relative) + "\n"))
		// Windows checkouts may convert line endings.
		hash.Write(bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n")))
	}
	if got := hex.EncodeToString(hash.Sum(nil)); got != pluginWiringDigest {
		t.Fatalf("plugin wiring changed (digest %s). Raise MinimumPluginVersion (now %s) to the release that ships it, then update pluginWiringDigest.", got, MinimumPluginVersion)
	}
}
