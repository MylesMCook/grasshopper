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
