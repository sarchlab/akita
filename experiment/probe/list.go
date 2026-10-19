package probe

import (
	"context"
	"path/filepath"
	"time"

	"github.com/sarchlab/akita/v5/experiment"
)

// ListSessions lists the sessions of one workspace of this machine (a
// light probe): their state and the last tailLines lines of their logs,
// with archived ones when archived is true; a died SLURM job's final state
// is added to its tail. Process trees, monitors, and health are not read.
// project names the tmux sessions (default: the workspace's name). It
// fails when the tmux sessions cannot be listed.
func ListSessions(ctx context.Context, wsDir, project string, archived bool, tailLines int,
) ([]experiment.Session, error) {
	wsDir = experiment.ExpandHome(wsDir)
	if project == "" {
		project = filepath.Base(wsDir)
	}

	opts := Options{now: time.Now()}
	opts.fill()

	p := newProber(ctx, opts)
	p.light, p.tailLines, p.withArchived = true, tailLines, archived
	p.workspaces = []wsRef{{project: project, dir: wsDir}}

	return p.sessions()
}
