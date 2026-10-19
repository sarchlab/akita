package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/sarchlab/akita/v5/herd"
	"github.com/sarchlab/akita/v5/herd/probe"
	"github.com/sarchlab/akita/v5/herd/workspace"
	"github.com/spf13/cobra"
)

// herdLocalCmd holds the commands that other akita herd commands run on a
// host (via ssh for remote hosts), so that they behave the same
// everywhere. They operate on a workspace path of this machine.
var herdLocalCmd = &cobra.Command{
	Use:    "_local",
	Short:  "Workspace operations on this machine (used by akita herd itself).",
	Hidden: true,
}

var herdLocalArchiveCmd = &cobra.Command{
	Use:                "archive <workspace> <project> [-n] [<group|name>...] [--results <run>...]",
	Short:              "Archive sessions of a workspace on this machine.",
	DisableFlagParsing: true,
	SilenceErrors:      true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return localArchive(cmd, args, workspace.Archive)
	},
}

var herdLocalUnarchiveCmd = &cobra.Command{
	Use:                "unarchive <workspace> <project> [-n] <group|name>... [--results <run>...]",
	Short:              "Unarchive sessions of a workspace on this machine.",
	DisableFlagParsing: true,
	SilenceErrors:      true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return localArchive(cmd, args, workspace.Unarchive)
	},
}

var herdLocalGroupCmd = &cobra.Command{
	Use:           "group <workspace> <group|-> <name>...",
	Short:         "Set the group of sessions of a workspace on this machine.",
	Args:          cobra.MinimumNArgs(3),
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		n, err := workspace.SetGroup(args[0], args[1], args[2:], cmd.OutOrStdout())
		if err != nil {
			return localFail(cmd, err)
		}

		if args[1] == "-" {
			fmt.Fprintf(cmd.OutOrStdout(), ">> %d sessions removed from their group\n", n)
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), ">> %d sessions put into group %s\n", n, args[1])
		}

		return nil
	},
}

var herdLocalSessionsCmd = &cobra.Command{
	Use:           "sessions <workspace>",
	Short:         "Print the sessions of a workspace on this machine as JSON.",
	Args:          cobra.ExactArgs(1),
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		project, _ := cmd.Flags().GetString("project")
		tail, _ := cmd.Flags().GetInt("tail")
		archived, _ := cmd.Flags().GetBool("archived")

		sessions, err := probe.ListSessions(cmd.Context(), args[0], project, archived, tail)
		if err != nil {
			return localFail(cmd, err)
		}

		if sessions == nil {
			sessions = []herd.Session{}
		}

		return json.NewEncoder(cmd.OutOrStdout()).Encode(sessions)
	},
}

func init() {
	herdLocalSessionsCmd.Flags().String("project", "", "project name (default: the workspace's name)")
	herdLocalSessionsCmd.Flags().Int("tail", 3, "log lines per session")
	herdLocalSessionsCmd.Flags().Bool("archived", false, "include archived sessions")
	herdLocalCmd.AddCommand(herdLocalArchiveCmd, herdLocalUnarchiveCmd, herdLocalGroupCmd, herdLocalSessionsCmd)
	herdCmd.AddCommand(herdLocalCmd)
}

func localArchive(cmd *cobra.Command, args []string,
	op func(context.Context, workspace.ArchiveRequest) error,
) error {
	if len(args) < 2 {
		return localFail(cmd, errLocalUsage)
	}

	tokens, results, dry := workspace.ParseArchiveArgs(args[2:])
	req := workspace.ArchiveRequest{
		Workspace: args[0], Project: args[1], Tokens: tokens, Results: results,
		DryRun: dry, Out: cmd.OutOrStdout(),
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	if err := op(ctx, req); err != nil {
		return localFail(cmd, err)
	}

	return nil
}

// localFail prints an error to stderr (the caller shows it) and returns it.
func localFail(cmd *cobra.Command, err error) error {
	fmt.Fprintln(cmd.ErrOrStderr(), err)
	return err
}

var errLocalUsage = errors.New("usage: akita herd _local archive|unarchive <workspace> <project> [args...]")
