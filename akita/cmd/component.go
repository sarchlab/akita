/*
Copyright © 2024 NAME HERE <EMAIL ADDRESS>

*/

package cmd

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// componentCmd represents the main component command
var componentCmd = &cobra.Command{
	Use:   "component",
	Short: "Manage components",
	Run: func(cmd *cobra.Command, args []string) {
		componentName, _ := cmd.Flags().GetString("create") // fetch the string value of the flag
		if componentName != "" {
			if !inGitRepo() {
				log.Fatalf("Error: This command must be run inside a Git repository.")
			}

			err := createComponentFolder(componentName)
			if err != nil {
				log.Fatalf("Error creating folder: %v", err)
			}

			if err != nil {
				log.Fatalf("Error saving component file: %v", err)
			}
			fmt.Printf("Component '%s' created successfully!\n", componentName)
		} else {
			fmt.Println("Action not valid.")
		}
	},
}

func init() {
	rootCmd.AddCommand(componentCmd)

	componentCmd.Flags().String("create", "", "Create a new component")
}

// Check if current operation is in a Git repository
func inGitRepo() bool {
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	cmd.Dir = filepath.Dir(".")
	output, err := cmd.Output()
	return strings.TrimSpace(string(output)) == "true" && err == nil
}

// Create folder
func createComponentFolder(name string) error {
	return os.Mkdir(name, 0755) // create folder and gives read/write/execute permission to the owner
}