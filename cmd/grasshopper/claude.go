package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

var claudeReadPermissions = []string{
	"mcp__plugin_grasshopper_grasshopper__context",
	"mcp__plugin_grasshopper_grasshopper__get",
	"mcp__plugin_grasshopper_grasshopper__search",
}

// Claude Code prompts for headless MCP reads unless these exact tools are allowed.
func allowClaudeReads(path string, dryRun bool) error {
	return claudePermissions(path, true, dryRun)
}

func removeClaudeReads(path string, dryRun bool) error {
	return claudePermissions(path, false, dryRun)
}

func claudePermissions(path string, install, dryRun bool) error {
	info, err := os.Lstat(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if !install && errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err == nil && !info.Mode().IsRegular() {
		return errors.New("Claude settings must be a regular file")
	}
	settings, err := readJSONObject(path)
	if err != nil {
		return err
	}
	permissions, ok := jsonObject(settings["permissions"])
	if settings["permissions"] != nil && !ok {
		return errors.New("Claude permissions must be an object")
	}
	if !ok {
		permissions = map[string]any{}
	}
	allow, ok := permissions["allow"].([]any)
	if permissions["allow"] != nil && !ok {
		return errors.New("Claude permissions.allow must be an array")
	}
	for _, entry := range allow {
		if _, ok := entry.(string); !ok {
			return errors.New("Claude permissions.allow must contain strings")
		}
	}
	newAllow := append([]any{}, allow...)
	if install {
		for _, read := range claudeReadPermissions {
			found := false
			for _, entry := range newAllow {
				found = found || entry == read
			}
			if !found {
				newAllow = append(newAllow, read)
			}
		}
	} else {
		newAllow = newAllow[:0]
		for _, entry := range allow {
			managed := false
			for _, read := range claudeReadPermissions {
				managed = managed || entry == read
			}
			if !managed {
				newAllow = append(newAllow, entry)
			}
		}
	}
	if sameJSON(allow, newAllow) && info != nil {
		return nil
	}
	if dryRun {
		return nil
	}
	permissions["allow"] = newAllow
	settings["permissions"] = permissions
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return writeJSONObject(path, settings)
}

func claudeCommand(args []string) error {
	if len(args) == 0 || args[0] != "remove" {
		return errors.New("use claude remove")
	}
	flags := flag.NewFlagSet("claude remove", flag.ContinueOnError)
	settings := flags.String("settings", "", "Claude user settings.json path")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected Claude arguments")
	}
	if *settings == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		*settings = filepath.Join(home, ".claude", "settings.json")
	}
	if !filepath.IsAbs(*settings) {
		return errors.New("Claude settings path must be absolute")
	}
	if err := removeClaudeReads(*settings, false); err != nil {
		return err
	}
	fmt.Fprintln(os.Stdout, "Grasshopper read permissions removed from Claude Code. Remove its plugin and marketplace separately; server memories remain.")
	return nil
}
