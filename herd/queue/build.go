package queue

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/sarchlab/akita/v5/herd"
	"github.com/sarchlab/akita/v5/herd/internal/hostsys"
)

// buildsFile, in the project's .akita-herd/, maps each binary name to its
// latest build.
const buildsFile = "builds.json"

// maxUntrackedBytes bounds the untracked files a dirty build copies.
const maxUntrackedBytes = 10 << 20

// diffHashLen is the length of the hash of uncommitted changes.
const diffHashLen = 8

// DefaultTarget is the GOOS/GOARCH of builds.
const DefaultTarget = "linux/amd64"

// BuildRequest is what `akita herd build` builds.
type BuildRequest struct {
	ProjectDir string
	Config     *herd.ProjectConfig
	Commit     string   // "" builds HEAD with the uncommitted changes
	Target     string   // GOOS/GOARCH
	Names      []string // binaries to build; all when empty
	Log        io.Writer
}

// Built is a binary that Build produced.
type Built struct {
	Name   string
	Binary herd.Binary
	Path   string // in the project's .akita-herd/bin/
}

// Build checks the commit out into a temporary git worktree (applying the
// uncommitted changes and untracked files when building HEAD), runs each
// binary's command there with GOOS/GOARCH of the target, and stores the
// output in the project's .akita-herd/bin/<sha12>/<name>.
func Build(ctx context.Context, req BuildRequest) ([]Built, error) {
	specs, err := buildSpecs(req.Config, req.Names)
	if err != nil {
		return nil, err
	}

	if goos, goarch, ok := strings.Cut(req.Target, "/"); !ok || goos == "" || goarch == "" {
		return nil, fmt.Errorf("bad target %q (want GOOS/GOARCH)", req.Target)
	}

	if err := ignoreCache(req.ProjectDir); err != nil {
		return nil, err
	}

	src, err := checkout(ctx, req.ProjectDir, req.Commit)
	if err != nil {
		return nil, err
	}
	defer src.remove(ctx)

	builds, err := LoadBuilds(req.ProjectDir)
	if err != nil {
		return nil, err
	}

	var out []Built

	for _, spec := range specs {
		b, err := buildOne(ctx, req, src, spec)
		if err != nil {
			return out, err
		}

		builds[spec.Name] = b.Binary
		out = append(out, b)
	}

	return out, writeBuilds(req.ProjectDir, builds)
}

func buildSpecs(cfg *herd.ProjectConfig, names []string) ([]herd.BinarySpec, error) {
	if cfg == nil || cfg.Build == nil || len(cfg.Build.Binaries) == 0 {
		return nil, errors.New("herd.json has no build.binaries")
	}

	if len(names) == 0 {
		return cfg.Build.Binaries, nil
	}

	specs := make([]herd.BinarySpec, 0, len(names))

	for _, n := range names {
		s, ok := cfg.Binary(n)
		if !ok {
			return nil, fmt.Errorf("herd.json build has no binary %q", n)
		}

		specs = append(specs, s)
	}

	return specs, nil
}

func buildOne(ctx context.Context, req BuildRequest, src *source, spec herd.BinarySpec) (Built, error) {
	outPath := filepath.Join(src.tmp, "out", spec.Name)
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return Built{}, err
	}

	args := make([]string, len(spec.Command))
	for i, a := range spec.Command {
		args[i] = strings.ReplaceAll(a, "{out}", outPath)
	}

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = filepath.Join(src.projectDir, spec.Dir)
	// -trimpath: the worktree's path would make every build differ.
	goos, goarch, _ := strings.Cut(req.Target, "/")
	cmd.Env = append(os.Environ(), "GOOS="+goos, "GOARCH="+goarch,
		"GOFLAGS="+strings.TrimSpace(os.Getenv("GOFLAGS")+" -trimpath"))

	for k, v := range spec.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	var log bytes.Buffer

	cmd.Stdout, cmd.Stderr = &log, &log
	if err := cmd.Run(); err != nil {
		return Built{}, fmt.Errorf("build %s: %w\n%s", spec.Name, err, strings.TrimSpace(log.String()))
	}

	if req.Log != nil {
		_, _ = req.Log.Write(log.Bytes())
	}

	sum, err := fileSHA(outPath)
	if err != nil {
		return Built{}, fmt.Errorf("build %s: %w", spec.Name, err)
	}

	b := herd.Binary{
		SHA: sum, File: spec.Name, Commit: src.commit, Dirty: src.diff != "", Diff: src.diff,
		Target: req.Target, Built: herd.UnixSeconds(time.Now()),
	}

	dst := LocalBinary(req.ProjectDir, b)
	if err := copyFile(outPath, dst, 0o755); err != nil {
		return Built{}, err
	}

	return Built{Name: spec.Name, Binary: b, Path: dst}, nil
}

// ignoreCache makes git ignore the project's .akita-herd/, so that builds
// are not untracked files of the next dirty build.
func ignoreCache(projectDir string) error {
	p := filepath.Join(projectDir, Dir, ".gitignore")
	if hostsys.Exists(p) {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}

	return os.WriteFile(p, []byte("*\n"), 0o644)
}

// LocalBinary is where a build is stored in the project.
func LocalBinary(projectDir string, b herd.Binary) string {
	return filepath.Join(projectDir, Dir, BinDir, b.ShortSHA(), b.File)
}

// LoadBuilds reads the latest build of each binary of a project.
func LoadBuilds(projectDir string) (map[string]herd.Binary, error) {
	builds := map[string]herd.Binary{}

	data, err := os.ReadFile(filepath.Join(projectDir, Dir, buildsFile))
	if errors.Is(err, fs.ErrNotExist) {
		return builds, nil
	}

	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(data, &builds); err != nil {
		return nil, fmt.Errorf("%s: %w", buildsFile, err)
	}

	return builds, nil
}

func writeBuilds(projectDir string, builds map[string]herd.Binary) error {
	data, err := json.MarshalIndent(builds, "", "  ")
	if err != nil {
		return err
	}

	return hostsys.WriteFileAtomic(filepath.Join(projectDir, Dir, buildsFile), append(data, '\n'), 0o644)
}

// source is a checkout of the project to build in.
type source struct {
	repo       string // the project's git top level
	tmp        string // temporary directory holding the worktree
	projectDir string // the project directory in the worktree
	commit     string // short commit
	diff       string // hash of the applied uncommitted changes; "" if none
}

// remove runs even when ctx is canceled, so the worktree is not left behind.
func (s *source) remove(ctx context.Context) {
	_, _ = git(context.WithoutCancel(ctx), s.repo, nil, "worktree", "remove", "--force", filepath.Join(s.tmp, "wt"))
	_ = os.RemoveAll(s.tmp)
}

// checkout makes a worktree of commit (HEAD with the uncommitted changes
// when commit is "").
func checkout(ctx context.Context, projectDir, commit string) (*source, error) {
	top, err := git(ctx, projectDir, nil, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("the project is not in a git repository: %w", err)
	}

	prefix, _ := git(ctx, projectDir, nil, "rev-parse", "--show-prefix")

	rev := commit
	if rev == "" {
		rev = "HEAD"
	}

	full, err := git(ctx, top, nil, "rev-parse", "--verify", rev+"^{commit}")
	if err != nil {
		return nil, fmt.Errorf("commit %s: %w", rev, err)
	}

	short, _ := git(ctx, top, nil, "rev-parse", "--short=7", full)

	tmp, err := os.MkdirTemp("", "akita-herd-build")
	if err != nil {
		return nil, err
	}

	s := &source{repo: top, tmp: tmp, projectDir: filepath.Join(tmp, "wt", prefix), commit: short}
	if _, err := git(ctx, top, nil, "worktree", "add", "--detach", filepath.Join(tmp, "wt"), full); err != nil {
		_ = os.RemoveAll(tmp)
		return nil, err
	}

	if commit == "" {
		if err := s.applyChanges(ctx); err != nil {
			s.remove(ctx)
			return nil, err
		}
	}

	return s, nil
}

// applyChanges copies the uncommitted changes and the untracked files (up
// to 10 MB each) of the repository into the worktree.
func (s *source) applyChanges(ctx context.Context) error {
	diff, err := git(ctx, s.repo, nil, "diff", "HEAD", "--binary")
	if err != nil {
		return err
	}

	h := sha256.New()
	h.Write([]byte(diff))

	if diff != "" {
		if _, err := git(ctx, filepath.Join(s.tmp, "wt"), []byte(diff+"\n"), "apply", "--binary",
			"--whitespace=nowarn", "-"); err != nil {
			return fmt.Errorf("applying the uncommitted changes: %w", err)
		}
	}

	list, err := git(ctx, s.repo, nil, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return err
	}

	files := strings.Split(strings.Trim(list, "\x00"), "\x00")
	slices.Sort(files)

	n, err := s.copyUntracked(files, h)
	if err != nil {
		return err
	}

	if diff != "" || n > 0 {
		s.diff = hex.EncodeToString(h.Sum(nil))[:diffHashLen]
	}

	return nil
}

func (s *source) copyUntracked(files []string, h io.Writer) (int, error) {
	n := 0

	for _, f := range files {
		src := filepath.Join(s.repo, f)

		st, err := os.Stat(src)
		if f == "" || err != nil || !st.Mode().IsRegular() || st.Size() > maxUntrackedBytes {
			continue
		}

		data, err := os.ReadFile(src)
		if err != nil {
			return n, err
		}

		fmt.Fprintf(h, "%s\x00%d\x00", f, len(data))
		h.Write(data)

		if err := copyFile(src, filepath.Join(s.tmp, "wt", f), st.Mode().Perm()); err != nil {
			return n, err
		}

		n++
	}

	return n, nil
}

// git runs git in dir and returns its trimmed stdout.
func git(ctx context.Context, dir string, stdin []byte, args ...string) (string, error) {
	var stdout, stderr bytes.Buffer

	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr

	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}

	return strings.TrimRight(stdout.String(), "\n"), nil
}

func fileSHA(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

func copyFile(src, dst string, perm os.FileMode) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	return hostsys.WriteFileAtomic(dst, data, perm)
}
