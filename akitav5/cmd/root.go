// Package cmd provides the command-line interface for Akita.
package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "akita",
	Short: "Akita CLI tool can perform common tasks related to developing simulators with Akita.",
	Long: `Akita CLI tool can perform common tasks related to developing simulators with Akita. ` +
		`The v5 CLI currently provides component scaffolding (component-create) and linting ` +
		`(component-lint).`,
}

// Execute adds all child commands to the root command and sets flags
// appropriately.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}
