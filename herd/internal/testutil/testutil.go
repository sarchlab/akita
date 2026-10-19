// Package testutil holds test helpers shared by the packages of akita herd.
package testutil

import (
	"database/sql"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	// The pure-Go SQLite driver "sqlite", as experiment registers it.
	_ "github.com/glebarez/go-sqlite"
)

// MustMkdir creates a directory and its parents.
func MustMkdir(t *testing.T, dir string) {
	t.Helper()

	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}

// WriteFile writes a file, creating its directory.
func WriteFile(t *testing.T, path, content string) {
	t.Helper()
	MustMkdir(t, filepath.Dir(path))

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// WriteFiles writes files given by their paths relative to root.
func WriteFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()

	for p, content := range files {
		WriteFile(t, filepath.Join(root, p), content)
	}
}

// ReadFile reads a file.
func ReadFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	return string(data)
}

// Exists tells whether a path exists.
func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Snapshot lists every file under root with its size and mtime.
func Snapshot(t *testing.T, root string) map[string]string {
	t.Helper()

	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		out[p] = fmt.Sprintf("%d %d", info.Size(), info.ModTime().UnixNano())

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	return out
}

// NeedTmux skips a test without tmux.
func NeedTmux(t *testing.T) {
	t.Helper()

	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
}

// NeedRsync skips a test without rsync.
func NeedRsync(t *testing.T) {
	t.Helper()

	if _, err := exec.LookPath("rsync"); err != nil {
		t.Skip("rsync not installed")
	}
}

// IsolateTmux points tmux to a private server for the test.
func IsolateTmux(t *testing.T) {
	t.Helper()

	//nolint:usetesting // tmux socket paths must be short; t.TempDir() is too long on macOS
	dir, err := os.MkdirTemp("/tmp", "akx")
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("TMUX_TMPDIR", dir)
	t.Setenv("TMUX", "")
	t.Cleanup(func() {
		_ = exec.Command("tmux", "kill-server").Run()
		_ = os.RemoveAll(dir)
	})
}

// MakeExecInfoDB writes an Akita SQLite file with an exec_info table.
func MakeExecInfoDB(t *testing.T, path string, rows map[string]string) {
	t.Helper()

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}

	defer db.Close()

	if _, err := db.Exec(`CREATE TABLE exec_info (Property TEXT, Value TEXT)`); err != nil {
		t.Fatal(err)
	}

	for k, v := range rows {
		if _, err := db.Exec(`INSERT INTO exec_info VALUES (?, ?)`, k, v); err != nil {
			t.Fatal(err)
		}
	}
}

// FakeSlurm puts fake sbatch/squeue/sacct/scancel on PATH and returns
// their directory. sbatch saves the script as job<id>.sh and queues the
// job (ids from 1000); RunFakeJob runs it. State files: state<id>
// (squeue's %T), final<id> (sacct); squeue-calls counts squeue calls.
func FakeSlurm(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	scripts := map[string]string{
		"sbatch": `id=$(cat "$D/next" 2>/dev/null || echo 1000); echo $((id+1)) > "$D/next"
cat > "$D/job$id.sh"; echo "$@" > "$D/args$id"; echo PENDING > "$D/state$id"; echo "$id;cluster"`,
		"squeue": `echo x >> "$D/squeue-calls"
while [ $# -gt 0 ]; do [ "$1" = -j ] && id=$2; shift; done
cat "$D/state$id" 2>/dev/null; exit 0`,
		"sacct": `while [ $# -gt 0 ]; do [ "$1" = -j ] && id=$2; shift; done
cat "$D/final$id" 2>/dev/null; exit 0`,
		"scancel": `rm -f "$D/state$1"; echo CANCELLED > "$D/final$1"; echo "$1" >> "$D/cancelled"`,
	}

	quoted := "'" + strings.ReplaceAll(dir, "'", `'\''`) + "'"

	for name, body := range scripts {
		content := "#!/bin/sh\nD=" + quoted + "\n" + body + "\n"
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	t.Setenv("PATH", dir+":/usr/bin:/bin:/usr/sbin:/sbin")

	return dir
}

// RunFakeJob runs a job queued by FakeSlurm's sbatch to its end.
func RunFakeJob(t *testing.T, fake, id string) {
	t.Helper()

	WriteFiles(t, fake, map[string]string{"state" + id: "RUNNING\n"})

	out, err := exec.Command("bash", filepath.Join(fake, "job"+id+".sh")).CombinedOutput()
	if err != nil {
		t.Fatalf("job %s: %v %s", id, err, out)
	}

	_ = os.Remove(filepath.Join(fake, "state"+id))
	WriteFiles(t, fake, map[string]string{"final" + id: "COMPLETED\n"})
}
