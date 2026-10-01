// Grasshopper is the stateless client for Codex, Cursor, and Claude Code.
// It never stores memory locally.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/MylesMCook/grasshopper/internal/client"
)

var clientVersion = "dev"

func run() (resultErr error) {
	defer func() {
		if errors.Is(resultErr, flag.ErrHelp) {
			resultErr = nil
		}
	}()
	if len(os.Args) < 2 || os.Args[1] == "--help" || os.Args[1] == "-h" || os.Args[1] == "help" {
		fmt.Fprintln(os.Stdout, `Grasshopper shares memories through your private server.

Usage: grasshopper COMMAND [options]

  connect        Connect this machine; request owner approval when needed
  check          Check the saved connection without changing it
  setup          Install agents from an extracted client archive
  claude remove  Remove local Claude Code connector wiring
  cursor remove  Remove local Cursor connector wiring
  configure      Configure an existing private device token manually
  config-path    Print this machine's client configuration path

For agents: bridge runs the MCP transport; hook supplies startup context.
Use grasshopper COMMAND --help for options, or --version for the version.`)
		return nil
	}
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Fprintln(os.Stdout, "grasshopper "+clientVersion)
		return nil
	}
	if len(os.Args) == 2 && os.Args[1] == "config-path" {
		path, err := client.ConfigPath("")
		if err != nil {
			return err
		}
		fmt.Fprintln(os.Stdout, path)
		return nil
	}
	switch os.Args[1] {
	case "connect":
		return connectClient(os.Args[2:])
	case "setup":
		return setupClient(os.Args[2:])
	case "configure":
		return configure(os.Args[2:])
	case "check":
		return checkConnection(os.Args[2:])
	case "cursor":
		return cursorCommand(os.Args[2:])
	case "claude":
		return claudeCommand(os.Args[2:])
	case "bridge":
		flags := flag.NewFlagSet("bridge", flag.ContinueOnError)
		config := flags.String("config", "", "client configuration path")
		if err := flags.Parse(os.Args[2:]); err != nil {
			return err
		}
		path, err := client.ConfigPath(*config)
		if err != nil {
			return err
		}
		return client.Bridge(context.Background(), path, os.Stdin, os.Stdout)
	case "hook":
		flags := flag.NewFlagSet("hook", flag.ContinueOnError)
		config := flags.String("config", "", "client configuration path")
		harness := flags.String("harness", "", "codex, cursor, or claude")
		globalPart := flags.Int("global-part", 0, "Claude user AGENTS.md part (1 or 2)")
		if err := flags.Parse(os.Args[2:]); err != nil {
			return err
		}
		if *harness == "" {
			return errors.New("hook needs --harness")
		}
		path, err := client.ConfigPath(*config)
		if err != nil {
			return err
		}
		input, err := client.ParseHookInput(os.Stdin)
		if err != nil {
			return err
		}
		var output map[string]any
		if *globalPart != 0 {
			if *harness != "claude" {
				return errors.New("global guidance parts are Claude-only")
			}
			output, err = client.HookGlobalPart(*globalPart, input)
		} else {
			output, err = client.Hook(path, *harness, input)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "Grasshopper hook unavailable; no persistence acknowledged")
			return json.NewEncoder(os.Stdout).Encode(client.HookUnavailable(*harness, input))
		}
		return json.NewEncoder(os.Stdout).Encode(output)
	default:
		return errors.New("use connect, setup, configure, check, cursor, claude, bridge, hook, or config-path")
	}
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Grasshopper client:", err)
		os.Exit(1)
	}
}
