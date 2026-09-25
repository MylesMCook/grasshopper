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

func cursorCommand(args []string) error {
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
	return cursorWiring(*cursorDir, configPath, binary, args[0] == "install", *update)
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
	expected := map[string]any{"type": "stdio", "command": binary, "args": []string{"bridge", "--config", configPath}}
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
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err := writeJSONObject(mcpPath, mcp); err != nil {
		return err
	}
	if err := writeJSONObject(hooksPath, hooks); err != nil {
		return err
	}
	if install {
		fmt.Fprintln(os.Stdout, "Cursor MCP and startup hook installed. Run `agent mcp enable grasshopper` once to approve this local server.")
	} else {
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
