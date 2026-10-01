//go:build windows

package client

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWindowsDefaultSurvivesAppDataChanges(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("GRASSHOPPER_CLIENT_CONFIG", "")
	wanted := filepath.Join(home, ".grasshopper", "client.json")
	for _, appdata := range []string{t.TempDir(), t.TempDir()} {
		t.Setenv("AppData", appdata)
		path, err := ConfigPath("")
		if err != nil || path != wanted {
			t.Fatalf("shared default %q: %v", path, err)
		}
	}
	legacy, err := LegacyConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Dir(legacy), 0700)
	os.WriteFile(legacy, []byte("legacy"), 0600)
	path, _ := ConfigPath("")
	if path != legacy {
		t.Fatal("working legacy path not retained")
	}
	os.MkdirAll(filepath.Dir(wanted), 0700)
	os.WriteFile(wanted, []byte("canonical"), 0600)
	path, _ = ConfigPath("")
	if path != wanted {
		t.Fatal("canonical path not authoritative")
	}
	t.Setenv("GRASSHOPPER_CLIENT_CONFIG", legacy)
	path, _ = ConfigPath("")
	if path != legacy {
		t.Fatal("environment override ignored")
	}
	path, _ = ConfigPath("explicit")
	if path != "explicit" {
		t.Fatal("explicit path ignored")
	}
}
