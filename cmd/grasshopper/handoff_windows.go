//go:build windows

package main

import (
	"errors"
	"os"
	"os/exec"
)

// handOff runs the managed client with this process's stdin, stdout and
// stderr, then exits with its status; Windows has no exec. It returns only if
// the managed client could not start.
func handOff(program string) error {
	command := exec.Command(program, os.Args[1:]...)
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	command.Env = append(os.Environ(), handedOffEnv+"="+clientVersion)
	err := command.Run()
	var exit *exec.ExitError
	if err == nil || errors.As(err, &exit) {
		os.Exit(command.ProcessState.ExitCode())
	}
	return err
}
