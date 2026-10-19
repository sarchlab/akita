package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/sarchlab/akita/v5/herd/backend"
	"github.com/spf13/cobra"
)

var herdCollectCmd = &cobra.Command{
	Use:   "collect [--watch] [host...]",
	Short: "Copy the results of the project's ended queued tasks to remote/<host>/.",
	Long: `For each ended task of the project in the hosts' queues (all SSH and local
hosts by default) that was not collected yet, copy results/<name>/ and
logs/<name>.* to remote/<host>/ (as pull does), leaving out the patterns
of herd.json collect.exclude, and mark the task collected. --watch repeats
every minute.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		env, err := loadHerdEnv(cmd)
		if err != nil {
			return err
		}

		bs, err := queueBackends(env, args)
		if err != nil {
			return err
		}

		watch, _ := cmd.Flags().GetBool("watch")

		ctx, cancel := commandContext(cmd)
		defer cancel()

		report := func(line string) { env.printf("%s %s\n", time.Now().Format("15:04:05"), line) }

		for {
			if n := collectOnce(ctx, env, bs, report); n == 0 && !watch {
				report("nothing to collect")
			}

			if !watch {
				return nil
			}

			select {
			case <-ctx.Done():
				return nil
			case <-time.After(balanceEvery):
			}
		}
	},
}

func init() {
	herdCollectCmd.Flags().Bool("watch", false, "repeat every minute")
	herdCmd.AddCommand(herdCollectCmd)
}

// collectOnce collects the ended tasks of the project that were not
// collected yet; it returns how many it collected.
func collectOnce(ctx context.Context, env *herdEnv, bs []backend.Backend, report func(string)) int {
	var exclude []string
	if env.projectCfg != nil && env.projectCfg.Collect != nil {
		exclude = env.projectCfg.Collect.Exclude
	}

	n := 0

	for _, b := range bs {
		host := b.Host().Name
		if env.isProjectWorkspace(b) {
			continue
		}

		s, err := remoteQueue(ctx, b)
		if err != nil {
			report(fmt.Sprintf("%s: %v", host, err))
			continue
		}

		dir := filepath.Join(env.projectDir, "remote", host)

		for _, t := range s.Done {
			if t.Project != env.project || t.Collected != nil {
				continue
			}

			if err := b.PullRun(ctx, env.project, t.Name, dir, exclude); err != nil {
				report(fmt.Sprintf("%s:%s: %v", host, t.Name, err))
				continue
			}

			if _, err := b.Akita(ctx, "herd", "queue", "mark", "--root", backend.Root(b), t.Project, t.Name,
				"--collected"); err != nil {
				report(fmt.Sprintf("%s:%s: marking it collected: %v", host, t.Name, err))
				continue
			}

			report(fmt.Sprintf("collected %s:%s (%s) -> remote/%s/", host, t.Name, t.State, host))
			n++
		}
	}

	return n
}
