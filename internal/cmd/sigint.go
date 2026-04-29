// Package cmd provides signal-safe subprocess execution utilities.
package cmd

import (
	"os"
	"os/exec"
)

// RunInteractive runs a command with stdin/stdout/stderr connected to the terminal.
// The child inherits the parent's process group so that Ctrl+C (SIGINT) is
// delivered to both the Go process and the child process, allowing clean
// cancellation of long-running operations like "hf download" or "docker logs -f".
//
// The caller is responsible for setting cmd.Env before calling this function.
func RunInteractive(cmd *exec.Cmd) error {
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
