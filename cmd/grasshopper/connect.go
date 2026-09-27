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
	"strings"
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

func connectWithRoot(parent context.Context, args []string, root string, run commandRunner) (resultErr error) {
	flags := flag.NewFlagSet("connect", flag.ContinueOnError)
	address := flags.String("url", "", "private Grasshopper server, memory-view, or /mcp link")
	tokenPath := flags.String("token-file", "", "private path for this device's new token")
	configArg := flags.String("config", "", "client configuration path")
	device := flags.String("device", "", "device name shown when approving")
	agents := flags.String("agents", "none", "agents to install; none for a marketplace plugin")
	cursorCLI := flags.Bool("cursor-cli", false, "also configure Cursor Agent CLI MCP")
	cursorDir := flags.String("cursor-dir", "", "Cursor user configuration directory")
	update := flags.Bool("update", false, "replace existing Grasshopper wiring after approval")
	reconnect := flags.Bool("reconnect", false, "request a replacement credential after access is rejected")
	switchServer := flags.Bool("switch-server", false, "confirm switching this device to another server")
	jsonOutput := flags.Bool("json", false, "report connection phases as JSON")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected connect arguments")
	}
	defer func() {
		if !*jsonOutput {
			return
		}
		state, next := "connected", "Open a fresh agent session after native hook or MCP approval."
		if resultErr != nil {
			state, next = connectErrorStatus(resultErr)
		}
		_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"status": state, "next_step": next})
	}()
	configPath, err := goclient.ConfigPath(*configArg)
	if err != nil {
		return err
	}
	existing, configErr := goclient.LoadConfig(configPath)
	if configErr != nil && !errors.Is(configErr, os.ErrNotExist) {
		return connectProblem("conflicting_configuration", "The saved Grasshopper configuration needs inspection before reconnecting.")
	}
	if *address == "" && configErr == nil {
		*address = existing.URL
	}
	if *address == "" {
		return connectProblem("missing_address", "Send your private Grasshopper server link once. No token is needed.")
	}
	if configErr == nil && *tokenPath == "" {
		*tokenPath = existing.TokenFile
	}
	mcpURL, base, err := goclient.NormalizeServerAddress(*address)
	if err != nil {
		return err
	}
	*address = mcpURL
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
	if configErr == nil {
		if existing.URL != *address && !*switchServer {
			return connectProblem("conflicting_configuration", "Grasshopper already uses another server. Confirm the change with --switch-server and its link.")
		}
		if existing.URL == *address {
			state, next := connectionStatus(configPath)
			if state == "unreachable_server" || state == "conflicting_configuration" {
				return connectProblem(state, next)
			}
			if state == "authentication_rejected" && !*reconnect {
				return connectProblem(state, next)
			}
			if state == "connected" {
				if *agents == "none" && !*cursorCLI && !*update {
					if !*jsonOutput {
						fmt.Fprintln(os.Stdout, "Grasshopper is already connected on this machine.")
					}
					return nil
				}
				setupArgs := []string{"--url", *address, "--token-file", existing.TokenFile, "--device", existing.Device, "--agents", *agents, "--config", configPath}
				if *jsonOutput {
					setupArgs = append(setupArgs, "--quiet")
				}
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
		}
		if existing.URL != *address && !*switchServer {
			return connectProblem("conflicting_configuration", "Switching servers needs --switch-server.")
		}
		if *tokenPath == existing.TokenFile {
			*tokenPath += ".replacement"
		}
		*update = true
	} else if *reconnect {
		if _, err := os.Stat(*tokenPath); err == nil {
			*tokenPath += ".replacement"
		}
	}
	setupArgs := []string{"--url", *address, "--token-file", *tokenPath, "--device", *device, "--agents", *agents, "--config", configPath}
	if *jsonOutput {
		setupArgs = append(setupArgs, "--quiet")
	}
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
		if configErr == nil && *tokenPath != existing.TokenFile && !*reconnect && !*switchServer {
			return connectProblem("conflicting_configuration", "A replacement credential already exists; inspect it before continuing.")
		}
		if err := setupClientWithRoot(setupArgs, root, run); err != nil {
			return fmt.Errorf("saved device credential could not be used; if the server rejected it, run connect --reconnect: %w", err)
		}
		if !*jsonOutput {
			fmt.Fprintln(os.Stdout, "Grasshopper connected with this device's existing credential.")
		}
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
		return connectProblem("unreachable_server", "The private server cannot be reached. No credential was saved; try again when it is online.")
	}
	if len(started.RequestID) != 64 || len(started.Code) != 8 || started.ExpiresIn < 1 || started.ExpiresIn > 300 {
		return errors.New("server returned an invalid pairing request")
	}
	approvalURL := base + "/visualizer/#connect=" + started.RequestID
	if *jsonOutput {
		_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"status": "approval_pending", "approval_url": approvalURL, "device": *device, "code": started.Code, "next_step": "Open the link in an already connected memory view and approve the matching code within five minutes."})
	} else {
		fmt.Fprintf(os.Stdout, "Open %s in a browser already connected to this server. Approve %s with code %s. Waiting up to five minutes.\n", approvalURL, *device, started.Code)
	}
	for {
		var result struct {
			Status string `json:"status"`
		}
		status, err := pairingRequest(ctx, client, base+"/pair/poll", map[string]string{"request_id": started.RequestID}, &result)
		if err != nil {
			return connectProblem("unreachable_server", "The pairing service became unavailable. No credential was saved; retry when it is online.")
		}
		if status == http.StatusOK && result.Status == "approved" {
			break
		}
		if status != http.StatusAccepted || result.Status != "pending" {
			if status == http.StatusForbidden {
				return connectProblem("approval_denied", "Access was denied. Ask the owner before trying again.")
			}
			return connectProblem("approval_expired", "The five-minute request expired. Run connect again for a new approval link.")
		}
		select {
		case <-ctx.Done():
			return connectProblem("approval_expired", "The five-minute request expired. Run connect again for a new approval link.")
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

type connectStatusError struct{ status, next string }

func (e connectStatusError) Error() string     { return e.next }
func connectProblem(status, next string) error { return connectStatusError{status, next} }
func connectErrorStatus(err error) (string, string) {
	var problem connectStatusError
	if errors.As(err, &problem) {
		return problem.status, problem.next
	}
	if strings.Contains(err.Error(), "setup needs attention") {
		return "conflicting_configuration", "Approval succeeded, but agent wiring needs attention. Existing configuration was retained."
	}
	return "conflicting_configuration", err.Error()
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
