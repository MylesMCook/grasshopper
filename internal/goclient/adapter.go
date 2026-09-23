package goclient

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func Bridge(ctx context.Context, configPath string, stdin io.Reader, stdout io.Writer) error {
	config, err := LoadConfig(configPath)
	if err != nil {
		return err
	}
	remote, err := NewRemote(config)
	if err != nil {
		return err
	}
	scanner := bufio.NewScanner(stdin)
	scanner.Buffer(make([]byte, 4096), 131073)
	for scanner.Scan() {
		body := append([]byte(nil), scanner.Bytes()...)
		id, _, _, _, err := rpcID(body)
		if err != nil {
			return err
		}
		requestCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		response, acknowledged, err := remote.Request(requestCtx, body)
		cancel()
		if err != nil && id != nil {
			response, _ = json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": -32000, "message": unavailable}})
			acknowledged = true
		}
		if acknowledged {
			if _, err := fmt.Fprintln(stdout, string(response)); err != nil {
				return err
			}
		}
	}
	return scanner.Err()
}

func Hook(configPath, harness string, input map[string]any) (map[string]any, error) {
	if harness != "codex" && harness != "cursor" && harness != "claude" {
		return nil, errors.New("unsupported harness")
	}
	config, err := LoadConfig(configPath)
	if err != nil {
		return nil, err
	}
	if filepath.Base(config.PolicyPath) != "AGENTS.md" {
		return nil, errors.New("policy must be named AGENTS.md")
	}
	policyBytes, err := os.ReadFile(config.PolicyPath)
	if err != nil || len(policyBytes) > 16384 {
		return nil, errors.New("canonical AGENTS.md unavailable or too large")
	}
	cwd := stringValue(input["cwd"])
	if cwd == "" {
		if roots, ok := input["workspace_roots"].([]any); ok && len(roots) > 0 {
			cwd = stringValue(roots[0])
		}
	}
	if cwd == "" {
		cwd, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}
	cwd, err = filepath.Abs(cwd)
	if err != nil {
		return nil, err
	}
	target := ""
	if tool, ok := input["tool_input"].(map[string]any); ok {
		target = stringValue(tool["file_path"])
	}
	directory := cwd
	if target != "" {
		if !filepath.IsAbs(target) {
			target = filepath.Join(cwd, target)
		}
		directory = filepath.Dir(target)
	}
	guidance, err := applicableGuidance(directory, config.PolicyPath)
	if err != nil {
		return nil, err
	}
	policy := string(policyBytes)
	if input["hook_event_name"] == "PreToolUse" {
		if harness != "claude" {
			return nil, errors.New("unsupported pre-tool adapter")
		}
		return map[string]any{"hookSpecificOutput": map[string]any{"hookEventName": "PreToolUse", "additionalContext": policy + "\n" + guidance}}, nil
	}
	scope, scopeErr := ResolveScope(cwd, config.Device)
	resolved := scopeErr == nil
	if !resolved {
		platform := Platform()
		scope = Scope{Device: &config.Device, Platform: &platform}
	}
	status := "Grasshopper context unavailable. Continue work with memory unavailable; do not claim an unacknowledged write was saved."
	if remote, err := NewRemote(config); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		loaded, err := remote.Context(ctx, scope)
		cancel()
		if err == nil {
			encodedScope, _ := json.Marshal(scope)
			status = fmt.Sprintf("Grasshopper context loaded for %s. project_resolved=%t; unresolved projects load only global/device/platform records. Historical data follows:\n%s", encodedScope, resolved, loaded)
		}
	}
	text := policy + "\n" + guidance + "\n" + status
	if harness == "cursor" {
		return map[string]any{"additional_context": text}, nil
	}
	event := stringValue(input["hook_event_name"])
	if event == "" {
		event = "SessionStart"
	}
	if event != "SessionStart" && event != "UserPromptSubmit" && event != "SubagentStart" && event != "PostCompact" {
		return nil, errors.New("unsupported hook event")
	}
	return map[string]any{"hookSpecificOutput": map[string]any{"hookEventName": event, "additionalContext": text}}, nil
}

func applicableGuidance(directory, policyPath string) (string, error) {
	var ancestors []string
	for current := directory; ; current = filepath.Dir(current) {
		ancestors = append(ancestors, current)
		if parent := filepath.Dir(current); parent == current {
			break
		}
	}
	var guidance strings.Builder
	for i := len(ancestors) - 1; i >= 0; i-- {
		path := filepath.Join(ancestors[i], "AGENTS.md")
		if path == policyPath {
			continue
		}
		content, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		if guidance.Len()+len(content) > 65536 {
			return "", errors.New("applicable AGENTS.md guidance exceeds adapter budget; read files directly")
		}
		fmt.Fprintf(&guidance, "\nAGENTS.md guidance from %s:\n%s\n", path, content)
	}
	return guidance.String(), nil
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

func ParseHookInput(reader io.Reader) (map[string]any, error) {
	data, err := io.ReadAll(io.LimitReader(reader, 131073))
	if err != nil || len(data) > 131072 {
		return nil, errors.New("hook input exceeds limit")
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return map[string]any{}, nil
	}
	var input map[string]any
	if err := json.Unmarshal(data, &input); err != nil {
		return nil, err
	}
	return input, nil
}
