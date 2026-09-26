//go:build !windows

package main

import (
	"os"
	"syscall"
)

// execReplace replaces this process with bin, so kiro-cli owns the terminal.
func execReplace(bin string, argv []string) error {
	return syscall.Exec(bin, argv, os.Environ())
}
