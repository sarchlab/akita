package hostsys

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// TmuxPanes maps the name of each tmux session of this machine to the pid
// of its first pane. It is empty, without an error, when tmux is not
// installed or no tmux server runs.
func TmuxPanes(ctx context.Context) (map[string]int, error) {
	panes := map[string]int{}

	if _, err := exec.LookPath("tmux"); err != nil {
		return panes, nil
	}

	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()

	var stdout, stderr bytes.Buffer

	cmd := exec.CommandContext(ctx, "tmux", "list-panes", "-a", "-F", "#{session_name}|#{pane_pid}")
	cmd.Stdout, cmd.Stderr = &stdout, &stderr

	if err := cmd.Run(); err != nil {
		msg := stderr.String()
		for _, s := range []string{"no server running", "error connecting", "No such file"} {
			if strings.Contains(msg, s) {
				return panes, nil
			}
		}

		return nil, fmt.Errorf("tmux list-panes: %w: %s", err, strings.TrimSpace(msg))
	}

	for _, line := range strings.Split(stdout.String(), "\n") {
		name, pid, ok := strings.Cut(line, "|")
		if !ok {
			continue
		}

		if n, err := strconv.Atoi(strings.TrimSpace(pid)); err == nil {
			if _, dup := panes[name]; !dup {
				panes[name] = n
			}
		}
	}

	return panes, nil
}
