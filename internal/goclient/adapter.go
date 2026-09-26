package goclient

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
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
	event := stringValue(input["hook_event_name"])
	if event == "" {
		event = "SessionStart"
	}
	if harness == "codex" && event == "UserPromptSubmit" && promptContextRecent(input) {
		return map[string]any{}, nil
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
	policy := string(policyBytes)
	if harness != "cursor" && event != "SessionStart" && event != "SubagentStart" && !(harness == "codex" && event == "UserPromptSubmit") {
		return nil, errors.New("unsupported hook event")
	}
	scope, scopeErr := ResolveScope(cwd, config.Device)
	resolved := scopeErr == nil
	if !resolved {
		platform := Platform()
		scope = Scope{Device: &config.Device, Platform: &platform}
	}
	status := "Grasshopper context unavailable. Continue work with memory unavailable; do not claim an unacknowledged write was saved."
	if remote, err := NewRemote(config); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		budget := 12000
		if harness == "claude" {
			budget = 3000 // Claude previews hook strings above 10,000 characters.
		}
		loaded, err := remote.Context(ctx, scope, budget)
		cancel()
		if err == nil {
			encodedScope, _ := json.Marshal(scope)
			status = fmt.Sprintf("Grasshopper context loaded for %s. project_resolved=%t; unresolved projects load only global/device/platform records. Historical data follows:\n%s", encodedScope, resolved, loaded)
		}
	}
	text := policy + "\n" + status
	if harness == "claude" && len(text) > 8200 {
		text = policy + "\nGrasshopper context exceeded Claude's startup hook budget. Call grasshopper/context once before substantive work; no records were delivered by this hook."
	}
	if harness == "claude" || event == "SubagentStart" {
		// Claude's native AGENTS.md mod may be unavailable; subagents can omit
		// project instructions. Keep complete hook context below Claude's cap.
		guidance, err := applicableGuidance(cwd, config.PolicyPath)
		if err != nil {
			return nil, err
		}
		if harness != "claude" || len(text)+len(guidance) <= 9000 {
			text += "\n" + guidance
		} else {
			text += fmt.Sprintf("\nBefore substantive work, read the applicable AGENTS.md files from %s. Their full text exceeds the hook budget.\n", cwd)
		}
		if harness == "claude" && len(text) > 9000 {
			return nil, errors.New("Claude startup context exceeds hook budget")
		}
	}
	if harness == "cursor" {
		return map[string]any{"additional_context": text}, nil
	}
	if harness == "codex" && event == "UserPromptSubmit" {
		markPromptContext(input)
	}
	return map[string]any{"hookSpecificOutput": map[string]any{"hookEventName": event, "additionalContext": text}}, nil
}

// HookUnavailable keeps a failed startup visible without exposing local paths.
func HookUnavailable(harness string, input map[string]any) map[string]any {
	event := stringValue(input["hook_event_name"])
	if event == "" {
		event = "SessionStart"
	}
	message := "Grasshopper startup unavailable. Read applicable AGENTS.md guidance and call grasshopper/context once before substantive work if connected. Continue if it is unavailable; no memory write was acknowledged."
	if harness == "cursor" {
		return map[string]any{"additional_context": message}
	}
	return map[string]any{"hookSpecificOutput": map[string]any{"hookEventName": event, "additionalContext": message}}
}

func promptContextPath(input map[string]any) string {
	dataDir, sessionID := os.Getenv("PLUGIN_DATA"), stringValue(input["session_id"])
	if dataDir == "" || sessionID == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(sessionID))
	return filepath.Join(dataDir, fmt.Sprintf("grasshopper-context-%x.stamp", sum[:16]))
}

func promptContextRecent(input map[string]any) bool {
	path := promptContextPath(input)
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && time.Since(info.ModTime()) < time.Hour
}

func markPromptContext(input map[string]any) {
	path := promptContextPath(input)
	if path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err == nil {
		_ = os.WriteFile(path, []byte("context attempted\n"), 0600)
	}
}

// HookGlobalPart loads Claude's user AGENTS.md through the same hook channel
// when native AGENTS.md loading is unavailable. Each part stays below its cap.
func HookGlobalPart(part int, input map[string]any) (map[string]any, error) {
	if part != 1 && part != 2 {
		return nil, errors.New("invalid global guidance part")
	}
	event := stringValue(input["hook_event_name"])
	if event == "" {
		event = "SessionStart"
	}
	if event != "SessionStart" && event != "SubagentStart" {
		return nil, errors.New("unsupported hook event")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(home, ".claude", "AGENTS.md")
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	const chunk = 8500
	if info.Size() > chunk*2 {
		message := fmt.Sprintf("User AGENTS.md at %s exceeds the two-part Claude hook limit. Its full guidance was not loaded; use a client with native AGENTS.md support before substantive work.", path)
		return map[string]any{"hookSpecificOutput": map[string]any{"hookEventName": event, "additionalContext": message}}, nil
	}
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	if len(content) > chunk*2 {
		return nil, errors.New("user AGENTS.md changed while loading")
	}
	start := (part - 1) * chunk
	if start >= len(content) {
		return map[string]any{}, nil
	}
	end := min(start+chunk, len(content))
	for end < len(content) && end > start && content[end]&0xc0 == 0x80 {
		end--
	}
	if part == 2 {
		for start > 0 && content[start]&0xc0 == 0x80 {
			start--
		}
	}
	text := fmt.Sprintf("User AGENTS.md from %s, part %d of 2. Apply the complete file across both parts:\n%s", path, part, content[start:end])
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
