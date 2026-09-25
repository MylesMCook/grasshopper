package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/MylesMCook/grasshopper/internal/goclient"
)

type commandRunner func(string, ...string) ([]byte, error)

func runAgentCommand(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, name, args...)
	output, err := command.CombinedOutput()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("%s timed out", name)
	}
	if err != nil {
		message := strings.TrimSpace(string(output))
		if len(message) > 1000 {
			message = message[:1000] + "…"
		}
		return nil, fmt.Errorf("%s %s failed: %s: %w", name, strings.Join(args, " "), message, err)
	}
	return output, nil
}

func clientPackageRoot(executable string) (string, error) {
	dir := filepath.Dir(executable)
	for range 7 {
		if info, err := os.Stat(filepath.Join(dir, "policy", "AGENTS.md")); err == nil && info.Mode().IsRegular() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", errors.New("client package not found; run setup from the extracted client archive")
}

func setupClient(args []string) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	root, err := clientPackageRoot(executable)
	if err != nil {
		return err
	}
	return setupClientWithRoot(args, root, runAgentCommand)
}

type agentInstall struct {
	name, marketplace, plugin    string
	installed, marketplaceExists bool
}

func pluginVerb(name string) string {
	if name == "claude" {
		return "install"
	}
	return "add"
}

func removePluginVerb(name string) string {
	if name == "claude" {
		return "uninstall"
	}
	return "remove"
}

func restoreAgent(state agentInstall, run commandRunner, newMarketplace, newPlugin bool) error {
	var failures []error
	if newPlugin {
		if _, err := run(state.name, "plugin", removePluginVerb(state.name), state.plugin); err != nil {
			failures = append(failures, err)
		}
	}
	if newMarketplace {
		if _, err := run(state.name, "plugin", "marketplace", "remove", "grasshopper-local"); err != nil {
			failures = append(failures, err)
		}
	}
	if state.marketplaceExists {
		if _, err := run(state.name, "plugin", "marketplace", "add", state.marketplace); err != nil {
			failures = append(failures, err)
		}
		if state.installed {
			if _, err := run(state.name, "plugin", pluginVerb(state.name), state.plugin); err != nil {
				failures = append(failures, err)
			}
		}
	}
	return errors.Join(failures...)
}

func changeAgent(state agentInstall, root string, update bool, run commandRunner) (func() error, error) {
	wanted := filepath.Join(root, state.name)
	if state.installed && !update {
		return nil, nil
	}
	changed := false
	if update && state.marketplaceExists {
		if state.installed {
			if _, err := run(state.name, "plugin", removePluginVerb(state.name), state.plugin); err != nil {
				return nil, err
			}
			changed = true
		}
		if _, err := run(state.name, "plugin", "marketplace", "remove", "grasshopper-local"); err != nil {
			if changed {
				return nil, errors.Join(err, restoreAgent(state, run, false, false))
			}
			return nil, err
		}
		changed = true
	}
	if !state.marketplaceExists || update {
		if _, err := run(state.name, "plugin", "marketplace", "add", wanted); err != nil {
			if changed {
				return nil, errors.Join(err, restoreAgent(state, run, false, false))
			}
			return nil, err
		}
		changed = true
	}
	if _, err := run(state.name, "plugin", pluginVerb(state.name), state.plugin); err != nil {
		if changed {
			return nil, errors.Join(err, restoreAgent(state, run, true, false))
		}
		return nil, err
	}
	return func() error {
		if state.marketplaceExists && !update {
			_, err := run(state.name, "plugin", removePluginVerb(state.name), state.plugin)
			return err
		}
		return restoreAgent(state, run, true, true)
	}, nil
}

type savedFile struct {
	path   string
	data   []byte
	exists bool
}

func snapshotFile(path string) (savedFile, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return savedFile{path: path}, nil
	}
	if err != nil {
		return savedFile{}, err
	}
	return savedFile{path: path, data: data, exists: true}, nil
}

func (saved savedFile) restore() error {
	if saved.exists {
		return privateFile(saved.path, saved.data)
	}
	if err := os.Remove(saved.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func jsonCommand(run commandRunner, name string, args ...string) (map[string]any, error) {
	output, err := run(name, args...)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("%s returned invalid JSON: %w", name, err)
	}
	return result, nil
}

func codexInstall(root string, run commandRunner) (agentInstall, error) {
	wanted := filepath.Join(root, "codex")
	marketplaces, err := jsonCommand(run, "codex", "plugin", "marketplace", "list", "--json")
	if err != nil {
		return agentInstall{}, err
	}
	plugins, err := jsonCommand(run, "codex", "plugin", "list", "--json")
	if err != nil {
		return agentInstall{}, err
	}
	state := agentInstall{name: "codex", marketplace: wanted, plugin: "grasshopper@grasshopper-local"}
	for _, item := range arrayObjects(marketplaces["marketplaces"]) {
		if item["name"] == "grasshopper-local" {
			state.marketplace, _ = item["root"].(string)
			state.marketplaceExists = true
			break
		}
	}
	for _, item := range arrayObjects(plugins["installed"]) {
		if item["pluginId"] == state.plugin {
			state.installed = true
			break
		}
	}
	return state, nil
}

func claudeInstall(root string, run commandRunner) (agentInstall, error) {
	marketplacesJSON, err := run("claude", "plugin", "marketplace", "list", "--json")
	if err != nil {
		return agentInstall{}, err
	}
	pluginsJSON, err := run("claude", "plugin", "list", "--json")
	if err != nil {
		return agentInstall{}, err
	}
	var marketplaces, plugins []map[string]any
	if err := json.Unmarshal(marketplacesJSON, &marketplaces); err != nil {
		return agentInstall{}, fmt.Errorf("Claude returned invalid marketplace list: %w", err)
	}
	if err := json.Unmarshal(pluginsJSON, &plugins); err != nil {
		return agentInstall{}, fmt.Errorf("Claude returned invalid plugin list: %w", err)
	}
	state := agentInstall{name: "claude", marketplace: filepath.Join(root, "claude"), plugin: "grasshopper@grasshopper-local"}
	for _, item := range marketplaces {
		if item["name"] == "grasshopper-local" {
			state.marketplace, _ = item["path"].(string)
			state.marketplaceExists = true
			break
		}
	}
	for _, item := range plugins {
		if item["id"] == state.plugin {
			state.installed = true
			break
		}
	}
	return state, nil
}

func arrayObjects(value any) []map[string]any {
	items, _ := value.([]any)
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if object, ok := item.(map[string]any); ok {
			result = append(result, object)
		}
	}
	return result
}

func setupClientWithRoot(args []string, root string, run commandRunner) (resultErr error) {
	flags := flag.NewFlagSet("setup", flag.ContinueOnError)
	url := flags.String("url", "http://127.0.0.1:8106/mcp", "private Grasshopper /mcp URL")
	tokenFile := flags.String("token-file", "", "absolute path to an existing private token file (defaults to local quickstart token)")
	device := flags.String("device", "", "stable device ID (defaults to this machine's hostname)")
	agents := flags.String("agents", "codex,cursor", "agents to connect: codex,cursor,claude, or none for a marketplace plugin")
	configArg := flags.String("config", "", "client configuration path")
	cursorDir := flags.String("cursor-dir", "", "Cursor user .cursor directory")
	cursorCLI := flags.Bool("cursor-cli", false, "configure Cursor Agent CLI MCP only; the marketplace plugin supplies its hook")
	update := flags.Bool("update", false, "replace this machine's existing Grasshopper client wiring")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected setup arguments")
	}
	if *cursorCLI && *agents != "none" {
		return errors.New("--cursor-cli requires --agents none")
	}
	if *tokenFile == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return err
		}
		*tokenFile = filepath.Join(base, "Grasshopper", "access-token")
	}
	if *device == "" {
		name, err := os.Hostname()
		if err != nil {
			return err
		}
		*device = name
	}
	selected := map[string]bool{}
	for _, name := range strings.Split(*agents, ",") {
		if name == "none" && *agents == "none" {
			break
		}
		if name != "codex" && name != "cursor" && name != "claude" {
			return fmt.Errorf("unknown agent %q; choose codex,cursor,claude, or none", name)
		}
		selected[name] = true
	}
	configPath, err := goclient.ConfigPath(*configArg)
	if err != nil {
		return err
	}
	if !filepath.IsAbs(configPath) || !filepath.IsAbs(*tokenFile) {
		return errors.New("config and token paths must be absolute")
	}
	policy := filepath.Join(root, "policy", "AGENTS.md")
	if info, err := os.Stat(policy); err != nil || !info.Mode().IsRegular() {
		return errors.New("client package policy/AGENTS.md is missing")
	}
	config := goclient.Config{URL: *url, TokenFile: *tokenFile, PolicyPath: filepath.Join(filepath.Dir(configPath), "AGENTS.md"), Device: *device}
	remote, err := goclient.NewRemote(config)
	if err != nil {
		return err
	}
	platform := goclient.Platform()
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	if _, err := remote.Context(ctx, goclient.Scope{Device: device, Platform: &platform}, 512); err != nil {
		return errors.New("server or token check failed; no agent settings were changed")
	}
	var native []agentInstall
	if selected["codex"] {
		if _, err := os.Stat(filepath.Join(root, "codex", ".agents", "plugins", "marketplace.json")); err != nil {
			return errors.New("Codex plugin is missing from the client package")
		}
		state, err := codexInstall(root, run)
		if err != nil {
			return err
		}
		native = append(native, state)
	}
	if selected["claude"] {
		if _, err := os.Stat(filepath.Join(root, "claude", ".claude-plugin", "marketplace.json")); err != nil {
			return errors.New("Claude plugin is missing from the client package")
		}
		state, err := claudeInstall(root, run)
		if err != nil {
			return err
		}
		native = append(native, state)
	}
	for _, state := range native {
		wanted := filepath.Join(root, state.name)
		if state.marketplace != wanted && !*update {
			return fmt.Errorf("%s already uses another Grasshopper package; rerun with --update", state.name)
		}
		if state.installed && !state.marketplaceExists {
			return fmt.Errorf("%s Grasshopper plugin has no local marketplace; inspect the installation", state.name)
		}
		if state.marketplaceExists && state.marketplace != wanted {
			manifest := filepath.Join(state.marketplace, ".agents", "plugins", "marketplace.json")
			if state.name == "claude" {
				manifest = filepath.Join(state.marketplace, ".claude-plugin", "marketplace.json")
			}
			if info, err := os.Stat(manifest); err != nil || !info.Mode().IsRegular() {
				return fmt.Errorf("%s existing marketplace is not a local Grasshopper package; inspect it before updating", state.name)
			}
		}
	}
	if *cursorDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		*cursorDir = filepath.Join(home, ".cursor")
	}
	if (selected["cursor"] || *cursorCLI) && !filepath.IsAbs(*cursorDir) {
		return errors.New("Cursor directory must be absolute")
	}
	if *cursorCLI {
		binary := filepath.Join(root, "bin", "grasshopper")
		if os.PathSeparator == '\\' {
			binary += ".exe"
		}
		if info, err := os.Stat(binary); err != nil || !info.Mode().IsRegular() {
			return errors.New("marketplace client binary is missing")
		}
		if err := cursorMCPOnly(*cursorDir, configPath, binary, *update, true); err != nil {
			return err
		}
	}
	if selected["cursor"] {
		binary := filepath.Join(root, "cursor", "plugins", "grasshopper", "bin", "grasshopper")
		if os.PathSeparator == '\\' {
			binary += ".exe"
		}
		if info, err := os.Stat(binary); err != nil || !info.Mode().IsRegular() {
			return errors.New("Cursor client binary is missing from the package")
		}
		if err := cursorWiringWithOptions(*cursorDir, configPath, binary, true, *update, true, false); err != nil {
			return err
		}
	}
	if *update {
		previous, err := os.Lstat(configPath)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err == nil && !previous.Mode().IsRegular() {
			return errors.New("existing Grasshopper config is not a regular file")
		}
		if errors.Is(err, os.ErrNotExist) {
			for _, state := range native {
				if state.marketplaceExists || state.installed {
					return errors.New("an agent has Grasshopper but the shared client config is missing; inspect it before updating")
				}
			}
			if selected["cursor"] {
				mcp, err := readJSONObject(filepath.Join(*cursorDir, "mcp.json"))
				if err != nil {
					return err
				}
				if servers, ok := jsonObject(mcp["mcpServers"]); ok && servers["grasshopper"] != nil {
					return errors.New("Cursor has Grasshopper but the shared client config is missing; inspect it before updating")
				}
			}
		}
	}
	paths := []string{configPath, filepath.Join(filepath.Dir(configPath), "AGENTS.md")}
	if selected["cursor"] {
		paths = append(paths, filepath.Join(*cursorDir, "mcp.json"), filepath.Join(*cursorDir, "hooks.json"))
	} else if *cursorCLI {
		paths = append(paths, filepath.Join(*cursorDir, "mcp.json"))
	}
	var saved []savedFile
	for _, path := range paths {
		entry, err := snapshotFile(path)
		if err != nil {
			return err
		}
		saved = append(saved, entry)
	}
	var rollbacks []func() error
	defer func() {
		if resultErr == nil {
			return
		}
		var failures []error
		for i := len(rollbacks) - 1; i >= 0; i-- {
			if err := rollbacks[i](); err != nil {
				failures = append(failures, err)
			}
		}
		for _, entry := range saved {
			if err := entry.restore(); err != nil {
				failures = append(failures, err)
			}
		}
		if len(failures) > 0 {
			resultErr = errors.Join(resultErr, fmt.Errorf("rollback needs attention: %w", errors.Join(failures...)))
		}
	}()
	configureArgs := []string{"--url", *url, "--token-file", *tokenFile, "--device", *device, "--policy-file", policy, "--config", configPath}
	if *update {
		configureArgs = append(configureArgs, "--update")
	}
	if err := configureWithReport(configureArgs, false); err != nil {
		return err
	}
	for _, state := range native {
		rollback, err := changeAgent(state, root, *update, run)
		if err != nil {
			return err
		}
		if rollback != nil {
			rollbacks = append(rollbacks, rollback)
		}
	}
	if selected["cursor"] {
		binary := filepath.Join(root, "cursor", "plugins", "grasshopper", "bin", "grasshopper")
		if os.PathSeparator == '\\' {
			binary += ".exe"
		}
		if err := cursorWiringWithOptions(*cursorDir, configPath, binary, true, *update, false, false); err != nil {
			return err
		}
	}
	if *cursorCLI {
		binary := filepath.Join(root, "bin", "grasshopper")
		if os.PathSeparator == '\\' {
			binary += ".exe"
		}
		if err := cursorMCPOnly(*cursorDir, configPath, binary, *update, false); err != nil {
			return err
		}
	}
	if *agents == "none" {
		fmt.Fprintln(os.Stdout, "Grasshopper connected. Token stays in its existing file.")
	} else {
		fmt.Fprintf(os.Stdout, "Grasshopper installed for %s. Token stays in its existing file.\n", *agents)
	}
	fmt.Fprintln(os.Stdout, "Start a fresh session. Check a known memory, or save one real preference and read it from another agent.")
	if selected["codex"] {
		fmt.Fprintln(os.Stdout, "Codex: review and trust Grasshopper startup hooks in /hooks.")
	}
	if selected["cursor"] {
		fmt.Fprintln(os.Stdout, "Cursor: enable Grasshopper in Tools & MCPs if prompted.")
	}
	return nil
}
