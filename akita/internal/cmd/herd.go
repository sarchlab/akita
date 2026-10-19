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

	"github.com/sarchlab/akita/v5/herd"
	"github.com/sarchlab/akita/v5/herd/backend"
	"github.com/spf13/cobra"
)

var herdCmd = &cobra.Command{
	Use: "herd",
	// "exp" is the command's name before the rename; it keeps scripts and
	// hosts that still call "akita exp ..." working.
	Aliases: []string{"exp"},
	Short:   "Herd: run and monitor experiments on local, SSH, and SLURM hosts.",
	Long: `Herd runs experiments on the local machine, SSH hosts, or SLURM
clusters, and monitors them in a web UI.

Run akita herd from the root of a project. Hosts are configured in
~/.config/akita/herd.json (or $AKITA_HERD_CONFIG); the project may describe
its result names, simulators, campaigns, and plots in ./herd.json.
` + "`akita herd init`" + ` writes both. Before the rename the tool was
akita exp: ~/.config/akita/exp.json, $AKITA_EXP_CONFIG, and ./exp.json are
still read when the new names do not exist.

On SSH and SLURM hosts, a project lives in <root>/<project> (root defaults
to ~/exp): logs/<name>.log holds a session's output, results/ the results.`,
	// Errors of herd commands are not usage errors; keep the output short.
	PersistentPreRun: func(cmd *cobra.Command, _ []string) { cmd.SilenceUsage = true },
}

// herdEnv is what most herd commands need: the user config and the project
// in the current directory.
type herdEnv struct {
	user       *herd.UserConfig
	projectDir string
	project    string
	projectCfg *herd.ProjectConfig
	out        io.Writer
	errOut     io.Writer
}

func loadHerdEnv(cmd *cobra.Command) (*herdEnv, error) {
	user, err := herd.LoadUserConfig()
	if err != nil {
		return nil, fmt.Errorf("user config %s: %w", herd.UserConfigPath(), err)
	}

	dir, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	pcfg, err := herd.LoadProjectConfig(dir)
	if err != nil {
		return nil, err
	}

	return &herdEnv{
		user: user, projectDir: dir, project: filepath.Base(dir), projectCfg: pcfg,
		out: cmd.OutOrStdout(), errOut: cmd.ErrOrStderr(),
	}, nil
}

// backend returns the backend of a configured host.
func (e *herdEnv) backend(name string) (backend.Backend, error) {
	h, ok := e.user.Host(name)
	if !ok {
		return nil, fmt.Errorf("unknown host %q (configured: %s; see %s)",
			name, e.hostNames(), herd.UserConfigPath())
	}

	return backend.New(h)
}

func (e *herdEnv) hostNames() string {
	names := make([]string, len(e.user.Hosts))
	for i, h := range e.user.Hosts {
		names[i] = h.Name
	}

	return strings.Join(names, ", ")
}

// backends returns the backends of the named hosts, or of all hosts.
func (e *herdEnv) backends(names []string) ([]backend.Backend, error) {
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

func (e *herdEnv) printf(format string, args ...any) {
	fmt.Fprintf(e.out, format, args...)
}

// isProjectWorkspace tells whether a backend's workspace is the project
// directory itself (the local host without a root).
func (e *herdEnv) isProjectWorkspace(b backend.Backend) bool {
	return b.Host().Type == herd.HostLocal && b.Host().Root == ""
}

// commandContext is cancelled by Ctrl+C.
func commandContext(cmd *cobra.Command) (context.Context, context.CancelFunc) {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	return signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
}

// getenvLegacy returns the environment variable name, or legacy (its name
// before the rename) when name is unset or empty.
func getenvLegacy(name, legacy string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}

	return os.Getenv(legacy)
}

// akitaOut runs akita on a host and copies its output, also on failure.
func akitaOut(ctx context.Context, b backend.Backend, out io.Writer, args ...string) error {
	data, err := b.Akita(ctx, args...)
	_, _ = out.Write(data)

	return err
}
