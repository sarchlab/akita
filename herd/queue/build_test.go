package queue

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarchlab/akita/v5/herd"
	"github.com/sarchlab/akita/v5/herd/internal/testutil"
)

func gitRepo(t *testing.T) (repo, project string) {
	t.Helper()

	repo = t.TempDir()
	project = filepath.Join(repo, "proj")
	testutil.WriteFiles(t, project, map[string]string{"input.txt": "v1", ".gitignore": "out/\n"})

	for _, args := range [][]string{
		{"init", "-q"}, {"add", "-A"},
		{"-c", "user.email=t@t", "-c", "user.name=t", "commit", "-q", "-m", "init"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	return repo, project
}

func buildCfg() *herd.ProjectConfig {
	return &herd.ProjectConfig{Build: &herd.BuildConfig{Binaries: []herd.BinarySpec{{
		Name: "sim", Command: []string{"sh", "-c",
			`printf '%s %s %s' "$GOOS/$GOARCH" "$FLAVOR" "$(cat input.txt extra.txt 2>/dev/null)" > {out}`},
		Env: map[string]string{"FLAVOR": "x"},
	}}}}
}

func buildOnce(t *testing.T, project, commit string) Built {
	t.Helper()

	built, err := Build(context.Background(), BuildRequest{
		ProjectDir: project, Config: buildCfg(), Commit: commit, Target: "linux/amd64",
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(built) != 1 {
		t.Fatalf("built %d binaries", len(built))
	}

	return built[0]
}

func TestBuildCleanDirtyCommit(t *testing.T) {
	repo, project := gitRepo(t)

	clean := buildOnce(t, project, "")
	if clean.Binary.Dirty || clean.Binary.Commit == "" || len(clean.Binary.SHA) != 64 {
		t.Fatalf("clean build = %+v", clean.Binary)
	}

	if got := testutil.ReadFile(t, clean.Path); got != "linux/amd64 x v1" {
		t.Fatalf("output = %q", got)
	}

	testutil.WriteFile(t, filepath.Join(project, "input.txt"), "v2")
	testutil.WriteFile(t, filepath.Join(project, "extra.txt"), "new")

	dirty := buildOnce(t, project, "")
	if !dirty.Binary.Dirty || dirty.Binary.Diff == "" || dirty.Binary.SHA == clean.Binary.SHA {
		t.Fatalf("dirty build = %+v", dirty.Binary)
	}

	if got := testutil.ReadFile(t, dirty.Path); got != "linux/amd64 x v2\nnew" && got != "linux/amd64 x v2new" {
		t.Fatalf("dirty output = %q", got)
	}

	if !strings.HasPrefix(dirty.Binary.Revision(), clean.Binary.Commit+"-dirty-") {
		t.Fatalf("revision = %q", dirty.Binary.Revision())
	}

	again := buildOnce(t, project, "HEAD")
	if again.Binary.Dirty || again.Binary.SHA != clean.Binary.SHA {
		t.Fatalf("--commit HEAD = %+v, want the clean build", again.Binary)
	}

	builds, err := LoadBuilds(project)
	if err != nil || builds["sim"].SHA != clean.Binary.SHA {
		t.Fatalf("builds.json = %+v, %v", builds, err)
	}

	out, _ := exec.Command("git", "-C", repo, "worktree", "list").Output()
	if n := strings.Count(strings.TrimSpace(string(out)), "\n"); n != 0 {
		t.Fatalf("worktrees left behind:\n%s", out)
	}

	if st, err := os.Stat(clean.Path); err != nil || st.Mode().Perm()&0o100 == 0 {
		t.Fatalf("binary not executable: %v", err)
	}
}

func TestBuildErrors(t *testing.T) {
	_, project := gitRepo(t)

	cfg := buildCfg()
	cfg.Build.Binaries[0].Command = []string{"sh", "-c", "echo broken >&2; exit 2"}

	_, err := Build(context.Background(), BuildRequest{ProjectDir: project, Config: cfg, Target: "linux/amd64"})
	if err == nil || !strings.Contains(err.Error(), "broken") {
		t.Fatalf("err = %v, want the build output", err)
	}

	_, err = Build(context.Background(), BuildRequest{ProjectDir: project, Config: cfg, Target: "linux",
		Names: []string{"sim"}})
	if err == nil {
		t.Fatal("bad target accepted")
	}

	_, err = Build(context.Background(), BuildRequest{ProjectDir: project, Config: cfg, Target: "linux/amd64",
		Names: []string{"nope"}})
	if err == nil {
		t.Fatal("unknown binary accepted")
	}
}
