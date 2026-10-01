package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// agentReport describes one Grasshopper connector an agent has on this
// machine. It is read-only: check never changes agent wiring.
type agentReport struct {
	Agent   string `json:"agent"`
	Route   string `json:"route"`
	Version string `json:"version,omitempty"`
	State   string `json:"state"`
	Detail  string `json:"detail,omitempty"`
	Update  string `json:"update,omitempty"`
}

const (
	agentCurrent = "current"
	agentBehind  = "behind"
	agentNewer   = "newer"
	agentUnknown = "unknown"
	// agentBroken means the agent points at a Grasshopper program that no
	// longer exists, so it cannot reach memory at all.
	agentBroken = "broken"
)

const cursorArchiveUpdate = "Run bin/grasshopper setup --agents cursor --update from the new client archive."

// cursorCLIUpdate refreshes MCP-only wiring from connect --cursor-cli, which
// leaves the startup hook to the marketplace plugin. A full Cursor update
// would add a second startup hook.
const cursorCLIUpdate = "Run bin/grasshopper setup --agents none --cursor-cli --update from the new client archive."

// inspectAgents lists the Grasshopper connectors of Claude Code, Codex and
// Cursor and compares each version with serverVersion. An agent whose CLI or
// settings are absent is left out. An empty serverVersion marks every
// version unknown because nothing can be compared.
func inspectAgents(run commandRunner, cursorDir, serverVersion string) []agentReport {
	var reports []agentReport
	reports = append(reports, pluginAgents(run, "Claude Code", "claude", serverVersion)...)
	reports = append(reports, pluginAgents(run, "Codex", "codex", serverVersion)...)
	reports = append(reports, cursorAgents(run, cursorDir, serverVersion)...)
	return reports
}

// pluginAgents reads an agent's own plugin list, which is the only reliable
// record of the version Claude Code or Codex loads.
func pluginAgents(run commandRunner, agent, cli, serverVersion string) []agentReport {
	output, err := run(cli, "plugin", "list", "--json")
	if errors.Is(err, exec.ErrNotFound) {
		return nil
	}
	if err != nil {
		return []agentReport{{Agent: agent, Route: cli + " plugin list", State: agentUnknown, Detail: "Could not list plugins: " + err.Error()}}
	}
	items, idKey := []map[string]any{}, "id"
	if cli == "codex" {
		var listed struct {
			Installed []map[string]any `json:"installed"`
		}
		err = json.Unmarshal(output, &listed)
		items, idKey = listed.Installed, "pluginId"
	} else {
		err = json.Unmarshal(output, &items)
	}
	if err != nil {
		return []agentReport{{Agent: agent, Route: cli + " plugin list", State: agentUnknown, Detail: "Plugin list was not valid JSON."}}
	}
	var reports []agentReport
	for _, item := range items {
		id, _ := item[idKey].(string)
		name, marketplace, ok := strings.Cut(id, "@")
		if !ok || !isGrasshopperPluginName(name) || (marketplace != "grasshopper-marketplace" && marketplace != "grasshopper-local") {
			continue
		}
		version, _ := item["version"].(string)
		report := agentReport{Agent: agent, Route: "plugin " + id, Version: version, State: versionState(version, serverVersion)}
		if dir := pluginDir(cli, item); dir != "" && !pluginProgramExists(dir) {
			report.State, report.Update = agentBroken, pluginReinstall(cli, id, marketplace)
			report.Detail = "Missing program in " + filepath.Join(dir, "bin")
			reports = append(reports, report)
			continue
		}
		if report.State == agentBehind {
			report.Update = pluginUpdate(cli, id, marketplace)
		}
		reports = append(reports, withNewerDetail(report))
	}
	return reports
}

// pluginDir returns the installed plugin folder an agent's plugin list
// reports, or "" when the list does not say.
func pluginDir(cli string, item map[string]any) string {
	if cli == "claude" {
		dir, _ := item["installPath"].(string)
		return dir
	}
	source, _ := item["source"].(map[string]any)
	dir, _ := source["path"].(string)
	return dir
}

// pluginProgramExists checks the bundled client that the plugin's MCP server
// and hooks run; plugin metadata can outlive a deleted program.
func pluginProgramExists(dir string) bool {
	for _, name := range []string{"grasshopper.exe", "grasshopper"} {
		if info, err := os.Stat(filepath.Join(dir, "bin", name)); err == nil && info.Mode().IsRegular() {
			return true
		}
	}
	return false
}

func pluginReinstall(cli, id, marketplace string) string {
	if marketplace == "grasshopper-local" {
		return "Run bin/grasshopper setup --agents " + cli + " --update from the new client archive."
	}
	if cli == "claude" {
		return "Reinstall it: claude plugin uninstall " + id + ", then claude plugin install " + id + "."
	}
	return "Reinstall it: remove " + id + " in the Codex plugin manager, then run codex plugin add " + id + "."
}

func isGrasshopperPluginName(name string) bool {
	return name == "grasshopper" || strings.HasPrefix(name, "grasshopper-")
}

func pluginUpdate(cli, id, marketplace string) string {
	if marketplace == "grasshopper-local" {
		return "Run bin/grasshopper setup --agents " + cli + " --update from the new client archive."
	}
	if cli == "claude" {
		return "Run claude plugin marketplace update grasshopper-marketplace, then claude plugin update " + id + "."
	}
	return "Run codex plugin marketplace upgrade grasshopper-marketplace, then codex plugin add " + id + "."
}

// cursorAgents reports both Cursor routes: direct wiring in mcp.json and
// hooks.json from a client archive, and marketplace plugin files. A machine
// can have both, and each needs its own update.
func cursorAgents(run commandRunner, cursorDir, serverVersion string) []agentReport {
	if cursorDir == "" {
		return nil
	}
	var reports []agentReport
	if report, ok := cursorWiringReport(run, cursorDir, serverVersion); ok {
		reports = append(reports, report)
	}
	return append(reports, cursorPluginReports(cursorDir, serverVersion)...)
}

func cursorWiringReport(run commandRunner, cursorDir, serverVersion string) (agentReport, bool) {
	report := agentReport{Agent: "Cursor", Route: "Cursor settings (mcp.json, hooks.json)"}
	var binaries []string
	add := func(binary string) {
		for _, seen := range binaries {
			if seen == binary {
				return
			}
		}
		binaries = append(binaries, binary)
	}
	// Read each settings file on its own, so one unreadable file cannot hide
	// a missing program the other points at.
	mcp, mcpErr := readJSONObject(filepath.Join(cursorDir, "mcp.json"))
	hooks, hooksErr := readJSONObject(filepath.Join(cursorDir, "hooks.json"))
	settingsErr := errors.Join(mcpErr, hooksErr)
	if servers, ok := jsonObject(mcp["mcpServers"]); ok {
		if entry, ok := jsonObject(servers["grasshopper"]); ok {
			if command, _ := entry["command"].(string); grasshopperExecutable(command) {
				add(command)
			}
		}
	}
	hasHook := false
	if events, ok := jsonObject(hooks["hooks"]); ok {
		list, _ := events["sessionStart"].([]any)
		for _, hook := range list {
			if binary, ok := grasshopperHookBinary(hook); ok {
				add(binary)
				hasHook = true
			}
		}
	}
	if len(binaries) == 0 && settingsErr == nil {
		return report, false
	}
	update := cursorArchiveUpdate
	if settingsErr != nil {
		// Route-specific repair waits until both files can be read.
		update = "Fix the unreadable Cursor settings first: " + settingsErr.Error()
	} else if !hasHook {
		report.Route, update = "Cursor settings (mcp.json)", cursorCLIUpdate
	}
	var missing, programs []string
	for _, binary := range binaries {
		if program, ok := resolveProgram(binary); ok {
			programs = append(programs, program)
		} else {
			missing = append(missing, binary)
		}
	}
	if len(missing) > 0 {
		report.State, report.Update = agentBroken, update
		report.Detail = "Missing program: " + strings.Join(missing, ", ")
		if settingsErr != nil {
			report.Detail += ". " + settingsErr.Error()
		}
		return report, true
	}
	if settingsErr != nil {
		report.State, report.Detail = agentUnknown, settingsErr.Error()
		return report, true
	}
	var versions []string
	for _, binary := range programs {
		output, err := run(binary, "--version")
		version, ok := strings.CutPrefix(strings.TrimSpace(string(output)), "grasshopper ")
		if err != nil || !ok {
			report.State, report.Detail = agentUnknown, "Could not read the version of "+binary
			return report, true
		}
		versions = append(versions, version)
	}
	report.Version = versions[0]
	for _, version := range versions[1:] {
		if version != report.Version {
			report.State, report.Version, report.Update = agentUnknown, "", update
			report.Detail = "The MCP server and startup hook run different versions: " + strings.Join(versions, ", ")
			return report, true
		}
	}
	report.State = versionState(report.Version, serverVersion)
	if report.State == agentBehind {
		report.Update = update
	}
	return withNewerDetail(report), true
}

// resolveProgram finds the program a Cursor command runs. A bare name such
// as grasshopper is looked up on PATH, as Cursor would; a path must exist.
func resolveProgram(command string) (string, bool) {
	if !strings.ContainsAny(command, `/\`) {
		path, err := exec.LookPath(command)
		return path, err == nil
	}
	info, err := os.Stat(command)
	return command, err == nil && info.Mode().IsRegular()
}

// cursorMarketplaceVersions reads the plugin versions in Cursor's copy of the
// Grasshopper marketplace. Cursor pins a marketplace to the commit its branch
// pointed at when it was added, so these are the versions an update in
// Cursor's Plugins menu would install, not the branch's current release.
func cursorMarketplaceVersions(cursorDir string) map[string]string {
	catalogs, _ := filepath.Glob(filepath.Join(cursorDir, "plugins", "marketplaces", "*", "*", "*", "*", ".cursor-plugin", "marketplace.json"))
	versions := map[string]string{}
	var newest time.Time
	for _, catalog := range catalogs {
		info, err := os.Stat(catalog)
		data, readErr := os.ReadFile(catalog)
		var marketplace struct {
			Name string `json:"name"`
		}
		if err != nil || readErr != nil || json.Unmarshal(data, &marketplace) != nil || marketplace.Name != "grasshopper-marketplace" || info.ModTime().Before(newest) {
			continue
		}
		newest = info.ModTime()
		versions = map[string]string{}
		manifests, _ := filepath.Glob(filepath.Join(filepath.Dir(filepath.Dir(catalog)), "plugins", "*", ".cursor-plugin", "plugin.json"))
		for _, manifest := range manifests {
			var plugin struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			}
			if data, err := os.ReadFile(manifest); err == nil && json.Unmarshal(data, &plugin) == nil {
				versions[plugin.Name] = plugin.Version
			}
		}
	}
	return versions
}

// cursorPluginUpdate repins the marketplace first when its pinned commit is
// itself behind, because updating the plugin alone would reinstall the old
// release.
func cursorPluginUpdate(name, pinnedVersion, serverVersion string) string {
	update := "Update " + name + " in Cursor's Plugins menu."
	if state := versionState(pinnedVersion, serverVersion); state != agentCurrent && state != agentNewer {
		update = "Run agent plugin marketplace remove grasshopper-marketplace, then agent plugin marketplace add " +
			"https://github.com/MylesMCook/grasshopper.git --git-ref marketplace, then update " + name + " in Cursor's Plugins menu."
	}
	return update + " If Cursor also lists an Imported Grasshopper plugin from Claude Code, uninstall this copy instead."
}

// cursorPluginReports reads the installed plugin files in Cursor's plugin
// cache. Cursor keeps one folder per fetched commit, so only the newest
// manifest for each plugin name counts. Cursor does not record whether the
// plugin is turned on in a readable place, so the route names files, not use.
func cursorPluginReports(cursorDir, serverVersion string) []agentReport {
	pinned := cursorMarketplaceVersions(cursorDir)
	manifests, _ := filepath.Glob(filepath.Join(cursorDir, "plugins", "cache", "grasshopper-marketplace", "*", "*", ".cursor-plugin", "plugin.json"))
	type found struct {
		version  string
		dir      string
		modified time.Time
	}
	newest := map[string]found{}
	var names []string
	for _, manifest := range manifests {
		info, err := os.Stat(manifest)
		if err != nil {
			continue
		}
		var plugin struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		}
		data, err := os.ReadFile(manifest)
		if err != nil || json.Unmarshal(data, &plugin) != nil || !isGrasshopperPluginName(plugin.Name) {
			continue
		}
		previous, seen := newest[plugin.Name]
		if !seen {
			names = append(names, plugin.Name)
		}
		if !seen || info.ModTime().After(previous.modified) {
			newest[plugin.Name] = found{plugin.Version, filepath.Dir(filepath.Dir(manifest)), info.ModTime()}
		}
	}
	var reports []agentReport
	for _, name := range names {
		version := newest[name].version
		report := agentReport{Agent: "Cursor", Route: "plugin files " + name + "@grasshopper-marketplace", Version: version, State: versionState(version, serverVersion)}
		if dir := newest[name].dir; !pluginProgramExists(dir) {
			report.State, report.Detail = agentBroken, "Missing program in "+filepath.Join(dir, "bin")
			report.Update = "Reinstall " + name + " in Cursor's Plugins menu. If Cursor also lists an Imported Grasshopper plugin from Claude Code, uninstall this copy instead."
			reports = append(reports, report)
			continue
		}
		if report.State == agentBehind {
			report.Update = cursorPluginUpdate(name, pinned[name], serverVersion)
		}
		reports = append(reports, withNewerDetail(report))
	}
	return reports
}

func withNewerDetail(report agentReport) agentReport {
	if report.State == agentNewer {
		report.Detail = "Newer than the server; update the server to match."
	}
	return report
}

// versionState compares release versions such as 2.9.1. A development build
// or missing server version cannot be compared and is unknown.
func versionState(version, serverVersion string) string {
	agent, agentOK := parseVersion(version)
	server, serverOK := parseVersion(serverVersion)
	if !agentOK || !serverOK {
		return agentUnknown
	}
	for i := range agent {
		switch {
		case agent[i] < server[i]:
			return agentBehind
		case agent[i] > server[i]:
			return agentNewer
		}
	}
	return agentCurrent
}

func parseVersion(version string) ([3]int, bool) {
	var parsed [3]int
	parts := strings.Split(strings.TrimPrefix(version, "v"), ".")
	if len(parts) != 3 {
		return parsed, false
	}
	for i, part := range parts {
		number, err := strconv.Atoi(part)
		if err != nil || number < 0 {
			return parsed, false
		}
		parsed[i] = number
	}
	return parsed, true
}

// agentProblem fails check only for wiring that cannot work. An agent behind
// the server still reaches memory, so it is reported but not an error.
func agentProblem(reports []agentReport) error {
	var broken []string
	for _, report := range reports {
		if report.State == agentBroken {
			broken = append(broken, report.Agent)
		}
	}
	if len(broken) == 0 {
		return nil
	}
	return fmt.Errorf("%s points at a missing Grasshopper program; follow the fix listed above", strings.Join(broken, " and "))
}

var agentStateLabels = map[string]string{
	agentCurrent: "current",
	agentBehind:  "behind server",
	agentNewer:   "newer than server",
	agentUnknown: "unknown",
	agentBroken:  "broken",
}

// renderCheck prints the connection result and agent list. The agents are
// listed even when the server is unreachable, because local wiring is often
// the reason.
func renderCheck(w io.Writer, result map[string]string, agents []agentReport) error {
	connected := result["status"] == "connected"
	if connected {
		fmt.Fprintf(w, "Grasshopper connected. Device: %s. Registration: %s. %s\n", result["device"], result["registration"], result["next_step"])
	}
	if version := result["host_version"]; version != "" {
		fmt.Fprintf(w, "Server %s.\n", version)
	} else {
		fmt.Fprintln(w, "Server version unknown.")
	}
	if len(agents) == 0 {
		fmt.Fprintln(w, "No Grasshopper agents found on this machine.")
	} else {
		fmt.Fprintln(w, "\nAgents on this machine")
		// Notes go under the version column, so they cannot widen the table.
		nameWidth, versionWidth, stateWidth := 0, 1, 0
		for _, agent := range agents {
			nameWidth = max(nameWidth, len(agent.Agent))
			versionWidth = max(versionWidth, len(agent.Version))
			stateWidth = max(stateWidth, len(agentStateLabels[agent.State]))
		}
		for _, agent := range agents {
			version := agent.Version
			if version == "" {
				version = "-"
			}
			fmt.Fprintf(w, "  %-*s  %-*s  %-*s  %s\n", nameWidth, agent.Agent, versionWidth, version, stateWidth, agentStateLabels[agent.State], agent.Route)
			for _, note := range []string{agent.Detail, agent.Update} {
				if note != "" {
					fmt.Fprintf(w, "  %-*s  %s\n", nameWidth, "", note)
				}
			}
		}
	}
	if !connected {
		return errors.New(result["next_step"])
	}
	return agentProblem(agents)
}
