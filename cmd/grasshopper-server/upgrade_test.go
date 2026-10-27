package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// TestMain lets the test binary stand in for a release's grasshopper-server,
// so upgrade runs real processes without the embedding model.
func TestMain(m *testing.M) {
	if version := os.Getenv("GRASSHOPPER_FAKE_SERVER_VERSION"); version != "" {
		os.Exit(fakeServer(version))
	}
	os.Exit(m.Run())
}

// fakeServer answers --version and an owner /connection. A version marked
// broken never starts.
func fakeServer(version string) int {
	values := map[string]string{}
	for i := 1; i < len(os.Args); i++ {
		if os.Args[i] == "--version" {
			fmt.Println("grasshopper server " + version)
			return 0
		}
		if i+1 < len(os.Args) && strings.HasPrefix(os.Args[i], "--") && !strings.HasPrefix(os.Args[i+1], "--") {
			values[os.Args[i]] = os.Args[i+1]
			i++
		}
	}
	if os.Getenv("GRASSHOPPER_FAKE_BROKEN") != "" {
		fmt.Fprintln(os.Stderr, "fake server refuses to start")
		return 1
	}
	if _, err := os.Stat(values["--model"]); err != nil {
		fmt.Fprintln(os.Stderr, "model path not rebased:", values["--model"])
		return 1
	}
	token, err := readToken(values["--token-file"])
	if err != nil {
		return 1
	}
	listener, err := net.Listen("tcp", values["--listen"])
	if err != nil {
		return 1
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/connection" || r.Header.Get("Authorization") != "Bearer "+token {
			http.Error(w, "no", 401)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"role": "owner", "version": version})
	})}
	go func() { _ = server.Serve(listener) }()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	return 0
}

// fakeLaunchd answers plutil and launchctl from a JSON "plist" and runs the
// job as a child process; other commands run for real. Like launchd it runs at
// most one process for the job.
type fakeLaunchd struct {
	mu     sync.Mutex
	plist  string
	job    *exec.Cmd
	starts int
}

type fakePlist struct {
	Label            string
	ProgramArguments []string
}

func readFakePlist(path string) (fakePlist, error) {
	var plist fakePlist
	data, err := os.ReadFile(path)
	if err == nil {
		err = json.Unmarshal(data, &plist)
	}
	return plist, err
}

func (f *fakeLaunchd) run(name string, args ...string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case name == "plutil" && args[0] == "-extract":
		plist, err := readFakePlist(args[len(args)-1])
		if err != nil {
			return nil, err
		}
		if args[1] == "Label" {
			return []byte(plist.Label + "\n"), nil
		}
		return json.Marshal(plist.ProgramArguments)
	case name == "plutil" && args[0] == "-replace":
		path := args[len(args)-1]
		plist, err := readFakePlist(path)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(args[3]), &plist.ProgramArguments); err != nil {
			return nil, err
		}
		data, _ := json.Marshal(plist)
		return nil, os.WriteFile(path, data, 0o600)
	case name == "plutil" && args[0] == "-lint":
		return nil, nil
	case name == "launchctl" && len(args) == 3 && strings.HasSuffix(args[2], ".upgrade.plist"):
		// The nightly job is only loaded, never run, in these tests.
		return nil, nil
	case name == "launchctl" && args[0] == "bootout":
		if f.job == nil {
			return nil, errors.New("not loaded")
		}
		f.kill()
		f.plist = ""
		return nil, nil
	case name == "launchctl" && args[0] == "bootstrap":
		if f.plist != "" {
			return nil, errors.New("already loaded")
		}
		f.plist = args[2]
		return nil, f.start()
	case name == "launchctl" && args[0] == "kickstart":
		if f.plist == "" {
			return nil, errors.New("not loaded")
		}
		f.kill()
		return nil, f.start()
	}
	return exec.Command(name, args...).Output()
}

func (f *fakeLaunchd) start() error {
	plist, err := readFakePlist(f.plist)
	if err != nil {
		return err
	}
	job := exec.Command(plist.ProgramArguments[0], plist.ProgramArguments[1:]...)
	if err := job.Start(); err != nil {
		return err
	}
	f.job = job
	f.starts++
	return nil
}

func (f *fakeLaunchd) kill() {
	if f.job != nil {
		_ = f.job.Process.Signal(os.Interrupt)
		_ = f.job.Wait()
		f.job = nil
	}
}

func (f *fakeLaunchd) stop() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.kill()
}

// writeArchive lays out a release archive whose programs are scripts around
// the test binary.
func writeArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, content := range files {
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		header.SetMode(0o644)
		if strings.HasPrefix(name, "bin/") {
			header.SetMode(0o755)
		}
		entry, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.WriteString(entry, content)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func archiveFiles(t *testing.T, version, env string) map[string]string {
	test, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return map[string]string{
		"bin/grasshopper-server": fmt.Sprintf("#!/bin/sh\nGRASSHOPPER_FAKE_SERVER_VERSION=%s %s exec '%s' \"$@\"\n", version, env, test),
		"bin/grasshopper-backup": "#!/bin/sh\ncp \"$2\" \"$4\" && echo \"backup verified\"\n",
		"models/model.onnx":      "model",
	}
}

type upgradeFixture struct {
	u        *upgrader
	launchd  *fakeLaunchd
	plist    string
	parent   string
	database string
	out      *bytes.Buffer
}

func newUpgradeFixture(t *testing.T, releases map[string]string) *upgradeFixture {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("upgrade drives launchd on macOS; this fixture uses shell scripts")
	}
	root := t.TempDir()
	parent := filepath.Join(root, "apps")
	current := filepath.Join(parent, "app")
	for name, content := range archiveFiles(t, "2.11.0", "") {
		path := filepath.Join(current, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	data := filepath.Join(root, "data")
	if err := os.MkdirAll(data, 0o700); err != nil {
		t.Fatal(err)
	}
	database := filepath.Join(data, "memory.db")
	tokenFile := filepath.Join(data, "token")
	if err := os.WriteFile(database, []byte("synthetic database"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tokenFile, []byte("synthetic-owner-token-0123456789-abcdef\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	listen := listener.Addr().String()
	listener.Close()
	plist := filepath.Join(root, "com.example.grasshopper.plist")
	encoded, _ := json.Marshal(fakePlist{Label: "com.example.grasshopper", ProgramArguments: []string{
		filepath.Join(current, "bin", "grasshopper-server"), "--db", database,
		"--onnx-library", filepath.Join(current, "runtime", "lib"), "--model", filepath.Join(current, "models", "model.onnx"),
		"--tokenizer", filepath.Join(current, "models", "tokenizer.json"),
		"--token-file", tokenFile, "--listen", listen, "--visualizer",
	}})
	if err := os.WriteFile(plist, encoded, 0o600); err != nil {
		t.Fatal(err)
	}

	served := map[string][]byte{}
	for version, env := range releases {
		name := "grasshopper-server-darwin-arm64-" + version + ".zip"
		archive := writeArchive(t, archiveFiles(t, version, env))
		sum := sha256.Sum256(archive)
		served["/v"+version+"/"+name] = archive
		served["/v"+version+"/SHA256SUMS"] = []byte(hex.EncodeToString(sum[:]) + "  " + name + "\n")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/latest" {
			_, _ = w.Write([]byte(`{"tag_name":"v2.11.2"}`))
			return
		}
		if data, ok := served[r.URL.Path]; ok {
			_, _ = w.Write(data)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)
	oldBase, oldLatest := releaseBaseURL, latestReleaseURL
	releaseBaseURL, latestReleaseURL = server.URL, server.URL+"/latest"
	t.Cleanup(func() { releaseBaseURL, latestReleaseURL = oldBase, oldLatest })

	launchd := &fakeLaunchd{}
	t.Cleanup(launchd.stop)
	out := &bytes.Buffer{}
	u := &upgrader{plist: plist, out: out, run: launchd.run, http: server.Client(),
		healthWait: 10 * time.Second}
	if _, err := launchd.run("launchctl", "bootstrap", "gui/0", plist); err != nil {
		t.Fatal(err)
	}
	if err := u.waitHealthy(context.Background(), listen, tokenFile, "2.11.0"); err != nil {
		t.Fatal(err)
	}
	return &upgradeFixture{u: u, launchd: launchd, plist: plist, parent: parent, database: database, out: out}
}

func (f *upgradeFixture) program(t *testing.T) string {
	t.Helper()
	plist, err := readFakePlist(f.plist)
	if err != nil {
		t.Fatal(err)
	}
	return plist.ProgramArguments[0]
}

func (f *upgradeFixture) links(t *testing.T) (string, string) {
	t.Helper()
	current, err := os.Readlink(filepath.Join(f.parent, "current"))
	if err != nil {
		t.Fatal(err)
	}
	previous, _ := os.Readlink(filepath.Join(f.parent, "previous"))
	return current, previous
}

func TestUpgradeSwitchesTheServiceAndRollsBack(t *testing.T) {
	f := newUpgradeFixture(t, map[string]string{"2.11.1": "", "2.11.2": ""})
	ctx := context.Background()

	if err := f.u.upgrade(ctx, "2.11.1"); err != nil {
		t.Fatalf("upgrade: %v\n%s", err, f.out)
	}
	if want := filepath.Join(f.parent, "current", "bin", "grasshopper-server"); f.program(t) != want {
		t.Fatalf("plist runs %s, want %s", f.program(t), want)
	}
	if current, previous := f.links(t); current != "grasshopper-server-2.11.1" || previous != "app" {
		t.Fatalf("current %s, previous %s", current, previous)
	}
	backups, _ := filepath.Glob(filepath.Join(filepath.Dir(f.database), "backups", "pre-2.11.1-*.db"))
	if len(backups) != 1 {
		t.Fatalf("want one pre-upgrade backup, got %v", backups)
	}
	if err := f.u.upgrade(ctx, "2.11.1"); err != nil || !strings.Contains(f.out.String(), "already runs 2.11.1") {
		t.Fatalf("a second upgrade should do nothing: %v\n%s", err, f.out)
	}

	if err := f.u.rollback(ctx); err != nil {
		t.Fatalf("rollback: %v\n%s", err, f.out)
	}
	if current, previous := f.links(t); current != "app" || previous != "grasshopper-server-2.11.1" {
		t.Fatalf("rollback left current %s, previous %s", current, previous)
	}
	if err := f.u.rollback(ctx); err != nil {
		t.Fatalf("a second rollback should go forward again: %v", err)
	}

	if err := f.u.upgrade(ctx, ""); err != nil {
		t.Fatalf("upgrade to latest: %v\n%s", err, f.out)
	}
	if current, previous := f.links(t); current != "grasshopper-server-2.11.2" || previous != "grasshopper-server-2.11.1" {
		t.Fatalf("current %s, previous %s", current, previous)
	}
	if _, err := os.Stat(filepath.Join(f.parent, "app")); err != nil {
		t.Fatal("pruning removed a folder upgrade did not create")
	}
	f.launchd.mu.Lock()
	defer f.launchd.mu.Unlock()
	if f.launchd.plist == "" || f.launchd.job == nil {
		t.Fatal("the service is not running")
	}
}

func TestFailedUpgradeRestoresThePreviousServer(t *testing.T) {
	f := newUpgradeFixture(t, map[string]string{"2.11.3": "GRASSHOPPER_FAKE_BROKEN=1"})
	f.u.healthWait = 3 * time.Second

	err := f.u.upgrade(context.Background(), "2.11.3")
	if err == nil || !strings.Contains(err.Error(), "2.11.0 is running again") {
		t.Fatalf("want a restored 2.11.0, got %v\n%s", err, f.out)
	}
	if current, previous := f.links(t); current != "app" || previous != "" {
		t.Fatalf("current %s, previous %s", current, previous)
	}
	if err := f.u.rollback(context.Background()); err == nil {
		t.Fatal("a failed upgrade left something to roll back to")
	}
}

func TestUpgradeRefusesAnArchiveThatDoesNotMatchSHA256SUMS(t *testing.T) {
	f := newUpgradeFixture(t, map[string]string{"2.11.1": ""})
	oldBase := releaseBaseURL
	tampered := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".zip") {
			_, _ = w.Write([]byte("not the archive"))
			return
		}
		response, err := http.Get(oldBase + r.URL.Path)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		defer response.Body.Close()
		_, _ = io.Copy(w, response.Body)
	}))
	defer tampered.Close()
	releaseBaseURL = tampered.URL

	if err := f.u.upgrade(context.Background(), "2.11.1"); err == nil || !strings.Contains(err.Error(), "SHA256SUMS") {
		t.Fatalf("want a checksum failure, got %v", err)
	}
}

func TestExtractRefusesPathsOutsideTheArchive(t *testing.T) {
	archive := writeArchive(t, map[string]string{"../escape": "x"})
	if err := extract(archive, t.TempDir()); err == nil {
		t.Fatal("zip-slip path accepted")
	}
}

func TestRebaseMovesOnlyPathsInsideTheOldArchive(t *testing.T) {
	cases := map[string]string{
		"/a/app/bin/grasshopper-server": "/a/new/bin/grasshopper-server",
		"/a/app":                        "/a/new",
		"/a/application/x":              "/a/application/x",
		"--listen":                      "--listen",
	}
	for in, want := range cases {
		in, want = filepath.FromSlash(in), filepath.FromSlash(want)
		if got := rebase(in, filepath.FromSlash("/a/app"), filepath.FromSlash("/a/new")); got != want {
			t.Errorf("rebase(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFindPlistNeedsExactlyOneGrasshopperJob(t *testing.T) {
	dir := t.TempDir()
	launchd := &fakeLaunchd{}
	write := func(name, program string) {
		data, _ := json.Marshal(fakePlist{Label: name, ProgramArguments: []string{program}})
		if err := os.WriteFile(filepath.Join(dir, name+".plist"), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("other", "/usr/bin/true")
	if _, err := findPlist(launchd.run, dir); err == nil {
		t.Fatal("no Grasshopper plist should fail")
	}
	write("grasshopper", "/apps/app/bin/grasshopper-server")
	if got, err := findPlist(launchd.run, dir); err != nil || got != filepath.Join(dir, "grasshopper.plist") {
		t.Fatalf("findPlist = %q, %v", got, err)
	}
	write("grasshopper2", "/apps/two/bin/grasshopper-server")
	if _, err := findPlist(launchd.run, dir); err == nil || !strings.Contains(err.Error(), "several") {
		t.Fatalf("two Grasshopper plists should fail, got %v", err)
	}
}

func TestScheduleWritesANightlyJobThatFollowsTheServicePlist(t *testing.T) {
	f := newUpgradeFixture(t, nil)
	t.Setenv("HOME", t.TempDir())
	if err := f.u.setSchedule(true); err != nil {
		t.Fatalf("schedule: %v", err)
	}
	job := filepath.Join(filepath.Dir(f.plist), "com.example.grasshopper.upgrade.plist")
	data, err := os.ReadFile(job)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<string>com.example.grasshopper.upgrade</string>", "ProgramArguments.0", "upgrade --plist", "<string>" + f.plist + "</string>", "<integer>3</integer>"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("job plist lacks %q", want)
		}
	}
	if err := xml.Unmarshal(data, new(any)); err != nil {
		t.Fatalf("job plist is not XML: %v", err)
	}
	if _, err := findPlist(f.launchd.run, filepath.Dir(f.plist)); err != nil {
		t.Fatalf("the job must not look like a second server plist: %v", err)
	}
	if err := f.u.setSchedule(false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(job); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("unschedule left the job plist")
	}
}
