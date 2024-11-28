package cmd

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	_ "embed"

	"github.com/spf13/cobra"
)

//go:embed builderTemplate.txt
var builderTemplate string
//go:embed compTemplate.txt
var compTemplate string

var componentCmd = &cobra.Command{
	Use:   "component",
	Short: "Create and manage components.",
	Long:  "`component --create [ComponentName]` creates a new component folder containing the builder and comp files",
	Run: func(cmd *cobra.Command, args []string) {
		componentName, _ := cmd.Flags().GetString("create")
		if componentName != "" {
			if !inGitRepo() {
				log.Fatalf("Error: This command must be run inside a Git repository.")
			}

			err := createComponentFolder(componentName)
			if err != nil {
				log.Fatalf("Error creating component: %v", err)
			}
			fmt.Printf("Component '%s' created successfully!\n", componentName)

			errFile := generateBuilderFile(componentName)
			if errFile != nil {
                log.Fatalf("Failed to generate builder file for component '%s': %v\n", componentName, errFile)
            } else {
				fmt.Println("Builder File generated successfully!")
			}

			errComp := generateCompFile(componentName)
			if errComp != nil {
                log.Fatalf("Failed to generate comp file for component '%s': %v\n", componentName, errFile)
            } else {
				fmt.Println("Comp File generated successfully!")
			}

		} else {
			fmt.Println("Action not valid.")
		}
	},
}

func init() {
	rootCmd.AddCommand(componentCmd)
	componentCmd.Flags().String("create", "", "create a new component")
}

// Check if current operation is in a Git repository
func inGitRepo() bool {
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	cmd.Dir = filepath.Dir(".")
	output, err := cmd.Output()
	if err != nil {
	    log.Fatalf("Error running git command: %v\n", err)
		return false
	}
	return strings.TrimSpace(string(output)) == "true"
}

// Create folder for the new component
func createComponentFolder(name string) error {
    parentPath := "./akita"
	folderPath := filepath.Join(parentPath, name)
	return os.MkdirAll(folderPath, 0755)
}

// Create basic files for the new component
func generateBuilderFile(folder string) error {
	// Ensure the folder exists before proceeding
	folderPath := filepath.Join("./akita", folder)
	_, errFind := os.Stat(folderPath)
    if os.IsNotExist(errFind) {
    	return fmt.Errorf("failed to find folder: %s", folderPath)
    } else if errFind != nil {
    	return fmt.Errorf("error checking folder: %v", errFind)
    }

	filePath := filepath.Join(folderPath, "builder.go")
	placeholder := "{{packageName}}"
	packageName := folder
	content := strings.Replace(builderTemplate, placeholder, packageName, -1)

	err := os.WriteFile(filePath, []byte(content), 0644)
	if err != nil {
		return fmt.Errorf("failed to write builder file: %v", err)
	}

	return nil
}

func generateCompFile(folder string) error {
	// Ensure the folder exists before proceeding
	folderPath := filepath.Join("./akita", folder)
	_, errFind := os.Stat(folderPath)
    if os.IsNotExist(errFind) {
    	return fmt.Errorf("failed to find folder: %s", folderPath)
    } else if errFind != nil {
    	return fmt.Errorf("error checking folder: %v", errFind)
    }

	filePath := filepath.Join(folderPath, "comp.go")
	placeholder := "{{packageName}}"
	packageName := folder
	content := strings.Replace(compTemplate, placeholder, packageName, -1)

	err := os.WriteFile(filePath, []byte(content), 0644)
	if err != nil {
		return fmt.Errorf("failed to write comp file: %v", err)
	}

	return nil
}
