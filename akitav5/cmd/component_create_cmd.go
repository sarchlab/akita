package cmd

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"
)

var componentCreateCmd = &cobra.Command{
	Use:   "component-create [path]",
	Short: "Generate boilerplate for a new component package",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cmd.SilenceUsage = true
		path := args[0]

		if !inGitRepo() {
			log.Fatalf("Error: component-create must be run inside a Git repository.")
		}

		if err := createComponentFolder(path); err != nil {
			log.Fatalf("Error creating component: %v", err)
		}
		fmt.Printf("Component '%s' created successfully!\n", path)

		if err := generateBuilderFile(path); err != nil {
			log.Fatalf("Error generating builder file: %v", err)
		}
		fmt.Println("Builder file generated successfully!")

		if err := generateCompFile(path); err != nil {
			log.Fatalf("Error generating comp file: %v", err)
		}
		fmt.Println("Comp file generated successfully!")
	},
}

func init() {
	rootCmd.AddCommand(componentCreateCmd)
}
