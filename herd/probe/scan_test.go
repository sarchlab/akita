package probe

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sarchlab/akita/v5/herd/internal/hostsys"
)

func TestScanFindsRoots(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	write := func(rel, content string) {
		p := filepath.Join(home, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// ~/exp with a workspace of one finished session and two results.
	write("exp/demo/logs/a.log", "[start] Mon Oct 19 02:02:35 PM UTC 2026  server1\n"+
		"[exit 0] Mon Oct 19 02:02:40 PM UTC 2026\n")
	write("exp/demo/results/1019_a/run.json", "{}")
	write("exp/demo/results/1019_b/run.json", "{}")
	// A root two levels down, known by its archive records.
	write("work/herd/old/.exp_archive.json", "{}")
	// A queue without workspaces yet.
	write("q/.akita-herd/seq", "0")
	// Not roots: a log that is not a session's, and plain directories.
	write("notes/x/logs/build.log", "compiling\n")
	write("src/project/main.go", "package main\n")

	f := &fakeSystem{procs: map[int]hostsys.ProcInfo{
		10: {Etime: 600, CPUTime: 590, Args: "./other"},
	}, owner: map[int]int{10: fakeUID + 1}}

	s := scan(context.Background(), nil, Options{sys: f, now: time.Now()})

	roots := map[string]bool{}
	for _, r := range s.Roots {
		roots[r.Root] = true
	}

	if len(s.Roots) != 3 || !roots["~/exp"] || !roots["~/work/herd"] || !roots["~/q"] {
		t.Fatalf("roots: %+v", s.Roots)
	}

	if r := s.Roots[0]; r.Root != "~/exp" || len(r.Projects) != 1 || r.Projects[0].Sessions != 1 ||
		r.Projects[0].Results != 2 {
		t.Errorf("~/exp: %+v", r)
	}

	if s.OthersBusy != 1 || s.Schema != "herd-scan/1" || s.CPUs != 8 {
		t.Errorf("scan: %+v", s)
	}
}
