package cmd

import (
	"path/filepath"

	"github.com/sarchlab/akita/v5/herd/queue"
	"github.com/spf13/cobra"
)

var herdBuildCmd = &cobra.Command{
	Use:   "build [--commit REV] [--target linux/amd64] [name...]",
	Short: "Build the binaries of herd.json build for the hosts.",
	Long: `Check the commit (default HEAD, with the uncommitted changes and untracked
files) out into a temporary git worktree, run each binary's command of
herd.json "build" there with GOOS/GOARCH of the target, and keep the output
in .akita-herd/bin/<sha12>/<name>. submit uploads the latest build of each
binary that a command names as {bin:<name>}, and records its sha and
commit in the task and the run's run.json.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		env, err := loadHerdEnv(cmd)
		if err != nil {
			return err
		}

		commit, _ := cmd.Flags().GetString("commit")
		target, _ := cmd.Flags().GetString("target")

		ctx, cancel := commandContext(cmd)
		defer cancel()

		built, err := queue.Build(ctx, queue.BuildRequest{
			ProjectDir: env.projectDir, Config: env.projectCfg, Commit: commit, Target: target,
			Names: args, Log: env.errOut,
		})

		for _, b := range built {
			rel, _ := filepath.Rel(env.projectDir, b.Path)
			env.printf(">> %s  %s  %s  sha %s  %s\n", b.Name, b.Binary.Revision(), b.Binary.Target,
				b.Binary.ShortSHA(), rel)
		}

		return err
	},
}

func init() {
	herdBuildCmd.Flags().String("commit", "", "build this commit instead of HEAD with the uncommitted changes")
	herdBuildCmd.Flags().String("target", queue.DefaultTarget, "GOOS/GOARCH of the binaries")
	herdCmd.AddCommand(herdBuildCmd)
}
