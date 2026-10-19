// Package cmd provides the Akita command-line shell.
package cmd

import (
	"errors"

	"github.com/spf13/cobra"
)

const cliStatus = "The legacy generator and checker are unavailable in Akita v5.0. " +
	"A replacement CLI is planned for v5.1: https://github.com/sarchlab/akita/issues/523"

// Execute runs the CLI and returns any command error to the executable.
func Execute() error {
	return newRootCommand().Execute()
}

func newRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "akita",
		Short:         "Akita simulator development CLI",
		Long:          "Akita simulator development CLI.\n\n" + cliStatus,
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
	}

	component := &cobra.Command{
		Use:   "component",
		Short: "Deprecated component-generation stub (unavailable)",
		Long:  "The deprecated component --create command creates no files.\n\n" + cliStatus,
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return errors.New("component generation is unavailable in Akita v5.0; " +
				"the legacy generator has been removed. A replacement is planned for v5.1 " +
				"(https://github.com/sarchlab/akita/issues/523)")
		},
	}
	component.Flags().String("create", "", "Legacy component name; generation is unavailable")
	root.AddCommand(component)
	root.AddCommand(&cobra.Command{
		Use:   "check [component folder path]",
		Short: "Deprecated component-checking stub (unavailable)",
		Long:  "The deprecated check command does not validate components.\n\n" + cliStatus,
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, _ []string) error {
			return errors.New("component checking is unavailable in Akita v5.0; " +
				"the legacy checker has been removed. A replacement is planned for v5.1 " +
				"(https://github.com/sarchlab/akita/issues/523)")
		},
	})

	root.AddCommand(herdCmd)

	return root
}
