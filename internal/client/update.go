package client

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Self-update keeps every agent on a machine on the client release that
// matches its server. Plugins and archives each bundle a client; when any of
// them runs bridge or hook, it hands off to the managed client if that one is
// newer. The long-running bridge downloads the server's release in the
// background, so a server upgrade reaches every agent from its next session.
//
// The managed folder sits beside client.json:
//
//	client/current            the active version, such as 2.11.0
//	client/versions/X.Y.Z/    one downloaded client per version (at most two)
//	client/updates-off        present: never download (bundled clients still run)
//	client/last-error         why the last download failed, for check
//
// Clients follow the server's version, never the newest release, so a client
// is never ahead of a server that has not been upgraded.

// ReleaseBaseURL is where release assets are downloaded from. Tests replace it.
var ReleaseBaseURL = "https://github.com/MylesMCook/grasshopper/releases/download"

// MinimumPluginVersion is the oldest plugin whose hook and MCP wiring is
// still current. Plugins at or above it run the self-updated client, so check
// asks for a plugin update only below it. Raise it when a release changes
// integrations/plugins templates.
const MinimumPluginVersion = "2.11.0"

const maxClientBinary = 64 << 20

// ParseVersion parses a release version such as 2.9.1 or v2.9.1. A
// development build does not parse.
func ParseVersion(version string) ([3]int, bool) {
	var parsed [3]int
	parts := strings.Split(strings.TrimPrefix(version, "v"), ".")
	if len(parts) != 3 {
		return parsed, false
	}
	for i, part := range parts {
		number, err := strconv.Atoi(part)
		if err != nil || number < 0 || strconv.Itoa(number) != part {
			return parsed, false
		}
		parsed[i] = number
	}
	return parsed, true
}

// CompareVersions orders two release versions. ok is false when either is
// not a release version, such as a development build.
func CompareVersions(a, b string) (order int, ok bool) {
	left, leftOK := ParseVersion(a)
	right, rightOK := ParseVersion(b)
	if !leftOK || !rightOK {
		return 0, false
	}
	for i := range left {
		if left[i] != right[i] {
			if left[i] < right[i] {
				return -1, true
			}
			return 1, true
		}
	}
	return 0, true
}

// ManagedDir is the self-update folder for a client configuration path.
func ManagedDir(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), "client")
}

func managedProgram(dir, version string) string {
	name := "grasshopper"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(dir, "versions", version, name)
}

// ManagedClient returns the active managed client, if one is installed.
func ManagedClient(dir string) (version, program string, ok bool) {
	data, err := os.ReadFile(filepath.Join(dir, "current"))
	if err != nil || len(data) > 32 {
		return "", "", false
	}
	version = strings.TrimSpace(string(data))
	if _, ok := ParseVersion(version); !ok {
		return "", "", false
	}
	program = managedProgram(dir, version)
	if info, err := os.Stat(program); err != nil || !info.Mode().IsRegular() {
		return "", "", false
	}
	return version, program, true
}

// HandOffTarget returns the managed client to run instead of this one, when
// it is newer than running. A development build never hands off.
func HandOffTarget(configPath, running string) (string, bool) {
	version, program, ok := ManagedClient(ManagedDir(configPath))
	if !ok {
		return "", false
	}
	if order, ok := CompareVersions(version, running); !ok || order <= 0 {
		return "", false
	}
	return program, true
}

// UpdatesDisabled reports whether this machine opted out of downloads.
func UpdatesDisabled(dir string) bool {
	_, err := os.Lstat(filepath.Join(dir, "updates-off"))
	return err == nil
}

// LastUpdateError returns why the last download failed, or "".
func LastUpdateError(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, "last-error"))
	if err != nil || len(data) > 1024 {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func releaseTarget() (string, bool) {
	switch runtime.GOOS + "-" + runtime.GOARCH {
	case "darwin-arm64", "windows-amd64", "linux-amd64":
		return runtime.GOOS + "-" + runtime.GOARCH, true
	}
	return "", false
}

// FollowServer makes the managed client match the server's version, in
// either direction, so a server rollback also rolls clients back. It returns
// the version it switched to, or "" when nothing changed. Failures are saved
// for check; they never affect the running agent.
func FollowServer(ctx context.Context, configPath, running string) (string, error) {
	dir := ManagedDir(configPath)
	if UpdatesDisabled(dir) {
		return "", nil
	}
	if _, ok := ParseVersion(running); !ok {
		return "", nil
	}
	config, err := LoadConfig(configPath)
	if err != nil {
		return "", err
	}
	remote, err := NewRemote(config)
	if err != nil {
		return "", err
	}
	identity, err := remote.Identity(ctx)
	if err != nil {
		return "", err
	}
	return Follow(ctx, dir, running, identity.Version)
}

// Follow points the managed client at server. running is the client that
// would run without a managed one; a server at or below it needs no managed
// client, so current is cleared and the bundled client runs.
func Follow(ctx context.Context, dir, running, server string) (string, error) {
	order, ok := CompareVersions(server, running)
	if !ok {
		return "", nil
	}
	managed, _, haveManaged := ManagedClient(dir)
	if order <= 0 {
		if haveManaged {
			return "", os.Remove(filepath.Join(dir, "current"))
		}
		return "", nil
	}
	if haveManaged && managed == server {
		return "", nil
	}
	if err := Install(ctx, dir, server); err != nil {
		_ = os.MkdirAll(dir, 0o700)
		_ = os.WriteFile(filepath.Join(dir, "last-error"), []byte(time.Now().UTC().Format(time.RFC3339)+" "+server+": "+err.Error()+"\n"), 0o600)
		return "", err
	}
	return server, nil
}

// Install downloads the client archive for version, checks it against the
// release's SHA256SUMS, extracts its program, checks that the program reports
// version, and makes it current. The previous current version is kept for
// rollback; any other version is removed.
func Install(ctx context.Context, dir, version string) error {
	if _, ok := ParseVersion(version); !ok {
		return errors.New("invalid version")
	}
	// A version kept from before (the previous release, after a server
	// rollback) is switched back to without downloading.
	if reportsVersion(ctx, managedProgram(dir, version), version) {
		return makeCurrent(dir, version)
	}
	target, ok := releaseTarget()
	if !ok {
		return fmt.Errorf("no client release for %s-%s", runtime.GOOS, runtime.GOARCH)
	}
	if err := os.MkdirAll(filepath.Join(dir, "versions"), 0o700); err != nil {
		return err
	}
	archive := "grasshopper-client-" + target + "-" + version + ".zip"
	base := strings.TrimSuffix(ReleaseBaseURL, "/") + "/v" + version + "/"
	httpClient := &http.Client{Timeout: 5 * time.Minute, CheckRedirect: func(request *http.Request, via []*http.Request) error {
		// GitHub redirects release assets to its storage host; never let a
		// redirect drop HTTPS.
		if len(via) > 5 || (via[0].URL.Scheme == "https" && request.URL.Scheme != "https") {
			return errors.New("refusing redirect")
		}
		return nil
	}}
	sums, err := download(ctx, httpClient, base+"SHA256SUMS", 64<<10)
	if err != nil {
		return fmt.Errorf("SHA256SUMS: %w", err)
	}
	want, err := checksumFor(sums, archive)
	if err != nil {
		return err
	}
	zipped, err := download(ctx, httpClient, base+archive, 256<<20)
	if err != nil {
		return fmt.Errorf("%s: %w", archive, err)
	}
	if got := sha256.Sum256(zipped); hex.EncodeToString(got[:]) != want {
		return fmt.Errorf("%s does not match SHA256SUMS", archive)
	}
	program, err := extractProgram(zipped)
	if err != nil {
		return err
	}

	staging, err := os.MkdirTemp(filepath.Join(dir, "versions"), ".tmp-"+version+"-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	staged := filepath.Join(staging, filepath.Base(managedProgram(dir, version)))
	if err := os.WriteFile(staged, program, 0o700); err != nil {
		return err
	}
	if !reportsVersion(ctx, staged, version) {
		return fmt.Errorf("downloaded client did not report version %s", version)
	}
	final := filepath.Dir(managedProgram(dir, version))
	if err := os.Rename(staging, final); err != nil {
		// Another bridge may have installed the same version first.
		if info, statErr := os.Stat(managedProgram(dir, version)); statErr != nil || !info.Mode().IsRegular() {
			return err
		}
	}
	return makeCurrent(dir, version)
}

func makeCurrent(dir, version string) error {
	previous, _, _ := ManagedClient(dir)
	if err := writeAtomic(filepath.Join(dir, "current"), []byte(version+"\n")); err != nil {
		return err
	}
	_ = os.Remove(filepath.Join(dir, "last-error"))
	pruneVersions(dir, version, previous)
	return nil
}

func reportsVersion(ctx context.Context, program, version string) bool {
	if info, err := os.Stat(program); err != nil || !info.Mode().IsRegular() {
		return false
	}
	versionCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	output, err := exec.CommandContext(versionCtx, program, "--version").Output()
	return err == nil && strings.TrimSpace(string(output)) == "grasshopper "+version
}

func download(ctx context.Context, httpClient *http.Client, url string, limit int64) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	response, err := httpClient.Do(request)
	if err != nil {
		return nil, errors.New("download failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download returned HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, errors.New("download interrupted")
	}
	if int64(len(data)) > limit {
		return nil, errors.New("download too large")
	}
	return data, nil
}

func checksumFor(sums []byte, name string) (string, error) {
	scanner := bufio.NewScanner(bytes.NewReader(sums))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == name {
			if decoded, err := hex.DecodeString(fields[0]); err == nil && len(decoded) == sha256.Size {
				return strings.ToLower(fields[0]), nil
			}
		}
	}
	return "", fmt.Errorf("SHA256SUMS does not list %s", name)
}

// extractProgram reads the top-level bin/grasshopper from a client archive.
func extractProgram(zipped []byte) ([]byte, error) {
	reader, err := zip.NewReader(bytes.NewReader(zipped), int64(len(zipped)))
	if err != nil {
		return nil, errors.New("client archive is not a zip file")
	}
	name := "bin/grasshopper"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	for _, file := range reader.File {
		if file.Name != name {
			continue
		}
		if file.UncompressedSize64 > maxClientBinary {
			return nil, errors.New("client program too large")
		}
		opened, err := file.Open()
		if err != nil {
			return nil, err
		}
		defer opened.Close()
		data, err := io.ReadAll(io.LimitReader(opened, maxClientBinary+1))
		if err != nil || len(data) > maxClientBinary {
			return nil, errors.New("client program unreadable")
		}
		return data, nil
	}
	return nil, fmt.Errorf("client archive has no %s", name)
}

func writeAtomic(path string, data []byte) error {
	temp, err := os.CreateTemp(filepath.Dir(path), ".tmp-"+filepath.Base(path)+"-")
	if err != nil {
		return err
	}
	_, writeErr := temp.Write(data)
	closeErr := temp.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(temp.Name())
		return errors.Join(writeErr, closeErr)
	}
	if err := os.Rename(temp.Name(), path); err != nil {
		_ = os.Remove(temp.Name())
		return err
	}
	return nil
}

// pruneVersions keeps current and the previous version. A client still
// running from a removed folder keeps working on macOS and Linux; Windows
// refuses to delete it, and a later install retries.
func pruneVersions(dir, current, previous string) {
	entries, err := os.ReadDir(filepath.Join(dir, "versions"))
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == current || name == previous {
			continue
		}
		if strings.HasPrefix(name, ".tmp-") {
			if info, err := entry.Info(); err != nil || time.Since(info.ModTime()) < time.Hour {
				continue
			}
		}
		_ = os.RemoveAll(filepath.Join(dir, "versions", name))
	}
}
