package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStartupExplainsOwnerSignInForEachPlatform(t *testing.T) {
	for _, tc := range []struct{ platform, path, command string }{
		{"darwin", "/custom state/access-token", "pbcopy < '/custom state/access-token'"},
		{"linux", "/custom state/access-token", "cat -- '/custom state/access-token'"},
		{"windows", `C:\custom state\access-token`, `Get-Content -LiteralPath 'C:\custom state\access-token' -Raw | Set-Clipboard`},
		{"darwin", `/state/owner's "token"`, `pbcopy < '/state/owner'"'"'s "token"'`},
		{"linux", `/state/owner's $(command)`, `cat -- '/state/owner'"'"'s $(command)'`},
		{"windows", `C:\owner's state\access-token`, `Get-Content -LiteralPath 'C:\owner''s state\access-token' -Raw | Set-Clipboard`},
	} {
		t.Run(tc.platform+tc.path, func(t *testing.T) {
			var output bytes.Buffer
			writeStartup(&output, tc.platform, "127.0.0.1:12345", "/actual/memory.db", tc.path, true)
			text := output.String()
			for _, want := range []string{"Memory view: http://127.0.0.1:12345/visualizer/", "Access token file: " + tc.path, "Sign in", tc.command, "Never paste the token into an agent chat"} {
				if !strings.Contains(text, want) {
					t.Errorf("startup guidance lacks %q: %s", want, text)
				}
			}
			if strings.Index(text, "Sign in") < strings.Index(text, "Access token file:") {
				t.Fatal("sign-in guidance must follow the token path")
			}
		})
	}
}

func TestStartupDoesNotOfferUnavailableMemoryView(t *testing.T) {
	var output bytes.Buffer
	writeStartup(&output, "darwin", "127.0.0.1:12345", "/actual/memory.db", "/actual/token", false)
	if got := output.String(); got != "Grasshopper listening on http://127.0.0.1:12345/mcp\n" {
		t.Fatal("disabled viewer emitted sign-in guidance:", got)
	}
}

func TestStartupNamesTokenFileWithoutReadingItsSecret(t *testing.T) {
	path := filepath.Join(t.TempDir(), "owner's token")
	secret := "synthetic-owner-secret-never-print-this"
	if err := os.WriteFile(path, []byte(secret), 0600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	writeStartup(&output, "darwin", "127.0.0.1:12345", "/manual/memory.db", path, true)
	if text := output.String(); !strings.Contains(text, "Access token file: "+path) || strings.Contains(text, secret) {
		t.Fatal("startup disclosed token contents or lost configured path")
	}
}
