package deploy

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPlatform(t *testing.T) {
	for _, c := range []struct{ kernel, machine, goos, goarch string }{
		{"Linux", "x86_64", "linux", "amd64"},
		{"Linux", "aarch64", "linux", "arm64"},
		{"Darwin", "arm64", "darwin", "arm64"},
	} {
		goos, goarch, err := Platform(c.kernel, c.machine)
		if err != nil || goos != c.goos || goarch != c.goarch {
			t.Errorf("Platform(%s, %s) = %s, %s, %v", c.kernel, c.machine, goos, goarch, err)
		}
	}

	if _, _, err := Platform("Plan9", "x86_64"); err == nil {
		t.Error("Plan9 is supported")
	}
}

func TestKeyAndSource(t *testing.T) {
	if k := Key(); k == "" || strings.ContainsAny(k, "/ +") || k != Key() {
		t.Errorf("Key() = %q", k)
	}

	src, err := SourceDir()
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(src, "akita", "entry.go")); err != nil {
		t.Errorf("source %s has no akita command: %v", src, err)
	}
}

func TestBinaryThisPlatform(t *testing.T) {
	exe, _ := os.Executable()

	got, err := Binary(context.Background(), runtime.GOOS, runtime.GOARCH)
	if err != nil || got != exe {
		t.Errorf("Binary for this platform = %s, %v; want %s", got, err, exe)
	}
}

// Building for another platform takes a while (the akita command, from
// the build cache when warm).
func TestBinaryOtherPlatform(t *testing.T) {
	if testing.Short() {
		t.Skip("builds akita")
	}

	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not installed")
	}

	// Keep Go's caches while the user cache directory moves.
	for _, v := range []string{"GOCACHE", "GOMODCACHE"} {
		out, err := exec.Command("go", "env", v).Output()
		if err != nil {
			t.Fatal(err)
		}

		t.Setenv(v, strings.TrimSpace(string(out)))
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))

	goos := "linux"
	if runtime.GOOS == "linux" {
		goos = "darwin"
	}

	got, err := Binary(context.Background(), goos, "arm64")
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasPrefix(got, home) || !strings.Contains(got, Key()) {
		t.Errorf("binary %s is not in the cache under %s", got, home)
	}

	if info, err := os.Stat(got); err != nil || info.Size() < 1<<20 {
		t.Errorf("binary %s: %v", got, err)
	}

	again, err := Binary(context.Background(), goos, "arm64")
	if err != nil || again != got {
		t.Errorf("second Binary = %s, %v", again, err)
	}
}
