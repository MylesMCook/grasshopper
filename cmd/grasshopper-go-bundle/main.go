// grasshopper-go-bundle assembles a portable server and client archive. It
// deliberately excludes credentials, databases, and machine configuration.
package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type input struct{ name, path string }

func goNotices() ([]input, error) {
	command := exec.Command("go", "list", "-buildvcs=false", "-deps", "-json", "./cmd/grasshopper", "./cmd/grasshopper-go-server", "./cmd/grasshopper-go-backup", "./cmd/grasshopper-go-migrate")
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
	var output, client, server, backup, migrate, library, model, tokenizer, runtimeLicense, runtimeNotices string
	flag.StringVar(&output, "output", "", "new zip archive path")
	flag.StringVar(&client, "client", "", "native Go client bridge binary")
	flag.StringVar(&server, "server", "", "native Go server binary")
	flag.StringVar(&backup, "backup", "", "native Go backup binary")
	flag.StringVar(&migrate, "migrate", "", "native Go migration binary")
	flag.StringVar(&library, "onnx-library", "", "ONNX Runtime shared library")
	flag.StringVar(&model, "model", "", "pinned BGE model.onnx")
	flag.StringVar(&tokenizer, "tokenizer", "", "pinned BGE tokenizer.json")
	flag.StringVar(&runtimeLicense, "onnx-license", "", "ONNX Runtime LICENSE")
	flag.StringVar(&runtimeNotices, "onnx-notices", "", "ONNX Runtime ThirdPartyNotices.txt")
	flag.Parse()
	if output == "" {
		return errors.New("output is required")
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
		{"bin/grasshopper-go-server" + exe, server},
		{"bin/grasshopper-go-backup" + exe, backup},
		{"bin/grasshopper-go-migrate" + exe, migrate},
		{lib, library},
		{"models/bge-small-en-v1.5/model.onnx", model},
		{"models/bge-small-en-v1.5/tokenizer.json", tokenizer},
		{"licenses/Grasshopper-LICENSE", "LICENSE"},
		{"licenses/ONNX-Runtime-LICENSE", runtimeLicense},
		{"licenses/ONNX-Runtime-ThirdPartyNotices.txt", runtimeNotices},
		{"licenses/BGE-NOTICE.txt", "docs/BGE-NOTICE.txt"},
		{"docs/go-package.md", "docs/go-package.md"},
		{"integrations/README.md", "integrations/README.md"},
		{"integrations/client.example.json", "integrations/client.example.json"},
		{"integrations/policy/AGENTS.md", "integrations/policy/AGENTS.md"},
		{"integrations/codex/config.toml.example", "integrations/codex/config.toml.example"},
		{"integrations/codex/hooks.json.example", "integrations/codex/hooks.json.example"},
		{"integrations/cursor/mcp.json.example", "integrations/cursor/mcp.json.example"},
		{"integrations/cursor/hooks.json.example", "integrations/cursor/hooks.json.example"},
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
