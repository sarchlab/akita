package probe

import (
	"math"
	"path/filepath"
	"strings"

	"github.com/sarchlab/akita/v5/experiment"
	"github.com/sarchlab/akita/v5/experiment/internal/hostsys"
	"github.com/sarchlab/akita/v5/experiment/workspace"
)

// simulatorOf recognises a simulator process with the project's
// simulator specs.
func simulatorOf(cfg *experiment.ProjectConfig, pid int, info *hostsys.ProcInfo) (experiment.Simulator, bool) {
	if cfg == nil {
		return experiment.Simulator{}, false
	}

	parts := strings.Fields(info.Args)
	if len(parts) == 0 {
		return experiment.Simulator{}, false
	}

	for _, spec := range cfg.Simulators {
		if !spec.Matches(parts) {
			continue
		}

		label, _ := workspace.FlagValue(parts, spec.LabelFlag)

		sim := experiment.Simulator{
			PID: pid, Label: label, Paused: strings.HasPrefix(info.Stat, "T"),
			Etime: info.Etime, CPU: cpuPercent(info), RSSMB: math.Round(info.RSSKB / kbPerMB),
		}

		if out, ok := workspace.FlagValue(parts, spec.OutputFlag); ok {
			sim.Campaign, sim.Config = outputTarget(out, spec.Executable, label)
		}

		return sim, true
	}

	return experiment.Simulator{}, false
}

// outputTarget derives the run (campaign) directory and the cell config
// from a simulator's output path: results/<run>/stats/<file> names run
// <run>; results/<campaign>/<exe>_<label>_<config>_metrics names the
// campaign and the config.
func outputTarget(out, exe, label string) (campaign, config string) {
	dir, stem := filepath.Split(out)
	dir = filepath.Clean(dir)

	if filepath.Base(dir) == "stats" {
		return filepath.Base(filepath.Dir(dir)), ""
	}

	stem = strings.TrimSuffix(stem, filepath.Ext(stem))
	stem = strings.TrimSuffix(stem, "_metrics")

	for _, prefix := range []string{exe + "_" + label + "_", label + "_"} {
		if c, ok := strings.CutPrefix(stem, prefix); ok && label != "" {
			return filepath.Base(dir), c
		}
	}

	return filepath.Base(dir), stem
}
