// The bundle tool assembles a portable server and client archive. It
// deliberately excludes credentials, databases, and machine configuration.
package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf16"
)

type input struct{ name, path string }

func goNotices(packages ...string) ([]input, error) {
	if len(packages) == 0 {
		packages = []string{"./cmd/grasshopper", "./cmd/grasshopper-go-server", "./cmd/grasshopper-go-backup", "./cmd/grasshopper-go-migrate"}
	}
	command := exec.Command("go", append([]string{"list", "-buildvcs=false", "-deps", "-json"}, packages...)...)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("list Go dependencies: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	type module struct {
		Path, Version, Dir string
		Main               bool
	}
	decoder := json.NewDecoder(strings.NewReader(string(output)))
	modules := map[string]module{}
	for {
		var pkg struct{ Module *module }
		if err := decoder.Decode(&pkg); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, err
		}
		if pkg.Module != nil && !pkg.Module.Main {
			modules[pkg.Module.Path] = *pkg.Module
		}
	}
	var notices []input
	for _, mod := range modules {
		entries, err := os.ReadDir(mod.Dir)
		if err != nil {
			return nil, err
		}
		found := false
		for _, entry := range entries {
			upper := strings.ToUpper(entry.Name())
			if entry.IsDir() || (!strings.HasPrefix(upper, "LICENSE") && !strings.HasPrefix(upper, "COPYING")) {
				continue
			}
			found = true
			name := "licenses/go/" + mod.Path + "@" + mod.Version + "/" + entry.Name()
			notices = append(notices, input{name, filepath.Join(mod.Dir, entry.Name())})
		}
		if !found {
			return nil, fmt.Errorf("missing license for %s@%s", mod.Path, mod.Version)
		}
	}
	sort.Slice(notices, func(i, j int) bool { return notices[i].name < notices[j].name })
	return notices, nil
}

var pluginVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

// Codex's Windows hook runner wraps commands in cmd.exe quotes. Resolve the
// plugin path inside PowerShell so installs under paths with spaces still work.
func windowsHookCommand() string {
	script := `& (Join-Path $env:PLUGIN_ROOT 'bin\grasshopper.exe') hook --harness codex; exit $LASTEXITCODE`
	units := utf16.Encode([]rune(script))
	data := make([]byte, len(units)*2)
	for i, unit := range units {
		binary.LittleEndian.PutUint16(data[i*2:], unit)
	}
	return "powershell.exe -NoProfile -NonInteractive -EncodedCommand " + base64.StdEncoding.EncodeToString(data)
}

func clientPluginFiles(client, target, version, output string) ([]input, func(), error) {
	if !pluginVersion.MatchString(version) {
		return nil, nil, errors.New("plugin version must be major.minor.patch")
	}
	exe := ""
	switch target {
	case "darwin-arm64", "linux-amd64":
		if strings.EqualFold(filepath.Ext(client), ".exe") {
			return nil, nil, errors.New("Unix client must not have .exe suffix")
		}
	case "windows-amd64":
		if !strings.EqualFold(filepath.Ext(client), ".exe") {
			return nil, nil, errors.New("Windows client must have .exe suffix")
		}
		exe = ".exe"
	default:
		return nil, nil, errors.New("target must be darwin-arm64, windows-amd64, or linux-amd64")
	}
	if info, err := os.Stat(client); err != nil || !info.Mode().IsRegular() {
		return nil, nil, errors.New("client binary must be a regular file")
	}
	temp, err := os.MkdirTemp(filepath.Dir(output), ".grasshopper-client-plugins-*")
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() { _ = os.RemoveAll(temp) }
	guide, err := os.ReadFile("SETUP.md")
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	guidePath := filepath.Join(temp, "SETUP.md")
	if err := os.WriteFile(guidePath, guide, 0600); err != nil {
		cleanup()
		return nil, nil, err
	}
	files := []input{
		{"bin/grasshopper" + exe, client},
		{"policy/AGENTS.md", "integrations/policy/AGENTS.md"},
		{"client.example.json", "integrations/plugins/client.example.json"},
		{"cursor-mcp.example.json", "integrations/cursor/mcp.json.example"},
		{"cursor-cli.example.json", "integrations/cursor/cli.json.example"},
		{"SETUP.md", guidePath},
		{"LICENSE", "LICENSE"},
	}
	marketplacePaths := map[string]string{
		"codex":  ".agents/plugins/marketplace.json",
		"cursor": ".cursor-plugin/marketplace.json",
		"claude": ".claude-plugin/marketplace.json",
	}
	for _, harness := range []string{"codex", "cursor", "claude"} {
		root := harness + "/plugins/grasshopper/"
		files = append(files, input{root + "bin/grasshopper" + exe, client}, input{root + "LICENSE", "LICENSE"})
		files = append(files, input{harness + "/" + marketplacePaths[harness], "integrations/plugins/marketplaces/" + harness + ".json"})
		source := filepath.Join("integrations", "plugins", harness)
		err := filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			if !entry.Type().IsRegular() {
				return fmt.Errorf("plugin template is not a regular file: %s", path)
			}
			contents, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			rendered := strings.NewReplacer("{{VERSION}}", version, "{{EXE}}", exe, "{{WINDOWS_HOOK_COMMAND}}", windowsHookCommand()).Replace(string(contents))
			if strings.Contains(rendered, "{{") || !json.Valid([]byte(rendered)) {
				return fmt.Errorf("invalid rendered plugin JSON: %s", path)
			}
			relative, err := filepath.Rel(source, path)
			if err != nil {
				return err
			}
			staged := filepath.Join(temp, harness, relative)
			if err := os.MkdirAll(filepath.Dir(staged), 0700); err != nil {
				return err
			}
			if err := os.WriteFile(staged, []byte(rendered), 0600); err != nil {
				return err
			}
			files = append(files, input{root + filepath.ToSlash(relative), staged})
			return nil
		})
		if err != nil {
			cleanup()
			return nil, nil, err
		}
	}
	notices, err := goNotices("./cmd/grasshopper")
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	return append(files, notices...), cleanup, nil
}

func bundle(output string, files []input) error {
	if _, err := os.Stat(output); err == nil {
		return errors.New("output already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	seen := map[string]bool{}
	for _, file := range files {
		if file.path == "" || file.name == "" || strings.HasPrefix(file.name, "/") || strings.Contains(file.name, "..") || seen[file.name] {
			return fmt.Errorf("invalid bundle entry %q", file.name)
		}
		seen[file.name] = true
		info, err := os.Stat(file.path)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("not a regular file: %s", file.path)
		}
	}
	temp, err := os.CreateTemp(filepath.Dir(output), ".grasshopper-bundle-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	writer := zip.NewWriter(temp)
	var sums strings.Builder
	for _, file := range files {
		if err := addFile(writer, file, &sums); err != nil {
			writer.Close()
			temp.Close()
			return err
		}
	}
	entry, err := writer.Create("SHA256SUMS")
	if err == nil {
		_, err = io.WriteString(entry, sums.String())
	}
	if err != nil {
		writer.Close()
		temp.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	// Both files live in the same directory. Linking refuses an existing output
	// while still publishing the fully closed archive in one step.
	return os.Link(temp.Name(), output)
}

func addFile(writer *zip.Writer, file input, sums *strings.Builder) error {
	source, err := os.Open(file.path)
	if err != nil {
		return err
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil {
		return err
	}
	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Name = file.name
	header.Method = zip.Deflate
	entry, err := writer.CreateHeader(header)
	if err != nil {
		return err
	}
	hash := sha256.New()
	if _, err := io.Copy(io.MultiWriter(entry, hash), source); err != nil {
		return err
	}
	fmt.Fprintf(sums, "%s  %s\n", hex.EncodeToString(hash.Sum(nil)), file.name)
	return nil
}

func runtimeLibraryEntry(library string) (string, error) {
	switch strings.ToLower(filepath.Ext(library)) {
	case ".dll":
		return "runtime/onnxruntime.dll", nil
	case ".dylib":
		return "runtime/libonnxruntime.dylib", nil
	case ".so":
		return "runtime/libonnxruntime.so", nil
	default:
		return "", fmt.Errorf("unsupported ONNX library extension: %s", filepath.Base(library))
	}
}

func run() error {
	var output, client, server, backup, migrate, library, model, tokenizer, runtimeLicense, runtimeNotices, target, version string
	var macClient, windowsClient, linuxClient string
	var clientPlugins, marketplacePlugins bool
	flag.StringVar(&output, "output", "", "new zip archive path")
	flag.StringVar(&client, "client", "", "native client bridge binary")
	flag.BoolVar(&clientPlugins, "client-plugins", false, "package three native client plugins without the server")
	flag.BoolVar(&marketplacePlugins, "marketplace-plugins", false, "package OS-specific Codex and Cursor marketplace plugins")
	flag.StringVar(&macClient, "client-macos", "", "macOS arm64 client for marketplace")
	flag.StringVar(&windowsClient, "client-windows", "", "Windows amd64 client for marketplace")
	flag.StringVar(&linuxClient, "client-linux", "", "Linux amd64 client for marketplace")
	flag.StringVar(&target, "target", "", "client plugin target: darwin-arm64, windows-amd64, or linux-amd64")
	flag.StringVar(&version, "plugin-version", "2.2.2", "client plugin version")
	flag.StringVar(&server, "server", "", "native server binary")
	flag.StringVar(&backup, "backup", "", "backup executable")
	flag.StringVar(&migrate, "migrate", "", "migration executable")
	flag.StringVar(&library, "onnx-library", "", "ONNX Runtime shared library")
	flag.StringVar(&model, "model", "", "pinned BGE model.onnx")
	flag.StringVar(&tokenizer, "tokenizer", "", "pinned BGE tokenizer.json")
	flag.StringVar(&runtimeLicense, "onnx-license", "", "ONNX Runtime LICENSE")
	flag.StringVar(&runtimeNotices, "onnx-notices", "", "ONNX Runtime ThirdPartyNotices.txt")
	flag.Parse()
	if output == "" {
		return errors.New("output is required")
	}
	if marketplacePlugins {
		files, cleanup, err := marketplaceFiles(map[string]string{
			"darwin-arm64": macClient, "windows-amd64": windowsClient, "linux-amd64": linuxClient,
		}, version, output)
		if err != nil {
			return err
		}
		defer cleanup()
		return bundle(output, files)
	}
	if clientPlugins {
		files, cleanup, err := clientPluginFiles(client, target, version, output)
		if err != nil {
			return err
		}
		defer cleanup()
		return bundle(output, files)
	}
	exe := ""
	if strings.EqualFold(filepath.Ext(server), ".exe") {
		exe = ".exe"
	}
	lib, err := runtimeLibraryEntry(library)
	if err != nil {
		return err
	}
	files := []input{
		{"bin/grasshopper" + exe, client},
		{"bin/grasshopper-server" + exe, server},
		{"bin/grasshopper-backup" + exe, backup},
		{"bin/grasshopper-migrate" + exe, migrate},
		{lib, library},
		{"models/bge-small-en-v1.5/model.onnx", model},
		{"models/bge-small-en-v1.5/tokenizer.json", tokenizer},
		{"licenses/Grasshopper-LICENSE", "LICENSE"},
		{"licenses/ONNX-Runtime-LICENSE", runtimeLicense},
		{"licenses/ONNX-Runtime-ThirdPartyNotices.txt", runtimeNotices},
		{"licenses/BGE-NOTICE.txt", "docs/BGE-NOTICE.txt"},
		{"AGENTS.md", "AGENTS.md"},
		{"README.md", "README.md"},
		{"SETUP.md", "SETUP.md"},
		{"docs/memory-acceptance.md", "docs/memory-acceptance.md"},
		{"docs/shared-memory.md", "docs/shared-memory.md"},
		{"docs/fonts/OFL-newsreader.txt", "docs/fonts/OFL-newsreader.txt"},
		{"docs/fonts/OFL-geist-mono.txt", "docs/fonts/OFL-geist-mono.txt"},
		{"integrations/client.example.json", "integrations/client.example.json"},
		{"packaging/macos/com.example.grasshopper.plist", "packaging/macos/com.example.grasshopper.plist"},
		{"integrations/policy/AGENTS.md", "integrations/policy/AGENTS.md"},
		{"integrations/codex/config.toml.example", "integrations/codex/config.toml.example"},
		{"integrations/codex/hooks.json.example", "integrations/codex/hooks.json.example"},
		{"integrations/cursor/mcp.json.example", "integrations/cursor/mcp.json.example"},
		{"integrations/cursor/hooks.json.example", "integrations/cursor/hooks.json.example"},
		{"integrations/cursor/cli.json.example", "integrations/cursor/cli.json.example"},
		{"integrations/claude/mcp.json.example", "integrations/claude/mcp.json.example"},
		{"integrations/claude/settings.json.example", "integrations/claude/settings.json.example"},
	}
	notices, err := goNotices()
	if err != nil {
		return err
	}
	return bundle(output, append(files, notices...))
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Grasshopper bundle:", err)
		os.Exit(1)
	}
}
