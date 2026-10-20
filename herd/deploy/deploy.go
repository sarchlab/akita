// Package deploy prepares this akita binary for other hosts, so that
// akita herd can put the same akita on a host by itself (the backend
// uploads it): for the host's platform, this executable, or one built
// from the same source.
package deploy

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
)

// mainPackage is the akita command.
const mainPackage = "github.com/sarchlab/akita/v5/akita"

// Version is the module version of this binary (go install ...@v), else
// its git revision ("-dirty" with uncommitted changes), else "unknown".
func Version() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}

	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v
	}

	var rev, dirty string

	for _, s := range info.Settings {
		switch {
		case s.Key == "vcs.revision" && len(s.Value) >= 12:
			rev = s.Value[:12]
		case s.Key == "vcs.modified" && s.Value == "true":
			dirty = "-dirty"
		}
	}

	if rev == "" {
		return "unknown"
	}

	return rev + dirty
}

var (
	keyOnce sync.Once
	key     string
	unsafe  = regexp.MustCompile(`[^A-Za-z0-9._-]+`)
)

// Key names this akita's copies on hosts and in the local cache: the
// version and a hash of this executable, so that a rebuilt akita (same
// version, other code) gets a copy of its own.
func Key() string {
	keyOnce.Do(func() {
		key = unsafe.ReplaceAllString(Version(), "_") + "-" + selfHash()
	})

	return key
}

func selfHash() string {
	exe, err := os.Executable()
	if err != nil {
		return "unknown"
	}

	f, err := os.Open(exe)
	if err != nil {
		return "unknown"
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "unknown"
	}

	return hex.EncodeToString(h.Sum(nil))[:12]
}

// Binary returns a local akita for a platform: this executable for this
// platform, else a build of the same source for that platform, kept in
// the user cache directory. Building needs Go and the source: the module
// directory this binary was built from (a checkout or the module cache).
func Binary(ctx context.Context, goos, goarch string) (string, error) {
	if goos == runtime.GOOS && goarch == runtime.GOARCH {
		return os.Executable()
	}

	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}

	out := filepath.Join(cache, "akita-herd", "akita", Key(), goos+"_"+goarch, "akita")
	if _, err := os.Stat(out); err == nil {
		return out, nil
	}

	src, err := SourceDir()
	if err != nil {
		return "", fmt.Errorf("cannot build akita for %s/%s: %w", goos, goarch, err)
	}

	if err := build(ctx, src, goos, goarch, out); err != nil {
		return "", fmt.Errorf("building akita for %s/%s in %s: %w", goos, goarch, src, err)
	}

	return out, nil
}

// SourceDir is the directory of the akita module this binary was built
// from, when it still exists (binaries built with -trimpath do not know).
func SourceDir() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok || !filepath.IsAbs(file) {
		return "", errors.New("this akita does not know its source (built with -trimpath)")
	}

	module := strings.TrimSuffix(mainPackage, "/akita")

	for dir := filepath.Dir(file); ; dir = filepath.Dir(dir) {
		if moduleOf(filepath.Join(dir, "go.mod")) == module {
			return dir, nil
		}

		if filepath.Dir(dir) == dir {
			return "", fmt.Errorf("the source of this akita (%s) is gone", filepath.Dir(file))
		}
	}
}

// moduleOf is the module path a go.mod declares ("" if none).
func moduleOf(gomod string) string {
	f, err := os.Open(gomod)
	if err != nil {
		return ""
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if m, ok := strings.CutPrefix(strings.TrimSpace(sc.Text()), "module "); ok {
			return strings.Trim(strings.TrimSpace(m), `"`)
		}
	}

	return ""
}

// build builds the akita command in src for a platform into out.
func build(ctx context.Context, src, goos, goarch, out string) error {
	gobin, err := exec.LookPath("go")
	if err != nil {
		return errors.New("go is not installed here")
	}

	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}

	tmp := out + ".tmp"
	cmd := exec.CommandContext(ctx, gobin, "build", "-trimpath", "-o", tmp, mainPackage)
	cmd.Dir = src
	cmd.Env = append(os.Environ(), "GOOS="+goos, "GOARCH="+goarch, "CGO_ENABLED=0")

	if msg, err := cmd.CombinedOutput(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(msg)))
	}

	return os.Rename(tmp, out)
}

// Platform maps `uname -s` and `uname -m` to GOOS and GOARCH.
func Platform(kernel, machine string) (goos, goarch string, err error) {
	switch strings.ToLower(kernel) {
	case "linux":
		goos = "linux"
	case "darwin":
		goos = "darwin"
	case "freebsd":
		goos = "freebsd"
	default:
		return "", "", fmt.Errorf("unsupported system %q", kernel)
	}

	switch strings.ToLower(machine) {
	case "x86_64", "amd64":
		goarch = "amd64"
	case "aarch64", "arm64":
		goarch = "arm64"
	case "ppc64le":
		goarch = "ppc64le"
	case "riscv64":
		goarch = "riscv64"
	default:
		return "", "", fmt.Errorf("unsupported machine %q", machine)
	}

	return goos, goarch, nil
}
