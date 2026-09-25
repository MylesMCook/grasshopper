package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/MylesMCook/grasshopper/internal/goclient"
)

func connectClient(args []string) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	root, err := clientPackageRoot(executable)
	if err != nil {
		return err
	}
	return connectWithRoot(context.Background(), args, root, runAgentCommand)
}

func connectWithRoot(parent context.Context, args []string, root string, run commandRunner) error {
	flags := flag.NewFlagSet("connect", flag.ContinueOnError)
	address := flags.String("url", "", "private Grasshopper /mcp address")
	tokenPath := flags.String("token-file", "", "private path for this device's new token")
	configArg := flags.String("config", "", "client configuration path")
	device := flags.String("device", "", "device name shown when approving")
	agents := flags.String("agents", "none", "agents to install; none for a marketplace plugin")
	cursorCLI := flags.Bool("cursor-cli", false, "also configure Cursor Agent CLI MCP")
	cursorDir := flags.String("cursor-dir", "", "Cursor user configuration directory")
	update := flags.Bool("update", false, "replace existing Grasshopper wiring after approval")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected connect arguments")
	}
	if *address == "" {
		return errors.New("connect needs --url with your private server address")
	}
	base, err := goclient.PairingBase(*address)
	if err != nil {
		return err
	}
	configPath, err := goclient.ConfigPath(*configArg)
	if err != nil {
		return err
	}
	if *tokenPath == "" {
		configDir, err := os.UserConfigDir()
		if err != nil {
			return err
		}
		*tokenPath = filepath.Join(configDir, "Grasshopper", "access-token")
	}
	if *device == "" {
		*device, err = os.Hostname()
		if err != nil {
			return err
		}
	}
	if !filepath.IsAbs(*tokenPath) || !filepath.IsAbs(configPath) {
		return errors.New("credential and configuration paths must be absolute")
	}
	if info, err := os.Stat(filepath.Join(root, "policy", "AGENTS.md")); err != nil || !info.Mode().IsRegular() {
		return errors.New("client package policy/AGENTS.md is missing")
	}
	if existing, err := goclient.LoadConfig(configPath); err == nil {
		if existing.URL != *address && !*update {
			return errors.New("Grasshopper already points at another server; inspect it before --update")
		}
		if existing.URL == *address && checkConnection([]string{"--config", configPath}) == nil {
			if *agents == "none" && !*cursorCLI && !*update {
				fmt.Fprintln(os.Stdout, "Grasshopper is already connected on this machine.")
				return nil
			}
			setupArgs := []string{"--url", *address, "--token-file", existing.TokenFile, "--device", existing.Device, "--agents", *agents, "--config", configPath}
			if *cursorCLI {
				setupArgs = append(setupArgs, "--cursor-cli")
			}
			if *cursorDir != "" {
				setupArgs = append(setupArgs, "--cursor-dir", *cursorDir)
			}
			if *update {
				setupArgs = append(setupArgs, "--update")
			}
			return setupClientWithRoot(setupArgs, root, run)
		}
		if !*update {
			return errors.New("existing Grasshopper connection is unavailable; inspect it before --update")
		}
	} else if !errors.Is(err, os.ErrNotExist) && !*update {
		return errors.New("existing Grasshopper configuration needs inspection before --update")
	}
	setupArgs := []string{"--url", *address, "--token-file", *tokenPath, "--device", *device, "--agents", *agents, "--config", configPath}
	if *cursorCLI {
		setupArgs = append(setupArgs, "--cursor-cli")
	}
	if *cursorDir != "" {
		setupArgs = append(setupArgs, "--cursor-dir", *cursorDir)
	}
	if *update {
		setupArgs = append(setupArgs, "--update")
	}
	if _, err := os.Stat(*tokenPath); err == nil {
		if err := setupClientWithRoot(setupArgs, root, run); err != nil {
			return fmt.Errorf("device token file already exists but setup could not use it: %w", err)
		}
		fmt.Fprintln(os.Stdout, "Grasshopper connected with this device's existing credential.")
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	secretBytes := make([]byte, 32)
	if _, err := rand.Read(secretBytes); err != nil {
		return err
	}
	secret := base64.RawURLEncoding.EncodeToString(secretBytes)
	hash := sha256.Sum256([]byte(secret))
	ctx, cancel := context.WithTimeout(parent, 5*time.Minute)
	defer cancel()
	client := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	var started struct {
		RequestID string `json:"request_id"`
		Code      string `json:"code"`
		ExpiresIn int    `json:"expires_in"`
	}
	status, err := pairingRequest(ctx, client, base+"/pair/start", map[string]string{"device": *device, "token_hash": hex.EncodeToString(hash[:])}, &started)
	if status == http.StatusNotFound || status == http.StatusUnauthorized {
		return errors.New("this server needs a Grasshopper version with viewer pairing; no credential was saved")
	}
	if err != nil || status != http.StatusCreated {
		return errors.New("could not reach this server's pairing service; no credential was saved")
	}
	if len(started.RequestID) != 64 || len(started.Code) != 8 || started.ExpiresIn < 1 || started.ExpiresIn > 300 {
		return errors.New("server returned an invalid pairing request")
	}
	fmt.Fprintf(os.Stdout, "Open %s/visualizer/ in a browser already connected to this server. Approve %s with code %s. Waiting up to five minutes.\n", base, *device, started.Code)
	for {
		var result struct {
			Status string `json:"status"`
		}
		status, err := pairingRequest(ctx, client, base+"/pair/poll", map[string]string{"request_id": started.RequestID}, &result)
		if err != nil {
			return errors.New("pairing service unavailable; no credential was saved")
		}
		if status == http.StatusOK && result.Status == "approved" {
			break
		}
		if status != http.StatusAccepted || result.Status != "pending" {
			return errors.New("connection denied or expired; no credential was saved")
		}
		select {
		case <-ctx.Done():
			return errors.New("approval timed out; no credential was saved")
		case <-time.After(5 * time.Second):
		}
	}
	if err := os.MkdirAll(filepath.Dir(*tokenPath), 0700); err != nil {
		return err
	}
	file, err := os.OpenFile(*tokenPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, writeErr := file.WriteString(secret)
	syncErr := file.Sync()
	closeErr := file.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		_ = os.Remove(*tokenPath)
		return err
	}
	if err := setupClientWithRoot(setupArgs, root, run); err != nil {
		return fmt.Errorf("device approved but setup needs attention; credential retained at %s: %w", *tokenPath, err)
	}
	return nil
}

func pairingRequest(ctx context.Context, client *http.Client, address string, input any, output any) (int, error) {
	encoded, err := json.Marshal(input)
	if err != nil {
		return 0, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, address, bytes.NewReader(encoded))
	if err != nil {
		return 0, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusCreated || response.StatusCode == http.StatusOK || response.StatusCode == http.StatusAccepted {
		body, err := io.ReadAll(io.LimitReader(response.Body, 2049))
		if err != nil || len(body) > 2048 {
			return response.StatusCode, errors.New("pairing response exceeds limit")
		}
		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(output); err != nil {
			return response.StatusCode, err
		}
		if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
			return response.StatusCode, errors.New("invalid pairing response")
		}
	}
	return response.StatusCode, nil
}
