package herd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestHostNameFor(t *testing.T) {
	for addr, want := range map[string]string{
		"server1":                "server1",
		"me@server2.example.edu": "server2",
		"10.0.0.7":               "10-0-0-7",
		"me@login.cluster:22":    "login",
	} {
		if got := HostNameFor(addr); got != want {
			t.Errorf("HostNameFor(%q) = %q, want %q", addr, got, want)
		}
	}
}

func TestSuggestHost(t *testing.T) {
	h := SuggestHost("s", "s", DefaultRoot, &HostScan{CPUs: 64, MemTotalGB: 251})
	if h.Root != "" || h.Queue == nil || h.Queue.Slots != 32 || h.Queue.MemReserveGB != 25 {
		t.Errorf("suggested %+v %+v", h, h.Queue)
	}

	if h := SuggestHost("s", "s", "~/w", &HostScan{CPUs: 1, MemTotalGB: 8}); h.Root != "~/w" ||
		h.Queue.Slots != 1 || h.Queue.MemReserveGB != 4 {
		t.Errorf("small host: %+v %+v", h, h.Queue)
	}
}

func TestAddHosts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "herd.json")

	// A missing config keeps the local host it stood for.
	if err := AddHosts(path, []HostConfig{{Name: "a", Type: HostSSH, Address: "a"}}); err != nil {
		t.Fatal(err)
	}

	var cfg UserConfig

	data, _ := os.ReadFile(path)
	if err := json.Unmarshal(data, &cfg); err != nil || len(cfg.Hosts) != 2 || cfg.Hosts[0].Name != HostLocal {
		t.Fatalf("new config: %s %v", data, err)
	}

	// An existing config keeps its keys, and a backup is written.
	if err := os.WriteFile(path, []byte(`{"hosts": [{"name": "x", "type": "local"}], "interval": 7}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := AddHosts(path, []HostConfig{{Name: "b", Type: HostSSH, Address: "b"}}); err != nil {
		t.Fatal(err)
	}

	data, _ = os.ReadFile(path)
	cfg = UserConfig{}

	if err := json.Unmarshal(data, &cfg); err != nil || len(cfg.Hosts) != 2 || cfg.Hosts[1].Name != "b" ||
		cfg.Interval != 7 {
		t.Errorf("updated config: %s %v", data, err)
	}

	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Errorf("no backup: %v", err)
	}
}
