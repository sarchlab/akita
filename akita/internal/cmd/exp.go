package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/sarchlab/akita/v5/experiment"
	"github.com/sarchlab/akita/v5/experiment/backend"
	"github.com/spf13/cobra"
)

var expCmd = &cobra.Command{
	Use:   "exp",
	Short: "Run and monitor experiments on local, SSH, and SLURM hosts.",
	Long: `Run experiments on the local machine, SSH hosts, or SLURM clusters, and
monitor them in a web UI.

Run akita exp from the root of a project. Hosts are configured in
~/.config/akita/exp.json (or $AKITA_EXP_CONFIG); the project may describe
its result names, simulators, campaigns, and plots in ./exp.json.
` + "`akita exp init`" + ` writes both.

On SSH and SLURM hosts, a project lives in <root>/<project> (root defaults
to ~/exp): logs/<name>.log holds a session's output, results/ the results.`,
	// Errors of exp commands are not usage errors; keep the output short.
	PersistentPreRun: func(cmd *cobra.Command, _ []string) { cmd.SilenceUsage = true },
}

// expEnv is what most exp commands need: the user config and the project
// in the current directory.
type expEnv struct {
	user       *experiment.UserConfig
	projectDir string
	project    string
	projectCfg *experiment.ProjectConfig
	out        io.Writer
	errOut     io.Writer
}

func loadExpEnv(cmd *cobra.Command) (*expEnv, error) {
	user, err := experiment.LoadUserConfig()
	if err != nil {
		return nil, fmt.Errorf("user config %s: %w", experiment.UserConfigPath(), err)
	}

	dir, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	pcfg, err := experiment.LoadProjectConfig(dir)
	if err != nil {
		return nil, err
	}

	return &expEnv{
		user: user, projectDir: dir, project: filepath.Base(dir), projectCfg: pcfg,
		out: cmd.OutOrStdout(), errOut: cmd.ErrOrStderr(),
	}, nil
}

// backend returns the backend of a configured host.
func (e *expEnv) backend(name string) (backend.Backend, error) {
	h, ok := e.user.Host(name)
	if !ok {
		return nil, fmt.Errorf("unknown host %q (configured: %s; see %s)",
			name, e.hostNames(), experiment.UserConfigPath())
	}

	return backend.New(h)
}

func (e *expEnv) hostNames() string {
	names := make([]string, len(e.user.Hosts))
	for i, h := range e.user.Hosts {
		names[i] = h.Name
	}

	return strings.Join(names, ", ")
}

// backends returns the backends of the named hosts, or of all hosts.
func (e *expEnv) backends(names []string) ([]backend.Backend, error) {
	if len(names) == 0 {
		for _, h := range e.user.Hosts {
			names = append(names, h.Name)
		}
	}

	out := make([]backend.Backend, 0, len(names))

	for _, n := range names {
		b, err := e.backend(n)
		if err != nil {
			return nil, err
		}

		out = append(out, b)
	}

	return out, nil
}

func (e *expEnv) printf(format string, args ...any) {
	fmt.Fprintf(e.out, format, args...)
}

// isProjectWorkspace tells whether a backend's workspace is the project
// directory itself (the local host without a root).
func (e *expEnv) isProjectWorkspace(b backend.Backend) bool {
	return b.Host().Type == experiment.HostLocal && b.Host().Root == ""
}

// commandContext is cancelled by Ctrl+C.
func commandContext(cmd *cobra.Command) (context.Context, context.CancelFunc) {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	return signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
}

// akitaOut runs akita on a host and copies its output, also on failure.
func akitaOut(ctx context.Context, b backend.Backend, out io.Writer, args ...string) error {
	data, err := b.Akita(ctx, args...)
	_, _ = out.Write(data)

	return err
}
