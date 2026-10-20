package probe

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sarchlab/akita/v5/herd/internal/hostsys"
)

func TestUnmanaged(t *testing.T) {
	home, _ := os.UserHomeDir()
	busy := func(args string) hostsys.ProcInfo {
		return hostsys.ProcInfo{Etime: 600, CPUTime: 590, RSSKB: 2 << 20, Stat: "R", Args: args}
	}

	f := &fakeSystem{
		panes: map[string]int{"herd-proj-live": 100, "herd-agent-server1": 110, "0": 200},
		ppid: map[int]int{
			101: 100, 111: 110, 201: 200, 202: 201, 203: 202, 306: 200, 305: 306,
		},
		procs: map[int]hostsys.ProcInfo{
			100: {Etime: 600, Args: "bash -lc ./run"},
			101: busy("./sim -benchmark fir"), // a Herd session's
			110: {Etime: 600, Args: "bash -c exec akita herd agent run"},
			111: busy("./sim -benchmark aes"), // a queued task's
			200: {Etime: 900, Args: "-bash"},
			201: {Etime: 600, Args: "bash run.sh 1019_diag_x"},
			202: {Etime: 600, Args: "/usr/bin/time -v ./sim -benchmark mm"},
			203: busy("./sim -benchmark mm"), // started by hand: found
			300: busy("./other -n 3"),        // another user's
			301: busy("/home/u/.vscode-server/bin/node server.js"),
			302: busy("./sim -benchmark bfs"),                             // a Herd session's environment
			303: {Etime: 30, CPUTime: 29, Args: "./sim -benchmark young"}, // too young
			304: {Etime: 600, CPUTime: 60, Args: "./sim -benchmark idle"}, // not busy
			305: busy("python3 train.py"),                                 // from an interactive shell
			306: {Etime: 900, Args: "bash"},
		},
		owner:   map[int]int{300: fakeUID + 1},
		cwd:     map[int]string{203: filepath.Join(home, "exp-test", "p")},
		herdEnv: map[int]bool{302: true},
	}

	p, err := Probe(context.Background(), t.TempDir(), Options{sys: f, now: time.Unix(10000, 0)})
	if err != nil {
		t.Fatal(err)
	}

	if len(p.Unmanaged) != 2 {
		t.Fatalf("unmanaged: %+v", p.Unmanaged)
	}

	got := map[int]string{}
	for _, u := range p.Unmanaged {
		got[u.PID] = u.Launcher
	}

	if l, ok := got[203]; !ok || l != "bash run.sh 1019_diag_x" {
		t.Errorf("pid 203: launcher %q (found %v)", l, ok)
	}

	if l, ok := got[305]; !ok || l != "" {
		t.Errorf("pid 305: launcher %q (found %v)", l, ok)
	}

	for _, u := range p.Unmanaged {
		if u.PID == 203 && (u.Cwd != "~/exp-test/p" || u.Exe != "sim" || u.CPU < 98 || u.Start != 10000-600) {
			t.Errorf("pid 203: %+v", u)
		}
	}
}
