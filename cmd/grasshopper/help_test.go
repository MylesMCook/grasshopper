package main

import (
	"os"
	"strings"
	"testing"
)

func TestHumanHelpWithoutPackage(t *testing.T) {
	before := os.Args
	t.Cleanup(func() { os.Args = before })
	for _, args := range [][]string{{}, {"--help"}, {"help"}, {"connect", "--help"}, {"setup", "--help"}, {"check", "--help"}, {"configure", "--help"}, {"claude", "--help"}, {"claude", "remove", "--help"}, {"cursor", "--help"}, {"cursor", "remove", "--help"}, {"bridge", "--help"}, {"hook", "--help"}} {
		os.Args = append([]string{"grasshopper"}, args...)
		if err := run(); err != nil {
			t.Errorf("%s: %v", strings.Join(args, " "), err)
		}
	}
}
