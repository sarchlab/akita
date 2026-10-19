package cmd

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/sarchlab/akita/v5/herd"
	"github.com/sarchlab/akita/v5/herd/workspace"
	"github.com/spf13/cobra"
)

var herdRecordCmd = &cobra.Command{
	Use:   "record <dir> [PROPERTY=VALUE...]",
	Short: "Write or update <dir>/run.json (fields from the name, properties).",
	Long: `Write <dir>/run.json (schema herd-run/1): the fields that the project's
naming rule reads from the directory name, and the given properties
(merged into an existing run.json; properties it lacks are filled from
meta.json and EXPERIMENT_METADATA.json).`,
	Example: `  akita herd record results/1015_abl_wg32k_base_3e9a41c "Command=./sim -benchmark=fir"`,
	Args:    cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		props, err := herd.ParseProperties(args[1:])
		if err != nil {
			return err
		}

		naming, err := projectNaming(cmd)
		if err != nil && !errors.Is(err, errNoNaming) {
			return err
		}

		name := filepath.Base(filepath.Clean(args[0]))
		if naming != nil {
			if err := herd.CheckName(name, naming); err != nil {
				cmd.PrintErrf("warning: %v\n", err)
			}
		}

		if _, err := herd.WriteRecord(args[0], naming, props); err != nil {
			return err
		}

		fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", filepath.Join(args[0], herd.RunRecordFile))

		return nil
	},
}

var herdNameCmd = &cobra.Command{
	Use:   "name",
	Short: "Check result directory names and rename old ones.",
	Long: `Check result directory names against the project's naming rule (herd.json
"naming") and rename directories with old names. Old names are mapped by
the project's naming.legacy command, which reads names on stdin and prints
"old<TAB>new" lines.`,
}

var herdNameCheckCmd = &cobra.Command{
	Use:   "check <name>...",
	Short: "Check names against the naming rule (exit status 1 if one breaks it).",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		naming, err := projectNaming(cmd)
		if err != nil {
			return err
		}

		bad := 0

		for _, n := range args {
			if err := herd.CheckName(n, naming); err != nil {
				fmt.Fprintln(cmd.OutOrStdout(), err)

				bad++
			}
		}

		if bad > 0 {
			return fmt.Errorf("%d of %d names break the naming rule", bad, len(args))
		}

		fmt.Fprintln(cmd.OutOrStdout(), "ok")

		return nil
	},
}

var herdNamePlanCmd = &cobra.Command{
	Use:   "plan <results dir>",
	Short: "Print \"old<TAB>new\" for every result directory (\"?\" if unknown).",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runNamePlan(cmd, args[0], false)
	},
}

var herdNameApplyCmd = &cobra.Command{
	Use:   "apply <results dir> [--dry-run]",
	Short: "Rename result directories to their planned names.",
	Long: `Rename the directories of ` + "`akita herd name plan`" + `: rewrite the absolute
paths recorded in their small text files (not metrics.csv or *_metrics.csv,
nor files over 64 MB), move their marks in ../.herd_archive.json, record
"Renamed From" in their run.json, and append to RENAMES.tsv. Nothing is
changed if two directories would get the same name or a process names one
of them on its command line.

With --host, the results directory is a path in the project's workspace on
that host (e.g. "results"), and the renaming runs there.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runNamePlan(cmd, args[0], true)
	},
}

func init() {
	herdRecordCmd.Flags().String("project-dir", "", "project directory with herd.json (default: current directory)")

	for _, c := range []*cobra.Command{herdNamePlanCmd, herdNameApplyCmd, herdNameCheckCmd} {
		c.Flags().String("project-dir", "", "project directory with herd.json (default: current directory)")
	}

	for _, c := range []*cobra.Command{herdNamePlanCmd, herdNameApplyCmd} {
		c.Flags().StringP("host", "H", "", "run on this host, in the project's workspace")
	}

	herdNameApplyCmd.Flags().BoolP("dry-run", "n", false, "only print the plan")
	herdNameCmd.AddCommand(herdNameCheckCmd, herdNamePlanCmd, herdNameApplyCmd)
	herdCmd.AddCommand(herdRecordCmd, herdNameCmd)
}

// errNoNaming tells that the project has no naming rule.
var errNoNaming = errors.New("no naming rule")

// projectNaming loads the naming rule of --project-dir (default: the
// current directory); errNoNaming if it has none.
func projectNaming(cmd *cobra.Command) (*herd.Naming, error) {
	dir, err := projectDirFlag(cmd)
	if err != nil {
		return nil, err
	}

	cfg, err := herd.LoadProjectConfig(dir)
	if err != nil {
		return nil, err
	}

	if cfg == nil || cfg.Naming == nil {
		return nil, fmt.Errorf("%w: add \"naming\" to %s", errNoNaming,
			herd.ProjectConfigPath(dir))
	}

	return cfg.Naming, nil
}

func projectDirFlag(cmd *cobra.Command) (string, error) {
	dir, _ := cmd.Flags().GetString("project-dir")
	if dir == "" {
		return os.Getwd()
	}

	return herd.ExpandHome(dir), nil
}

func runNamePlan(cmd *cobra.Command, results string, apply bool) error {
	if strings.TrimSpace(results) == "" {
		return errors.New("empty results directory; name one, e.g. results")
	}

	if host, _ := cmd.Flags().GetString("host"); host != "" {
		return runNameRemote(cmd, host, results, apply)
	}

	naming, err := projectNaming(cmd)
	if err != nil {
		return err
	}

	dir, _ := projectDirFlag(cmd)

	ctx, cancel := commandContext(cmd)
	defer cancel()

	plan, err := workspace.PlanRenames(ctx, herd.ExpandHome(results), naming, dir)
	if err != nil {
		return err
	}

	for _, w := range plan.Warnings {
		cmd.PrintErrln("warning: " + w)
	}

	if !apply {
		for _, r := range plan.Renames {
			fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", r.Old, dash2q(r.New))
		}

		return nil
	}

	dry, _ := cmd.Flags().GetBool("dry-run")

	return workspace.ApplyRenames(herd.ExpandHome(results), naming, plan, dry, cmd.OutOrStdout())
}

func dash2q(s string) string {
	if s == "" {
		return "?"
	}

	return s
}

// runNameRemote runs `akita herd name plan|apply` in a host's workspace.
func runNameRemote(cmd *cobra.Command, host, results string, apply bool) error {
	env, err := loadHerdEnv(cmd)
	if err != nil {
		return err
	}

	b, err := env.backend(host)
	if err != nil {
		return err
	}

	ws := b.Workspace(env.project)
	if !path.IsAbs(results) && !strings.HasPrefix(results, "~") {
		results = path.Join(ws, results)
	}

	args := []string{"herd", "name", "plan", results, "--project-dir", ws}
	if apply {
		args[2] = "apply"

		if dry, _ := cmd.Flags().GetBool("dry-run"); dry {
			args = append(args, "--dry-run")
		}
	}

	ctx, cancel := commandContext(cmd)
	defer cancel()

	return akitaOut(ctx, b, env.out, args...)
}
