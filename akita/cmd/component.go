package cmd

import (
	_ "embed"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

//go:embed builderTemplate.txt
var builderTemplate string

//go:embed compTemplate.txt
var compTemplate string

var componentCmd = &cobra.Command{
	Use:   "component",
	Short: "Create and manage components.",
	Long:  "`component --create [ComponentName]` creates a new component.",
	Run: func(cmd *cobra.Command, args []string) {
		componentName, _ := cmd.Flags().GetString("create")
		if componentName != "" {
			if !inGitRepo() {
				log.Fatalf(
					"Error: This command must be run inside a Git repository.",
				)
			}

			err := createComponentFolder(componentName)
			if err != nil {
				log.Fatalf("Error creating component: %v", err)
			} else {
				fmt.Printf(
					"Component '%s' created successfully!\n",
					componentName,
				)
			}

			errFile := generateBuilderFile(componentName)
			if errFile != nil {
				log.Fatalf("Error generating builder file: %v\n", errFile)
			} else {
				fmt.Println("Builder file generated successfully!")
			}

			errComp := generateCompFile(componentName)
			if errComp != nil {
				log.Fatalf("Error generating comp file: %v\n", errComp)
			} else {
				fmt.Println("Comp file generated successfully!")
			}
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
	if err != nil {
		return false
	}

	return strings.TrimSpace(string(output)) == "true"
}

// Create folder for the new component
func createComponentFolder(name string) error {
	_, err := os.Stat(name)
	if err == nil {
		return fmt.Errorf("folder '%s' already exists", name)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("%v", err)
	}

	return os.MkdirAll(name, 0755)
}

// Create builder file for the new component
func generateBuilderFile(folder string) error {
	// Ensure the folder exists before proceeding
	_, errFind := os.Stat(folder)
	if os.IsNotExist(errFind) {
		return fmt.Errorf("failed to find folder %s", folder)
	} else if errFind != nil {
		return fmt.Errorf("%v", errFind)
	}

	filePath := filepath.Join(folder, "builder.go")
	placeholder := "{{packageName}}"
	packageName := folder
	content := strings.Replace(builderTemplate, placeholder, packageName, -1)

	err := os.WriteFile(filePath, []byte(content), 0644)
	if err != nil {
		return fmt.Errorf("%v", err)
	}

	return nil
}

// Create comp file for the new component
func generateCompFile(folder string) error {
	// Ensure the folder exists before proceeding
	_, errFind := os.Stat(folder)
	if os.IsNotExist(errFind) {
		return fmt.Errorf("failed to find folder: %s", folder)
	} else if errFind != nil {
		return fmt.Errorf("%v", errFind)
	}

	filePath := filepath.Join(folder, "comp.go")
	placeholder := "{{packageName}}"
	packageName := folder
	content := strings.Replace(compTemplate, placeholder, packageName, -1)

	err := os.WriteFile(filePath, []byte(content), 0644)
	if err != nil {
		return fmt.Errorf("%v", err)
	}

	return nil
}
