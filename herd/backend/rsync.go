package backend

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sarchlab/akita/v5/herd"
	ws "github.com/sarchlab/akita/v5/herd/workspace"
)

// ignoreFiles hold more rsync excludes: .herdignore, and .expignore (its
// former name, also the original exp tool's exclude file). Both are read.
var ignoreFiles = []string{".herdignore", ".expignore"}

// alwaysExcluded are never synced, and never deleted on the host: they
// hold what the experiments produce there.
var alwaysExcluded = []string{
	".git/", ws.LogsDir + "/", ws.ResultsDir + "/", "remote/", "__pycache__/", ".DS_Store",
	"/" + ws.ArchiveFile, "/" + ws.LegacyArchiveFile, "/.akita-herd/",
}

// rsyncArgs are the rsync arguments that copy projectDir to dst
// (an rsync destination such as "host:exp/proj"): an exact copy except for
// .git/, the workspace's own logs/, results/, remote/ and
// .herd_archive.json (and .exp_archive.json), the project's .gitignore'd
// paths, herd.json sync.exclude, and .herdignore and .expignore.
func rsyncArgs(projectDir, dst string, cfg *herd.ProjectConfig) []string {
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

	for _, f := range ignoreFiles {
		if fi, err := os.Stat(filepath.Join(projectDir, f)); err == nil && !fi.IsDir() {
			args = append(args, "--exclude-from", filepath.Join(projectDir, f))
		}
	}

	return append(args, strings.TrimRight(projectDir, "/")+"/", strings.TrimRight(dst, "/")+"/")
}

// rsyncProject copies a project directory to an rsync destination.
func rsyncProject(ctx context.Context, projectDir, dst string, cfg *herd.ProjectConfig) error {
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
