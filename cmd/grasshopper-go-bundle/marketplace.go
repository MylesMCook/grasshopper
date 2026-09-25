package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// marketplaceFiles builds one OS-specific plugin per platform. Codex and
// Cursor share its executable and canonical policy, but load their own native
// manifests and hooks. No credentials or writable memory are packaged.
func marketplaceFiles(binaries map[string]string, version, output string) ([]input, func(), error) {
	if !pluginVersion.MatchString(version) {
		return nil, nil, errors.New("plugin version must be major.minor.patch")
	}
	for _, target := range []string{"darwin-arm64", "windows-amd64", "linux-amd64"} {
		path := binaries[target]
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			return nil, nil, fmt.Errorf("missing %s client binary", target)
		}
		if (target == "windows-amd64") != strings.EqualFold(filepath.Ext(path), ".exe") {
			return nil, nil, fmt.Errorf("wrong binary suffix for %s", target)
		}
	}
	staged, err := os.MkdirTemp(filepath.Dir(output), ".grasshopper-marketplace-*")
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() { _ = os.RemoveAll(staged) }
	files := []input{
		{"README.md", "README.md"},
		{"LICENSE", "LICENSE"},
	}
	stage := func(name string, data []byte) error {
		path := filepath.Join(staged, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return err
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			return err
		}
		files = append(files, input{name, path})
		return nil
	}
	stageTemplate := func(name, source, exe string, changes map[string]any) error {
		data, err := os.ReadFile(source)
		if err != nil {
			return err
		}
		rendered := strings.NewReplacer("{{VERSION}}", version, "{{EXE}}", exe, "{{WINDOWS_HOOK_COMMAND}}", windowsHookCommand()).Replace(string(data))
		if strings.Contains(rendered, "{{") {
			return fmt.Errorf("unrendered template: %s", source)
		}
		if len(changes) != 0 {
			var object map[string]any
			if err := json.Unmarshal([]byte(rendered), &object); err != nil {
				return err
			}
			for key, value := range changes {
				if value == nil {
					delete(object, key)
				} else {
					object[key] = value
				}
			}
			data, err = json.MarshalIndent(object, "", "  ")
			if err != nil {
				return err
			}
			rendered = string(data) + "\n"
		} else if !json.Valid([]byte(rendered)) {
			return fmt.Errorf("invalid template JSON: %s", source)
		}
		return stage(name, []byte(rendered))
	}
	platforms := []struct{ target, slug string }{
		{"darwin-arm64", "macos"}, {"windows-amd64", "windows"}, {"linux-amd64", "linux"},
	}
	for _, platform := range platforms {
		plugin := "grasshopper-" + platform.slug
		root := "plugins/" + plugin + "/"
		exe := ""
		if platform.slug == "windows" {
			exe = ".exe"
		}
		files = append(files,
			input{root + "bin/grasshopper" + exe, binaries[platform.target]},
			input{root + "policy/AGENTS.md", "integrations/policy/AGENTS.md"},
			input{root + "LICENSE", "LICENSE"},
		)
		if err := stageTemplate(root+".codex-plugin/plugin.json", "integrations/plugins/codex/.codex-plugin/plugin.json", exe, map[string]any{
			"name":  plugin,
			"hooks": nil,
			"interface": map[string]any{
				"displayName":      "Grasshopper for " + platform.slug,
				"shortDescription": "Shared memory from your private server.",
				"longDescription":  "Connect this agent to your private Grasshopper server. The plugin includes no token or writable memory database; connection creates a revocable device credential.",
				"developerName":    "Grasshopper",
				"defaultPrompt":    "Use the connect-grasshopper skill to connect to my private server.",
				"category":         "Productivity",
				"capabilities":     []string{"Read", "Write"},
			},
		}); err != nil {
			cleanup()
			return nil, nil, err
		}
		if err := stageTemplate(root+".mcp.json", "integrations/plugins/codex/.mcp.json", exe, nil); err != nil {
			cleanup()
			return nil, nil, err
		}
		if err := stageTemplate(root+"hooks/hooks.json", "integrations/plugins/codex/hooks/hooks.json", exe, nil); err != nil {
			cleanup()
			return nil, nil, err
		}
		if err := stageTemplate(root+".cursor-plugin/plugin.json", "integrations/plugins/cursor/.cursor-plugin/plugin.json", exe, map[string]any{
			"name": plugin, "hooks": "./hooks/cursor.json",
		}); err != nil {
			cleanup()
			return nil, nil, err
		}
		if err := stageTemplate(root+"mcp.json", "integrations/plugins/cursor/mcp.json", exe, nil); err != nil {
			cleanup()
			return nil, nil, err
		}
		if err := stageTemplate(root+"hooks/cursor.json", "integrations/plugins/cursor/hooks/hooks.json", exe, nil); err != nil {
			cleanup()
			return nil, nil, err
		}
		skill, err := os.ReadFile("integrations/plugins/connect/SKILL.md")
		if err != nil {
			cleanup()
			return nil, nil, err
		}
		skill = []byte(strings.ReplaceAll(string(skill), "{{EXE}}", exe))
		if err := stage(root+"skills/connect-grasshopper/SKILL.md", skill); err != nil {
			cleanup()
			return nil, nil, err
		}
	}
	var codexPlugins, cursorPlugins []map[string]any
	for _, platform := range platforms {
		name := "grasshopper-" + platform.slug
		codexPlugins = append(codexPlugins, map[string]any{
			"name":     name,
			"source":   map[string]any{"source": "local", "path": "./plugins/" + name},
			"policy":   map[string]any{"installation": "AVAILABLE", "authentication": "ON_INSTALL"},
			"category": "Productivity",
		})
		cursorPlugins = append(cursorPlugins, map[string]any{
			"name": name, "source": "./plugins/" + name, "description": "Connect to your private Grasshopper memory server",
		})
	}
	for name, object := range map[string]any{
		".agents/plugins/marketplace.json": map[string]any{"name": "grasshopper-marketplace", "interface": map[string]any{"displayName": "Grasshopper"}, "plugins": codexPlugins},
		".cursor-plugin/marketplace.json":  map[string]any{"name": "grasshopper-marketplace", "owner": map[string]any{"name": "Myles Cook"}, "plugins": cursorPlugins},
	} {
		data, err := json.MarshalIndent(object, "", "  ")
		if err != nil {
			cleanup()
			return nil, nil, err
		}
		if err := stage(name, append(data, '\n')); err != nil {
			cleanup()
			return nil, nil, err
		}
	}
	return files, cleanup, nil
}
