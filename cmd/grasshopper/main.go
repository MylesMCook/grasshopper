// grasshopper is the stateless client for Codex, Cursor, and Claude Code.
// It never stores memory locally.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/MylesMCook/grasshopper/internal/goclient"
)

func run() error {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Fprintln(os.Stdout, "grasshopper 2.1.0")
		return nil
	}
	if len(os.Args) == 2 && os.Args[1] == "config-path" {
		path, err := goclient.ConfigPath("")
		if err != nil {
			return err
		}
		fmt.Fprintln(os.Stdout, path)
		return nil
	}
	if len(os.Args) < 2 {
		return errors.New("use bridge or hook")
	}
	switch os.Args[1] {
	case "configure":
		return configure(os.Args[2:])
	case "check":
		return checkConnection(os.Args[2:])
	case "cursor":
		return cursorCommand(os.Args[2:])
	case "bridge":
		flags := flag.NewFlagSet("bridge", flag.ContinueOnError)
		config := flags.String("config", "", "client configuration path")
		if err := flags.Parse(os.Args[2:]); err != nil {
			return err
		}
		path, err := goclient.ConfigPath(*config)
		if err != nil {
			return err
		}
		return goclient.Bridge(context.Background(), path, os.Stdin, os.Stdout)
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
		path, err := goclient.ConfigPath(*config)
		if err != nil {
			return err
		}
		input, err := goclient.ParseHookInput(os.Stdin)
		if err != nil {
			return err
		}
		var output map[string]any
		if *globalPart != 0 {
			if *harness != "claude" {
				return errors.New("global guidance parts are Claude-only")
			}
			output, err = goclient.HookGlobalPart(*globalPart, input)
		} else {
			output, err = goclient.Hook(path, *harness, input)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "Grasshopper hook unavailable; no persistence acknowledged")
			return nil
		}
		return json.NewEncoder(os.Stdout).Encode(output)
	default:
		return errors.New("use configure, check, cursor, bridge, hook, or config-path")
	}
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Grasshopper client:", err)
		os.Exit(1)
	}
}
