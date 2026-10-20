package example

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarchlab/akita/v5/herd"
)

func TestWrite(t *testing.T) {
	dir := t.TempDir()
	if err := Write(dir); err != nil {
		t.Fatal(err)
	}

	for _, f := range Files {
		info, err := os.Stat(filepath.Join(dir, filepath.FromSlash(f.Path)))
		if err != nil {
			t.Fatal(err)
		}

		if exec := info.Mode()&0o111 != 0; exec != strings.HasSuffix(f.Path, ".sh") {
			t.Errorf("%s: mode %v", f.Path, info.Mode())
		}
	}

	cfg, err := herd.LoadProjectConfig(dir)
	if err != nil {
		t.Fatal(err)
	}

	// The walkthrough shows every key of herd.json.
	if cfg.Naming == nil || len(cfg.Simulators) == 0 || cfg.Campaigns == nil ||
		len(cfg.Plots) == 0 || cfg.Build == nil || cfg.Collect == nil || len(cfg.Sync.Exclude) == 0 {
		t.Errorf("herd.json lacks a key: %+v", cfg)
	}

	if got := herd.ParseName("1018_fir_base", cfg.Naming); got["workload"] != "fir" {
		t.Errorf("naming: %v", got)
	}

	if err := Write(dir); err == nil || !strings.Contains(err.Error(), "exists") {
		t.Errorf("second Write: %v", err)
	}
}

func TestWriteLeavesAnExistingFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "run.sh"), []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Write(dir); err == nil {
		t.Fatal("Write over an existing run.sh succeeded")
	}

	if _, err := os.Stat(filepath.Join(dir, "herd.json")); err == nil {
		t.Error("Write wrote herd.json although run.sh exists")
	}
}

func TestHosts(t *testing.T) {
	cfg := Hosts("/tmp/demo-parent/demo")
	if len(cfg.Hosts) != 2 || cfg.Hosts[0].Root != "/tmp/demo-parent/server1" ||
		cfg.Hosts[1].Root != "/tmp/demo-parent/server2" || cfg.Hosts[0].Queue.Slots != 1 {
		t.Errorf("hosts: %+v", cfg.Hosts)
	}
}
