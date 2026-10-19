package probe

import (
	"context"
	"time"

	"github.com/sarchlab/akita/v5/experiment/internal/hostsys"
	"github.com/sarchlab/akita/v5/experiment/workspace"
)

// system is what the probe asks of the machine; tests replace it.
type system interface {
	workspace.LiveSystem

	tmuxPanes(ctx context.Context) (map[string]int, error)
	processes(ctx context.Context) *hostsys.ProcTable
	newestWritten(pid int) float64
	httpGet(ctx context.Context, url string) ([]byte, error)
	hostStats(ctx context.Context) hostsys.Stats
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
