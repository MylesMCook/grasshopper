package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/MylesMCook/grasshopper/internal/goclient"
)

func cursorCommand(args []string) (resultErr error) {
	if len(args) == 0 || (args[0] != "install" && args[0] != "remove") {
		return errors.New("use cursor install or cursor remove")
	}
	flags := flag.NewFlagSet("cursor "+args[0], flag.ContinueOnError)
	configArg := flags.String("config", "", "client configuration path")
	cursorDir := flags.String("cursor-dir", "", "Cursor user or project .cursor directory")
	update := flags.Bool("update", false, "replace existing Grasshopper Cursor wiring")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected Cursor arguments")
	}
	if *cursorDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		*cursorDir = filepath.Join(home, ".cursor")
	}
	if !filepath.IsAbs(*cursorDir) {
		return errors.New("Cursor directory must be absolute")
	}
	configPath, err := goclient.ConfigPath(*configArg)
	if err != nil {
		return err
	}
	if !filepath.IsAbs(configPath) {
		return errors.New("client configuration path must be absolute")
	}
	if args[0] == "install" {
		config, err := goclient.LoadConfig(configPath)
		if err != nil {
			return fmt.Errorf("configure Grasshopper first: %w", err)
		}
		if _, err := goclient.NewRemote(config); err != nil {
			return err
		}
		if _, err := os.Stat(config.PolicyPath); err != nil {
			return fmt.Errorf("shared AGENTS.md unavailable: %w", err)
		}
	}
	binary, err := os.Executable()
	if err != nil {
		return err
	}
	cliName := "cli.json"
	if home, err := os.UserHomeDir(); err == nil && filepath.Clean(*cursorDir) == filepath.Join(home, ".cursor") {
		cliName = "cli-config.json"
	}
	cliPath := filepath.Join(*cursorDir, cliName)
	install := args[0] == "install"
	if err := cursorWiringWithOptions(*cursorDir, configPath, binary, install, *update, true, false); err != nil {
		return err
	}
	if err := cursorCLIPermissions(cliPath, install, true); err != nil {
		return err
	}
	var saved []savedFile
	for _, path := range []string{filepath.Join(*cursorDir, "mcp.json"), filepath.Join(*cursorDir, "hooks.json"), cliPath} {
		entry, err := snapshotFile(path)
		if err != nil {
			return err
		}
		saved = append(saved, entry)
	}
	defer func() {
		if resultErr == nil {
			return
		}
		var failures []error
		for _, entry := range saved {
			if err := entry.restore(); err != nil {
				failures = append(failures, err)
			}
		}
		if len(failures) > 0 {
			resultErr = errors.Join(resultErr, fmt.Errorf("Cursor rollback needs attention: %w", errors.Join(failures...)))
		}
	}()
	if err := cursorWiringWithOptions(*cursorDir, configPath, binary, install, *update, false, false); err != nil {
		return err
	}
	if err := cursorCLIPermissions(cliPath, install, false); err != nil {
		return err
	}
	if install {
		fmt.Fprintln(os.Stdout, "Cursor MCP, startup hook, and read permissions installed. Run `agent mcp enable grasshopper` once if prompted.")
	} else {
		fmt.Fprintln(os.Stdout, "Cursor Grasshopper MCP, startup hook, and read permissions removed. Shared memory remains on the server.")
	}
	return nil
}

func readJSONObject(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	var object map[string]any
	if err := json.Unmarshal(data, &object); err != nil || object == nil {
		return nil, fmt.Errorf("%s must be plain JSON; inspect it before changing Grasshopper wiring", path)
	}
	return object, nil
}

func jsonObject(value any) (map[string]any, bool) {
	object, ok := value.(map[string]any)
	return object, ok
}

func shellQuoted(value string) string {
	if runtime.GOOS == "windows" {
		return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func isGrasshopperHook(value any) bool {
	entry, ok := jsonObject(value)
	if !ok {
		return false
	}
	command, _ := entry["command"].(string)
	for _, quote := range []string{"'", `"`} {
		if !strings.HasPrefix(command, quote) {
			continue
		}
		end := strings.Index(command[1:], quote)
		if end < 0 {
			continue
		}
		binary := command[1 : end+1]
		if grasshopperExecutable(binary) && strings.HasPrefix(command[end+2:], " hook --config ") && strings.HasSuffix(command, " --harness cursor") {
			return true
		}
	}
	return false
}

func grasshopperExecutable(path string) bool {
	name := strings.ToLower(filepath.Base(path))
	return name == "grasshopper" || name == "grasshopper.exe"
}

func cursorWiring(dir, configPath, binary string, install, update bool) error {
	return cursorWiringWithOptions(dir, configPath, binary, install, update, false, true)
}

func grasshopperMCPEntry(binary, configPath string) map[string]any {
	return map[string]any{"type": "stdio", "command": binary, "args": []string{"bridge", "--config", configPath}}
}

var cursorReadPermissions = []string{
	"Mcp(grasshopper:context)",
	"Mcp(grasshopper:get)",
	"Mcp(grasshopper:search)",
}

// Cursor CLI requires explicit permission for headless reads. Writes stay gated.
func cursorCLIPermissions(path string, install, dryRun bool) error {
	info, statErr := os.Lstat(path)
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return statErr
	}
	if statErr == nil && !info.Mode().IsRegular() {
		return errors.New("Cursor CLI config must be a regular file")
	}
	if !install && errors.Is(statErr, os.ErrNotExist) {
		return nil
	}
	config, err := readJSONObject(path)
	if err != nil {
		return err
	}
	if version := config["version"]; version != nil && version != float64(1) {
		return errors.New("unsupported Cursor CLI config version")
	}
	permissions, ok := jsonObject(config["permissions"])
	if config["permissions"] != nil && !ok {
		return errors.New("Cursor CLI permissions must be an object")
	}
	if !ok {
		permissions = map[string]any{}
	}
	allow, ok := permissions["allow"].([]any)
	if permissions["allow"] != nil && !ok {
		return errors.New("Cursor CLI permissions.allow must be an array")
	}
	for _, entry := range allow {
		if _, ok := entry.(string); !ok {
			return errors.New("Cursor CLI permissions.allow must contain strings")
		}
	}
	kept := make([]any, 0, len(allow)+len(cursorReadPermissions))
	for _, entry := range allow {
		value := entry.(string)
		managed := false
		for _, read := range cursorReadPermissions {
			if value == read {
				managed = true
				break
			}
		}
		if !managed {
			kept = append(kept, value)
		}
	}
	if install {
		for _, read := range cursorReadPermissions {
			kept = append(kept, read)
		}
	}
	if sameJSON(allow, kept) && statErr == nil {
		return nil
	}
	if dryRun {
		return nil
	}
	permissions["allow"] = kept
	if permissions["deny"] == nil {
		permissions["deny"] = []any{}
	}
	config["permissions"] = permissions
	if filepath.Base(path) == "cli-config.json" {
		if config["version"] == nil {
			config["version"] = 1
		}
		if config["editor"] == nil {
			config["editor"] = map[string]any{"vimMode": false}
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return writeJSONObject(path, config)
}

// Cursor Agent CLI does not currently register plugin MCP servers from
// --plugin-dir. Add only its MCP entry; the marketplace plugin owns the hook.
func cursorMCPOnly(dir, configPath, binary string, update, dryRun bool) error {
	path := filepath.Join(dir, "mcp.json")
	mcp, err := readJSONObject(path)
	if err != nil {
		return err
	}
	servers, ok := jsonObject(mcp["mcpServers"])
	if mcp["mcpServers"] != nil && !ok {
		return errors.New("Cursor mcpServers must be an object")
	}
	if !ok {
		servers = map[string]any{}
	}
	expected := grasshopperMCPEntry(binary, configPath)
	if existing, exists := servers["grasshopper"]; exists {
		entry, valid := jsonObject(existing)
		command, _ := entry["command"].(string)
		if !valid || !grasshopperExecutable(command) {
			return errors.New("Cursor already has an unrelated grasshopper MCP entry")
		}
		if !sameJSON(existing, expected) && !update {
			return errors.New("Cursor Grasshopper MCP differs; inspect it before using --update")
		}
	}
	if dryRun {
		return nil
	}
	servers["grasshopper"] = expected
	mcp["mcpServers"] = servers
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	return writeJSONObject(path, mcp)
}

func cursorWiringWithOptions(dir, configPath, binary string, install, update, dryRun, report bool) error {
	mcpPath, hooksPath := filepath.Join(dir, "mcp.json"), filepath.Join(dir, "hooks.json")
	mcp, err := readJSONObject(mcpPath)
	if err != nil {
		return err
	}
	hooks, err := readJSONObject(hooksPath)
	if err != nil {
		return err
	}
	servers, ok := jsonObject(mcp["mcpServers"])
	if mcp["mcpServers"] != nil && !ok {
		return errors.New("Cursor mcpServers must be an object")
	}
	if !ok {
		servers = map[string]any{}
	}
	expected := grasshopperMCPEntry(binary, configPath)
	if existing, exists := servers["grasshopper"]; exists {
		entry, valid := jsonObject(existing)
		command, _ := entry["command"].(string)
		if !valid || !grasshopperExecutable(command) {
			return errors.New("Cursor already has an unrelated grasshopper MCP entry")
		}
		if install && !update && !sameJSON(existing, expected) {
			return errors.New("Cursor Grasshopper MCP differs; inspect it before using --update")
		}
	}
	if install {
		servers["grasshopper"] = expected
		mcp["mcpServers"] = servers
	} else {
		delete(servers, "grasshopper")
		if len(servers) == 0 {
			delete(mcp, "mcpServers")
		} else {
			mcp["mcpServers"] = servers
		}
	}
	if hooks["version"] != nil && hooks["version"] != float64(1) {
		return errors.New("unsupported Cursor hooks version")
	}
	events, ok := jsonObject(hooks["hooks"])
	if hooks["hooks"] != nil && !ok {
		return errors.New("Cursor hooks must be an object")
	}
	if !ok {
		events = map[string]any{}
	}
	list, ok := events["sessionStart"].([]any)
	if events["sessionStart"] != nil && !ok {
		return errors.New("Cursor sessionStart hooks must be an array")
	}
	command := shellQuoted(binary) + " hook --config " + shellQuoted(configPath) + " --harness cursor"
	newHook := map[string]any{"command": command, "timeout": 8}
	kept := make([]any, 0, len(list)+1)
	for _, hook := range list {
		if isGrasshopperHook(hook) {
			if install && !update && !sameJSON(hook, newHook) {
				return errors.New("Cursor Grasshopper hook differs; inspect it before using --update")
			}
			continue
		}
		kept = append(kept, hook)
	}
	if install {
		kept = append(kept, newHook)
		events["sessionStart"] = kept
		hooks["version"] = 1
		hooks["hooks"] = events
	} else {
		if len(kept) == 0 {
			delete(events, "sessionStart")
		} else {
			events["sessionStart"] = kept
		}
		if len(events) == 0 {
			delete(hooks, "hooks")
		} else {
			hooks["hooks"] = events
		}
	}
	if dryRun {
		return nil
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err := writeJSONObject(mcpPath, mcp); err != nil {
		return err
	}
	if err := writeJSONObject(hooksPath, hooks); err != nil {
		return err
	}
	if install && report {
		fmt.Fprintln(os.Stdout, "Cursor MCP and startup hook installed. Run `agent mcp enable grasshopper` once to approve this local server.")
	} else if !install && report {
		fmt.Fprintln(os.Stdout, "Cursor Grasshopper MCP and startup hook removed. Shared client config and token remain available to other agents.")
	}
	return nil
}

func sameJSON(left, right any) bool {
	a, _ := json.Marshal(left)
	b, _ := json.Marshal(right)
	return string(a) == string(b)
}

func writeJSONObject(path string, value map[string]any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return privateFile(path, append(data, '\n'))
}
