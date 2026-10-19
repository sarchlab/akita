package backend

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/sarchlab/akita/v5/experiment"
	"github.com/sarchlab/akita/v5/experiment/internal/testutil"
)

func TestSbatchScript(t *testing.T) {
	h := experiment.HostConfig{Name: "hpc", Type: "slurm", Slurm: &experiment.SlurmConfig{
		Partition: "cpu", Account: "lab", Time: "24:00:00", CPUs: 4, Mem: "16G",
		Extra: []string{"--gres=gpu:1", "#SBATCH --qos=low"},
	}}

	s := sbatchScript(h, "~/exp/p", LaunchSpec{Project: "p", Name: "a.b", Command: "./sim -x"})
	for _, want := range []string{
		"#!/bin/bash -l\n", "#SBATCH --job-name=exp-p-a_b\n", "#SBATCH --output=/dev/null\n",
		"#SBATCH --partition=cpu\n", "#SBATCH --account=lab\n", "#SBATCH --time=24:00:00\n",
		"#SBATCH --cpus-per-task=4\n", "#SBATCH --mem=16G\n", "#SBATCH --gres=gpu:1\n",
		"#SBATCH --qos=low\n", `cd "$HOME"/exp/p`, "( eval './sim -x' )",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in\n%s", want, s)
		}
	}

	if strings.Contains(sbatchScript(experiment.HostConfig{Name: "x"}, "/w", LaunchSpec{Name: "n"}), "--partition") {
		t.Errorf("options without a slurm config")
	}
}

func TestSlurmBackend(t *testing.T) {
	fake := testutil.FakeSlurm(t)
	ctx := context.Background()
	root := t.TempDir()

	b, err := New(experiment.HostConfig{Name: "hpc", Type: "slurm", Root: root,
		Slurm: &experiment.SlurmConfig{Partition: "cpu"}})
	if err != nil {
		t.Fatal(err)
	}

	ws := b.Workspace("sp")

	for i, spec := range []LaunchSpec{
		{Project: "sp", Name: "a", Group: "g", Command: "echo hello; exit 5"},
		{Project: "sp", Name: "b", Command: "echo never"},
	} {
		job, err := b.Launch(ctx, spec)
		if err != nil {
			t.Fatal(err)
		}

		if want := strconv.Itoa(1000 + i); job != want {
			t.Errorf("Launch returned job %q, want %q", job, want)
		}
	}

	if got := readFile(t, filepath.Join(ws, "logs", "a.job")); got != "1000\nslurm\n" {
		t.Errorf("a.job = %q", got)
	}

	if !strings.Contains(readFile(t, filepath.Join(fake, "job1000.sh")), "#SBATCH --partition=cpu") {
		t.Errorf("sbatch script without options")
	}

	s := waitState(t, ws, "sp", "a", "queued")
	if s.Job != "1000" || s.Group != "g" {
		t.Errorf("queued session %+v", s)
	}

	if _, err := b.Launch(ctx, LaunchSpec{Project: "sp", Name: "a", Command: "x"}); err == nil {
		t.Errorf("relaunch of a queued session accepted")
	}

	testutil.RunFakeJob(t, fake, "1000")

	s = waitState(t, ws, "sp", "a", "failed")
	if s.ExitCode == nil || *s.ExitCode != 5 || !strings.Contains(strings.Join(s.Tail, "\n"), "hello") {
		t.Errorf("finished session %+v", s)
	}

	if err := b.Stop(ctx, "sp", "b"); err != nil {
		t.Fatal(err)
	}

	if strings.TrimSpace(readFile(t, filepath.Join(fake, "cancelled"))) != "1001" {
		t.Errorf("scancel not called for 1001")
	}

	waitState(t, ws, "sp", "b", "stopped")
}
