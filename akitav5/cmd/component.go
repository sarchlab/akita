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
	Long:  "`component --create [ComponentName]` creates a new component; `component --lint [path]` lints a component.",
	Run: func(cmd *cobra.Command, args []string) {
		// Handle --lint
		doLint, _ := cmd.Flags().GetBool("lint")
		if doLint {
			target := "."
			if len(args) >= 1 {
				target = args[0]
			}
			recursive, _ := cmd.Flags().GetBool("recursive")
			if recursive && !strings.Contains(target, "...") {
				clean := filepath.Clean(target)
				if clean == "." {
					target = "./..."
				} else {
					target = filepath.ToSlash(clean) + "/..."
				}
			}
			hasErr := lintTarget(target)
			if hasErr {
				os.Exit(1)
			}
			os.Exit(0)
		}

		// Handle --create
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
	componentCmd.Flags().Bool("lint", false, "Lint a component (usage: akita component --lint [-r] [path])")
	componentCmd.Flags().BoolP("recursive", "r", false, "With --lint, lint components recursively from the target path")
}

// lintTarget lints either a single folder or expands Go package patterns (./...).
func lintTarget(target string) bool {
	if strings.Contains(target, "...") {
		// Expand using `go list` for reliability
		dirs := goListDirs(target)
		anyErr := false
		for _, d := range dirs {
			if isComponentDir(d) {
				if LintComponentFolder(d) {
					anyErr = true
				}
			}
		}
		return anyErr
	}
	// Single folder path
	return LintComponentFolder(target)
}

// isComponentDir returns true if the directory appears to define a component.
// A directory is considered a component if it declares the //akita:component marker
// or contains component scaffolding files. This allows linting legacy packages so
// they surface the missing marker error.
func isComponentDir(dir string) bool {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return false
	}
	if hasComponentMarker(dir) {
		return true
	}
	if _, err := os.Stat(filepath.Join(dir, "comp.go")); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(dir, "builder.go")); err == nil {
		return true
	}
	return false
}

// goListDirs uses `go list -f {{.Dir}}` to expand a pattern like ./... to folders.
func goListDirs(pattern string) []string {
	cmd := exec.Command("go", "list", "-f", "{{.Dir}}", pattern)
	out, err := cmd.Output()
	if err != nil {
		return []string{} // fall back to empty
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	// Normalize empty output
	dirs := make([]string, 0, len(lines))
	for _, l := range lines {
		if l != "" {
			dirs = append(dirs, l)
		}
	}
	return dirs
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
