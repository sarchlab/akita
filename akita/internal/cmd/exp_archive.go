package cmd

import (
	"errors"
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

const archiveUsage = "akita exp archive [-n] <host> <group|name>... [--results <run>...]"

var expArchiveCmd = &cobra.Command{
	Use:   "archive [-n] <host> [<group|name>...] [--results <run>...]",
	Short: "Archive finished sessions and hide their result directories.",
	Long: `Archive finished sessions, named by session or group name (a group archives
all its sessions). Their files move from logs/ to logs/archived/; the
sessions, groups, and result directories are recorded in .exp_archive.json
and hidden in the web UI by default. Result directories are never moved.

A session's result directories are those whose names appear in its
command. Name others (e.g. of campaigns that scripts launch) after
--results; archive then also hints at result directories that the logs
mention. --results alone hides only result directories.

Nothing is changed if any named session is still running. A session
archived when an older archived copy has its name becomes <name>~2 (~3...).
With only <host>, lists what is archived there. -n shows what would be
done.`,
	Example: `  akita exp archive -n server1 old-ablation
  akita exp archive server1 old-ablation --results 1013_abl_full_wide_5d72b18
  akita exp archive server1`,
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runArchive(cmd, "archive", args)
	},
}

var expUnarchiveCmd = &cobra.Command{
	Use:   "unarchive [-n] <host> <group|name>... [--results <run>...]",
	Short: "Undo archive: move sessions back to logs/ and unhide their results.",
	Long: `Move archived sessions (by session or group name) back to logs/ under their
original names (keeping <name>~2 if a newer session has the name), and
drop the archive marks they put on result directories; a directory that
another archived session still marks stays hidden. --results unhides
result directories. -n shows what would be done.`,
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runArchive(cmd, "unarchive", args)
	},
}

func init() {
	expCmd.AddCommand(expArchiveCmd, expUnarchiveCmd)
}

func runArchive(cmd *cobra.Command, mode string, args []string) error {
	if slices.Contains(args, "-h") || slices.Contains(args, "--help") {
		return cmd.Help()
	}

	host, rest, dry := splitArchiveArgs(args)
	if host == "" || (mode == "unarchive" && len(rest) == 0) {
		return errors.New("usage: " + archiveUsage +
			"\n       akita exp archive <host>    lists what is archived on the host")
	}

	env, err := loadExpEnv(cmd)
	if err != nil {
		return err
	}

	b, err := env.backend(host)
	if err != nil {
		return err
	}

	ws := b.Workspace(env.project)
	env.printf(">> %s: %s\n", host, ws)

	callArgs := []string{"exp", "_local", mode, ws, env.project}
	if dry {
		callArgs = append(callArgs, "-n")
	}

	ctx, cancel := commandContext(cmd)
	defer cancel()

	return akitaOut(ctx, b, env.out, append(callArgs, rest...)...)
}

// splitArchiveArgs takes the host (the first argument that is not -n) and
// the dry-run flag out of archive arguments.
func splitArchiveArgs(args []string) (host string, rest []string, dry bool) {
	for _, a := range args {
		switch {
		case a == "-n" || a == "--dry-run":
			dry = true
		case host == "":
			host = a
		default:
			rest = append(rest, a)
		}
	}

	if strings.HasPrefix(host, "-") {
		return "", nil, dry
	}

	return host, rest, dry
}
