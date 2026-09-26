//go:build windows

package main

import (
	"errors"
	"os"
	"os/exec"
	"os/signal"
)

// execReplace runs bin attached to this console and exits with its status.
// Windows has no exec(2), so this process waits for kiro-cli to finish.
func execReplace(bin string, argv []string) error {
	cmd := exec.Command(bin, argv[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	// Ctrl+C belongs to kiro-cli; don't let it kill the waiting parent.
	signal.Ignore(os.Interrupt)
	err := cmd.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		os.Exit(exitErr.ExitCode())
	}
	return err
}
