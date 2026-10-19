package backend

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sarchlab/akita/v5/herd"
)

// localBackend runs sessions on this machine. Without "root" in its host
// config, a project's workspace is the project directory itself (the
// current directory when the backend was created, or its sibling of the
// project's name); with "root", it is <root>/<project> and Sync copies the
// project there.
type localBackend struct {
	baseBackend

	dir string // the current directory when the backend was created
}

func newLocalBackend(h herd.HostConfig) *localBackend {
	b := &localBackend{baseBackend: baseBackend{host: h, runner: localRunner{}}}
	b.dir, _ = os.Getwd()
	b.workspace = b.localWorkspace

	return b
}

func (b *localBackend) localWorkspace(project string) string {
	switch {
	case b.host.Root != "":
		return filepath.Join(herd.ExpandHome(b.host.Root), project)
	case b.dir == "":
		return project
	case filepath.Base(b.dir) == project:
		return b.dir
	default:
		return filepath.Join(filepath.Dir(b.dir), project)
	}
}

// Sync copies the project to <root>/<project>; it does nothing when the
// workspace is the project directory.
func (b *localBackend) Sync(ctx context.Context, projectDir string, cfg *herd.ProjectConfig) error {
	if b.host.Root == "" {
		return nil
	}

	wsp := b.Workspace(projectBase(projectDir))
	if err := os.MkdirAll(wsp, 0o755); err != nil {
		return err
	}

	return rsyncProject(ctx, projectDir, wsp, cfg)
}

// Pull copies logs/ and results/ unless the workspace is the project.
func (b *localBackend) Pull(ctx context.Context, project, localDir string) error {
	if b.host.Root == "" {
		return nil
	}

	return b.baseBackend.Pull(ctx, project, localDir)
}

// localRunner runs scripts with the local bash.
type localRunner struct{}

func (localRunner) run(ctx context.Context, script string, stdout, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, "bash", "-s")
	cmd.Stdin = strings.NewReader(wrapStdin(script))
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	return cmd.Run()
}

func (localRunner) rsyncPath(p string) string { return herd.ExpandHome(p) }
