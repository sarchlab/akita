// Package backend reaches the hosts of akita exp: the local machine, SSH
// hosts, and SLURM clusters (over ssh or locally). A Backend syncs the
// project, launches and stops sessions with the workspace's log markers,
// runs akita and shell commands in a workspace, and pulls results.
package backend

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"strings"

	"github.com/sarchlab/akita/v5/experiment"
)

// LaunchSpec is a command to start as a session.
type LaunchSpec struct {
	Project string // project name (the project directory's base name)
	Name    string // session name
	Group   string // optional group
	Command string // shell command, run in the workspace
}

// Backend is how the tool reaches one host. Create one with New.
type Backend interface {
	// Host returns the host's configuration.
	Host() experiment.HostConfig

	// Workspace returns the workspace path of a project on the host
	// ("~" expanded on the host side).
	Workspace(project string) string

	// Sync copies the local project directory to the host's workspace
	// (no-op for the local host without a root).
	Sync(ctx context.Context, projectDir string, cfg *experiment.ProjectConfig) error

	// Launch starts a session; it fails if a live session has the name or
	// its tmux session name. It returns the SLURM job id of a batch job,
	// "" for tmux and nohup sessions.
	Launch(ctx context.Context, spec LaunchSpec) (job string, err error)

	// Stop stops a session and appends the "[stopped]" marker.
	Stop(ctx context.Context, project, name string) error

	// Akita runs `akita <args...>` on the host and returns its stdout.
	Akita(ctx context.Context, args ...string) ([]byte, error)

	// Shell runs a shell command in a project's workspace on the host and
	// returns its stdout (the status fallback without akita on the host).
	Shell(ctx context.Context, project, command string) ([]byte, error)

	// Stream runs a shell command in a project's workspace and streams its
	// output (`akita exp log`).
	Stream(ctx context.Context, project, command string, stdout, stderr io.Writer) error

	// Pull copies the workspace's logs/ and results/ to localDir.
	Pull(ctx context.Context, project, localDir string) error

	// StartAkita starts `akita <args...>` on the host in the background
	// (tmux session session when tmux is available, nohup setsid
	// otherwise), detached from this command. It fails if the tmux session
	// exists.
	StartAkita(ctx context.Context, session string, args ...string) error

	// PullRun copies a run's results/<name>/ (without the exclude
	// patterns) and its session files logs/<name>.* to localDir/results/
	// and localDir/logs/.
	PullRun(ctx context.Context, project, name, localDir string, exclude []string) error

	// Upload copies a local file to a path on the host ("~/" expanded
	// there) unless the path exists: uploads are named by content.
	Upload(ctx context.Context, localPath, hostPath string) error
}

// errNotRunning is returned by Stop for a session that is not alive.
var errNotRunning = errors.New("session is not running")

// New returns the backend of a host.
func New(h experiment.HostConfig) (Backend, error) {
	if h.Name == "" {
		return nil, errors.New("host without a name")
	}

	switch h.Type {
	case experiment.HostLocal, "":
		h.Type = experiment.HostLocal
		return newLocalBackend(h), nil
	case experiment.HostSSH:
		if h.Address == "" {
			return nil, fmt.Errorf("host %s: ssh host without an address", h.Name)
		}

		return newSSHBackend(h), nil
	case experiment.HostSlurm:
		return newSlurmBackend(h), nil
	default:
		return nil, fmt.Errorf("host %s: unknown type %q (want local, ssh, or slurm)",
			h.Name, h.Type)
	}
}

// ProbeScope is the directory holding a host's workspaces and, when only
// some of them belong to the tool, their names. The local host without a
// root knows only the project it was started in (its workspace is the
// project directory itself, whose siblings are other directories).
func ProbeScope(b Backend) (root string, projects []string) {
	if lb, ok := b.(*localBackend); ok && lb.host.Root == "" && lb.dir != "" {
		return filepath.Dir(lb.dir), []string{filepath.Base(lb.dir)}
	}

	return path.Dir(b.Workspace("_")), nil
}

// Root is the directory that holds a host's workspaces and its queue
// ("~/" kept for the host to expand).
func Root(b Backend) string {
	root, _ := ProbeScope(b)
	return root
}

// scriptError is a failed host command with its standard error.
type scriptError struct {
	Host   string
	Stderr string
	Err    error
}

func (e *scriptError) Error() string {
	msg := strings.TrimSpace(e.Stderr)
	if msg == "" {
		return fmt.Sprintf("%s: %v", e.Host, e.Err)
	}

	return fmt.Sprintf("%s: %s", e.Host, msg)
}

func (e *scriptError) Unwrap() error { return e.Err }
