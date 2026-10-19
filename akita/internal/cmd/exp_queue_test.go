package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sarchlab/akita/v5/experiment"
)

// queueCLI is a CLI whose user config has local hosts with roots under
// the test directory ("qa", "qb", ...), as fake separate machines.
func queueCLI(t *testing.T, hosts ...experiment.HostConfig) *cli {
	t.Helper()

	c := newCLI(t, "akxq")
	c.env = append(c.env, agentIntervalEnv+"=300ms")
	base := filepath.Dir(c.project)

	for i := range hosts {
		hosts[i].Type = experiment.HostLocal
		hosts[i].Root = filepath.Join(base, hosts[i].Name)
	}

	data, err := json.Marshal(experiment.UserConfig{Hosts: hosts})
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(base, "user.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		for _, h := range hosts {
			_, _ = c.run("exp", "agent", "stop", h.Name)
		}
	})

	return c
}

func (c *cli) root(host string) string { return filepath.Join(filepath.Dir(c.project), host) }

// queueStatus reads a host's queue with the hidden `queue list`.
func (c *cli) queueStatus(host string) *experiment.QueueStatus {
	c.t.Helper()

	s := &experiment.QueueStatus{}
	if err := json.Unmarshal([]byte(c.must("exp", "queue", "list", "--root", c.root(host))), s); err != nil {
		c.t.Fatal(err)
	}

	return s
}

// waitQueue polls a host's queue until cond holds.
func (c *cli) waitQueue(host string, cond func(*experiment.QueueStatus) bool) *experiment.QueueStatus {
	c.t.Helper()

	var s *experiment.QueueStatus

	for range 150 {
		if s = c.queueStatus(host); cond(s) {
			return s
		}

		time.Sleep(100 * time.Millisecond)
	}

	c.t.Fatalf("queue of %s never reached the state: %+v", host, s)

	return s
}

func TestExpCLIAgent(t *testing.T) {
	c := queueCLI(t, experiment.HostConfig{Name: "qa", Queue: &experiment.QueueConfig{Slots: 1}})

	tasks := `[{"project": "akxq", "name": "one", "cmd": "echo one"},
	           {"project": "akxq", "name": "two", "cmd": "sleep 1; echo two"}]`
	c.must("exp", "queue", "put", "--root", c.root("qa"), tasks)

	if out := c.must("exp", "agent", "start", "qa"); !strings.Contains(out, "1 slots") {
		t.Fatalf("agent start: %s", out)
	}

	if out := c.must("exp", "agent", "start", "qa"); strings.Contains(out, "starting") {
		t.Fatalf("second start started another agent: %s", out)
	}

	s := c.waitQueue("qa", func(s *experiment.QueueStatus) bool { return len(s.Done) == 2 })
	for _, d := range s.Done {
		if d.State != "ok" || d.Host != "qa" || len(d.Attempts) != 1 {
			t.Errorf("done: %+v", d)
		}
	}

	if !s.Alive || s.Agent.Slots != 1 {
		t.Fatalf("agent: %+v alive %v", s.Agent, s.Alive)
	}

	if out := c.must("exp", "queue", "qa"); !strings.Contains(out, "agent: running") ||
		!strings.Contains(out, "two") {
		t.Errorf("queue: %s", out)
	}

	if out := c.must("exp", "log", "qa", "two"); !strings.Contains(out, "[exit 0]") {
		t.Errorf("log of a queued task: %s", out)
	}

	if out := c.must("exp", "agent", "stop", "qa"); !strings.Contains(out, "stopped the agent") {
		t.Fatalf("agent stop: %s", out)
	}

	if out := c.must("exp", "agent", "status", "qa"); !strings.Contains(out, "not running") {
		t.Errorf("agent status after stop: %s", out)
	}
}

func tasksOf(ts []experiment.Task) map[string]experiment.Task {
	out := map[string]experiment.Task{}
	for _, t := range ts {
		out[t.Name] = t
	}

	return out
}

func TestExpCLISubmitBalance(t *testing.T) {
	needRsync(t)

	c := queueCLI(t,
		experiment.HostConfig{Name: "qa", Queue: &experiment.QueueConfig{Slots: 1, AvoidSignals: []string{"SIGBUS"}}},
		experiment.HostConfig{Name: "qb", Queue: &experiment.QueueConfig{Slots: 3}})
	c.write("exp.json", `{"schema": "exp-project/1"}`)

	sha := strings.Repeat("5", 64)
	c.write(".akita-exp/builds.json", `{"tool": {"sha": "`+sha+`", "file": "tool", "commit": "abc1234"}}`)
	c.write(".akita-exp/bin/555555555555/tool", "#!/bin/sh\necho tool-ran \"$@\"\n")

	if err := os.Chmod(filepath.Join(c.project, ".akita-exp/bin/555555555555/tool"), 0o755); err != nil {
		t.Fatal(err)
	}

	c.must("exp", "submit", "--host", "qa", "long", "sleep 3")
	c.must("exp", "submit", "--host", "qa", "-g", "g1", "bin", "{bin:tool} hello")
	c.must("exp", "submit", "--host", "qa", "--priority", "9", "bus", "sh -c 'kill -BUS $$'")
	c.must("exp", "submit", "--host", "qa", "--priority", "9", "gone", "true")

	if out := c.must("exp", "cancel", "qa", "gone"); !strings.Contains(out, "cancelled gone") {
		t.Fatalf("cancel: %s", out)
	}

	if out, err := c.run("exp", "cancel", "qa", "long"); err == nil || !strings.Contains(out, "akita exp stop") {
		t.Fatalf("cancel of a running task: %v %s", err, out)
	}

	if _, err := c.run("exp", "submit", "--host", "qa", "x", "{bin:nope}"); err == nil {
		t.Fatal("a command naming an unbuilt binary was accepted")
	}

	c.must("exp", "agent", "start", "qb")

	out := c.must("exp", "balance")
	if !strings.Contains(out, "move bin: qa -> qb") || !strings.Contains(out, "move bus: qa -> qb") {
		t.Fatalf("balance: %s", out)
	}

	sb := c.waitQueue("qb", func(s *experiment.QueueStatus) bool { return len(s.Done) == 2 })
	done := tasksOf(sb.Done)

	if done["bin"].State != "ok" || done["bus"].Signal != "SIGBUS" || done["bus"].Retryable {
		t.Fatalf("qb done: %+v", sb.Done)
	}

	log := readText(t, filepath.Join(c.root("qb"), "akxq", "logs", "bin.log"))
	if !strings.Contains(log, "tool-ran hello") {
		t.Fatalf("bin.log: %s", log)
	}

	if g := readText(t, filepath.Join(c.root("qb"), "akxq", "logs", "bin.group")); g != "g1\n" {
		t.Fatalf("group = %q", g)
	}

	checkRetry(t, c)
}

// checkRetry runs a SIGBUS task on qa (which avoids SIGBUS) and lets
// balance resubmit it to qb.
func checkRetry(t *testing.T, c *cli) {
	t.Helper()

	c.must("exp", "submit", "--host", "qa", "bus2", "sh -c 'kill -BUS $$'")
	c.waitQueue("qa", func(s *experiment.QueueStatus) bool { return tasksOf(s.Done)["bus2"].Retryable })

	if out := c.must("exp", "balance", "-n"); !strings.Contains(out, "would retry bus2 (SIGBUS on qa) -> qb") {
		t.Fatalf("balance -n: %s", out)
	}

	c.must("exp", "balance")

	sa := c.queueStatus("qa")
	if tasksOf(sa.Done)["bus2"].Retried != "qb" {
		t.Fatalf("qa bus2 = %+v", tasksOf(sa.Done)["bus2"])
	}

	sb := c.waitQueue("qb", func(s *experiment.QueueStatus) bool { return tasksOf(s.Done)["bus2"].State != "" })
	if r := tasksOf(sb.Done)["bus2"]; len(r.Attempts) != 2 || !r.Avoids("qa") || r.Attempts[1].Host != "qb" {
		t.Fatalf("retried task = %+v", r)
	}

	if out := c.must("exp", "balance"); !strings.Contains(out, "nothing to move") {
		t.Fatalf("second balance: %s", out)
	}
}

func TestExpCLISubmitPlacement(t *testing.T) {
	needRsync(t)

	c := queueCLI(t,
		experiment.HostConfig{Name: "qa", Queue: &experiment.QueueConfig{Slots: 1}},
		experiment.HostConfig{Name: "qb", Queue: &experiment.QueueConfig{Slots: 1}})

	c.write("tasks.jsonl", `{"name": "a", "cmd": "sleep 2"}
{"name": "b", "cmd": "sleep 2", "priority": 1}
# a comment
{"name": "c", "cmd": "mkdir -p results/c && echo k > results/c/keep.txt && echo b > results/c/big.bin", "mem_gb": 0.001}
`)
	c.write("exp.json", `{"schema": "exp-project/1", "collect": {"exclude": ["*.bin"]}}`)

	out := c.must("exp", "submit", "--file", "tasks.jsonl")
	if !strings.Contains(out, "queued 2 tasks on qa") || !strings.Contains(out, "queued 1 tasks on qb") {
		t.Fatalf("submit: %s", out)
	}

	for _, h := range []string{"qa", "qb"} {
		c.waitQueue(h, func(s *experiment.QueueStatus) bool { return len(s.Pending)+len(s.Running) == 0 })
	}

	out = c.must("exp", "collect")
	if strings.Count(out, "collected ") != 3 || !strings.Contains(out, "collected qa:c (ok)") {
		t.Fatalf("collect: %s", out)
	}

	remote := filepath.Join(c.project, "remote")
	if readText(t, filepath.Join(remote, "qa", "results", "c", "keep.txt")) != "k\n" ||
		!strings.Contains(readText(t, filepath.Join(remote, "qb", "logs", "b.log")), "[exit 0]") {
		t.Fatal("collect copied the wrong files")
	}

	if _, err := os.Stat(filepath.Join(remote, "qa", "results", "c", "big.bin")); err == nil {
		t.Fatal("collect.exclude was ignored")
	}

	if out := c.must("exp", "collect"); !strings.Contains(out, "nothing to collect") {
		t.Fatalf("second collect: %s", out)
	}
}

func needRsync(t *testing.T) {
	t.Helper()

	if _, err := exec.LookPath("rsync"); err != nil {
		t.Skip("rsync not installed")
	}
}

func readText(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	return string(data)
}
