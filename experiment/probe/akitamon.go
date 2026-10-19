package probe

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/sarchlab/akita/v5/experiment"
	"github.com/sarchlab/akita/v5/experiment/workspace"
)

// maxMonitors bounds the Akita monitors read per session (the newest).
const maxMonitors = 4

var monitorRe = regexp.MustCompile(`Monitoring simulation with (http://localhost:\d+)`)

// akitaBar is monitoring.ProgressBar as /api/progress encodes it.
type akitaBar struct {
	ID         uint64    `json:"id"`
	Name       string    `json:"name"`
	StartTime  time.Time `json:"start_time"`
	Total      uint64    `json:"total"`
	Finished   uint64    `json:"finished"`
	InProgress uint64    `json:"in_progress"`
}

// attachMonitors reads the progress bars of the Akita monitors the
// session's log announces.
func (p *prober) attachMonitors(s *experiment.Session, lc *workspace.Log) {
	seen := map[string]bool{}

	var urls []string

	for _, text := range []string{lc.Head, lc.Tail} {
		for _, m := range monitorRe.FindAllStringSubmatch(text, -1) {
			if !seen[m[1]] {
				seen[m[1]] = true
				urls = append(urls, m[1])
			}
		}
	}

	if len(urls) > maxMonitors {
		urls = urls[len(urls)-maxMonitors:]
	}

	for _, u := range urls {
		bars, ok := p.monitorBars(u)
		if !ok {
			continue
		}

		s.Progress = append(s.Progress, bars...)
		p.attachMonitorSim(s, u)
	}
}

func (p *prober) monitorBars(base string) ([]experiment.Progress, bool) {
	data, err := p.opts.sys.httpGet(p.ctx, base+"/api/progress")
	if err != nil {
		return nil, false
	}

	var raw []akitaBar
	if json.Unmarshal(data, &raw) != nil {
		return nil, false
	}

	port := base[strings.LastIndex(base, ":")+1:]
	bars := make([]experiment.Progress, 0, len(raw))

	for _, b := range raw {
		bar := experiment.Progress{
			ID:   fmt.Sprintf("%s%s:%d", experiment.ProgressPrefixAkita, port, b.ID),
			Name: b.Name, Finished: b.Finished, InProgress: b.InProgress,
		}

		if b.Total > 0 {
			bar.Total = new(b.Total)
		}

		if !b.StartTime.IsZero() && b.StartTime.Year() > 1 {
			bar.StartTime = new(experiment.UnixSeconds(b.StartTime))
		}

		bars = append(bars, bar)
	}

	return bars, true
}

// attachMonitorSim records the monitor URL on the session's simulator, or
// adds a simulator entry for it.
func (p *prober) attachMonitorSim(s *experiment.Session, url string) {
	var free []int

	for i := range s.Sims {
		if s.Sims[i].Monitor == "" {
			free = append(free, i)
		}
	}

	if len(free) == 1 {
		s.Sims[free[0]].Monitor = url
		return
	}

	sim := experiment.Simulator{Monitor: url}

	if data, err := p.opts.sys.httpGet(p.ctx, url+"/api/execution/info"); err == nil {
		var props []experiment.Property
		if json.Unmarshal(data, &props) == nil {
			for _, pr := range props {
				if pr.Property == "Command" {
					sim.Label = truncate(pr.Value, maxProcArgs)
				}
			}
		}
	}

	s.Sims = append(s.Sims, sim)
}
