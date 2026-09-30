// Package goclient connects coding harnesses to one authenticated memory
// service. It never opens a local memory database.
package goclient

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"
)

// Config names one remote service, a credential source, local policy, and device.
// Exactly one of TokenEnv and TokenFile is required when connecting.
type Config struct {
	URL        string `json:"url"`
	TokenEnv   string `json:"token_env,omitempty"`
	TokenFile  string `json:"token_file,omitempty"`
	PolicyPath string `json:"policy_path"`
	Device     string `json:"device"`
}

// ConfigPath lets installed plugins use one client configuration without
// embedding machine-specific paths in their manifests.
func ConfigPath(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if path := os.Getenv("GRASSHOPPER_CLIENT_CONFIG"); path != "" {
		return path, nil
	}
	path, err := DefaultConfigPath()
	if err != nil {
		return "", err
	}
	// Reading an old connection is compatible and has no migration side effects.
	if runtime.GOOS == "windows" {
		if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
			if _, err := os.Lstat(path + ".pairing"); errors.Is(err, os.ErrNotExist) {
				legacy, err := LegacyConfigPath()
				if err != nil {
					return "", err
				}
				if _, err := os.Lstat(legacy); err == nil || !errors.Is(err, os.ErrNotExist) {
					return legacy, nil
				}
				if _, err := os.Lstat(legacy + ".pairing"); err == nil || !errors.Is(err, os.ErrNotExist) {
					return legacy, nil
				}
			}
		}
	}
	return path, nil
}

// DefaultConfigPath is the canonical location for new connections. Windows
// uses the user profile so packaged and ordinary agents share the same path.
func DefaultConfigPath() (string, error) {
	if runtime.GOOS == "windows" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".grasshopper", "client.json"), nil
	}
	return LegacyConfigPath()
}

// LegacyConfigPath names the prior default; it is only a compatibility input.
func LegacyConfigPath() (string, error) {
	directory, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, "grasshopper", "client.json"), nil
}

// DefaultTokenPath names a device credential, separate from the server owner token.
func DefaultTokenPath() (string, error) {
	if runtime.GOOS == "windows" {
		path, err := DefaultConfigPath()
		if err != nil {
			return "", err
		}
		return filepath.Join(filepath.Dir(path), "device-token"), nil
	}
	directory, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, "Grasshopper", "device-token"), nil
}

// LoadConfig parses a bounded, strict JSON file. NewRemote separately
// validates the address and credential before making requests.
func LoadConfig(path string) (Config, error) {
	var config Config
	file, err := os.Open(path)
	if err != nil {
		return config, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 16384 {
		return config, errors.New("client config must be a small regular file")
	}
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return config, err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return config, errors.New("client config has trailing or invalid data")
	}
	if config.Device == "" || len(config.Device) > 512 || strings.IndexFunc(config.Device, unicode.IsControl) >= 0 {
		return config, errors.New("invalid device ID")
	}
	if config.PolicyPath == "" {
		config.PolicyPath = filepath.Join(filepath.Dir(path), "AGENTS.md")
	}
	return config, nil
}

func (c Config) credential() (string, error) {
	if (c.TokenEnv == "") == (c.TokenFile == "") {
		return "", errors.New("configure exactly one backend credential source")
	}
	var token string
	if c.TokenEnv != "" {
		token = os.Getenv(c.TokenEnv)
	} else {
		if !filepath.IsAbs(c.TokenFile) {
			return "", errors.New("token file path must be absolute")
		}
		info, err := os.Stat(c.TokenFile)
		if err != nil {
			return "", fmt.Errorf("token file unavailable: %w", err)
		}
		if !info.Mode().IsRegular() || info.Size() > 4096 {
			return "", errors.New("token file must be a small regular file")
		}
		if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
			return "", errors.New("token file must be private to its owner")
		}
		data, err := os.ReadFile(c.TokenFile)
		if err != nil {
			return "", err
		}
		token = string(data)
	}
	token = strings.TrimRight(token, "\r\n")
	if len(token) < 32 || strings.IndexFunc(token, unicode.IsSpace) >= 0 {
		return "", errors.New("invalid backend credential")
	}
	return token, nil
}

func (c Config) endpoint() (*url.URL, error) {
	endpoint, err := url.Parse(c.URL)
	if err != nil || endpoint == nil || endpoint.Host == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.ForceQuery || endpoint.Fragment != "" {
		return nil, errors.New("invalid backend URL")
	}
	if endpoint.Scheme != "https" {
		host := endpoint.Hostname()
		if endpoint.Scheme != "http" || (host != "localhost" && net.ParseIP(host) == nil) {
			return nil, errors.New("TLS required outside loopback")
		}
		if host != "localhost" && !net.ParseIP(host).IsLoopback() {
			return nil, errors.New("TLS required outside loopback")
		}
	}
	return endpoint, nil
}

// NormalizeServerAddress accepts server, memory-view, or /mcp links and
// returns the canonical /mcp URL and server origin. It rejects URL
// credentials, parameters, and fragments.
func NormalizeServerAddress(address string) (string, string, error) {
	endpoint, err := (Config{URL: address}).endpoint()
	if err != nil {
		return "", "", err
	}
	if endpoint.RawPath != "" || (endpoint.Path != "" && endpoint.Path != "/" && endpoint.Path != "/mcp" && endpoint.Path != "/visualizer" && endpoint.Path != "/visualizer/") {
		return "", "", errors.New("use the server link, memory-view link, or /mcp link")
	}
	base := endpoint.Scheme + "://" + endpoint.Host
	return base + "/mcp", base, nil
}
