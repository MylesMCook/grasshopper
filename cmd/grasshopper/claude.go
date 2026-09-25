package main

import (
	"errors"
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
	info, err := os.Lstat(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
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
	for _, read := range claudeReadPermissions {
		found := false
		for _, entry := range newAllow {
			if entry == read {
				found = true
				break
			}
		}
		if !found {
			newAllow = append(newAllow, read)
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
