package backend

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sarchlab/akita/v5/experiment"
	"github.com/sarchlab/akita/v5/experiment/internal/hostsys"
	"github.com/sarchlab/akita/v5/experiment/internal/testutil"
	"github.com/sarchlab/akita/v5/experiment/probe"
)

func TestNew(t *testing.T) {
	cases := []struct {
		h    experiment.HostConfig
		ok   bool
		want string
	}{
		{experiment.HostConfig{Name: "l", Type: "local"}, true, "*backend.localBackend"},
		{experiment.HostConfig{Name: "l"}, true, "*backend.localBackend"},
		{experiment.HostConfig{Name: "s", Type: "ssh", Address: "x"}, true, "*backend.sshBackend"},
		{experiment.HostConfig{Name: "s", Type: "ssh"}, false, ""},
		{experiment.HostConfig{Name: "q", Type: "slurm"}, true, "*backend.slurmBackend"},
		{experiment.HostConfig{Name: "z", Type: "pbs"}, false, ""},
		{experiment.HostConfig{Type: "local"}, false, ""},
	}
	for _, c := range cases {
		b, err := New(c.h)
		if (err == nil) != c.ok {
			t.Errorf("%+v: err %v", c.h, err)
			continue
		}

		if c.ok {
			if got := fmt.Sprintf("%T", b); got != c.want {
				t.Errorf("%+v: %s", c.h, got)
			}
		}
	}
}

func TestWorkspaces(t *testing.T) {
	ssh, _ := New(experiment.HostConfig{Name: "s", Type: "ssh", Address: "s"})
	if got := ssh.Workspace("p"); got != "~/exp/p" {
		t.Errorf("ssh default workspace %s", got)
	}

	ssh2, _ := New(experiment.HostConfig{Name: "s", Type: "ssh", Address: "s", Root: "/data/x/"})
	if got := ssh2.Workspace("p"); got != "/data/x/p" {
		t.Errorf("ssh rooted workspace %s", got)
	}

	dir := t.TempDir()
	proj := filepath.Join(dir, "proj")
	testutil.MustMkdir(t, proj)
	t.Chdir(proj)

	cwd, _ := os.Getwd()
	local, _ := New(experiment.HostConfig{Name: "l", Type: "local"})

	if got := local.Workspace("proj"); got != cwd {
		t.Errorf("local workspace %s, want %s", got, cwd)
	}

	if got := local.Workspace("other"); got != filepath.Join(filepath.Dir(cwd), "other") {
		t.Errorf("local sibling workspace %s", got)
	}

	rooted, _ := New(experiment.HostConfig{Name: "l", Type: "local", Root: dir + "/ws"})
	if got := rooted.Workspace("proj"); got != filepath.Join(dir, "ws", "proj") {
		t.Errorf("local rooted workspace %s", got)
	}
}

func TestSSHRsyncPath(t *testing.T) {
	r := sshRunner{address: "h"}
	if got := r.rsyncPath("~/exp/p"); got != "h:exp/p" {
		t.Errorf("got %s", got)
	}

	if got := r.rsyncPath("/abs/p"); got != "h:/abs/p" {
		t.Errorf("got %s", got)
	}
}

// localProject makes a project directory and a local backend for it.
func localProject(t *testing.T, name string) (Backend, string) {
	t.Helper()

	dir := filepath.Join(t.TempDir(), name)
	testutil.MustMkdir(t, dir)
	t.Chdir(dir)

	cwd, _ := os.Getwd()

	b, err := New(experiment.HostConfig{Name: "local", Type: "local"})
	if err != nil {
		t.Fatal(err)
	}

	return b, cwd
}

// waitState polls probe.ListSessions until the session has the state.
func waitState(t *testing.T, ws, project, name, state string) experiment.Session {
	t.Helper()

	var last experiment.Session

	for range 100 {
		ss, err := probe.ListSessions(context.Background(), ws, project, true, 10)
		if err != nil {
			t.Fatal(err)
		}

		for _, s := range ss {
			if s.Name == name && !s.Archived {
				last = s
				if s.State == state {
					return s
				}
			}
		}

		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("session %s: state %q, want %q (tail %q)", name, last.State, state, last.Tail)

	return last
}

func TestLocalBackendTmux(t *testing.T) {
	testutil.NeedTmux(t)
	testutil.IsolateTmux(t)

	ctx := context.Background()
	b, ws := localProject(t, "akxproj")

	launch := func(name, group, cmd string) error {
		_, err := b.Launch(ctx, LaunchSpec{Project: "akxproj", Name: name, Group: group, Command: cmd})

		return err
	}

	if err := launch("ok1", "g", "echo hi; echo 'quoted'"); err != nil {
		t.Fatal(err)
	}

	if err := launch("fail3", "", "echo bad; exit 3"); err != nil {
		t.Fatal(err)
	}

	if err := launch("long", "g", "sleep 30"); err != nil {
		t.Fatal(err)
	}

	s := waitState(t, ws, "akxproj", "ok1", "ok")
	if s.Group != "g" || s.Cmd != "echo hi; echo 'quoted'" || !slices.Contains(s.Tail, "quoted") {
		t.Errorf("ok1: %+v", s)
	}

	s = waitState(t, ws, "akxproj", "fail3", "failed")
	if s.ExitCode == nil || *s.ExitCode != 3 {
		t.Errorf("fail3 exit code %v", s.ExitCode)
	}

	waitState(t, ws, "akxproj", "long", "running")

	if err := launch("long", "", "echo dup"); err == nil || !strings.Contains(err.Error(), "already running") {
		t.Errorf("duplicate launch: %v", err)
	}

	if err := b.Stop(ctx, "akxproj", "long"); err != nil {
		t.Fatal(err)
	}

	waitState(t, ws, "akxproj", "long", "stopped")

	if err := b.Stop(ctx, "akxproj", "long"); !errors.Is(err, errNotRunning) {
		t.Errorf("second stop: %v", err)
	}

	if log := readFile(t, filepath.Join(ws, "logs", "long.log")); !strings.Contains(log, "[stopped] ") {
		t.Errorf("no stopped marker: %q", log)
	}

	checkRelaunchAndShell(t, b, ws)
}

// checkRelaunchAndShell relaunches finished session ok1 and runs a shell
// command in the workspace.
func checkRelaunchAndShell(t *testing.T, b Backend, ws string) {
	t.Helper()

	ctx := context.Background()

	// Relaunching a finished name starts a fresh log.
	if _, err := b.Launch(ctx, LaunchSpec{Project: "akxproj", Name: "ok1", Command: "echo second"}); err != nil {
		t.Fatal(err)
	}

	waitState(t, ws, "akxproj", "ok1", "ok")

	if log := readFile(t, filepath.Join(ws, "logs", "ok1.log")); strings.Contains(log, "hi\n") {
		t.Errorf("old log kept: %q", log)
	}

	out, err := b.Shell(ctx, "akxproj", "pwd")
	if err != nil || strings.TrimSpace(string(out)) != ws {
		t.Errorf("Shell pwd: %q %v", out, err)
	}
}

func TestLocalBackendNohup(t *testing.T) {
	// With "tmux": false, sessions run under nohup and record their pid.
	// The marker dates do not depend on the locale.
	t.Setenv("LC_ALL", "de_DE.UTF-8")
	t.Setenv("LC_TIME", "de_DE.UTF-8")

	ctx := context.Background()
	b, ws := localProject(t, "akxnohup")
	noTmux := false
	b.(*localBackend).host.Tmux = &noTmux

	if _, err := b.Launch(ctx, LaunchSpec{Project: "akxnohup", Name: "n1", Command: "echo hi; exit 2"}); err != nil {
		t.Fatal(err)
	}

	if _, err := b.Launch(ctx, LaunchSpec{Project: "akxnohup", Name: "n2", Command: "sleep 30"}); err != nil {
		t.Fatal(err)
	}

	s := waitState(t, ws, "akxnohup", "n1", "failed")
	if s.Job == "" {
		t.Errorf("no job recorded: %+v", s)
	}

	s = waitState(t, ws, "akxnohup", "n2", "running")
	pid := s.Job

	if err := b.Stop(ctx, "akxnohup", "n2"); err != nil {
		t.Fatal(err)
	}

	s = waitState(t, ws, "akxnohup", "n2", "stopped")
	if s.Start == nil || s.End == nil {
		t.Errorf("marker dates not parsed: start %v end %v (log %q)", s.Start, s.End, s.Tail)
	}

	if n, _ := strconv.Atoi(pid); hostsys.PIDAlive(n) {
		t.Errorf("process %s still alive after stop", pid)
	}
}

func TestLocalBackendRootSyncPull(t *testing.T) {
	testutil.NeedRsync(t)

	ctx := context.Background()
	base := t.TempDir()
	proj := filepath.Join(base, "src", "akxsync")
	writeFiles(t, proj, map[string]string{"a.txt": "a", "results/r/x": "local result"})

	b, err := New(experiment.HostConfig{Name: "l2", Type: "local", Root: filepath.Join(base, "ws")})
	if err != nil {
		t.Fatal(err)
	}

	if err := b.Sync(ctx, proj, nil); err != nil {
		t.Fatal(err)
	}

	ws := b.Workspace("akxsync")
	if readFile(t, filepath.Join(ws, "a.txt")) != "a" || exists(filepath.Join(ws, "results", "r")) {
		t.Fatalf("sync result wrong")
	}

	writeFiles(t, ws, map[string]string{"logs/s.log": "log", "results/r2/y": "remote result"})

	pulled := filepath.Join(proj, "remote", "l2")
	if err := b.Pull(ctx, "akxsync", pulled); err != nil {
		t.Fatal(err)
	}

	if readFile(t, filepath.Join(pulled, "logs", "s.log")) != "log" ||
		readFile(t, filepath.Join(pulled, "results", "r2", "y")) != "remote result" {
		t.Errorf("pull incomplete")
	}
}

func TestTmuxNameCollisions(t *testing.T) {
	testutil.NeedTmux(t)
	testutil.IsolateTmux(t)

	ctx := context.Background()
	b, ws := localProject(t, "akxcol")

	launch := func(project, name string) error {
		_, err := b.Launch(ctx, LaunchSpec{Project: project, Name: name, Command: "sleep 30"})

		return err
	}

	// Sessions "a.b" and "a_b" share tmux session exp-akxcol-a_b.
	if err := launch("akxcol", "a.b"); err != nil {
		t.Fatal(err)
	}

	waitState(t, ws, "akxcol", "a.b", "running")

	if err := launch("akxcol", "a_b"); err == nil || !strings.Contains(err.Error(), "in use by another") {
		t.Errorf("colliding launch: %v", err)
	}

	if err := launch("akxcol", "a.b"); err == nil || !strings.Contains(err.Error(), "already running") {
		t.Errorf("relaunch: %v", err)
	}

	// So do session "x-y" of this project and session "y" of project
	// "akxcol-x" (a sibling workspace).
	if err := launch("akxcol", "x-y"); err != nil {
		t.Fatal(err)
	}

	if err := launch("akxcol-x", "y"); err == nil || !strings.Contains(err.Error(), "in use by another") {
		t.Errorf("colliding launch of another project: %v", err)
	}

	// Stopping the other session never kills the tmux session.
	if err := b.Stop(ctx, "akxcol-x", "y"); !errors.Is(err, errNotRunning) {
		t.Errorf("stop of the other session: %v", err)
	}

	waitState(t, ws, "akxcol", "x-y", "running")

	for _, n := range []string{"a.b", "x-y"} {
		if err := b.Stop(ctx, "akxcol", n); err != nil {
			t.Fatal(err)
		}

		waitState(t, ws, "akxcol", n, "stopped")
	}
}

func TestUploadKeepsExisting(t *testing.T) {
	testutil.NeedRsync(t)

	ctx := context.Background()
	base := t.TempDir()
	writeFiles(t, base, map[string]string{"one": "first", "two": "second"})

	b, err := New(experiment.HostConfig{Name: "l2", Type: "local", Root: filepath.Join(base, "ws")})
	if err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(Root(b), ".akita-exp", "bin", "abc", "sim")
	for _, src := range []string{"one", "two"} {
		if err := b.Upload(ctx, filepath.Join(base, src), dst); err != nil {
			t.Fatal(err)
		}
	}

	if got := readFile(t, dst); got != "first" {
		t.Fatalf("upload = %q, want the first upload kept", got)
	}
}

func TestPullRun(t *testing.T) {
	testutil.NeedRsync(t)

	ctx := context.Background()
	base := t.TempDir()

	b, err := New(experiment.HostConfig{Name: "l2", Type: "local", Root: filepath.Join(base, "ws")})
	if err != nil {
		t.Fatal(err)
	}

	writeFiles(t, b.Workspace("p"), map[string]string{
		"results/r1/a.txt": "a", "results/r1/big.bin": "big", "results/r10/x": "x",
		"logs/r1.log": "log", "logs/r1.cmd": "cmd", "logs/r10.log": "other", "logs/r1.x.log": "other",
	})

	dst := filepath.Join(base, "remote", "l2")
	if err := b.PullRun(ctx, "p", "r1", dst, []string{"*.bin"}); err != nil {
		t.Fatal(err)
	}

	for _, p := range []string{"results/r1/a.txt", "logs/r1.log", "logs/r1.cmd"} {
		if !exists(filepath.Join(dst, p)) {
			t.Errorf("%s not pulled", p)
		}
	}

	for _, p := range []string{"results/r1/big.bin", "results/r10", "logs/r10.log", "logs/r1.x.log"} {
		if exists(filepath.Join(dst, p)) {
			t.Errorf("%s pulled", p)
		}
	}

	if err := b.PullRun(ctx, "p", "nores", dst, nil); err != nil {
		t.Fatalf("a run without results: %v", err)
	}
}
