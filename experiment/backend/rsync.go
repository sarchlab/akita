package backend

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sarchlab/akita/v5/experiment"
	ws "github.com/sarchlab/akita/v5/experiment/workspace"
)

// expIgnoreFile holds more rsync excludes (the original exp tool's).
const expIgnoreFile = ".expignore"

// alwaysExcluded are never synced, and never deleted on the host: they
// hold what the experiments produce there.
var alwaysExcluded = []string{
	".git/", ws.LogsDir + "/", ws.ResultsDir + "/", "remote/", "__pycache__/", ".DS_Store",
	"/" + ws.ArchiveFile, "/.akita-exp/",
}

// rsyncArgs are the rsync arguments that copy projectDir to dst
// (an rsync destination such as "host:exp/proj"): an exact copy except for
// .git/, the workspace's own logs/, results/, remote/ and
// .exp_archive.json, the project's .gitignore'd paths, exp.json
// sync.exclude, and .expignore (the original exp tool's exclude file).
func rsyncArgs(projectDir, dst string, cfg *experiment.ProjectConfig) []string {
	args := []string{"-az", "--delete"}
	for _, e := range alwaysExcluded {
		args = append(args, "--exclude", e)
	}

	args = append(args, "--filter=:- .gitignore")

	if cfg != nil {
		for _, e := range cfg.Sync.Exclude {
			args = append(args, "--exclude", e)
		}
	}

	if fi, err := os.Stat(filepath.Join(projectDir, expIgnoreFile)); err == nil && !fi.IsDir() {
		args = append(args, "--exclude-from", filepath.Join(projectDir, expIgnoreFile))
	}

	return append(args, strings.TrimRight(projectDir, "/")+"/", strings.TrimRight(dst, "/")+"/")
}

// rsyncProject copies a project directory to an rsync destination.
func rsyncProject(ctx context.Context, projectDir, dst string, cfg *experiment.ProjectConfig) error {
	return runRsync(ctx, rsyncArgs(projectDir, dst, cfg))
}

// runRsync runs rsync, returning its standard error in the error.
func runRsync(ctx context.Context, args []string) error {
	var stderr bytes.Buffer

	cmd := exec.CommandContext(ctx, "rsync", args...)
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("rsync %s: %w: %s", strings.Join(args, " "), err,
			strings.TrimSpace(stderr.String()))
	}

	return nil
}

// projectBase is the project name of a project directory.
func projectBase(projectDir string) string {
	return filepath.Base(filepath.Clean(projectDir))
}
