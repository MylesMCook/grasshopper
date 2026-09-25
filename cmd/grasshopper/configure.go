package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/MylesMCook/grasshopper/internal/goclient"
)

func bundledPolicy() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(executable)
	for range 7 {
		for _, relative := range []string{"policy/AGENTS.md", "integrations/policy/AGENTS.md"} {
			path := filepath.Join(dir, relative)
			if info, err := os.Lstat(path); err == nil && info.Mode().IsRegular() {
				return path, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", errors.New("bundled policy/AGENTS.md not found; use --policy-file")
}

func checkConnection(args []string) error {
	flags := flag.NewFlagSet("check", flag.ContinueOnError)
	configArg := flags.String("config", "", "client configuration path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected check arguments")
	}
	path, err := goclient.ConfigPath(*configArg)
	if err != nil {
		return err
	}
	config, err := goclient.LoadConfig(path)
	if err != nil {
		return err
	}
	if info, err := os.Stat(config.PolicyPath); err != nil || !info.Mode().IsRegular() {
		return errors.New("shared AGENTS.md unavailable")
	}
	remote, err := goclient.NewRemote(config)
	if err != nil {
		return err
	}
	platform := goclient.Platform()
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	if _, err := remote.Context(ctx, goclient.Scope{Device: &config.Device, Platform: &platform}, 512); err != nil {
		return errors.New("backend unavailable or authentication rejected; no memory was changed")
	}
	fmt.Fprintln(os.Stdout, "Authenticated Grasshopper context received. No memory was changed.")
	return nil
}

func privateFile(path string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".grasshopper-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

func configure(args []string) error {
	return configureWithReport(args, true)
}

func configureWithReport(args []string, report bool) error {
	flags := flag.NewFlagSet("configure", flag.ContinueOnError)
	url := flags.String("url", "", "private Grasshopper /mcp URL")
	tokenFile := flags.String("token-file", "", "absolute path to an existing private token file")
	device := flags.String("device", "", "stable device ID")
	policySource := flags.String("policy-file", "", "canonical AGENTS.md in the extracted package")
	configArg := flags.String("config", "", "client configuration path")
	update := flags.Bool("update", false, "replace an existing Grasshopper client configuration")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *url == "" || *tokenFile == "" || *device == "" {
		return errors.New("configure needs --url, --token-file, and --device")
	}
	configPath, err := goclient.ConfigPath(*configArg)
	if err != nil {
		return err
	}
	if !filepath.IsAbs(configPath) || !filepath.IsAbs(*tokenFile) {
		return errors.New("config and token file paths must be absolute")
	}
	if *policySource == "" {
		*policySource, err = bundledPolicy()
		if err != nil {
			return err
		}
	}
	if filepath.Base(*policySource) != "AGENTS.md" {
		return errors.New("policy file must be named AGENTS.md")
	}
	info, err := os.Lstat(*policySource)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 16384 {
		return errors.New("canonical AGENTS.md unavailable or too large")
	}
	policy, err := os.ReadFile(*policySource)
	if err != nil {
		return err
	}
	config := goclient.Config{URL: *url, TokenFile: *tokenFile, PolicyPath: filepath.Join(filepath.Dir(configPath), "AGENTS.md"), Device: *device}
	if _, err := goclient.NewRemote(config); err != nil {
		return fmt.Errorf("client connection invalid: %w", err)
	}
	encoded, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	installedPolicy := filepath.Join(dir, "AGENTS.md")
	for _, path := range []string{installedPolicy, configPath} {
		if existing, err := os.ReadFile(path); err == nil {
			wanted := policy
			if path == configPath {
				wanted = encoded
			}
			if !bytes.Equal(existing, wanted) && !*update {
				return fmt.Errorf("%s already differs; inspect it before using --update", path)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	previousPolicy, err := snapshotFile(installedPolicy)
	if err != nil {
		return err
	}
	if err := privateFile(installedPolicy, policy); err != nil {
		return err
	}
	if err := privateFile(configPath, encoded); err != nil {
		return errors.Join(err, previousPolicy.restore())
	}
	if report {
		fmt.Fprintf(os.Stdout, "Client configured: %s\nShared policy: %s\nToken remains in its existing file.\n", configPath, installedPolicy)
	}
	return nil
}
