package backend

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarchlab/akita/v5/herd"
)

// fakeRemote runs scripts with the local bash, as another host would
// (with its own HOME and a PATH without akita), but is not a localRunner,
// so backends treat it as remote.
type fakeRemote struct{ home string }

func (r fakeRemote) run(ctx context.Context, script string, stdout, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, "bash", "-s")
	cmd.Stdin = strings.NewReader(wrapStdin(script))
	cmd.Stdout, cmd.Stderr = stdout, stderr
	cmd.Env = []string{"HOME=" + r.home, "PATH=/usr/bin:/bin"}

	return cmd.Run()
}

func (r fakeRemote) rsyncPath(p string) string { return p }

// fakeAkita is a script that stands in for akita.
func fakeAkita(t *testing.T, dir, name string) string {
	t.Helper()

	p := filepath.Join(dir, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(p, []byte("#!/bin/sh\necho \""+name+" $*\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	return p
}

func remoteBackend(home, akitaDir string) *sshBackend {
	return &sshBackend{baseBackend{
		host:   herd.HostConfig{Name: "r", Type: herd.HostSSH, Address: "r", AkitaDir: akitaDir},
		runner: fakeRemote{home: home},
	}}
}

func TestDeployAkita(t *testing.T) {
	dir := t.TempDir()
	home, akitaDir := filepath.Join(dir, "home"), filepath.Join(dir, "deployed")
	local := fakeAkita(t, filepath.Join(dir, "local"), "deployed-akita")

	builds, notes := 0, 0
	SetDeployer(&Deployer{
		Key: func() string { return "k1" },
		Binary: func(_ context.Context, goos, goarch string) (string, error) {
			builds++

			if goos == "" || goarch == "" {
				t.Errorf("platform %s/%s", goos, goarch)
			}

			return local, nil
		},
		Notify: func(host, path string) {
			notes++

			if host != "r" || path != filepath.Join(akitaDir, "k1", "akita") {
				t.Errorf("notify %s %s", host, path)
			}
		},
	})
	t.Cleanup(func() { SetDeployer(nil) })

	ctx := context.Background()
	b := remoteBackend(home, akitaDir)

	for range 2 {
		out, err := b.Akita(ctx, "herd", "probe")
		if err != nil || strings.TrimSpace(string(out)) != "deployed-akita herd probe" {
			t.Fatalf("Akita: %q %v", out, err)
		}
	}

	if builds != 1 || notes != 1 {
		t.Errorf("%d builds, %d notes; want 1 each", builds, notes)
	}

	// Another backend (another command) finds the copy in place.
	p, err := remoteBackend(home, akitaDir).ensureAkita(ctx)
	if err != nil || p != filepath.Join(akitaDir, "k1", "akita") {
		t.Errorf("ensureAkita: %s %v", p, err)
	}

	if builds != 1 {
		t.Errorf("ensureAkita built again")
	}
}

func TestDeployFallsBackToPath(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")

	SetDeployer(&Deployer{
		Key: func() string { return "k2" },
		Binary: func(context.Context, string, string) (string, error) {
			return "", errors.New("no go here")
		},
	})
	t.Cleanup(func() { SetDeployer(nil) })

	ctx := context.Background()

	if _, err := remoteBackend(home, filepath.Join(dir, "d")).Akita(ctx, "herd"); err == nil ||
		!strings.Contains(err.Error(), "no go here") {
		t.Fatalf("no akita anywhere: %v", err)
	}

	// An akita installed by hand in ~/go/bin is used instead.
	fakeAkita(t, filepath.Join(home, "go", "bin"), "akita")

	out, err := remoteBackend(home, filepath.Join(dir, "d")).Akita(ctx, "herd", "x")
	if err != nil || strings.TrimSpace(string(out)) != "akita herd x" {
		t.Errorf("fallback: %q %v", out, err)
	}
}

func TestConfiguredAkitaIsNotDeployed(t *testing.T) {
	SetDeployer(&Deployer{Key: func() string { return "k3" }})
	t.Cleanup(func() { SetDeployer(nil) })

	b := remoteBackend(t.TempDir(), "")
	b.host.Akita = "/opt/akita"

	if m := b.managedAkita(); m != "" {
		t.Errorf("managed akita %s with a configured one", m)
	}

	b.host.Akita = ""
	if m := b.managedAkita(); m != DefaultAkitaDir+"/k3/akita" {
		t.Errorf("managed akita %s", m)
	}
}
