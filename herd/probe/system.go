package probe

import (
	"context"
	"os"
	"time"

	"github.com/sarchlab/akita/v5/herd/internal/hostsys"
	"github.com/sarchlab/akita/v5/herd/workspace"
)

// system is what the probe asks of the machine; tests replace it.
type system interface {
	workspace.LiveSystem

	tmuxPanes(ctx context.Context) (map[string]int, error)
	processes(ctx context.Context) *hostsys.ProcTable
	newestWritten(pid int) float64
	httpGet(ctx context.Context, url string) ([]byte, error)
	hostStats(ctx context.Context) hostsys.Stats

	// uid is the user the probe runs as; procOwner, procCwd, and
	// procStartedByHerd describe a process (see hostsys).
	uid() int
	procOwner(pid int) int
	procCwd(pid int) string
	procStartedByHerd(pid int) bool
}

// realSystem is the machine the probe runs on.
type realSystem struct {
	workspace.HostSystem

	monitorTimeout time.Duration
}

func (s *realSystem) tmuxPanes(ctx context.Context) (map[string]int, error) {
	return hostsys.TmuxPanes(ctx)
}

func (s *realSystem) processes(ctx context.Context) *hostsys.ProcTable {
	return hostsys.Processes(ctx)
}

func (s *realSystem) newestWritten(pid int) float64 { return hostsys.NewestWritten(pid) }

func (s *realSystem) httpGet(ctx context.Context, url string) ([]byte, error) {
	return hostsys.HTTPGet(ctx, url, s.monitorTimeout)
}

func (s *realSystem) hostStats(ctx context.Context) hostsys.Stats { return hostsys.HostStats(ctx) }

func (s *realSystem) uid() int                       { return os.Getuid() }
func (s *realSystem) procOwner(pid int) int          { return hostsys.ProcOwner(pid) }
func (s *realSystem) procCwd(pid int) string         { return hostsys.ProcCwd(pid) }
func (s *realSystem) procStartedByHerd(pid int) bool { return hostsys.ProcStartedByHerd(pid) }
