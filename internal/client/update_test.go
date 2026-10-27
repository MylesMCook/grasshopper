package client

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

// fakeRelease serves SHA256SUMS and one client archive per version, built
// from the real client so the installed program's --version check runs.
type fakeRelease struct {
	server    *httptest.Server
	downloads atomic.Int32
	corrupt   bool
}

func newFakeRelease(t *testing.T, versions ...string) *fakeRelease {
	t.Helper()
	target, ok := releaseTarget()
	if !ok {
		t.Skip("no client release for this platform")
	}
	release := &fakeRelease{}
	archives := map[string][]byte{}
	sums := map[string]string{}
	for _, version := range versions {
		program := filepath.Join(t.TempDir(), "grasshopper")
		build := exec.Command("go", "build", "-ldflags=-X main.clientVersion="+version, "-o", program, "../../cmd/grasshopper")
		if output, err := build.CombinedOutput(); err != nil {
			t.Fatalf("build client %s: %v\n%s", version, err, output)
		}
		data, err := os.ReadFile(program)
		if err != nil {
			t.Fatal(err)
		}
		name := "bin/grasshopper"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		var zipped bytes.Buffer
		writer := zip.NewWriter(&zipped)
		entry, _ := writer.Create(name)
		_, _ = entry.Write(data)
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		archive := "grasshopper-client-" + target + "-" + version + ".zip"
		archives["/v"+version+"/"+archive] = zipped.Bytes()
		sum := sha256.Sum256(zipped.Bytes())
		sums["/v"+version+"/SHA256SUMS"] = hex.EncodeToString(sum[:]) + "  " + archive + "\n"
	}
	release.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if sum, ok := sums[r.URL.Path]; ok {
			_, _ = w.Write([]byte(sum))
			return
		}
		if archive, ok := archives[r.URL.Path]; ok {
			release.downloads.Add(1)
			if release.corrupt {
				archive = append([]byte("x"), archive...)
			}
			_, _ = w.Write(archive)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(release.server.Close)
	previous := ReleaseBaseURL
	ReleaseBaseURL = release.server.URL
	t.Cleanup(func() { ReleaseBaseURL = previous })
	return release
}

func TestFollowInstallsTheServerVersionAndHandsOff(t *testing.T) {
	release := newFakeRelease(t, "2.11.1")
	dir := filepath.Join(t.TempDir(), "client")
	ctx := context.Background()

	installed, err := Follow(ctx, dir, "2.11.0", "2.11.1")
	if err != nil || installed != "2.11.1" {
		t.Fatalf("Follow = %q, %v", installed, err)
	}
	version, program, ok := ManagedClient(dir)
	if !ok || version != "2.11.1" || program != managedProgram(dir, "2.11.1") {
		t.Fatalf("ManagedClient = %q %q %v", version, program, ok)
	}
	configPath := filepath.Join(filepath.Dir(dir), "client.json")
	if target, ok := HandOffTarget(configPath, "2.11.0"); !ok || target != program {
		t.Fatalf("older bundled client should hand off, got %q %v", target, ok)
	}
	for _, running := range []string{"2.11.1", "2.12.0", "dev"} {
		if _, ok := HandOffTarget(configPath, running); ok {
			t.Fatalf("client %s must not hand off to 2.11.1", running)
		}
	}
	if installed, err := Follow(ctx, dir, "2.11.0", "2.11.1"); err != nil || installed != "" || release.downloads.Load() != 1 {
		t.Fatalf("matching version should not download again: %q %v %d", installed, err, release.downloads.Load())
	}
}

func TestFollowRefusesAChecksumMismatch(t *testing.T) {
	release := newFakeRelease(t, "2.11.1")
	release.corrupt = true
	dir := filepath.Join(t.TempDir(), "client")
	if _, err := Follow(context.Background(), dir, "2.11.0", "2.11.1"); err == nil || !strings.Contains(err.Error(), "SHA256SUMS") {
		t.Fatalf("expected checksum failure, got %v", err)
	}
	if _, _, ok := ManagedClient(dir); ok {
		t.Fatal("a corrupt download must not become current")
	}
	if last := LastUpdateError(dir); !strings.Contains(last, "2.11.1") {
		t.Fatalf("last-error should name the version, got %q", last)
	}
}

func TestFollowRollsBackWithTheServer(t *testing.T) {
	release := newFakeRelease(t, "2.11.1", "2.11.2", "2.11.3")
	dir := filepath.Join(t.TempDir(), "client")
	ctx := context.Background()
	for _, server := range []string{"2.11.1", "2.11.2"} {
		if _, err := Follow(ctx, dir, "2.11.0", server); err != nil {
			t.Fatal(err)
		}
	}
	// A server rollback switches to the kept version without downloading.
	if installed, err := Follow(ctx, dir, "2.11.0", "2.11.1"); err != nil || installed != "2.11.1" || release.downloads.Load() != 2 {
		t.Fatalf("rollback = %q %v, downloads %d", installed, err, release.downloads.Load())
	}
	// Only the current and previous versions are kept.
	if _, err := Follow(ctx, dir, "2.11.0", "2.11.3"); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "versions"))
	var kept []string
	for _, entry := range entries {
		kept = append(kept, entry.Name())
	}
	if strings.Join(kept, ",") != "2.11.1,2.11.3" {
		t.Fatalf("kept versions = %v", kept)
	}
	// A server back at the bundled version needs no managed client.
	if _, err := Follow(ctx, dir, "2.11.0", "2.11.0"); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := ManagedClient(dir); ok {
		t.Fatal("managed client should be cleared when the bundled client matches the server")
	}
}

func TestUpdatesOffSkipsFollowServer(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "client.json")
	dir := ManagedDir(configPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "updates-off"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	// No client.json exists, so anything past the opt-out check would fail.
	if installed, err := FollowServer(context.Background(), configPath, "2.11.0"); err != nil || installed != "" {
		t.Fatalf("FollowServer with updates off = %q, %v", installed, err)
	}
}

func TestVersionParsing(t *testing.T) {
	for version, want := range map[string]bool{"2.10.5": true, "v2.10.5": true, "dev": false, "2.10": false, "2.10.05": false, "2.-1.0": false} {
		if _, ok := ParseVersion(version); ok != want {
			t.Errorf("ParseVersion(%q) ok = %v", version, ok)
		}
	}
	if order, ok := CompareVersions("2.10.10", "2.10.9"); !ok || order != 1 {
		t.Errorf("2.10.10 should be newer than 2.10.9")
	}
}

func TestChecksumForReadsBothSHA256SUMSForms(t *testing.T) {
	sum := strings.Repeat("ab", 32)
	for _, line := range []string{sum + "  a.zip\n", sum + " *a.zip\n"} {
		if got, err := checksumFor([]byte("deadbeef  other.zip\n"+line), "a.zip"); err != nil || got != sum {
			t.Errorf("checksumFor(%q) = %q, %v", line, got, err)
		}
	}
	if _, err := checksumFor([]byte(sum+"  b.zip\n"), "a.zip"); err == nil {
		t.Error("missing archive should fail")
	}
}
