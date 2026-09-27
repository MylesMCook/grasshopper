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
	"syscall"
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
	var jsonPhase map[string]string
	defer func() {
		if !*jsonOutput {
			return
		}
		if jsonPhase != nil {
			_ = json.NewEncoder(os.Stdout).Encode(jsonPhase)
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
	pending, pendingErr := loadPendingConnection(configPath)
	if pendingErr != nil && !errors.Is(pendingErr, os.ErrNotExist) {
		return connectProblem("conflicting_configuration", "The private pending connection needs inspection before continuing.")
	}
	if pending != nil {
		currentHash, err := connectionConfigHash(configPath)
		if err != nil || currentHash != pending.ConfigHash {
			return connectProblem("conflicting_configuration", "Grasshopper configuration changed during approval. Inspect it before continuing.")
		}
		if !*jsonOutput {
			return connectProblem("approval_pending", "Finish the pending connection with connect --json after owner approval.")
		}
		if *address != "" {
			wanted, _, err := goclient.NormalizeServerAddress(*address)
			if err != nil {
				return err
			}
			if wanted != pending.Address {
				return connectProblem("conflicting_configuration", "Another server already has a pending connection. Finish or let it expire before switching.")
			}
		}
		*address, *tokenPath, *device = pending.Address, pending.TokenPath, pending.Device
		*agents, *cursorCLI, *cursorDir, *update = pending.Agents, pending.CursorCLI, pending.CursorDir, pending.Update
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
	if configErr == nil && pending == nil {
		if existing.URL != *address && !*switchServer {
			return connectProblem("conflicting_configuration", "Grasshopper already uses another server. Confirm the change with --switch-server and its link.")
		}
		if existing.URL == *address {
			state, next := connectionStatus(configPath)
			if state == "unreachable_server" || state == "network_permission_required" || state == "conflicting_configuration" {
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
	} else if *reconnect && pending == nil {
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
		if configErr == nil && pending == nil && *tokenPath != existing.TokenFile && !*reconnect && !*switchServer {
			return connectProblem("conflicting_configuration", "A replacement credential already exists; inspect it before continuing.")
		}
		if err := setupClientWithRoot(setupArgs, root, run); err != nil {
			return fmt.Errorf("saved device credential could not be used; if the server rejected it, run connect --reconnect: %w", err)
		}
		if pending != nil {
			_ = os.Remove(pendingPath(configPath))
		}
		if !*jsonOutput {
			fmt.Fprintln(os.Stdout, "Grasshopper connected with this device's existing credential.")
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	client := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	secret := ""
	requestID := ""
	approvalURL := ""
	code := ""
	ctx, cancel := context.WithTimeout(parent, 5*time.Minute)
	defer cancel()
	if pending != nil {
		secret, requestID, approvalURL, code = pending.Secret, pending.RequestID, pending.ApprovalURL, pending.Code
	} else {
		secretBytes := make([]byte, 32)
		if _, err := rand.Read(secretBytes); err != nil {
			return err
		}
		secret = base64.RawURLEncoding.EncodeToString(secretBytes)
		// Pair with the token hash; keep the credential local until approval.
		hash := sha256.Sum256([]byte(secret))
		var started struct {
			RequestID string `json:"request_id"`
			Code      string `json:"code"`
			ExpiresIn int    `json:"expires_in"`
		}
		status, err := pairingRequest(ctx, client, base+"/pair/start", map[string]string{"device": *device, "token_hash": hex.EncodeToString(hash[:])}, &started)
		if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
			return connectProblem("network_permission_required", "Allow Grasshopper to reach this private server through your agent's normal network permission, then retry once. No credential was saved.")
		}
		if status == http.StatusNotFound || status == http.StatusUnauthorized {
			return errors.New("this server needs a Grasshopper version with viewer pairing; no credential was saved")
		}
		if err != nil || status != http.StatusCreated {
			return connectProblem("unreachable_server", "The private server cannot be reached. No credential was saved; try again when it is online.")
		}
		if len(started.RequestID) != 64 || len(started.Code) != 8 || started.ExpiresIn < 1 || started.ExpiresIn > 300 {
			return errors.New("server returned an invalid pairing request")
		}
		requestID, code = started.RequestID, started.Code
		approvalURL = base + "/visualizer/#connect=" + requestID
		if *jsonOutput {
			configHash, err := connectionConfigHash(configPath)
			if err != nil {
				return err
			}
			state := pendingConnection{Address: *address, TokenPath: *tokenPath, Device: *device, Agents: *agents, CursorCLI: *cursorCLI, CursorDir: *cursorDir, Update: *update, Secret: secret, RequestID: requestID, Code: code, ApprovalURL: approvalURL, ExpiresAt: time.Now().Add(time.Duration(started.ExpiresIn) * time.Second), ConfigHash: configHash}
			if err := savePendingConnection(configPath, state); err != nil {
				return err
			}
			jsonPhase = pendingStatus(state)
			return nil
		}
		fmt.Fprintf(os.Stdout, "Open %s in a browser already connected to this server. Approve %s with code %s. Waiting up to five minutes.\n", approvalURL, *device, code)
	}
	for {
		var result struct {
			Status string `json:"status"`
		}
		status, err := pairingRequest(ctx, client, base+"/pair/poll", map[string]string{"request_id": requestID}, &result)
		if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
			return connectProblem("network_permission_required", "Allow Grasshopper to reach this private server through your agent's normal network permission, then retry once. Pending approval was kept.")
		}
		if err != nil {
			return connectProblem("unreachable_server", "The pairing service became unavailable. Pending approval was kept; retry when it is online.")
		}
		if status == http.StatusOK && result.Status == "approved" {
			break
		}
		if status != http.StatusAccepted || result.Status != "pending" {
			if pending != nil {
				_ = os.Remove(pendingPath(configPath))
			}
			if status == http.StatusForbidden {
				return connectProblem("approval_denied", "Access was denied. Ask the owner before trying again.")
			}
			return connectProblem("approval_expired", "The five-minute request expired. Run connect again for a new approval link.")
		}
		if pending != nil {
			jsonPhase = pendingStatus(*pending)
			return nil
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
	// Keep an approved credential if setup fails so a later attempt can
	// finish locally without another pairing approval.
	if err := setupClientWithRoot(setupArgs, root, run); err != nil {
		return fmt.Errorf("device approved but setup needs attention; credential retained at %s: %w", *tokenPath, err)
	}
	if pending != nil {
		_ = os.Remove(pendingPath(configPath))
	}
	return nil
}

func pendingStatus(pending pendingConnection) map[string]string {
	return map[string]string{"status": "approval_pending", "approval_url": pending.ApprovalURL, "device": pending.Device, "code": pending.Code, "next_step": "Open the link in an already connected memory view and approve the matching code within five minutes. Then run connect --json again to finish."}
}

type connectStatusError struct{ status, next string }

// Error returns the next action for this connection state.
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
