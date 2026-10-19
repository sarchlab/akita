package backend

import (
	"context"
	"io"
	"os/exec"
	"strings"

	"github.com/sarchlab/akita/v5/experiment"
	ws "github.com/sarchlab/akita/v5/experiment/workspace"
)

// sshBackend runs sessions on a host reached with ssh; the workspace is
// <root>/<project> there (root defaults to ~/exp).
type sshBackend struct {
	baseBackend
}

func newSSHBackend(h experiment.HostConfig) *sshBackend {
	return &sshBackend{baseBackend{host: h, runner: sshRunner{address: h.Address}}}
}

// Sync rsyncs the project to the workspace.
func (b *sshBackend) Sync(ctx context.Context, projectDir string, cfg *experiment.ProjectConfig) error {
	return syncRemote(ctx, &b.baseBackend, projectDir, cfg)
}

// syncRemote creates the workspace and rsyncs the project into it.
func syncRemote(ctx context.Context, b *baseBackend, projectDir string, cfg *experiment.ProjectConfig) error {
	wsp := b.Workspace(projectBase(projectDir))
	if _, err := b.output(ctx, "mkdir -p "+ws.ShellPath(wsp)); err != nil {
		return err
	}

	return rsyncProject(ctx, projectDir, b.runner.rsyncPath(wsp), cfg)
}

// sshRunner runs scripts with `ssh <address> bash -s`.
type sshRunner struct {
	address string
}

func (r sshRunner) run(ctx context.Context, script string, stdout, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, "ssh", r.address, "bash -s")
	cmd.Stdin = strings.NewReader(wrapStdin(script))
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	return cmd.Run()
}

// rsyncPath is "<address>:<path>", relative to the home directory for
// paths under "~/".
func (r sshRunner) rsyncPath(p string) string {
	switch {
	case p == "~":
		p = "."
	case strings.HasPrefix(p, "~/"):
		p = p[2:]
	}

	return r.address + ":" + p
}
