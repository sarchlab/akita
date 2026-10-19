// Package hostsys is what akita herd asks of the machine it runs on: files
// read with a size limit and written atomically, commands run with a
// timeout, tmux sessions, processes, SLURM jobs, and host statistics. It
// knows nothing about workspaces.
package hostsys

import (
	"bytes"
	"context"
	"os/exec"
	"time"
)

// commandTimeout bounds the commands this package runs.
const commandTimeout = 10 * time.Second

// runQuiet runs a command with commandTimeout and returns its stdout ("" on
// any failure without output).
func runQuiet(ctx context.Context, name string, args ...string) string {
	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()

	var stdout bytes.Buffer

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil && stdout.Len() == 0 {
		return ""
	}

	return stdout.String()
}
