package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every font the memory view embeds ships in the server archive with its
// SIL OFL notice, as the license requires for redistribution.
func TestEveryEmbeddedFontShipsItsLicense(t *testing.T) {
	fonts, err := filepath.Glob("../../internal/service/visualizer/*.woff2")
	if err != nil || len(fonts) == 0 {
		t.Fatalf("embedded fonts: %v %v", fonts, err)
	}
	shipped := map[string]bool{}
	for _, notice := range fontNotices {
		shipped[notice.path] = true
	}
	for _, font := range fonts {
		family := strings.TrimSuffix(strings.TrimSuffix(filepath.Base(font), ".woff2"), "-latin")
		notice := "docs/fonts/OFL-" + family + ".txt"
		if !shipped[notice] {
			t.Errorf("%s ships without %s in the server archive", filepath.Base(font), notice)
		}
		if _, err := os.Stat(filepath.Join("../..", notice)); err != nil {
			t.Errorf("%s is missing: %v", notice, err)
		}
	}
}
