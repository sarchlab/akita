package cmd

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarchlab/akita/v5/herd"
)

// {name} in a command is the session's name, which the session also has
// as HERD_NAME.
func TestHerdCLIRunName(t *testing.T) {
	c := newCLI(t, "akxname2")

	c.must("herd", "run", "local", "s1", "echo {name}:$HERD_NAME")
	c.waitFor("s1", "ok")

	if out := c.must("herd", "log", "local", "s1"); !strings.Contains(out, "s1:s1") {
		t.Errorf("log: %s", out)
	}

	if out, err := c.run("herd", "run", "local", "a b", "echo", "{name}"); err == nil ||
		!strings.Contains(out, "HERD_NAME") {
		t.Errorf("{name} of a name with a space: %v %s", err, out)
	}
}

func TestHerdCLISubmitEachAndWait(t *testing.T) {
	c := queueCLI(t, herd.HostConfig{Name: "qa", Queue: &herd.QueueConfig{Slots: 2}},
		herd.HostConfig{Name: "qb", Queue: &herd.QueueConfig{Slots: 2}})

	out := c.must("herd", "submit", "--hosts", "qa,qb", "--each", "w=a,b", "--each", "c=x,y",
		"1019_{w}_{c}", "echo {name}:$HERD_NAME:{w}")
	if n := strings.Count(out, ">> sync"); n != 2 {
		t.Errorf("want one sync per host, got %d:\n%s", n, out)
	}

	c.must("herd", "wait", "--timeout", "30s")

	ended := 0

	for _, h := range []string{"qa", "qb"} {
		for _, d := range c.queueStatus(h).Done {
			ended++

			want := d.Name + ":" + d.Name + ":" + strings.Split(d.Name, "_")[1]
			if d.State != "ok" || !strings.Contains(c.must("herd", "log", h, d.Name), want) {
				t.Errorf("%s on %s: %s, log lacks %s", d.Name, h, d.State, want)
			}
		}
	}

	if ended != 4 {
		t.Errorf("%d tasks ended, want 4", ended)
	}

	for _, bad := range [][]string{
		{"--each", "w=a,b", "1019_x", "true"},   // the name does not use w
		{"--each", "w=a", "1019_{z}", "true"},   // no --each z
		{"--each", "w=a b", "1019_{w}", "true"}, // a space in a value
	} {
		if out, err := c.run(append([]string{"herd", "submit", "--host", "qa"}, bad...)...); err == nil {
			t.Errorf("submit %v succeeded: %s", bad, out)
		}
	}

	c.must("herd", "submit", "--host", "qa", "slow", "sleep 30")

	if out := c.must("herd", "wait", "--until", "running", "--timeout", "30s", "qa", "slow"); !strings.Contains(
		out, "qa:slow running") {
		t.Errorf("wait --until running: %s", out)
	}

	if out, err := c.run("herd", "wait", "--timeout", "1s", "qa", "slow"); err == nil ||
		!strings.Contains(out, "timed out") {
		t.Errorf("wait with a timeout: %v %s", err, out)
	}

	c.must("herd", "stop", "qa", "slow")

	if out, err := c.run("herd", "wait", "qa", "nosuch"); err == nil || !strings.Contains(out, "nosuch") {
		t.Errorf("wait for an unknown name: %v %s", err, out)
	}
}

func TestHerdCLIDoctor(t *testing.T) {
	c := queueCLI(t, herd.HostConfig{Name: "qa", Queue: &herd.QueueConfig{Slots: 1}})

	out := c.must("herd", "doctor", "qa")
	if !strings.Contains(out, "===== qa (local)") || strings.Contains(out, "FAIL") ||
		!strings.Contains(out, "agent not running") {
		t.Errorf("doctor: %s", out)
	}

	c.write("../bad.json", `{"hosts": [{"name": "broken", "type": "local", "root": "`+
		filepath.Join(filepath.Dir(c.project), "r")+`", "akita": "/nonexistent/akita"}]}`)
	c.env = append(c.env, "AKITA_HERD_CONFIG="+filepath.Join(filepath.Dir(c.project), "bad.json"))

	if out, err := c.run("herd", "doctor"); err == nil || !strings.Contains(out, "FAIL  akita not found") {
		t.Errorf("doctor with a missing akita: %v %s", err, out)
	}
}
