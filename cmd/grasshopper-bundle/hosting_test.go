package main

import (
	"archive/zip"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServerArchiveLaunchdPathResolvesAndOmitsManualClients(t *testing.T) {
	working, _ := os.Getwd()
	oldFlags, oldArgs := flag.CommandLine, os.Args
	defer func() { os.Chdir(working); flag.CommandLine = oldFlags; os.Args = oldArgs }()
	if err := os.Chdir(filepath.Join(working, "..", "..")); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	file := func(name string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("synthetic"), 0700); err != nil {
			t.Fatal(err)
		}
		return p
	}
	binary := file("binary")
	library := file("libonnxruntime.dylib")
	model := file("model.onnx")
	file("model.onnx_data")
	tokenizer := file("tokenizer.json")
	license := file("LICENSE")
	output := filepath.Join(dir, "server.zip")
	flag.CommandLine = flag.NewFlagSet("bundle", flag.ContinueOnError)
	os.Args = []string{"bundle", "--output", output, "--client", binary, "--server", binary, "--backup", binary, "--migrate", binary, "--onnx-library", library, "--model", model, "--tokenizer", tokenizer, "--onnx-license", license, "--onnx-notices", license}
	if err := run(); err != nil {
		t.Fatal(err)
	}
	archive, err := zip.OpenReader(output)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	entries := map[string]bool{}
	var plist string
	for _, entry := range archive.File {
		entries[entry.Name] = true
		if strings.HasPrefix(entry.Name, "integrations/") {
			t.Fatalf("obsolete manual client wiring shipped: %s", entry.Name)
		}
		if entry.Name == "packaging/macos/com.example.grasshopper.plist" {
			reader, _ := entry.Open()
			data, _ := io.ReadAll(reader)
			reader.Close()
			plist = string(data)
		}
	}
	if !entries["bin/grasshopper-server"] || !strings.Contains(plist, "app/bin/grasshopper-server</string>") || strings.Contains(plist, "grasshopper-go-server") {
		t.Fatal("launchd server path does not resolve in archive")
	}
}
