package client

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode"
)

// Scope carries optional project, device, and platform dimensions.
// In reads, a nil field matches only unscoped records in that dimension.
type Scope struct {
	Project  *string `json:"project"`
	Device   *string `json:"device"`
	Platform *string `json:"platform"`
	Legacy   bool    `json:"legacy"`
}

// ProjectIdentity derives a credential-free Git scope from a remote.
// It normalizes the host and default port but preserves repository path case.
func ProjectIdentity(remote string) (string, error) {
	remote = strings.TrimSpace(remote)
	if remote == "" || strings.IndexFunc(remote, unicode.IsSpace) >= 0 {
		return "", errors.New("invalid Git remote")
	}
	clean := strings.SplitN(strings.SplitN(remote, "?", 2)[0], "#", 2)[0]
	var host, path, port, scheme string
	if strings.Contains(clean, "://") {
		parsed, err := url.Parse(clean)
		if err != nil || parsed.Host == "" {
			return "", errors.New("invalid Git remote")
		}
		scheme = parsed.Scheme
		if scheme != "https" && scheme != "http" && scheme != "ssh" && scheme != "git" {
			return "", errors.New("local remotes require explicit project ID")
		}
		host, port, path = parsed.Hostname(), parsed.Port(), parsed.Path
	} else {
		at := strings.LastIndex(clean, "@")
		if at < 0 {
			return "", errors.New("local remotes require explicit project ID")
		}
		colon := strings.Index(clean[at+1:], ":")
		if colon < 0 {
			return "", errors.New("invalid SSH remote")
		}
		host = clean[at+1 : at+1+colon]
		path = clean[at+2+colon:]
		scheme = "ssh"
	}
	host = strings.ToLower(host)
	defaults := map[string]string{"https": "443", "http": "80", "ssh": "22", "git": "9418"}
	if port != "" && port != defaults[scheme] {
		host += ":" + port
	}
	path = strings.Trim(path, "/")
	path = strings.TrimSuffix(path, ".git")
	if host == "" || path == "" || strings.ContainsAny(path, "@?#\\") || strings.Contains(path, "://") {
		return "", errors.New("invalid repository identity")
	}
	for _, part := range strings.Split(path, "/") {
		if part == "" || part == "." || part == ".." {
			return "", errors.New("invalid repository path")
		}
	}
	return "git:" + host + "/" + path, nil
}

func gitValue(ctx context.Context, cwd string, args ...string) string {
	arguments := append([]string{"-C", cwd}, args...)
	command := exec.CommandContext(ctx, "git", arguments...)
	command.WaitDelay = 100 * time.Millisecond
	output, err := command.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// ResolveScope prefers a local grasshopper.project-id, then the origin remote.
// It returns an error when neither provides a durable project identity.
// Both Git lookups share a two-second budget so a stalled process cannot
// exhaust the startup hook's deadline before it requests memory context.
func ResolveScope(cwd, device string) (Scope, error) {
	if !filepath.IsAbs(cwd) {
		return Scope{}, errors.New("workspace path must be absolute")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	project := ""
	if id := gitValue(ctx, cwd, "config", "--local", "--get", "grasshopper.project-id"); id != "" {
		project = "id:" + id
	} else {
		if err := ctx.Err(); err != nil {
			return Scope{}, err
		}
		remote := gitValue(ctx, cwd, "remote", "get-url", "origin")
		if err := ctx.Err(); err != nil {
			return Scope{}, err
		}
		if remote == "" {
			return Scope{}, errors.New("no Git remote or explicit project ID")
		}
		var err error
		project, err = ProjectIdentity(remote)
		if err != nil {
			return Scope{}, err
		}
	}
	if len(project) > 512 || strings.IndexFunc(project, unicode.IsControl) >= 0 || strings.ContainsAny(project, "@?#\\") || strings.Contains(project, "://") {
		return Scope{}, errors.New("invalid project ID")
	}
	platform := Platform()
	if platform != "macos" && platform != "windows" && platform != "linux" {
		return Scope{}, fmt.Errorf("unsupported platform %s", platform)
	}
	return Scope{Project: &project, Device: &device, Platform: &platform}, nil
}

// Platform returns the memory scope's OS name, mapping Go's darwin to macos.
func Platform() string {
	platform := runtime.GOOS
	if platform == "darwin" {
		platform = "macos"
	}
	return platform
}
