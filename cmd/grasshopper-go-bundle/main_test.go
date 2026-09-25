package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBundleIncludesChecksumsAndRejectsOverwrite(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "binary")
	if err := os.WriteFile(source, []byte("synthetic executable"), 0700); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(dir, "release.zip")
	if err := bundle(output, []input{{"bin/grasshopper-server", source}}); err != nil {
		t.Fatal(err)
	}
	archive, err := zip.OpenReader(output)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	if len(archive.File) != 2 || archive.File[0].Name != "bin/grasshopper-server" || archive.File[1].Name != "SHA256SUMS" {
		t.Fatalf("unexpected entries: %+v", archive.File)
	}
	reader, err := archive.File[1].Open()
	if err != nil {
		t.Fatal(err)
	}
	sums, err := io.ReadAll(reader)
	reader.Close()
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte("synthetic executable"))
	if !strings.Contains(string(sums), hex.EncodeToString(hash[:])+"  bin/grasshopper-server") {
		t.Fatalf("missing checksum: %s", sums)
	}
	if err := bundle(output, []input{{"bin/grasshopper-server", source}}); err == nil {
		t.Fatal("archive was overwritten")
	}
}

func TestClientPluginsPackageThreeHarnessesOnePolicy(t *testing.T) {
	working, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(filepath.Join(working, "..", "..")); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(working)
	dir := t.TempDir()
	client := filepath.Join(dir, "grasshopper")
	if err := os.WriteFile(client, []byte("synthetic client binary"), 0700); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(dir, "client-plugins.zip")
	files, cleanup, err := clientPluginFiles(client, "darwin-arm64", "0.1.0", output)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if err := bundle(output, files); err != nil {
		t.Fatal(err)
	}
	archive, err := zip.OpenReader(output)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	entries := map[string]*zip.File{}
	for _, entry := range archive.File {
		entries[entry.Name] = entry
		if strings.Contains(entry.Name, "token") || strings.HasSuffix(entry.Name, ".db") || strings.Contains(entry.Name, "model.onnx") {
			t.Fatalf("private or server data included: %s", entry.Name)
		}
	}
	if entries["bin/grasshopper"] == nil || entries["policy/AGENTS.md"] == nil || entries["cursor-mcp.example.json"] == nil || entries["cursor-cli.example.json"] == nil || entries["cursor-cli.md"] == nil || entries["SHA256SUMS"] == nil {
		t.Fatal("canonical policy, Cursor setup, or checksums missing")
	}
	installReader, err := entries["INSTALL.md"].Open()
	if err != nil {
		t.Fatal(err)
	}
	installGuide, err := io.ReadAll(installReader)
	installReader.Close()
	if err != nil || !strings.Contains(string(installGuide), "(cursor-cli.md)") || strings.Contains(string(installGuide), "../../docs/cursor-cli.md") {
		t.Fatalf("client install guide has a broken Cursor evidence link: %v", err)
	}
	cursorReader, err := entries["cursor-cli.md"].Open()
	if err != nil {
		t.Fatal(err)
	}
	cursorGuide, err := io.ReadAll(cursorReader)
	cursorReader.Close()
	if err != nil || !strings.Contains(string(cursorGuide), "(INSTALL.md)") {
		t.Fatalf("client Cursor guide has a broken install link: %v", err)
	}
	for _, harness := range []string{"codex", "cursor", "claude"} {
		root := harness + "/plugins/grasshopper/"
		if entries[root+"bin/grasshopper"] == nil || entries[root+"hooks/hooks.json"] == nil {
			t.Fatalf("%s binary or hooks missing", harness)
		}
		var manifest string
		switch harness {
		case "codex":
			manifest = root + ".codex-plugin/plugin.json"
		case "cursor":
			manifest = root + ".cursor-plugin/plugin.json"
		case "claude":
			manifest = root + ".claude-plugin/plugin.json"
		}
		reader, err := entries[manifest].Open()
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(reader)
		reader.Close()
		if err != nil || !json.Valid(content) || !strings.Contains(string(content), `"version": "0.1.0"`) {
			t.Fatalf("invalid %s manifest: %s, %v", harness, content, err)
		}
	}
	if entries["codex/plugins/grasshopper/.mcp.json"] == nil {
		t.Fatal("Codex MCP bridge missing")
	}
	for name, want := range map[string]string{
		"codex/plugins/grasshopper/hooks/hooks.json":            windowsHookCommand(),
		"cursor/plugins/grasshopper/.cursor-plugin/plugin.json": `"mcpServers": "./mcp.json"`,
	} {
		reader, err := entries[name].Open()
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(reader)
		reader.Close()
		if err != nil || !strings.Contains(string(content), want) {
			t.Fatalf("missing %q in %s: %v", want, name, err)
		}
		if strings.HasPrefix(name, "codex/") && (!strings.Contains(string(content), `"UserPromptSubmit"`) || strings.Contains(string(content), "%PLUGIN_ROOT%")) {
			t.Fatal("Codex Windows prompt fallback is missing or uses the broken percent-variable launcher")
		}
	}
	if entries["codex/.agents/plugins/marketplace.json"] == nil || entries["cursor/.cursor-plugin/marketplace.json"] == nil || entries["claude/.claude-plugin/marketplace.json"] == nil {
		t.Fatal("one or more marketplace manifests missing")
	}
}

func TestBundleRejectsUnsafeEntriesAndMissingFiles(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "file")
	if err := os.WriteFile(source, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, files := range [][]input{
		{{"../secret", source}},
		{{"same", source}, {"same", source}},
		{{"safe", filepath.Join(dir, "missing")}},
	} {
		if err := bundle(filepath.Join(dir, "out.zip"), files); err == nil {
			t.Fatalf("accepted unsafe inputs: %+v", files)
		}
	}
}

func TestRuntimeLibraryEntryUsesNativeExtension(t *testing.T) {
	for _, test := range []struct {
		path string
		want string
	}{
		{"/lib/libonnxruntime.so", "runtime/libonnxruntime.so"},
		{"/lib/libonnxruntime.dylib", "runtime/libonnxruntime.dylib"},
		{"C:\\runtime\\onnxruntime.dll", "runtime/onnxruntime.dll"},
	} {
		got, err := runtimeLibraryEntry(test.path)
		if err != nil || got != test.want {
			t.Errorf("runtimeLibraryEntry(%q) = %q, %v; want %q", test.path, got, err, test.want)
		}
	}
	if _, err := runtimeLibraryEntry("/lib/onnxruntime.txt"); err == nil {
		t.Fatal("accepted unsupported ONNX library extension")
	}
}
