package cmd

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var componentLintCmd = &cobra.Command{
	Use:   "component-lint [path]",
	Short: "Run Akita component lints",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cmd.SilenceUsage = true
		target := "."
		if len(args) == 1 {
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

		if lintTarget(target) {
			os.Exit(1)
		}
	},
}

func init() {
	componentLintCmd.Flags().BoolP("recursive", "r", false, "lint recursively by appending ./... to the provided path")
	rootCmd.AddCommand(componentLintCmd)
}

// lintTarget lints either a single folder or expands Go package patterns (./...).
func lintTarget(target string) bool {
	if strings.Contains(target, "...") {
		dirs := goListDirs(target)
		if len(dirs) == 0 {
			dirs = expandLocalPattern(target)
		}
		anyErr := false
		seen := make(map[string]struct{})
		for _, d := range dirs {
			if _, ok := seen[d]; ok {
				continue
			}
			seen[d] = struct{}{}
			if LintComponentFolder(d) {
				anyErr = true
			}
		}
		return anyErr
	}
	return LintComponentFolder(target)
}

// goListDirs uses `go list -f {{.Dir}}` to expand a pattern like ./... to folders.
func goListDirs(pattern string) []string {
	cmd := exec.Command("go", "list", "-f", "{{.Dir}}", pattern)
	out, err := cmd.Output()
	if err != nil {
		return []string{}
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	dirs := make([]string, 0, len(lines))
	for _, l := range lines {
		if l != "" {
			dirs = append(dirs, l)
		}
	}
	return dirs
}

func expandLocalPattern(pattern string) []string {
	base := strings.TrimSuffix(pattern, "...")
	base = strings.TrimSuffix(base, string(os.PathSeparator))
	if base == "" {
		base = "."
	}
	base = filepath.Clean(base)

	var dirs []string
	filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if path != base {
			name := d.Name()
			if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") || name == "vendor" {
				return filepath.SkipDir
			}
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			return nil
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			if strings.HasSuffix(entry.Name(), ".go") {
				dirs = append(dirs, path)
				break
			}
		}
		return nil
	})
	return dirs
}
