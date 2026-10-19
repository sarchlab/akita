package backend

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/sarchlab/akita/v5/herd"
	"github.com/sarchlab/akita/v5/herd/internal/testutil"
)

func TestRsyncArgs(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{".herdignore": "data/\n", ".expignore": "old/\n"})

	args := rsyncArgs(dir, "h:exp/p", &herd.ProjectConfig{Sync: herd.SyncConfig{Exclude: []string{"big/"}}})
	joined := strings.Join(args, " ")

	for _, want := range []string{
		"-az --delete", "--exclude .git/", "--exclude logs/", "--exclude results/",
		"--exclude /.herd_archive.json", "--filter=:- .gitignore", "--exclude big/",
		"--exclude /.exp_archive.json",
		"--exclude-from " + filepath.Join(dir, ".herdignore"),
		"--exclude-from " + filepath.Join(dir, ".expignore"),
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in %s", want, joined)
		}
	}

	if args[len(args)-2] != dir+"/" || args[len(args)-1] != "h:exp/p/" {
		t.Errorf("paths: %v", args[len(args)-2:])
	}

	if slices.Contains(rsyncArgs(t.TempDir(), "x", nil), "--exclude-from") {
		t.Errorf("--exclude-from without .herdignore")
	}
}

func TestRsyncProject(t *testing.T) {
	testutil.NeedRsync(t)

	src, dst := filepath.Join(t.TempDir(), "p"), t.TempDir()
	writeFiles(t, src, map[string]string{
		"main.go":           "package main",
		".gitignore":        "*.tmp\nbuild/\n",
		"x.tmp":             "ignored",
		"build/out":         "ignored",
		"sub/.gitignore":    "*.log\n",
		"sub/keep.txt":      "kept",
		"sub/run.log":       "ignored",
		".git/HEAD":         "ref",
		"big/blob":          "excluded by herd.json",
		"data/d":            "excluded by .herdignore",
		".herdignore":       "data/\n",
		"old/o":             "excluded by .expignore (the former name)",
		".expignore":        "old/\n",
		"results/local/r":   "local results are not synced",
		"node/results/x":    "nested results/ dirs are excluded too",
		"deleted-later.txt": "x",
	})
	// What the host already has: experiment output must survive --delete.
	writeFiles(t, dst, map[string]string{
		"logs/a.log":         "log",
		"results/r1/m":       "result",
		".herd_archive.json": "{}",
		".exp_archive.json":  "{}",
		"stale.txt":          "deleted by --delete",
		"remote/server1/x":   "kept",
		"sub/__pycache__/c":  "kept",
		"logs/archived/b.ok": "kept",
	})

	cfg := &herd.ProjectConfig{Sync: herd.SyncConfig{Exclude: []string{"big/"}}}
	if err := rsyncProject(context.Background(), src, dst, cfg); err != nil {
		t.Fatal(err)
	}

	for _, p := range []string{"main.go", "sub/keep.txt", ".gitignore", "logs/a.log", "results/r1/m",
		".herd_archive.json", ".exp_archive.json", "remote/server1/x", "logs/archived/b.ok", "deleted-later.txt"} {
		if !exists(filepath.Join(dst, p)) {
			t.Errorf("%s missing", p)
		}
	}

	for _, p := range []string{"x.tmp", "build/out", "sub/run.log", ".git", "big", "data", "old",
		"results/local", "stale.txt", "node/results"} {
		if exists(filepath.Join(dst, p)) {
			t.Errorf("%s should not be there", p)
		}
	}
}
