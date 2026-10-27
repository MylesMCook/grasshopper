package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Upgrade replaces a launchd-supervised server with another release in one
// command: download and check the archive, back up the database, rehearse the
// new server on a copy, stop the service, point its plist at the new archive,
// start it and check authenticated health. Any failure after the stop puts
// the previous plist back and restarts it. The previous archive and plist stay
// on disk for --rollback.
//
// Archives are extracted beside the running one as grasshopper-server-X.Y.Z.
// Only plist arguments under the old archive folder change; the database,
// token, listener and proxy flags are kept as they are.

// releaseBaseURL and latestReleaseURL are where releases come from. Tests
// replace them.
var (
	releaseBaseURL   = "https://github.com/MylesMCook/grasshopper/releases/download"
	latestReleaseURL = "https://api.github.com/repos/MylesMCook/grasshopper/releases/latest"
)

// upgrader holds the parts of an upgrade that touch the machine, so tests can
// replace launchd and the network.
type upgrader struct {
	plist       string
	out         io.Writer
	run         func(name string, args ...string) ([]byte, error)
	http        *http.Client
	healthWait  time.Duration
	stopWait    time.Duration
	processGone func(pid int) bool
}

func upgradeCommand(args []string) error {
	flags := flag.NewFlagSet("upgrade", flag.ContinueOnError)
	plist := flags.String("plist", "", "the server's launchd plist (default: the one Grasshopper plist in ~/Library/LaunchAgents)")
	to := flags.String("to", "", "release to install (default: the latest release)")
	rollback := flags.Bool("rollback", false, "switch back to the plist saved by the last upgrade")
	schedule := flags.Bool("schedule", false, "install a launchd job that runs upgrade every night")
	unschedule := flags.Bool("unschedule", false, "remove the nightly upgrade job")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected upgrade arguments")
	}
	if runtime.GOOS != "darwin" {
		return errors.New("upgrade supports a macOS launchd service; on other systems follow docs/operations.md")
	}
	if *plist == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		found, err := findPlist(runCommand, filepath.Join(home, "Library", "LaunchAgents"))
		if err != nil {
			return err
		}
		*plist = found
	}
	if !filepath.IsAbs(*plist) {
		return errors.New("--plist must be the absolute path of the server's launchd plist")
	}
	u := &upgrader{
		plist: *plist, out: os.Stdout, run: runCommand,
		http:        &http.Client{Timeout: 10 * time.Minute},
		healthWait:  2 * time.Minute,
		stopWait:    time.Minute,
		processGone: processGone,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	if *schedule || *unschedule {
		if *to != "" || *rollback || (*schedule && *unschedule) {
			return errors.New("schedule and unschedule take only --plist")
		}
		return u.setSchedule(*schedule)
	}
	if *rollback {
		if *to != "" {
			return errors.New("rollback cannot be combined with --to")
		}
		return u.rollback(ctx)
	}
	return u.upgrade(ctx, *to)
}

// setSchedule installs or removes a second launchd job, beside the service
// plist, that runs upgrade at 03:30 each night. The job reads the service's
// current program from its plist at run time, so it keeps working after
// upgrades move the server to a new archive. A failed night leaves the
// previous server running; the log says why.
func (u *upgrader) setSchedule(install bool) error {
	job, err := u.readService()
	if err != nil {
		return err
	}
	label := job.label + ".upgrade"
	path := filepath.Join(filepath.Dir(u.plist), label+".plist")
	domain := "gui/" + strconv.Itoa(os.Getuid())
	_, _ = u.run("launchctl", "bootout", domain, path)
	if !install {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		u.step("Nightly upgrades are off.")
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	logs := filepath.Join(home, "Library", "Logs", "Grasshopper")
	if err := os.MkdirAll(logs, 0o700); err != nil {
		return err
	}
	script := `program=$(/usr/bin/plutil -extract ProgramArguments.0 raw -o - "$1") && exec "$program" upgrade --plist "$1"`
	if err := os.WriteFile(path, []byte(schedulePlist(label, script, u.plist, filepath.Join(logs, "upgrade.log"))), 0o600); err != nil {
		return err
	}
	if _, err := u.run("plutil", "-lint", path); err != nil {
		return err
	}
	if _, err := u.run("launchctl", "bootstrap", domain, path); err != nil {
		return err
	}
	u.step("Nightly upgrades are on (03:30). Log: %s", filepath.Join(logs, "upgrade.log"))
	return nil
}

func schedulePlist(label, script, servicePlist, log string) string {
	escape := func(value string) string {
		var buffer bytes.Buffer
		_ = xml.EscapeText(&buffer, []byte(value))
		return buffer.String()
	}
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>` + escape(label) + `</string>
  <key>ProgramArguments</key>
  <array>
    <string>/bin/sh</string>
    <string>-c</string>
    <string>` + escape(script) + `</string>
    <string>sh</string>
    <string>` + escape(servicePlist) + `</string>
  </array>
  <key>StartCalendarInterval</key>
  <dict>
    <key>Hour</key>
    <integer>3</integer>
    <key>Minute</key>
    <integer>30</integer>
  </dict>
  <key>ProcessType</key>
  <string>Background</string>
  <key>Umask</key>
  <integer>63</integer>
  <key>StandardOutPath</key>
  <string>` + escape(log) + `</string>
  <key>StandardErrorPath</key>
  <string>` + escape(log) + `</string>
</dict>
</plist>
`
}

// findPlist returns the one launchd plist in dir whose program is a
// grasshopper-server, so the usual single-server Mac needs no --plist.
func findPlist(run func(string, ...string) ([]byte, error), dir string) (string, error) {
	candidates, _ := filepath.Glob(filepath.Join(dir, "*.plist"))
	var found []string
	for _, candidate := range candidates {
		raw, err := run("plutil", "-extract", "ProgramArguments", "json", "-o", "-", candidate)
		var arguments []string
		if err == nil && json.Unmarshal(raw, &arguments) == nil && len(arguments) > 0 && filepath.Base(arguments[0]) == "grasshopper-server" {
			found = append(found, candidate)
		}
	}
	switch len(found) {
	case 1:
		return found[0], nil
	case 0:
		return "", fmt.Errorf("no launchd plist in %s runs grasshopper-server; pass --plist", dir)
	}
	return "", fmt.Errorf("several plists run grasshopper-server (%s); pass --plist", strings.Join(found, ", "))
}

func runCommand(name string, args ...string) ([]byte, error) {
	command := exec.Command(name, args...)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		return output, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return output, nil
}

func processGone(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return true
	}
	return errors.Is(process.Signal(syscall.Signal(0)), os.ErrProcessDone)
}

// launchdJob is what upgrade reads from the plist.
type launchdJob struct {
	label     string
	arguments []string
	root      string // extracted archive folder holding bin/grasshopper-server
	listen    string
	database  string
	tokenFile string
}

func (u *upgrader) readService() (launchdJob, error) {
	var s launchdJob
	label, err := u.run("plutil", "-extract", "Label", "raw", "-o", "-", u.plist)
	if err != nil {
		return s, err
	}
	s.label = strings.TrimSpace(string(label))
	raw, err := u.run("plutil", "-extract", "ProgramArguments", "json", "-o", "-", u.plist)
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(raw, &s.arguments); err != nil || len(s.arguments) == 0 {
		return s, errors.New("plist ProgramArguments is not a list of strings")
	}
	program := s.arguments[0]
	if filepath.Base(program) != "grasshopper-server" || filepath.Base(filepath.Dir(program)) != "bin" || !filepath.IsAbs(program) {
		return s, fmt.Errorf("plist runs %s, not an archive's bin/grasshopper-server", program)
	}
	s.root = filepath.Dir(filepath.Dir(program))
	values := map[string]string{"--listen": "127.0.0.1:8106"}
	quickstart := false
	for i := 1; i < len(s.arguments); i++ {
		name, value, inline := strings.Cut(s.arguments[i], "=")
		name = "--" + strings.TrimLeft(name, "-")
		if name == "--quickstart" || name == "--visualizer" || name == "--create-db" {
			quickstart = quickstart || name == "--quickstart"
			continue
		}
		if !inline && i+1 < len(s.arguments) {
			i++
			value = s.arguments[i]
		}
		values[name] = value
	}
	s.listen, s.database, s.tokenFile = values["--listen"], values["--db"], values["--token-file"]
	if quickstart {
		dir := values["--data-dir"]
		if dir == "" {
			// The server's own quickstart default.
			if base, err := os.UserConfigDir(); err == nil {
				dir = filepath.Join(base, "Grasshopper")
			}
		}
		s.database, s.tokenFile = filepath.Join(dir, "memory.db"), filepath.Join(dir, "access-token")
	}
	if s.database == "" || s.tokenFile == "" {
		return s, errors.New("plist names no database and token file")
	}
	return s, nil
}

func (u *upgrader) step(format string, args ...any) {
	fmt.Fprintf(u.out, format+"\n", args...)
}

func (u *upgrader) upgrade(ctx context.Context, target string) error {
	current, err := u.readService()
	if err != nil {
		return err
	}
	running, err := archiveVersion(current.root)
	if err != nil {
		return err
	}
	if target == "" {
		if target, err = u.latestVersion(ctx); err != nil {
			return err
		}
	}
	if _, ok := parseVersion(target); !ok {
		return fmt.Errorf("invalid release %q", target)
	}
	if target == running {
		u.step("The server already runs %s.", running)
		return nil
	}
	u.step("Upgrading the server from %s to %s.", running, target)

	root, err := u.fetchArchive(ctx, filepath.Dir(current.root), target)
	if err != nil {
		return err
	}
	backup, err := u.backup(root, current, target)
	if err != nil {
		return err
	}
	u.step("Backed up and verified the database: %s", backup)
	if err := u.rehearse(ctx, root, current, backup, target); err != nil {
		return fmt.Errorf("rehearsal on a copy failed, live server untouched: %w", err)
	}
	u.step("Rehearsed %s on a copy of the backup.", target)

	next := make([]string, len(current.arguments))
	for i, argument := range current.arguments {
		next[i] = rebase(argument, current.root, root)
	}
	if err := copyPrivate(u.plist, u.plist+".previous"); err != nil {
		return err
	}
	if err := u.switchTo(ctx, current, next, target); err != nil {
		u.step("Upgrade failed: %v", err)
		u.step("Restoring %s.", running)
		if restoreErr := u.switchTo(ctx, current, current.arguments, running); restoreErr != nil {
			return fmt.Errorf("upgrade failed (%v) and restoring %s also failed: %w", err, running, restoreErr)
		}
		return fmt.Errorf("upgrade failed; %s is running again: %w", running, err)
	}
	u.pruneArchives(filepath.Dir(root), root, current.root)
	u.step("The server runs %s. The previous version stays for upgrade --rollback.", target)
	return nil
}

// rollback swaps the plist with the one the last upgrade saved, so a second
// rollback goes forward again.
func (u *upgrader) rollback(ctx context.Context) error {
	current, err := u.readService()
	if err != nil {
		return err
	}
	saved := &upgrader{plist: u.plist + ".previous", run: u.run}
	previous, err := saved.readService()
	if err != nil {
		return fmt.Errorf("no plist saved by an earlier upgrade: %w", err)
	}
	version, err := archiveVersion(previous.root)
	if err != nil {
		return err
	}
	if err := copyPrivate(u.plist, u.plist+".rollback"); err != nil {
		return err
	}
	if err := u.switchTo(ctx, current, previous.arguments, version); err != nil {
		running, _ := archiveVersion(current.root)
		if restoreErr := u.switchTo(ctx, current, current.arguments, running); restoreErr != nil {
			return fmt.Errorf("rollback failed (%v) and restoring also failed: %w", err, restoreErr)
		}
		return fmt.Errorf("rollback failed; the server runs %s again: %w", running, err)
	}
	if err := os.Rename(u.plist+".rollback", u.plist+".previous"); err != nil {
		return err
	}
	u.step("The server runs %s. Run upgrade --rollback again to go back.", version)
	return nil
}

// switchTo stops the service, waits for its process to exit so the database
// lock is released, writes arguments to the plist, starts the service and
// waits for authenticated health to report version.
func (u *upgrader) switchTo(ctx context.Context, s launchdJob, arguments []string, version string) error {
	domain := "gui/" + strconv.Itoa(os.Getuid())
	pid := u.servicePID(domain + "/" + s.label)
	if _, err := u.run("launchctl", "bootout", domain, u.plist); err != nil && pid != 0 {
		return err
	}
	if pid != 0 {
		deadline := time.Now().Add(u.stopWait)
		for !u.processGone(pid) {
			if time.Now().After(deadline) {
				return fmt.Errorf("server process %d did not exit", pid)
			}
			time.Sleep(200 * time.Millisecond)
		}
	}
	encoded, _ := json.Marshal(arguments)
	if _, err := u.run("plutil", "-replace", "ProgramArguments", "-json", string(encoded), u.plist); err != nil {
		return err
	}
	if _, err := u.run("plutil", "-lint", u.plist); err != nil {
		return err
	}
	if _, err := u.run("launchctl", "bootstrap", domain, u.plist); err != nil {
		return err
	}
	return u.waitHealthy(ctx, s.listen, s.tokenFile, version)
}

// servicePID reads the running job's PID, or 0 when it is not running.
func (u *upgrader) servicePID(target string) int {
	output, err := u.run("launchctl", "print", target)
	if err != nil {
		return 0
	}
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		if value, ok := strings.CutPrefix(strings.TrimSpace(scanner.Text()), "pid = "); ok {
			if pid, err := strconv.Atoi(value); err == nil && pid > 0 {
				return pid
			}
		}
	}
	return 0
}

// waitHealthy waits for an owner-authenticated /connection on listen to
// report version. The embedding model takes some seconds to load.
func (u *upgrader) waitHealthy(ctx context.Context, listen, tokenFile, version string) error {
	token, err := readToken(tokenFile)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(u.healthWait)
	last := errors.New("no answer")
	for time.Now().Before(deadline) {
		request, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+listen+"/connection", nil)
		request.Header.Set("Authorization", "Bearer "+token)
		client := &http.Client{Timeout: 5 * time.Second}
		if response, err := client.Do(request); err == nil {
			var identity struct {
				Role    string `json:"role"`
				Version string `json:"version"`
			}
			decodeErr := json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&identity)
			response.Body.Close()
			switch {
			case response.StatusCode != http.StatusOK || decodeErr != nil:
				last = fmt.Errorf("health returned HTTP %d", response.StatusCode)
			case identity.Role != "owner" || identity.Version != version:
				last = fmt.Errorf("server reports %s, not %s", identity.Version, version)
			default:
				return nil
			}
		} else {
			last = err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return fmt.Errorf("server did not become healthy: %w", last)
}

func (u *upgrader) latestVersion(ctx context.Context) (string, error) {
	data, err := u.download(ctx, latestReleaseURL, 1<<20)
	if err != nil {
		return "", fmt.Errorf("latest release: %w", err)
	}
	var release struct {
		Tag string `json:"tag_name"`
	}
	if err := json.Unmarshal(data, &release); err != nil || release.Tag == "" {
		return "", errors.New("latest release has no tag")
	}
	return strings.TrimPrefix(release.Tag, "v"), nil
}

// fetchArchive downloads, checks and extracts the server archive for version
// into parent, reusing an earlier extraction of the same version.
func (u *upgrader) fetchArchive(ctx context.Context, parent, version string) (string, error) {
	root := filepath.Join(parent, "grasshopper-server-"+version)
	if got, err := archiveVersion(root); err == nil && got == version {
		u.step("Using the %s archive already extracted at %s.", version, root)
		return root, nil
	}
	name := "grasshopper-server-darwin-arm64-" + version + ".zip"
	base := strings.TrimSuffix(releaseBaseURL, "/") + "/v" + version + "/"
	sums, err := u.download(ctx, base+"SHA256SUMS", 64<<10)
	if err != nil {
		return "", fmt.Errorf("SHA256SUMS: %w", err)
	}
	want, err := checksumFor(sums, name)
	if err != nil {
		return "", err
	}
	zipped, err := u.download(ctx, base+name, 1<<30)
	if err != nil {
		return "", fmt.Errorf("%s: %w", name, err)
	}
	if got := sha256.Sum256(zipped); hex.EncodeToString(got[:]) != want {
		return "", fmt.Errorf("%s does not match SHA256SUMS", name)
	}
	staging, err := os.MkdirTemp(parent, ".tmp-grasshopper-server-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(staging)
	if err := extract(zipped, staging); err != nil {
		return "", err
	}
	if got, err := archiveVersion(staging); err != nil || got != version {
		return "", fmt.Errorf("downloaded archive is not server %s", version)
	}
	_ = os.RemoveAll(root)
	if err := os.Rename(staging, root); err != nil {
		return "", err
	}
	u.step("Downloaded and checked %s.", name)
	return root, nil
}

func (u *upgrader) download(ctx context.Context, url string, limit int64) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	response, err := u.http.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, err
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

// extract writes a release archive under dir, refusing paths that leave it.
func extract(zipped []byte, dir string) error {
	reader, err := zip.NewReader(bytes.NewReader(zipped), int64(len(zipped)))
	if err != nil {
		return errors.New("archive is not a zip file")
	}
	for _, file := range reader.File {
		name := filepath.FromSlash(file.Name)
		if !filepath.IsLocal(name) {
			return fmt.Errorf("archive path leaves its folder: %s", file.Name)
		}
		path := filepath.Join(dir, name)
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(path, 0o755); err != nil {
				return err
			}
			continue
		}
		if !file.Mode().IsRegular() {
			return fmt.Errorf("archive entry is not a regular file: %s", file.Name)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		mode := os.FileMode(0o644)
		if file.Mode().Perm()&0o111 != 0 {
			mode = 0o755
		}
		opened, err := file.Open()
		if err != nil {
			return err
		}
		target, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
		if err != nil {
			opened.Close()
			return err
		}
		_, copyErr := io.Copy(target, io.LimitReader(opened, 1<<30))
		closeErr := target.Close()
		opened.Close()
		if copyErr != nil || closeErr != nil {
			return errors.Join(copyErr, closeErr)
		}
	}
	return nil
}

func archiveVersion(root string) (string, error) {
	output, err := exec.Command(filepath.Join(root, "bin", "grasshopper-server"), "--version").Output()
	if err != nil {
		return "", fmt.Errorf("cannot read the server version in %s: %w", root, err)
	}
	version, ok := strings.CutPrefix(strings.TrimSpace(string(output)), "grasshopper server ")
	if !ok {
		return "", fmt.Errorf("unexpected version output from %s", root)
	}
	return version, nil
}

// backup snapshots the live database with the new archive's backup tool,
// which also verifies the snapshot. SQLite backup is safe while it runs.
func (u *upgrader) backup(root string, s launchdJob, version string) (string, error) {
	dir := filepath.Join(filepath.Dir(s.database), "backups")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "pre-"+version+"-"+time.Now().UTC().Format("20060102T150405Z")+".db")
	if output, err := u.run(filepath.Join(root, "bin", "grasshopper-backup"), "--source", s.database, "--dest", path); err != nil {
		return "", fmt.Errorf("backup failed: %w", err)
	} else {
		fmt.Fprint(u.out, string(output))
	}
	return path, nil
}

// rehearse starts the new server on a copy of the backup at a free loopback
// port, waits for it to report version, then stops it. Migrations therefore
// run on a copy before they touch the live database.
func (u *upgrader) rehearse(ctx context.Context, root string, s launchdJob, backup, version string) error {
	dir, err := os.MkdirTemp(filepath.Dir(backup), ".rehearsal-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	database := filepath.Join(dir, "memory.db")
	if err := copyPrivate(backup, database); err != nil {
		return err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	listen := listener.Addr().String()
	listener.Close()
	arguments := []string{}
	for i := 1; i < len(s.arguments); i++ {
		name, _, inline := strings.Cut(s.arguments[i], "=")
		name = "--" + strings.TrimLeft(name, "-")
		takesValue := name != "--quickstart" && name != "--visualizer" && name != "--create-db"
		switch name {
		case "--db", "--listen", "--data-dir", "--quickstart", "--token-file", "--create-db":
			if takesValue && !inline {
				i++
			}
			continue
		}
		arguments = append(arguments, rebase(s.arguments[i], s.root, root))
	}
	program := filepath.Join(root, "bin", "grasshopper-server")
	arguments = append(arguments, "--db", database, "--token-file", s.tokenFile, "--listen", listen)
	if !containsFlag(arguments, "--onnx-library") {
		// A quickstart service finds the model beside its program.
		library, model, tokenizer, err := quickstartFiles(program)
		if err != nil {
			return err
		}
		arguments = append(arguments, "--onnx-library", library, "--model", model, "--tokenizer", tokenizer)
	}
	command := exec.CommandContext(ctx, program, arguments...)
	var logs bytes.Buffer
	command.Stdout, command.Stderr = &logs, &logs
	if err := command.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	healthy := make(chan error, 1)
	go func() { healthy <- u.waitHealthy(ctx, listen, s.tokenFile, version) }()
	select {
	case err = <-healthy:
		_ = command.Process.Signal(os.Interrupt)
		<-done
	case waitErr := <-done:
		err = fmt.Errorf("server exited: %v: %s", waitErr, strings.TrimSpace(logs.String()))
	}
	return err
}

func containsFlag(arguments []string, flag string) bool {
	for _, argument := range arguments {
		if argument == flag || strings.HasPrefix(argument, flag+"=") {
			return true
		}
	}
	return false
}

// rebase moves an argument under the old archive folder to the new one.
func rebase(argument, oldRoot, newRoot string) string {
	if argument == oldRoot {
		return newRoot
	}
	if rest, ok := strings.CutPrefix(argument, oldRoot+string(filepath.Separator)); ok {
		return filepath.Join(newRoot, rest)
	}
	return argument
}

func copyPrivate(source, destination string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	temp := destination + ".tmp"
	if err := os.WriteFile(temp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(temp, destination)
}

// pruneArchives removes archives an earlier upgrade extracted, keeping the
// running and previous ones. Folders it did not create are left alone.
func (u *upgrader) pruneArchives(parent string, keep ...string) {
	entries, err := os.ReadDir(parent)
	if err != nil {
		return
	}
	for _, entry := range entries {
		path := filepath.Join(parent, entry.Name())
		version, ok := strings.CutPrefix(entry.Name(), "grasshopper-server-")
		if !ok || !entry.IsDir() || path == keep[0] || path == keep[1] {
			continue
		}
		if _, ok := parseVersion(version); ok {
			_ = os.RemoveAll(path)
		}
	}
}

func parseVersion(version string) ([3]int, bool) {
	var parsed [3]int
	parts := strings.Split(version, ".")
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
