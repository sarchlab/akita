package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/sarchlab/akita/v5/herd"
)

// cliEnv makes the test binary act as the akita CLI; the local backend
// runs it again for `akita herd _local ...`.
const cliEnv = "AKITA_HERD_TEST_CLI"

func TestMain(m *testing.M) {
	if os.Getenv(cliEnv) == "1" {
		if err := Execute(); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}

		os.Exit(0)
	}

	os.Exit(m.Run())
}

// cli is a project directory and a user config for CLI tests.
type cli struct {
	t       *testing.T
	project string
	env     []string
}

func newCLI(t *testing.T, name string) *cli {
	t.Helper()

	base := t.TempDir()
	project := filepath.Join(base, name)

	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}

	//nolint:usetesting // tmux socket paths must be short; t.TempDir() is too long on macOS
	tmuxDir, err := os.MkdirTemp("/tmp", "akxc")
	if err != nil {
		t.Fatal(err)
	}

	c := &cli{t: t, project: project, env: append(os.Environ(),
		cliEnv+"=1", "AKITA_HERD_CONFIG="+filepath.Join(base, "user.json"),
		"TMUX_TMPDIR="+tmuxDir, "TMUX=", "HERD_GROUP=", "HERD_NO_SYNC=",
		"AKITA_EXP_CONFIG=", "EXP_GROUP=", "EXP_NO_SYNC=")}

	t.Cleanup(func() {
		k := exec.Command("tmux", "kill-server")
		k.Env = c.env
		_ = k.Run()
		_ = os.RemoveAll(tmuxDir)
	})

	return c
}

// run runs akita with args in the project directory.
func (c *cli) run(args ...string) (string, error) {
	c.t.Helper()

	cmd := exec.Command(os.Args[0], args...)
	cmd.Dir = c.project
	cmd.Env = c.env
	out, err := cmd.CombinedOutput()

	return string(out), err
}

func (c *cli) must(args ...string) string {
	c.t.Helper()

	out, err := c.run(args...)
	if err != nil {
		c.t.Fatalf("akita %s: %v\n%s", strings.Join(args, " "), err, out)
	}

	return out
}

func (c *cli) write(rel, content string) {
	c.t.Helper()

	p := filepath.Join(c.project, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		c.t.Fatal(err)
	}

	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		c.t.Fatal(err)
	}
}

// waitFor runs `exp status -a` until it shows the state of the session.
func (c *cli) waitFor(name, state string) string {
	c.t.Helper()

	var out string

	for range 100 {
		out = c.must("herd", "status", "-a", "-n", "0")
		for _, l := range strings.Split(out, "\n") {
			f := strings.Fields(l)
			if len(f) >= 2 && f[1] == name && strings.HasPrefix(f[0], state) {
				return out
			}
		}

		time.Sleep(100 * time.Millisecond)
	}

	c.t.Fatalf("session %s never became %s:\n%s", name, state, out)

	return out
}

func TestHerdCLILocal(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}

	c := newCLI(t, "akxcli")

	out := c.must("herd", "init")
	if !strings.Contains(out, "akxcli/herd.json") || !strings.Contains(out, "user.json") {
		t.Fatalf("init: %s", out)
	}

	if out := c.must("herd", "init"); !strings.Contains(out, "left unchanged") {
		t.Errorf("second init: %s", out)
	}

	c.must("herd", "sync")
	c.must("herd", "run", "local", "a", "echo", "hello", "world")
	c.must("herd", "run", "-g", "g", "local", "b", "sleep 30")
	c.must("herd", "run", "--no-sync", "local", "c", "d=R; echo results/${d}2 here; exit 2")

	c.waitFor("a", "ok")
	c.waitFor("b", "running")
	out = c.waitFor("c", "failed(2)")

	if !strings.Contains(out, "===== local =====") {
		t.Errorf("status: %s", out)
	}

	if out := c.must("herd", "log", "local", "a"); !strings.Contains(out, "hello world") ||
		!strings.Contains(out, "[exit 0]") {
		t.Errorf("log: %s", out)
	}

	if _, err := c.run("herd", "run", "local", "b", "echo dup"); err == nil {
		t.Errorf("duplicate run accepted")
	}

	if out, err := c.run("herd", "archive", "local", "g"); err == nil || !strings.Contains(out, "still running") {
		t.Errorf("archive of a running group: %v %s", err, out)
	}

	checkStopAndArchive(t, c)
}

// checkStopAndArchive stops session b, then archives and unarchives
// group g (sessions b and c).
func checkStopAndArchive(t *testing.T, c *cli) {
	t.Helper()

	c.must("herd", "stop", "local", "b")
	c.waitFor("b", "stopped")

	if out, err := c.run("herd", "stop", "local", "b"); err == nil || !strings.Contains(out, "not running") {
		t.Errorf("second stop: %v %s", err, out)
	}

	c.must("herd", "group", "local", "g", "c")
	c.write("results/R2/m", "1")

	if out := c.must("herd", "archive", "-n", "local", "g"); !strings.Contains(out, "Would archive 2 sessions") ||
		!strings.Contains(out, "--results R2") {
		t.Errorf("archive -n: %s", out)
	}

	c.must("herd", "archive", "local", "g", "--results", "R2")

	if out := c.must("herd", "archive", "local"); !strings.Contains(out, "group g  2") || !strings.Contains(out, "R2") {
		t.Errorf("archive listing: %s", out)
	}

	if out := c.must("herd", "log", "local", "c"); !strings.Contains(out, "[exit 2]") {
		t.Errorf("log of an archived session: %s", out)
	}

	c.must("herd", "unarchive", "local", "g")
	c.waitFor("c", "failed")

	if out := c.must("herd", "pull"); strings.Contains(out, "pulled") {
		t.Errorf("pull of the project workspace: %s", out)
	}
}

func TestHerdCLISessionsJSON(t *testing.T) {
	c := newCLI(t, "akxjson")
	c.write("logs/s.log", "[start] Fri Oct 16 01:56:33 AM UTC 2026  h  \nx\n[exit 0] Fri Oct 16 01:57:33 AM UTC 2026\n")
	c.write("logs/s.cmd", "echo x\n")

	out := c.must("herd", "_local", "sessions", c.project, "--tail", "1")

	var ss []herd.Session
	if err := json.Unmarshal([]byte(out), &ss); err != nil {
		t.Fatalf("%v: %s", err, out)
	}

	if len(ss) != 1 || ss[0].State != "ok" || ss[0].Cmd != "echo x" || ss[0].Project != "akxjson" ||
		!slices.Equal(ss[0].Tail, []string{"[exit 0] Fri Oct 16 01:57:33 AM UTC 2026"}) {
		t.Errorf("got %+v", ss)
	}

	// The monitor's probe of a local host without a root: this project only.
	c.write("../sibling/logs/x.log", "x\n")
	out = c.must("herd", "probe", "--json", "--root", filepath.Dir(c.project), "--project", "akxjson")

	var p herd.HostProbe
	if err := json.Unmarshal([]byte(out), &p); err != nil || len(p.Sessions) != 1 || len(p.Projects) != 1 {
		t.Errorf("probe --project: %v %s", err, out)
	}
}

// A project set up before the rename (exp.json, schema exp-project/1) and
// the former command name (akita exp) still work.
func TestHerdCLILegacyNames(t *testing.T) {
	c := newCLI(t, "akxlegacy")
	c.write("exp.json", `{"schema": "exp-project/1", "naming": {"separator": "_", "fields": [
		{"name": "date", "pattern": "\\d{4}"}]}}`)

	if out := c.must("exp", "name", "check", "1011"); strings.TrimSpace(out) != "ok" {
		t.Errorf("check via exp.json: %s", out)
	}

	if out, err := c.run("herd", "name", "check", "bad"); err == nil || !strings.Contains(out, `"bad"`) {
		t.Errorf("check of a bad name: %v %s", err, out)
	}

	if out := c.must("herd", "init"); !strings.Contains(out, "exp.json exists, left unchanged") {
		t.Errorf("init with exp.json: %s", out)
	}

	if _, err := os.Stat(filepath.Join(c.project, "herd.json")); err == nil {
		t.Errorf("init wrote herd.json next to exp.json")
	}
}

func TestHerdCLINamesAndRecord(t *testing.T) {
	c := newCLI(t, "akxname")
	c.write("herd.json", `{"schema": "herd-project/1", "naming": {"separator": "_", "fields": [
		{"name": "date", "pattern": "\\d{4}"}, {"name": "item", "pattern": "[a-z0-9]+"}],
		"legacy": "sed -n 's/^old-\\(.*\\)$/&\t1010_\\1/p'"}}`)
	c.write("results/old-x1/meta.json", `{"commit": "abc"}`)
	c.write("results/1011_y/m", "")
	c.write("results/odd_name_z/m", "")

	if out := c.must("herd", "name", "check", "1011_y"); strings.TrimSpace(out) != "ok" {
		t.Errorf("check: %s", out)
	}

	if out, err := c.run("herd", "name", "check", "1011_y", "bad"); err == nil || !strings.Contains(out, `"bad"`) {
		t.Errorf("check of a bad name: %v %s", err, out)
	}

	out := c.must("herd", "name", "plan", "results")
	if !strings.Contains(out, "old-x1\t1010_x1\n") || !strings.Contains(out, "1011_y\t1011_y\n") ||
		!strings.Contains(out, "odd_name_z\t?\n") {
		t.Errorf("plan: %s", out)
	}

	c.must("herd", "name", "apply", "results", "--dry-run")

	if _, err := os.Stat(filepath.Join(c.project, "results", "old-x1")); err != nil {
		t.Errorf("dry run renamed")
	}

	c.must("herd", "name", "apply", "results")

	rec, err := herd.ReadRecord(filepath.Join(c.project, "results", "1010_x1"))
	if err != nil || rec.Property("Renamed From") != "old-x1" {
		t.Errorf("record after apply: %+v %v", rec, err)
	}

	c.must("herd", "record", "results/1011_y", "Command=./sim -x", "Note=a=b")

	rec, err = herd.ReadRecord(filepath.Join(c.project, "results", "1011_y"))
	if err != nil || rec.Property("Command") != "./sim -x" || rec.Property("Note") != "a=b" ||
		rec.Fields["item"] != "y" {
		t.Errorf("record: %+v %v", rec, err)
	}
}

// Stopping `log -f` (SIGTERM) also stops its tail, which would otherwise
// keep writing to the terminal.
func TestHerdCLILogFollowStops(t *testing.T) {
	c := newCLI(t, "akxfollow")
	c.must("herd", "init")
	c.write("logs/f.log", "[start] Fri Oct 16 01:56:33 AM UTC 2026  h  \nline\n")

	var out strings.Builder

	cmd := exec.Command(os.Args[0], "herd", "log", "-f", "local", "f")
	cmd.Dir, cmd.Env, cmd.Stdout, cmd.Stderr = c.project, c.env, &out, &out

	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Second)

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		if err != nil || !strings.Contains(out.String(), "line") {
			t.Errorf("log -f: %v %s", err, out.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the output of log -f stayed open after it was stopped (tail still running)")
	}
}

func TestHerdCLIErrors(t *testing.T) {
	c := newCLI(t, "akxerr")

	for _, args := range [][]string{
		{"herd", "run", "nohost", "a", "echo"},
		{"herd", "run", "local", "a/b", "echo"},
		{"herd", "archive"},
		{"herd", "unarchive", "local"},
		{"herd", "name", "plan", "results"},
	} {
		if out, err := c.run(args...); err == nil {
			t.Errorf("%v accepted: %s", args, out)
		}
	}

	// An empty results directory is an error, not a panic.
	for _, args := range [][]string{
		{"herd", "name", "plan", "-H", "local", ""},
		{"herd", "name", "apply", "-H", "local", ""},
		{"herd", "name", "plan", ""},
	} {
		if out, err := c.run(args...); err == nil || strings.Contains(out, "panic") ||
			!strings.Contains(out, "empty results directory") {
			t.Errorf("%q: %v %s", args, err, out)
		}
	}

	out, _ := c.run("herd", "run", "nohost", "a", "echo")
	if !strings.Contains(out, `unknown host "nohost"`) || strings.Contains(out, "Usage:") {
		t.Errorf("unknown host error: %s", out)
	}
}

func TestSplitArchiveArgs(t *testing.T) {
	host, rest, dry := splitArchiveArgs([]string{"-n", "h", "g", "--results", "r"})
	if host != "h" || !dry || !slices.Equal(rest, []string{"g", "--results", "r"}) {
		t.Errorf("got %q %v %v", host, rest, dry)
	}

	if host, _, _ := splitArchiveArgs([]string{"--results", "r"}); host != "" {
		t.Errorf("host %q", host)
	}
}

func TestRawStatusMatchesExactTmuxNames(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}

	c := newCLI(t, "p")
	c.write("logs/a.b.log", "[start] x\nhello\n")
	c.write("logs/old.log", "[start] x\nhello\n")

	// herd-p-a_b is session a.b of project p; herd-p-x-y is session y of
	// project p-x, which a prefix match of "herd-p-" would also list;
	// exp-p-old is session old started before the rename.
	for _, s := range []string{"herd-p-a_b", "herd-p-x-y", "exp-p-old"} {
		cmd := exec.Command("tmux", "new-session", "-d", "-s", s, "sleep 30")
		cmd.Env = c.env

		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("tmux: %v %s", err, out)
		}
	}

	cmd := exec.Command("bash", "-c", rawStatusScript("p"))
	cmd.Dir, cmd.Env = c.project, c.env

	out, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "herd-p-a_b:") || strings.Contains(string(out), "herd-p-x-y") ||
		!strings.Contains(string(out), "exp-p-old:") || !strings.Contains(string(out), "--- a.b.log") {
		t.Errorf("%v\n%s", err, out)
	}
}
