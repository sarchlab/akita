package cmd

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/sarchlab/akita/v5/herd"
	"github.com/sarchlab/akita/v5/herd/queue"
	"github.com/spf13/cobra"
)

// The queue operations that laptop commands run on a host, on the queue of
// a root of that machine; every change holds the queue's lock.

var herdQueuePutCmd = &cobra.Command{
	Use:    "put --root DIR <tasks-json>",
	Short:  "Add tasks (a JSON array) to the queue of a root on this machine.",
	Hidden: true,
	Args:   cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var tasks []*herd.Task
		if err := json.Unmarshal([]byte(args[0]), &tasks); err != nil {
			return fmt.Errorf("tasks: %w", err)
		}

		if err := rootQueue(cmd).Put(tasks); err != nil {
			return err
		}

		fmt.Fprintf(cmd.OutOrStdout(), ">> queued %d tasks\n", len(tasks))

		return nil
	},
}

var herdQueueTakeCmd = &cobra.Command{
	Use:    "take --root DIR <project> <name>",
	Short:  "Remove a pending task from the queue of a root on this machine and print it.",
	Hidden: true,
	Args:   cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		t, err := rootQueue(cmd).Take(args[0], args[1])
		if err != nil {
			return err
		}

		return writeJSONTo(cmd.OutOrStdout(), t)
	},
}

var herdQueueListCmd = &cobra.Command{
	Use:    "list --root DIR",
	Short:  "Print the queue of a root on this machine as JSON.",
	Hidden: true,
	Args:   cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return writeJSONTo(cmd.OutOrStdout(), rootQueue(cmd).Status(time.Now()))
	},
}

var herdQueueMarkCmd = &cobra.Command{
	Use:    "mark --root DIR <project> <name> [--collected] [--retried HOST]",
	Short:  "Mark an ended task of the queue of a root on this machine.",
	Hidden: true,
	Args:   cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		collected, _ := cmd.Flags().GetBool("collected")
		retried, _ := cmd.Flags().GetString("retried")

		return rootQueue(cmd).Update(args[0], args[1], func(t *herd.Task) {
			if collected {
				t.Collected = new(herd.UnixSeconds(time.Now()))
			}

			if retried != "" {
				t.Retried = retried
			}
		})
	},
}

func init() {
	for _, c := range []*cobra.Command{herdQueuePutCmd, herdQueueTakeCmd, herdQueueListCmd, herdQueueMarkCmd} {
		c.Flags().String("root", "", "the root on this machine")
		_ = c.MarkFlagRequired("root")
	}

	herdQueueMarkCmd.Flags().Bool("collected", false, "the task's results were collected")
	herdQueueMarkCmd.Flags().String("retried", "", "the task was resubmitted to this host")
	herdQueueCmd.AddCommand(herdQueuePutCmd, herdQueueTakeCmd, herdQueueListCmd, herdQueueMarkCmd)
}

func rootQueue(cmd *cobra.Command) *queue.Queue {
	root, _ := cmd.Flags().GetString("root")
	return queue.Open(root)
}
