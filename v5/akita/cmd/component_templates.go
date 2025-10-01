package cmd

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

//go:embed builderTemplate.txt
var builderTemplate string

//go:embed compTemplate.txt
var compTemplate string

// inGitRepo returns true if the current working directory is inside a Git repository.
func inGitRepo() bool {
	cmd := execCommand("git", "rev-parse", "--is-inside-work-tree")
	cmd.Dir = filepath.Dir(".")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(output)) == "true"
}

// createComponentFolder creates the folder if it does not already exist.
func createComponentFolder(name string) error {
	if _, err := os.Stat(name); err == nil {
		return fmt.Errorf("folder '%s' already exists", name)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("%v", err)
	}
	return os.MkdirAll(name, 0755)
}

// generateBuilderFile materialises builder.go from the template.
func generateBuilderFile(folder string) error {
	if _, err := os.Stat(folder); os.IsNotExist(err) {
		return fmt.Errorf("failed to find folder %s", folder)
	} else if err != nil {
		return fmt.Errorf("%v", err)
	}

	filePath := filepath.Join(folder, "builder.go")
	packageName := filepath.Base(filepath.Clean(folder))
	content := strings.ReplaceAll(builderTemplate, "{{packageName}}", packageName)
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("%v", err)
	}
	return nil
}

// generateCompFile materialises comp.go from the template.
func generateCompFile(folder string) error {
	if _, err := os.Stat(folder); os.IsNotExist(err) {
		return fmt.Errorf("failed to find folder: %s", folder)
	} else if err != nil {
		return fmt.Errorf("%v", err)
	}

	filePath := filepath.Join(folder, "comp.go")
	packageName := filepath.Base(filepath.Clean(folder))
	content := strings.ReplaceAll(compTemplate, "{{packageName}}", packageName)
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("%v", err)
	}
	return nil
}

// execCommand is wrapped for testability.
var execCommand = exec.Command
