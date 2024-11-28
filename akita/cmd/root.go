// Package cmd provides the command-line interface for Akita.
package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "akita",
	Short: "akita command",
	Long: `akita command is the root command for Akita framework.
	It contains child commands that help users to manage components and files
	in Akita.`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}