package cmd

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/sarchlab/akita/v5/herd"
	"github.com/sarchlab/akita/v5/herd/backend"
	"github.com/sarchlab/akita/v5/herd/deploy"
	"github.com/sarchlab/akita/v5/herd/workspace"
	"github.com/spf13/cobra"
)

var herdDoctorCmd = &cobra.Command{
	Use:   "doctor [host...]",
	Short: "Check this machine and the hosts: tools, akita and its version, roots, agents.",
	Long: `Check what akita herd needs, on this machine and on each host (all hosts
by default): that the host answers (ssh without a password prompt), that
bash, rsync, and tmux (optional) are there, sbatch on SLURM hosts, that
akita is found and has the same version as here, that the root exists and
is writable (or can be created), and whether the agent of a host with a
queue runs. Each line is ok, warn (works, but not as well), or FAIL; the
exit status is 1 when a line FAILs.`,
	RunE: runDoctor,
}

func init() {
	herdCmd.AddCommand(herdDoctorCmd)
}

// doctor prints the result lines of the checks and counts the failures.
type doctor struct {
	env   *herdEnv
	fails int
}

func (d *doctor) linef(level, format string, args ...any) {
	if level == "FAIL" {
		d.fails++
	}

	d.env.printf("%-5s %s\n", level, fmt.Sprintf(format, args...))
}

func runDoctor(cmd *cobra.Command, args []string) error {
	env, err := loadHerdEnv(cmd)
	if err != nil {
		return err
	}

	bs, err := env.backends(args)
	if err != nil {
		return err
	}

	ctx, cancel := commandContext(cmd)
	defer cancel()

	d := &doctor{env: env}
	d.local(bs)

	for _, b := range bs {
		d.host(ctx, b)
	}

	if d.fails > 0 {
		return fmt.Errorf("%d checks failed", d.fails)
	}

	return nil
}

// local checks the tools this machine needs for the hosts and the project.
func (d *doctor) local(bs []backend.Backend) {
	d.env.printf("===== this machine =====\n")
	d.linef("ok", "akita %s", deploy.Version())

	need := map[string]string{"rsync": "sync and pull"}
	for _, b := range bs {
		switch h := b.Host(); {
		case h.Type == herd.HostSSH || h.Type == herd.HostSlurm && h.Address != "":
			need["ssh"] = "reaching " + h.Name
		case h.Type == herd.HostLocal:
			need["tmux"] = "sessions on " + h.Name + " (optional: nohup otherwise)"
		}
	}

	if cfg := d.env.projectCfg; cfg != nil && cfg.Build != nil {
		need["git"] = "akita herd build"
		need["go"] = "akita herd build (the build commands may need other tools)"
	}

	for _, tool := range []string{"rsync", "ssh", "git", "go", "tmux"} {
		why, ok := need[tool]
		if !ok {
			continue
		}

		switch _, err := exec.LookPath(tool); {
		case err == nil:
			d.linef("ok", "%s", tool)
		case tool == "tmux":
			d.linef("warn", "%s missing, for %s", tool, why)
		default:
			d.linef("FAIL", "%s missing, needed for %s", tool, why)
		}
	}
}

// hostScript prints "have <tool>" or "miss <tool>" for each tool, then
// "root <state>" for the root.
func hostScript(root string) string {
	s := "for t in bash rsync tmux sbatch; do if command -v $t >/dev/null 2>&1; " +
		"then echo \"have $t\"; else echo \"miss $t\"; fi; done\n"
	if root != "" {
		r := workspace.ShellPath(root)
		s += "if [ -d " + r + " ]; then if [ -w " + r + " ]; then echo 'root writable'; " +
			"else echo 'root read-only'; fi; else echo 'root missing'; fi\n"
	}

	return s
}

// host checks one host.
func (d *doctor) host(ctx context.Context, b backend.Backend) {
	h := b.Host()
	where := h.Type
	if h.Address != "" {
		where += " " + h.Address
	}

	d.env.printf("===== %s (%s) =====\n", h.Name, where)

	root := ""
	if h.Type != herd.HostLocal || h.Root != "" {
		root = backend.Root(b)
	}

	start := time.Now()

	// A host that asks for a password would wait for it forever.
	reach, cancel := context.WithTimeout(ctx, doctorReachTimeout)
	out, err := b.Script(reach, hostScript(root))

	cancel()

	if err != nil {
		if reach.Err() != nil {
			err = fmt.Errorf("no answer in %v", doctorReachTimeout)
		}

		d.linef("FAIL", "cannot run commands there: %s", firstLine(err))

		if h.Type != herd.HostLocal {
			d.env.printf("      check that `ssh %s true` works without a password prompt\n", sshTarget(h))
		}

		return
	}

	if h.Type != herd.HostLocal {
		d.linef("ok", "reachable (%.1fs)", time.Since(start).Seconds())
	}

	d.tools(h, string(out), root)
	d.akita(ctx, b)

	if h.Queue != nil && h.Type != herd.HostSlurm {
		d.agent(ctx, b)
	}
}

// doctorReachTimeout bounds the first command on a host.
const doctorReachTimeout = 20 * time.Second

// firstLine is the first line of an error (host errors carry stderr).
func firstLine(err error) string {
	s, _, _ := strings.Cut(strings.TrimSpace(err.Error()), "\n")
	return s
}

func sshTarget(h herd.HostConfig) string {
	if h.Address != "" {
		return h.Address
	}

	return h.Name
}

// tools reports the output of hostScript.
func (d *doctor) tools(h herd.HostConfig, out, root string) {
	have := map[string]bool{}

	for _, l := range strings.Split(out, "\n") {
		if t, ok := strings.CutPrefix(l, "have "); ok {
			have[t] = true
		}

		switch l {
		case "root writable":
			d.linef("ok", "root %s", root)
		case "root read-only":
			d.linef("FAIL", "root %s is not writable", root)
		case "root missing":
			d.linef("ok", "root %s does not exist yet; the first sync creates it", root)
		}
	}

	for _, t := range []string{"bash", "rsync"} {
		if !have[t] {
			d.linef("FAIL", "%s missing", t)
		}
	}

	switch {
	case h.Type == herd.HostSlurm && !have["sbatch"]:
		d.linef("FAIL", "sbatch missing")
	case h.Type != herd.HostSlurm && !have["tmux"]:
		d.linef("warn", "tmux missing: sessions run under nohup setsid")
	}
}

// akita checks that akita runs on the host, with this machine's version.
func (d *doctor) akita(ctx context.Context, b backend.Backend) {
	out, err := b.Akita(ctx, "herd", "_local", "version")

	switch here, there := deploy.Version(), strings.TrimSpace(string(out)); {
	case err != nil && strings.Contains(err.Error(), "unknown command"):
		d.linef("FAIL", "akita there is older than herd doctor; install the same akita as here (%s)", here)
	case err != nil:
		d.linef("FAIL", "akita not found or failed (%s); install it, or set \"akita\" in the host's config",
			firstLine(err))
	case here == "unknown" || there == "unknown":
		d.linef("warn", "akita runs there, but its version (%s) or this one (%s) is unknown", there, here)
	case there != here:
		d.linef("warn", "akita %s there, %s here; install the same version on both", there, here)
	default:
		d.linef("ok", "akita %s (same as here)", there)
	}
}

// agent reports whether the host's agent runs.
func (d *doctor) agent(ctx context.Context, b backend.Backend) {
	st, err := remoteAgent(ctx, b)

	switch {
	case err != nil:
		d.linef("FAIL", "agent status: %s", firstLine(err))
	case st.Alive && st.Agent != nil:
		d.linef("ok", "agent running (pid %d)", st.Agent.PID)
	default:
		d.linef("ok", "agent not running (submit starts it)")
	}
}
