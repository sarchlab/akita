package backend

import (
	"context"
	"strconv"
	"strings"

	"github.com/sarchlab/akita/v5/experiment"
	ws "github.com/sarchlab/akita/v5/experiment/workspace"
)

// slurmBackend submits sessions as SLURM batch jobs, with sbatch on the
// host's address (over ssh) or on this machine when it has no address. The
// job runs the same wrapper as tmux sessions, so the log markers are the
// same; logs/<name>.job holds "<job id>\nslurm\n".
type slurmBackend struct {
	baseBackend
}

func newSlurmBackend(h experiment.HostConfig) *slurmBackend {
	b := &slurmBackend{baseBackend{host: h}}
	if h.Address == "" {
		b.runner = localRunner{}
	} else {
		b.runner = sshRunner{address: h.Address}
	}

	return b
}

// Sync rsyncs the project to the workspace (a no-op copy for a local
// SLURM host whose workspace is under its root).
func (b *slurmBackend) Sync(ctx context.Context, projectDir string, cfg *experiment.ProjectConfig) error {
	return syncRemote(ctx, &b.baseBackend, projectDir, cfg)
}

// Launch submits the session with sbatch and returns its job id.
func (b *slurmBackend) Launch(ctx context.Context, spec LaunchSpec) (string, error) {
	if err := ws.ValidateSessionName(spec.Name); err != nil {
		return "", err
	}

	script := sbatchScript(b.host, b.Workspace(spec.Project), spec)

	delim := "AKITA_EXP_SBATCH"
	for strings.Contains(script, delim) {
		delim += "_"
	}

	submit := "id=$(sbatch --parsable <<'" + delim + "'\n" + script + delim + "\n) || exit 1\n" +
		"id=${id%%;*}\n" +
		"printf '%s\\n" + ws.JobSlurm + "\\n' \"$id\" > " + ws.ShellQuote(ws.ScriptFile(spec.Name, ws.JobExt)) + "\n" +
		"echo \"$id\"\n"
	out, err := b.output(ctx, b.launchPrelude(spec)+submit)

	return strings.TrimSpace(string(out)), err
}

// sbatchScript is the batch script of a session: the host's #SBATCH options
// and the session wrapper.
func sbatchScript(h experiment.HostConfig, workspace string, spec LaunchSpec) string {
	var s strings.Builder

	s.WriteString("#!/bin/bash -l\n")
	s.WriteString("#SBATCH --job-name=" + ws.TmuxSessionName(spec.Project, spec.Name) + "\n")
	s.WriteString("#SBATCH --output=/dev/null\n")

	for _, o := range sbatchOptions(h.Slurm) {
		s.WriteString("#SBATCH " + o + "\n")
	}

	s.WriteString(ws.WrapperScript(h.Name, workspace, spec.Name, spec.Command))

	return s.String()
}

func sbatchOptions(c *experiment.SlurmConfig) []string {
	if c == nil {
		return nil
	}

	var opts []string

	add := func(flag, v string) {
		if v != "" {
			opts = append(opts, flag+"="+v)
		}
	}
	add("--partition", c.Partition)
	add("--account", c.Account)
	add("--time", c.Time)

	if c.CPUs > 0 {
		add("--cpus-per-task", strconv.Itoa(c.CPUs))
	}

	add("--mem", c.Mem)

	for _, e := range c.Extra {
		opts = append(opts, strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(e), "#SBATCH")))
	}

	return opts
}
