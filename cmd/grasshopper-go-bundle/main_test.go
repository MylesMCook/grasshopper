package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
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
	if err := bundle(output, []input{{"bin/grasshopper-go-server", source}}); err != nil {
		t.Fatal(err)
	}
	archive, err := zip.OpenReader(output)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	if len(archive.File) != 2 || archive.File[0].Name != "bin/grasshopper-go-server" || archive.File[1].Name != "SHA256SUMS" {
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
	if !strings.Contains(string(sums), hex.EncodeToString(hash[:])+"  bin/grasshopper-go-server") {
		t.Fatalf("missing checksum: %s", sums)
	}
	if err := bundle(output, []input{{"bin/grasshopper-go-server", source}}); err == nil {
		t.Fatal("archive was overwritten")
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
