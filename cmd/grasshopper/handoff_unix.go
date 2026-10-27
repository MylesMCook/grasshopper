//go:build !windows

package main

import (
	"os"
	"syscall"
)

// handOff replaces this process with the managed client, keeping the agent's
// stdin, stdout and stderr. It returns only if the exec fails.
func handOff(program string) error {
	return syscall.Exec(program, append([]string{program}, os.Args[1:]...), append(os.Environ(), handedOffEnv+"="+clientVersion))
}
