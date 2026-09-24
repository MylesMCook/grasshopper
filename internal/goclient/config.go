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
	directory, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, "grasshopper", "client.json"), nil
}

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
	if err != nil || endpoint == nil || endpoint.Host == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" {
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
