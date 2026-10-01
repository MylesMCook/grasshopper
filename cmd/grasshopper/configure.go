package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/MylesMCook/grasshopper/internal/client"
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
	jsonOutput := flags.Bool("json", false, "report connection status as JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected check arguments")
	}
	path, err := client.ConfigPath(*configArg)
	if err != nil {
		return err
	}
	result := connectionReport(path)
	// The connect skill polls check --json during approval, so JSON stays a
	// fast connection-only report without agent CLI calls.
	if *jsonOutput {
		return json.NewEncoder(os.Stdout).Encode(result)
	}
	return renderCheck(os.Stdout, result, checkAgents(result["host_version"]))
}

// checkAgents inspects this machine's agents. Tests replace it so check never
// runs the developer's real agent CLIs or reads their Cursor settings.
var checkAgents = func(serverVersion string) []agentReport {
	cursorDir := ""
	if home, err := os.UserHomeDir(); err == nil {
		cursorDir = filepath.Join(home, ".cursor")
	}
	return inspectAgents(runAgentCommand, cursorDir, serverVersion)
}

func connectionStatus(path string) (string, string) {
	report := connectionReport(path)
	return report["status"], report["next_step"]
}

func connectionReport(path string) map[string]string {
	result := map[string]string{}
	report := func(state, next string) map[string]string {
		result["status"], result["next_step"] = state, next
		return result
	}
	if pending, err := loadPendingConnection(path); err == nil {
		currentHash, hashErr := connectionConfigHash(path)
		if hashErr != nil || currentHash != pending.ConfigHash {
			return report("conflicting_configuration", "Grasshopper configuration changed during approval. Inspect it before continuing.")
		}
		result["server"], result["device"] = pending.Address, pending.Device
		_, base, err := client.NormalizeServerAddress(pending.Address)
		if err != nil {
			return report("conflicting_configuration", "Inspect the private pending connection before continuing.")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		client := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		var polled struct {
			Status string `json:"status"`
		}
		status, err := pairingRequest(ctx, client, base+"/pair/poll", map[string]string{"request_id": pending.RequestID}, &polled)
		if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
			return report("network_permission_required", "Allow access through the agent's normal network permission, then retry once. Pending approval was kept.")
		}
		if err != nil {
			return report("unreachable_server", "The private server is unavailable. Pending approval was kept; try again when it is online.")
		}
		switch {
		case status == 200 && polled.Status == "approved":
			return report("approval_ready", "Approval succeeded. Run connect --json to finish using this approval.")
		case status == 202 && polled.Status == "pending":
			result["approval_url"], result["code"] = pending.ApprovalURL, pending.Code
			return report("approval_pending", approvalPendingNextStep)
		case status == 403:
			return report("approval_denied", "Access was denied. Ask the owner before trying again.")
		case status == 404 || status == 410:
			return report("approval_expired", "The approval request expired. Run connect again for a new approval link.")
		default:
			return report("unreachable_server", "The pairing service is unavailable. Pending approval was kept; try again when it is online.")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return report("conflicting_configuration", "Inspect the private pending connection before continuing.")
	}
	config, err := client.LoadConfig(path)
	if errors.Is(err, os.ErrNotExist) {
		return report("missing_address", "Connect Grasshopper with your private server link.")
	}
	if err != nil {
		return report("conflicting_configuration", "Inspect the existing Grasshopper client configuration before changing it.")
	}
	result["server"], result["device"] = config.URL, config.Device
	if info, err := os.Stat(config.PolicyPath); err != nil || !info.Mode().IsRegular() {
		return report("conflicting_configuration", "The shared AGENTS.md is missing; reinstall the plugin without replacing your credential.")
	}
	remote, err := client.NewRemote(config)
	if err != nil {
		return report("conflicting_configuration", "The saved Grasshopper credential or address needs inspection.")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	identity, err := remote.Identity(ctx)
	if errors.Is(err, client.ErrIdentityUnsupported) {
		platform := client.Platform()
		_, err = remote.Context(ctx, client.Scope{Device: &config.Device, Platform: &platform}, 512)
		if err == nil {
			result["registration"] = "unverified"
			return report("connected", "Memory access works; this older host cannot verify registration. Upgrade the host to verify device identity.")
		}
	}
	if err != nil {
		if errors.Is(err, client.ErrAuthenticationRejected) {
			return report("authentication_rejected", "This device's access was rejected. Run connect --reconnect to request approval for a replacement.")
		}
		if errors.Is(err, client.ErrNetworkRestricted) {
			return report("network_permission_required", "Allow Grasshopper to reach this private server through your agent's normal network permission, then retry once.")
		}
		return report("unreachable_server", "The private server is unavailable. Keep this connection and try again when it is online.")
	}
	result["credential_role"], result["host_version"], result["registered_device"] = identity.Role, identity.Version, identity.Device
	if identity.Role == "owner" {
		result["registration"] = "unregistered"
		return report("owner_credential", "This machine uses the server owner's access. Run connect --reconnect to request its own device approval; current access is kept until setup succeeds.")
	}
	result["registration"] = "verified"
	if identity.Device != config.Device {
		return report("device_mismatch", "The host's registered device differs from this configuration. Inspect the device identity before changing it; saved access was kept.")
	}
	return report("connected", connectedNextStep)
}

// privateFile writes beside the target and renames after closing, so a
// failed write cannot leave a partial configuration at the target path.
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
	configPath, err := client.ConfigPath(*configArg)
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
	config := client.Config{URL: *url, TokenFile: *tokenFile, PolicyPath: filepath.Join(filepath.Dir(configPath), "AGENTS.md"), Device: *device}
	if _, err := client.NewRemote(config); err != nil {
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
	unchanged := true
	for _, path := range []string{installedPolicy, configPath} {
		if existing, err := os.ReadFile(path); err == nil {
			wanted := policy
			if path == configPath {
				wanted = encoded
			}
			if !bytes.Equal(existing, wanted) && !*update {
				return fmt.Errorf("%s already differs; inspect it before using --update", path)
			}
			if !bytes.Equal(existing, wanted) {
				unchanged = false
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		} else {
			unchanged = false
		}
	}
	if unchanged {
		return nil
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
