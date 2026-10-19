package hostsys

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/sarchlab/akita/v5/herd/internal/testutil"
)

func TestSlurmState(t *testing.T) {
	cases := map[string]string{
		"PENDING": "queued", "RUNNING": "running", "COMPLETING": "running",
		"": "", "COMPLETED": "", "RUNNING\nRUNNING": "running",
	}
	for in, want := range cases {
		if got := slurmState(in); got != want {
			t.Errorf("slurmState(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseNvidiaSMI(t *testing.T) {
	gpus := parseNvidiaSMI("NVIDIA A100, 37, 1024, 40960\nbad line\nX, a, 1, 2\n")
	if len(gpus) != 1 || gpus[0] != (GPU{Name: "NVIDIA A100", Util: 37, MemUsedMiB: 1024, MemTotalMiB: 40960}) {
		t.Errorf("got %+v", gpus)
	}
}

func TestTmuxPanes(t *testing.T) {
	testutil.NeedTmux(t)
	testutil.IsolateTmux(t)

	panes, err := TmuxPanes(context.Background())
	if err != nil || len(panes) != 0 {
		t.Fatalf("no server: %v %v", panes, err)
	}

	if out, err := exec.Command("tmux", "new-session", "-d", "-s", "akx-a.b", "sleep 30").CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}

	panes, err = TmuxPanes(context.Background())
	if err != nil || panes["akx-a_b"] <= 0 || !PIDAlive(panes["akx-a_b"]) {
		t.Errorf("got %v %v", panes, err)
	}
}

func TestFiles(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.json")

	if err := WriteFileAtomic(p, []byte(`{"a": 1}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if m := ReadJSONMap(p, 100); m["a"] != 1.0 {
		t.Errorf("ReadJSONMap: %v", m)
	}

	if ReadJSONMap(p, 3) != nil || ReadJSONMap(filepath.Join(dir, "missing"), 100) != nil {
		t.Errorf("ReadJSONMap of a cut or missing file")
	}

	if got := ReadText(p, 4); got != `{"a"` {
		t.Errorf("ReadText: %q", got)
	}

	if !Exists(p) || IsDir(p) || !IsDir(dir) || Exists(filepath.Join(dir, "missing")) {
		t.Errorf("Exists/IsDir")
	}

	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("temporary files left: %v", entries)
	}
}
