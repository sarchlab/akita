package probe

import (
	"github.com/sarchlab/akita/v5/experiment"
	"github.com/sarchlab/akita/v5/experiment/workspace"
)

// heartbeatBarID is the ID of the bar of a legacy wgs_completed counter.
const heartbeatBarID = "wg"

// heartbeatBars turns a running session's heartbeat into progress bars:
// its "progress" list, or the legacy "wgs_completed" counter with the
// "-max-wg" argument as the total (start is the session's start).
func heartbeatBars(hb *workspace.Heartbeat, start *float64) []experiment.Progress {
	switch {
	case hb == nil:
		return nil
	case hb.HasBars:
		return hb.Bars
	case hb.WGsCompleted == nil || hb.Finished:
		return nil
	}

	bar := experiment.Progress{
		ID: heartbeatBarID, Name: "WG", StartTime: start, Finished: uint64(max(*hb.WGsCompleted, 0)),
	}
	if total := hb.MaxWG(); total > 0 {
		bar.Total = new(uint64(total))
	}

	return []experiment.Progress{bar}
}
