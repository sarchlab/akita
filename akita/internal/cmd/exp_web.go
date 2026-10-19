package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"

	"github.com/pkg/browser"
	"github.com/sarchlab/akita/v5/experiment"
	"github.com/sarchlab/akita/v5/experiment/monitor"
	"github.com/sarchlab/akita/v5/experiment/probe"
	"github.com/spf13/cobra"
)

var expWebCmd = &cobra.Command{
	Use:   "web [--port 8765] [--no-browser] [host...]",
	Short: "Serve the web UI that monitors the experiments on hosts.",
	Long: `Poll the hosts (all hosts by default) with ` + "`akita exp probe`" + ` and serve
a web UI of their sessions, runs, and campaigns on http://127.0.0.1:<port>.
akita must be installed on SSH and SLURM hosts (or set "akita" in the
host's config). Read-only: the UI never stops or changes experiments.
With --balance, it also runs akita exp balance for the project every
minute and shows what it did in the Queue view.`,
	RunE: runExpWeb,
}

var expProbeCmd = &cobra.Command{
	Use:   "probe --json [--root DIR]",
	Short: "Print the workspaces of this machine as JSON (used by akita exp web).",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		root, _ := cmd.Flags().GetString("root")
		projects, _ := cmd.Flags().GetStringArray("project")

		ctx, cancel := commandContext(cmd)
		defer cancel()

		p, err := probe.Probe(ctx, root, probe.Options{Projects: projects})
		if err != nil {
			return err
		}

		return writeJSONTo(cmd.OutOrStdout(), p)
	},
}

var expProbeRunCmd = &cobra.Command{
	Use:   "run [--root DIR] <project> <name>",
	Short: "Print the details of a result directory as JSON.",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, _ := cmd.Flags().GetString("root")

		ctx, cancel := commandContext(cmd)
		defer cancel()

		d, err := probe.Run(ctx, root, args[0], args[1])
		if err != nil {
			return err
		}

		return writeJSONTo(cmd.OutOrStdout(), d)
	},
}

var expProbeCampaignCmd = &cobra.Command{
	Use:   "campaign [--root DIR] <project> <name>",
	Short: "Print the cells of a campaign as JSON.",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, _ := cmd.Flags().GetString("root")

		ctx, cancel := commandContext(cmd)
		defer cancel()

		c, err := probe.Campaign(ctx, root, args[0], args[1])
		if err != nil {
			return err
		}

		return writeJSONTo(cmd.OutOrStdout(), c)
	},
}

func init() {
	expWebCmd.Flags().IntP("port", "p", 8765, "port of the web UI")
	expWebCmd.Flags().Bool("no-browser", false, "do not open a browser")
	expWebCmd.Flags().Bool("balance", false, "balance the project's queued tasks every minute")
	expProbeCmd.Flags().Bool("json", false, "print JSON (the only format)")
	// The monitor probes only the project of a local host without a root.
	expProbeCmd.Flags().StringArray("project", nil, "probe only this workspace under the root (repeatable)")
	_ = expProbeCmd.Flags().MarkHidden("project")
	expProbeCmd.PersistentFlags().String("root", experiment.DefaultRoot, "directory holding the workspaces")
	expProbeCmd.AddCommand(expProbeRunCmd, expProbeCampaignCmd)
	expCmd.AddCommand(expWebCmd, expProbeCmd)
}

func runExpWeb(cmd *cobra.Command, args []string) error {
	user, err := experiment.LoadUserConfig()
	if err != nil {
		return fmt.Errorf("user config %s: %w", experiment.UserConfigPath(), err)
	}

	m, err := monitor.New(user, args)
	if err != nil {
		return err
	}

	port, _ := cmd.Flags().GetInt("port")
	noBrowser, _ := cmd.Flags().GetBool("no-browser")

	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return err
	}

	url := "http://" + ln.Addr().String() + "/"
	fmt.Fprintf(cmd.OutOrStdout(), "akita exp web: %s (Ctrl+C to quit)\n", url)

	if !noBrowser {
		browser.Stdout, browser.Stderr = io.Discard, io.Discard
		_ = browser.OpenURL(url)
	}

	ctx, cancel := commandContext(cmd)
	defer cancel()

	if balance, _ := cmd.Flags().GetBool("balance"); balance {
		if err := startBalanceLoop(ctx, cmd, args, m); err != nil {
			return err
		}
	}

	err = m.ServeListener(ctx, ln)
	if ctx.Err() != nil && (err == nil || errors.Is(err, ctx.Err())) {
		return nil
	}

	return err
}

func writeJSONTo(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)

	return enc.Encode(v)
}

// startBalanceLoop runs balance for the project in the current directory
// every minute, reporting to the monitor.
func startBalanceLoop(ctx context.Context, cmd *cobra.Command, hosts []string, m *monitor.Monitor) error {
	env, err := loadExpEnv(cmd)
	if err != nil {
		return err
	}

	env.out = io.Discard

	bs, err := queueBackends(env, hosts)
	if err != nil {
		return err
	}

	report := func(line string) { m.AddActivity(time.Now().Format("01-02 15:04:05") + " " + line) }

	go func() {
		for {
			balanceOnce(ctx, env, bs, false, report)

			select {
			case <-ctx.Done():
				return
			case <-time.After(balanceEvery):
			}
		}
	}()

	return nil
}
