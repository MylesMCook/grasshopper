package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestServerRequiresExplicitLoopback(t *testing.T) {
	for _, address := range []string{"127.0.0.1:8106", "[::1]:8106"} {
		if err := validateListen(address); err != nil {
			t.Errorf("loopback %q rejected: %v", address, err)
		}
	}
	for _, address := range []string{":8106", "0.0.0.0:8106", "[::]:8106", "192.0.2.1:8106", "localhost:8106"} {
		if err := validateListen(address); err == nil {
			t.Errorf("non-explicit loopback %q accepted", address)
		}
	}
}

func TestServerReadsPrivateTokenWithoutExposingIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	value := strings.Repeat("s", 40)
	if err := os.WriteFile(path, []byte(value+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := readToken(path)
	if err != nil || loaded != value {
		t.Fatal("private token did not load")
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0644); err != nil {
			t.Fatal(err)
		}
		if _, err := readToken(path); err == nil {
			t.Fatal("world-readable token accepted")
		}
	}
}

func TestQuickstartCreatesPrivateStateAndReusesIt(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Grasshopper")
	database, tokenPath, err := quickstartState(dir)
	if err != nil {
		t.Fatal(err)
	}
	first, err := readToken(tokenPath)
	if err != nil || len(first) < 40 {
		t.Fatal("quickstart did not create a usable private token")
	}
	if runtime.GOOS != "windows" {
		for _, path := range []string{dir, tokenPath, database} {
			info, err := os.Stat(path)
			if err != nil || info.Mode().Perm()&0077 != 0 {
				t.Fatalf("quickstart state is not private: %s: %v", path, err)
			}
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "sentinel"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	secondDatabase, secondTokenPath, err := quickstartState(dir)
	if err != nil || secondDatabase != database || secondTokenPath != tokenPath {
		t.Fatalf("complete state was not reused: %s %s %v", secondDatabase, secondTokenPath, err)
	}
	second, err := readToken(tokenPath)
	if err != nil || second != first {
		t.Fatal("quickstart replaced an existing token")
	}
	if contents, err := os.ReadFile(filepath.Join(dir, "sentinel")); err != nil || string(contents) != "keep" {
		t.Fatal("quickstart changed unrelated state")
	}
}

func TestQuickstartRefusesPartialOrLinkedState(t *testing.T) {
	for _, existing := range []string{"memory.db", "access-token"} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, existing), []byte("existing"), 0600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := quickstartState(dir); err == nil {
			t.Fatalf("accepted partial state with %s", existing)
		}
		if _, err := os.Stat(filepath.Join(dir, "memory.db")); err == nil && existing != "memory.db" {
			t.Fatal("created a database over partial state")
		}
	}
	occupied := t.TempDir()
	if err := os.WriteFile(filepath.Join(occupied, "unrelated"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := quickstartState(occupied); err == nil {
		t.Fatal("created new state in a nonempty directory")
	}
	if runtime.GOOS != "windows" {
		root := t.TempDir()
		link := filepath.Join(root, "link")
		if err := os.Symlink(root, link); err != nil {
			t.Fatal(err)
		}
		if _, _, err := quickstartState(link); err == nil {
			t.Fatal("accepted a linked data directory")
		}
		shared := filepath.Join(root, "shared")
		if err := os.Mkdir(shared, 0755); err != nil {
			t.Fatal(err)
		}
		if _, _, err := quickstartState(shared); err == nil {
			t.Fatal("accepted a shared data directory")
		}
	}
}

func TestQuickstartRequiresAnExtractedBundle(t *testing.T) {
	executable := filepath.Join(t.TempDir(), "bin", "grasshopper-go-server")
	if _, _, _, err := quickstartFiles(executable); err == nil {
		t.Fatal("accepted a missing model and runtime")
	}
}
