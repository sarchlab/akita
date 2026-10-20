package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/sarchlab/akita/v5/herd"
	"github.com/sarchlab/akita/v5/herd/backend"
	"github.com/sarchlab/akita/v5/herd/probe"
	"github.com/spf13/cobra"
)

var herdHostCmd = &cobra.Command{
	Use:   "host",
	Short: "Look at a server by its address, and add it to the hosts.",
}

var herdHostScanCmd = &cobra.Command{
	Use:   "scan <address|host>",
	Short: "Show what runs on a server: Herd workspaces and simulations started by hand.",
	Long: `Connect to a server (an ssh address, or a configured host), put akita
there if it has none (see akita herd doctor), and show the machine, the
directories under the home directory that hold workspaces of Herd (or of
the former exp tool), and the user's simulations that no Herd session
started (busy processes, with their directory and the script that started
them). It changes nothing on the server.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		env, err := loadHerdEnv(cmd)
		if err != nil {
			return err
		}

		ctx, cancel := commandContext(cmd)
		defer cancel()

		h, configured := hostFor(env, args[0])
		if dir, _ := cmd.Flags().GetString("akita-dir"); dir != "" && !configured {
			h.AkitaDir = dir
		}

		scan, err := scanHost(ctx, h)
		if err != nil {
			return err
		}

		printScan(env.out, h, scan)

		return nil
	},
}

var herdHostAddCmd = &cobra.Command{
	Use:   "add <address> [--name N] [--root DIR] [--yes]",
	Short: "Scan a server and add it to the user config.",
	Long: `Scan a server (see akita herd host scan), then propose a host entry for
the user config: the name (default: the address without the user and the
domain), the address, the root (default: ~/exp, or the busiest directory
of workspaces the scan found), and a queue sized to the machine (half the
CPUs; a tenth of the memory kept free). After you confirm (or with --yes),
the entry is appended to the user config, whose previous version is kept
as <config>.bak. Other roots the scan found are listed with the command
that adds them as hosts of their own.`,
	Args: cobra.ExactArgs(1),
	RunE: runHostAdd,
}

var herdLocalScanCmd = &cobra.Command{
	Use:   "scan [--root DIR]...",
	Short: "Print what akita herd host scan shows about this machine, as JSON.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		roots, _ := cmd.Flags().GetStringArray("root")
		return writeJSONTo(cmd.OutOrStdout(), probe.Scan(cmd.Context(), roots))
	},
}

func init() {
	herdHostAddCmd.Flags().String("name", "", "name of the host (default: from the address)")
	herdHostAddCmd.Flags().String("root", "", "root of the workspaces on the host (default: from the scan)")
	herdHostAddCmd.Flags().BoolP("yes", "y", false, "add without asking")

	for _, c := range []*cobra.Command{herdHostScanCmd, herdHostAddCmd} {
		c.Flags().String("akita-dir", "", "where to put akita on the host (default "+backend.DefaultAkitaDir+")")
	}

	herdLocalScanCmd.Flags().StringArray("root", nil, "a root to look at besides the usual places")
	herdHostCmd.AddCommand(herdHostScanCmd, herdHostAddCmd)
	herdLocalCmd.AddCommand(herdLocalScanCmd)
	herdCmd.AddCommand(herdHostCmd)
}

// hostFor is the configured host named name, or an ssh host at the
// address name; ok tells which.
func hostFor(env *herdEnv, name string) (herd.HostConfig, bool) {
	if h, ok := env.user.Host(name); ok {
		return h, true
	}

	return herd.HostConfig{Name: herd.HostNameFor(name), Type: herd.HostSSH, Address: name}, false
}

// scanHost scans a host (see backend.Scan).
func scanHost(ctx context.Context, h herd.HostConfig) (*herd.HostScan, error) {
	b, err := backend.New(h)
	if err != nil {
		return nil, err
	}

	return backend.Scan(ctx, b)
}

func printScan(w io.Writer, h herd.HostConfig, s *herd.HostScan) {
	load := ""
	if len(s.Load) > 0 {
		load = fmt.Sprintf(", load %.1f", s.Load[0])
	}

	fmt.Fprintf(w, "===== %s (%s): %s, %s/%s, %d CPUs, %.0f GB memory (%.0f GB free)%s =====\n",
		h.Name, dash(h.Address), s.Hostname, s.OS, s.Arch, s.CPUs, s.MemTotalGB, s.MemAvailGB, load)

	if !s.Tmux {
		fmt.Fprintln(w, "(no tmux: sessions will run under nohup setsid)")
	}

	if len(s.Roots) == 0 {
		fmt.Fprintln(w, "Herd workspaces: none yet")
	} else {
		fmt.Fprintln(w, "Herd workspaces:")
	}

	for _, r := range s.Roots {
		parts := make([]string, 0, len(r.Projects))
		for _, p := range r.Projects {
			parts = append(parts, fmt.Sprintf("%s (%s, %d running, %s)",
				p.Name, plural(p.Sessions, "session"), p.Running, plural(p.Results, "result")))
		}

		queue := ""
		if r.Queue {
			queue = "  [queue]"
		}

		fmt.Fprintf(w, "  %s  %s%s\n", r.Root, strings.Join(parts, ", "), queue)
	}

	printUnmanaged(w, s.Unmanaged)

	if s.OthersBusy > 0 {
		fmt.Fprintf(w, "Other users: %d busy processes.\n", s.OthersBusy)
	}
}

func printUnmanaged(w io.Writer, us []herd.Unmanaged) {
	if len(us) == 0 {
		fmt.Fprintln(w, "Simulations not started by Herd: none")
		return
	}

	fmt.Fprintf(w, "Simulations not started by Herd (%d):\n", len(us))

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  PID\tCPU\tMEM\tRUNNING\tDIRECTORY\tCOMMAND")

	now := float64(time.Now().Unix())
	for _, u := range us {
		fmt.Fprintf(tw, "  %d\t%.0f%%\t%.1fG\t%s\t%s\t%s\n", u.PID, u.CPU, u.RSSGB,
			(time.Duration(now-u.Start) * time.Second).String(), dash(u.Cwd), shorten(u.Args, 70))

		if u.Launcher != "" {
			fmt.Fprintf(tw, "  \t\t\t\t\tstarted by: %s\n", shorten(u.Launcher, 70))
		}
	}

	_ = tw.Flush()
}

func plural(n int, what string) string {
	if n == 1 {
		return "1 " + what
	}

	return fmt.Sprintf("%d %ss", n, what)
}

func shorten(s string, n int) string {
	if len(s) <= n {
		return s
	}

	return s[:n-3] + "..."
}

func runHostAdd(cmd *cobra.Command, args []string) error {
	env, err := loadHerdEnv(cmd)
	if err != nil {
		return err
	}

	h, err := newHostOf(cmd, env, args[0])
	if err != nil {
		return err
	}

	root, akitaDir := h.Root, h.AkitaDir
	yes, _ := cmd.Flags().GetBool("yes")

	ctx, cancel := commandContext(cmd)
	defer cancel()

	scan, err := scanHost(ctx, h)
	if err != nil {
		return err
	}

	printScan(env.out, h, scan)

	if root == "" {
		root = herd.PickRoot(scan)
	}

	entry := herd.SuggestHost(h.Name, h.Address, root, scan)
	entry.AkitaDir = akitaDir

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	path := herd.UserConfigPath()
	env.printf("\nHost entry for %s:\n  %s\n", path, data)

	printOtherRoots(env, scan, h, root, akitaDir)

	if !yes && !confirm(cmd.InOrStdin(), env, "Add it?") {
		env.printf("Not added. Edit the entry as you like and add it to %s, or run again with --yes.\n", path)
		return nil
	}

	if err := herd.AddHosts(path, []herd.HostConfig{entry}); err != nil {
		return err
	}

	env.printf(">> added host %s to %s\n", entry.Name, path)

	if len(scan.Unmanaged) > 0 {
		env.printf("The %d simulations not started by Herd show up under %s in akita herd web.\n",
			len(scan.Unmanaged), entry.Name)
	}

	return nil
}

// newHostOf is the host host add adds at address, from the flags; its
// name must be free.
func newHostOf(cmd *cobra.Command, env *herdEnv, address string) (herd.HostConfig, error) {
	if err := herd.ValidateAddress(address); err != nil {
		return herd.HostConfig{}, err
	}

	h := herd.HostConfig{Type: herd.HostSSH, Address: address}
	h.Name, _ = cmd.Flags().GetString("name")
	h.Root, _ = cmd.Flags().GetString("root")
	h.AkitaDir, _ = cmd.Flags().GetString("akita-dir")

	if h.Name == "" {
		h.Name = herd.HostNameFor(address)
	}

	if old, ok := env.user.Host(h.Name); ok {
		return h, fmt.Errorf("a host named %s is configured already (address %s); pick another with --name",
			h.Name, dash(old.Address))
	}

	return h, nil
}

// printOtherRoots lists the roots of a scan other than root, with the
// command that adds each as a host of its own.
func printOtherRoots(env *herdEnv, scan *herd.HostScan, h herd.HostConfig, root, akitaDir string) {
	extra := ""
	if akitaDir != "" {
		extra = " --akita-dir " + akitaDir
	}

	for _, r := range scan.Roots {
		if r.Root != root {
			env.printf("  (also has workspaces in %s: akita herd host add %s --name %s-%s --root %s%s)\n",
				r.Root, h.Address, h.Name, lastPart(r.Root), r.Root, extra)
		}
	}
}

func lastPart(p string) string {
	p = strings.TrimRight(p, "/")
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}

	return p
}

// confirm asks a yes/no question on the terminal; no answer is no.
func confirm(in io.Reader, env *herdEnv, question string) bool {
	if f, ok := in.(*os.File); ok {
		if st, err := f.Stat(); err != nil || st.Mode()&os.ModeCharDevice == 0 {
			return false // not a terminal
		}
	}

	env.printf("%s [y/N] ", question)

	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false
	}

	answer := strings.ToLower(strings.TrimSpace(line))

	return answer == "y" || answer == "yes"
}
