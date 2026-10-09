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
		if strings.Contains(entry.Name, "/plugins/") && !strings.Contains(entry.Name, "/bin/") && entry.Mode().Perm()&0444 != 0444 {
			t.Fatalf("plugin file is not readable after system-wide install: %s (%o)", entry.Name, entry.Mode().Perm())
		}
	}
	if entries["bin/grasshopper"] == nil || entries["policy/AGENTS.md"] == nil || entries["cursor-mcp.example.json"] == nil || entries["cursor-cli.example.json"] == nil || entries["SHA256SUMS"] == nil {
		t.Fatal("canonical policy, Cursor setup, or checksums missing")
	}
	if entries["README.md"] == nil || entries["SETUP.md"] != nil || entries["INSTALL.md"] != nil {
		t.Fatal("client archive must carry one README for setup")
	}
	installReader, err := entries["README.md"].Open()
	if err != nil {
		t.Fatal(err)
	}
	installGuide, err := io.ReadAll(installReader)
	installReader.Close()
	if err != nil || !strings.Contains(string(installGuide), "codex plugin marketplace add") || !strings.Contains(string(installGuide), "agent plugin marketplace add") || !strings.Contains(string(installGuide), "--agents claude") || entries["cursor-cli.md"] != nil {
		t.Fatalf("README must cover all three agent installs: %v", err)
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
		"codex/plugins/grasshopper/hooks/hooks.json":            windowsHookCommandJSON(),
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
		if strings.HasPrefix(name, "codex/") {
			var config struct {
				Hooks map[string][]struct {
					Hooks []struct {
						CommandWindows string `json:"commandWindows"`
						Timeout        uint64 `json:"timeout"`
					} `json:"hooks"`
				} `json:"hooks"`
			}
			if err := json.Unmarshal(content, &config); err != nil {
				t.Fatal(err)
			}
			for _, event := range []string{"SessionStart", "UserPromptSubmit", "SubagentStart"} {
				groups := config.Hooks[event]
				if len(groups) != 1 || len(groups[0].Hooks) != 1 {
					t.Fatalf("Windows %s hook is missing or duplicated", event)
				}
				hook := groups[0].Hooks[0]
				if hook.CommandWindows != windowsHookCommand() || hook.Timeout != 8 {
					t.Fatalf("Windows %s launcher or timeout changed: %+v", event, hook)
				}
			}
		}
	}
	if entries["codex/.agents/plugins/marketplace.json"] == nil || entries["cursor/.cursor-plugin/marketplace.json"] == nil || entries["claude/.claude-plugin/marketplace.json"] == nil {
		t.Fatal("one or more marketplace manifests missing")
	}
}

func TestMarketplacePackagesOneStatelessClientPerPlatform(t *testing.T) {
	working, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(filepath.Join(working, "..", "..")); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(working)
	dir := t.TempDir()
	binaries := map[string]string{}
	for target, name := range map[string]string{
		"darwin-arm64": "mac", "windows-amd64": "win.exe", "linux-amd64": "linux",
	} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("synthetic "+target), 0700); err != nil {
			t.Fatal(err)
		}
		binaries[target] = path
	}
	output := filepath.Join(dir, "marketplace.zip")
	files, cleanup, err := marketplaceFiles(binaries, "2.2.0", output)
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
		if strings.Contains(entry.Name, "token") || strings.HasSuffix(entry.Name, ".db") {
			t.Fatalf("private state in marketplace: %s", entry.Name)
		}
		if strings.HasPrefix(entry.Name, "plugins/") && !strings.Contains(entry.Name, "/bin/") && entry.Mode().Perm()&0444 != 0444 {
			t.Fatalf("plugin file is not readable after system-wide install: %s (%o)", entry.Name, entry.Mode().Perm())
		}
	}
	for _, slug := range []string{"macos", "windows", "linux"} {
		root := "plugins/grasshopper-" + slug + "/"
		if entries[root+"README.md"] != nil {
			t.Fatalf("duplicate %s README in marketplace", slug)
		}
		exe := ""
		if slug == "windows" {
			exe = ".exe"
		}
		for _, path := range []string{
			"bin/grasshopper" + exe, "policy/AGENTS.md", ".codex-plugin/plugin.json",
			".cursor-plugin/plugin.json", ".mcp.json", "mcp.json",
			"hooks/hooks.json", "hooks/cursor.json", "skills/connect-grasshopper/SKILL.md",
		} {
			if entries[root+path] == nil {
				t.Errorf("missing %s%s", root, path)
			}
		}
		manifestEntry := entries[root+".codex-plugin/plugin.json"]
		if manifestEntry == nil {
			t.Fatal("missing Codex manifest")
		}
		manifest, err := manifestEntry.Open()
		if err != nil {
			t.Fatal(err)
		}
		var codex map[string]any
		err = json.NewDecoder(manifest).Decode(&codex)
		manifest.Close()
		if err != nil {
			t.Fatal(err)
		}
		if _, copied := codex["hooks"]; copied {
			t.Fatal("Codex hooks must use native hooks/hooks.json discovery")
		}
	}
	if entries["README.md"] == nil || entries["SETUP.md"] != nil {
		t.Fatal("marketplace must carry one README")
	}
	readme, err := entries["README.md"].Open()
	if err != nil {
		t.Fatal(err)
	}
	copy, err := io.ReadAll(readme)
	readme.Close()
	if err != nil || !strings.Contains(string(copy), "codex plugin marketplace add") || !strings.Contains(string(copy), "agent plugin marketplace add") || !strings.Contains(string(copy), "--agents claude") {
		t.Fatal("marketplace README must cover all three agent installs", err)
	}
	for _, path := range []string{".agents/plugins/marketplace.json", ".cursor-plugin/marketplace.json"} {
		entry := entries[path]
		if entry == nil {
			t.Fatal("missing", path)
		}
		reader, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(reader)
		reader.Close()
		if err != nil || !json.Valid(data) {
			t.Fatal("invalid marketplace", path, err)
		}
		for _, slug := range []string{"macos", "windows", "linux"} {
			if !strings.Contains(string(data), "grasshopper-"+slug) {
				t.Errorf("%s omits %s", path, slug)
			}
		}
	}
	read := func(name string) string {
		t.Helper()
		entry := entries[name]
		if entry == nil {
			t.Fatalf("missing %s", name)
		}
		reader, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		defer reader.Close()
		data, err := io.ReadAll(reader)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	var claudeMarketplace struct {
		Name    string `json:"name"`
		Plugins []struct {
			Name   string `json:"name"`
			Source string `json:"source"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal([]byte(read(".claude-plugin/marketplace.json")), &claudeMarketplace); err != nil || claudeMarketplace.Name != "grasshopper-marketplace" || len(claudeMarketplace.Plugins) != 3 {
		t.Fatalf("Claude marketplace must list one plugin per platform: %+v err=%v", claudeMarketplace, err)
	}
	for _, plugin := range claudeMarketplace.Plugins {
		slug := strings.TrimPrefix(plugin.Name, "grasshopper-")
		root := strings.TrimPrefix(plugin.Source, "./") + "/"
		if plugin.Source != "./plugins/claude-grasshopper-"+slug {
			t.Fatalf("entry %s points at %s", plugin.Name, plugin.Source)
		}
		exe := ""
		if slug == "windows" {
			exe = ".exe"
		}
		for _, path := range []string{"bin/grasshopper" + exe, "policy/AGENTS.md", "LICENSE", ".claude-plugin/plugin.json", ".mcp.json", "hooks/hooks.json", "skills/connect-grasshopper/SKILL.md"} {
			if entries[root+path] == nil {
				t.Errorf("Claude plugin %s is missing %s", plugin.Name, path)
			}
		}
		var manifest struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		}
		if err := json.Unmarshal([]byte(read(root+".claude-plugin/plugin.json")), &manifest); err != nil || manifest.Name != plugin.Name || manifest.Version != "2.2.0" {
			t.Fatalf("manifest name must equal the marketplace entry name: %+v err=%v", manifest, err)
		}
		hooks := read(root + "hooks/hooks.json")
		if !strings.Contains(hooks, "--harness claude") || strings.Contains(hooks, "--harness codex") || !strings.Contains(hooks, "${CLAUDE_PLUGIN_ROOT}/bin/grasshopper"+exe) {
			t.Fatalf("Claude hooks must run the Claude harness from the plugin root: %s", hooks)
		}
		if mcp := read(root + ".mcp.json"); !strings.Contains(mcp, "${CLAUDE_PLUGIN_ROOT}/bin/grasshopper"+exe) || strings.Contains(mcp, "\"cwd\"") {
			t.Fatalf("Claude MCP config must not reuse Codex's: %s", mcp)
		}
		for _, skillPath := range []string{root + "skills/connect-grasshopper/SKILL.md", "plugins/grasshopper-" + slug + "/skills/connect-grasshopper/SKILL.md"} {
			if skill := read(skillPath); !strings.Contains(skill, "Quote the returned `next_step`") || !strings.Contains(skill, "`invalid_address`") {
				t.Fatalf("%s must relay the client's guidance", skillPath)
			}
		}
		// Codex's directory keeps its own hook and MCP files, untouched by Claude's.
		if codexHooks := read("plugins/grasshopper-" + slug + "/hooks/hooks.json"); !strings.Contains(codexHooks, "--harness codex") {
			t.Fatalf("Codex hooks were replaced: %s", codexHooks)
		}
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
