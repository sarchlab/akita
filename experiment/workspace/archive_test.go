package workspace

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// archiveWorkspace makes a workspace with three finished sessions and
// three result directories; tmux is kept off PATH.
func archiveWorkspace(t *testing.T) string {
	t.Helper()
	t.Setenv("PATH", "/usr/bin:/bin")

	ws := filepath.Join(t.TempDir(), "proj")
	writeFiles(t, ws, map[string]string{
		"logs/a.log":        "[start] x  h  \nwrote results/R1/m\n[exit 0] x\n",
		"logs/a.cmd":        "./sim -out results/R1\n",
		"logs/a.group":      "g\n",
		"logs/b.log":        "[exit 1] x\n",
		"logs/b.cmd":        "./other\n",
		"logs/b.group":      "g\n",
		"logs/c.log":        "campaign in results/R2/cell3 and results/missing\n[exit 0] x\n",
		"logs/c.cmd":        "./run_all.sh\n",
		"results/R1/m":      "1",
		"results/R2/m":      "2",
		"results/R3/m":      "3",
		".exp_archive.json": `{"custom": {"keep": true}, "results": {"R9": {"archived_at": "old", "by": ["manual"]}}}`,
	})

	return ws
}

func runArchive(t *testing.T, op func(context.Context, ArchiveRequest) error, ws string,
	args ...string,
) (string, error) {
	t.Helper()

	tokens, results, dry := ParseArchiveArgs(args)

	var out bytes.Buffer
	err := op(context.Background(), ArchiveRequest{
		Workspace: ws, Project: "proj", Tokens: tokens, Results: results, DryRun: dry, Out: &out,
	})

	return out.String(), err
}

func loadDB(t *testing.T, ws string) map[string]any {
	t.Helper()

	var m map[string]any
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(ws, ArchiveFile))), &m); err != nil {
		t.Fatal(err)
	}

	return m
}

func dbBy(t *testing.T, ws, result string) []string {
	t.Helper()

	e, ok := loadDB(t, ws)["results"].(map[string]any)[result].(map[string]any)
	if !ok {
		return nil
	}

	return strList(e["by"])
}

func TestParseArchiveArgs(t *testing.T) {
	tokens, results, dry := ParseArchiveArgs([]string{"g", "-n", "x", "--results", "results/R1/", "R2"})
	if !dry || !slices.Equal(tokens, []string{"g", "x"}) || !slices.Equal(results, []string{"R1", "R2"}) {
		t.Errorf("got %v %v %v", tokens, results, dry)
	}
}

func TestArchiveDryRunChangesNothing(t *testing.T) {
	ws := archiveWorkspace(t)
	before := readFile(t, filepath.Join(ws, ArchiveFile))

	out, err := runArchive(t, Archive, ws, "-n", "g")
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out, "Would archive 2 sessions") || !strings.Contains(out, "result directory R1") ||
		!strings.Contains(out, "dry run") {
		t.Errorf("output:\n%s", out)
	}

	if readFile(t, filepath.Join(ws, ArchiveFile)) != before || !exists(filepath.Join(ws, "logs", "a.log")) {
		t.Errorf("dry run changed files")
	}
}

func TestArchiveRoundTrip(t *testing.T) {
	ws := archiveWorkspace(t)
	arch := filepath.Join(ws, "logs", "archived")

	out, err := runArchive(t, Archive, ws, "g")
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out, "Archiving 2 sessions") || !strings.Contains(out, "Marked 1 result directories") {
		t.Errorf("output:\n%s", out)
	}

	for _, f := range []string{"a.log", "a.cmd", "a.group", "b.log", "b.cmd", "b.group"} {
		if !exists(filepath.Join(arch, f)) || exists(filepath.Join(ws, "logs", f)) {
			t.Errorf("%s not moved", f)
		}
	}

	db := loadDB(t, ws)
	if db["custom"] == nil || db["version"] == nil {
		t.Errorf("unknown keys lost: %v", db)
	}

	if by := dbBy(t, ws, "R1"); !slices.Equal(by, []string{"session:a"}) {
		t.Errorf("R1 by %v", by)
	}

	g := db["groups"].(map[string]any)["g"].(map[string]any)
	if !slices.Equal(strList(g["sessions"]), []string{"a", "b"}) {
		t.Errorf("group %v", g)
	}

	checkArchivedGroupAndUndo(t, ws)
}

func checkArchivedGroupAndUndo(t *testing.T, ws string) {
	t.Helper()

	arch := filepath.Join(ws, "logs", "archived")

	// Already archived group: --results attach to it.
	out, err := runArchive(t, Archive, ws, "g", "--results", "R3")
	if err != nil || !strings.Contains(out, "group g is already archived") {
		t.Fatalf("%v\n%s", err, out)
	}

	if by := dbBy(t, ws, "R3"); !slices.Equal(by, []string{"group:g"}) {
		t.Errorf("R3 by %v", by)
	}

	// Listing.
	out, _ = runArchive(t, Archive, ws)
	if !strings.Contains(out, "group g  2") || !strings.Contains(out, "R3") {
		t.Errorf("listing:\n%s", out)
	}

	// Unarchive the group: logs move back, its marks go, others stay.
	out, err = runArchive(t, Unarchive, ws, "g")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}

	if !exists(filepath.Join(ws, "logs", "a.log")) || exists(filepath.Join(arch, "a.log")) {
		t.Errorf("a not moved back")
	}

	db := loadDB(t, ws)
	if len(db["groups"].(map[string]any)) != 0 || len(db["sessions"].(map[string]any)) != 0 {
		t.Errorf("db after unarchive: %v", db)
	}

	if dbBy(t, ws, "R1") != nil || dbBy(t, ws, "R3") != nil || dbBy(t, ws, "R9") == nil {
		t.Errorf("result marks after unarchive: %v", db["results"])
	}
}

func TestArchiveHintsAndResults(t *testing.T) {
	ws := archiveWorkspace(t)

	out, err := runArchive(t, Archive, ws, "c")
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out, "Hint:") || !strings.Contains(out, "--results R2") ||
		strings.Contains(out, "missing") {
		t.Errorf("output:\n%s", out)
	}

	if _, err := runArchive(t, Archive, ws, "--results", "R2"); err != nil {
		t.Fatal(err)
	}

	if by := dbBy(t, ws, "R2"); !slices.Equal(by, []string{"manual"}) {
		t.Errorf("R2 by %v", by)
	}

	out, err = runArchive(t, Unarchive, ws, "--results", "R2")
	if err != nil || dbBy(t, ws, "R2") != nil {
		t.Errorf("unarchive --results: %v\n%s", err, out)
	}
}

func TestArchiveErrors(t *testing.T) {
	ws := archiveWorkspace(t)
	before := readFile(t, filepath.Join(ws, ArchiveFile))

	for _, args := range [][]string{{"nosuch"}, {"g", "--results", "R404"}} {
		_, err := runArchive(t, Archive, ws, args...)
		if err == nil || !strings.Contains(err.Error(), "Nothing was changed") {
			t.Errorf("%v: %v", args, err)
		}
	}

	if _, err := runArchive(t, Unarchive, ws, "a"); err == nil {
		t.Errorf("unarchive of a live session name accepted")
	}

	if _, err := runArchive(t, Unarchive, ws); err == nil {
		t.Errorf("unarchive without arguments accepted")
	}

	// A live process (this test) owns session b, whose log has no exit
	// line yet.
	writeFiles(t, ws, map[string]string{
		"logs/b.job": strconv.Itoa(os.Getpid()) + "\npid\n",
		"logs/b.log": "[start] x  h  \nworking\n",
	})

	_, err := runArchive(t, Archive, ws, "g")
	if err == nil || !strings.Contains(err.Error(), "still running") {
		t.Errorf("busy: %v", err)
	}

	if readFile(t, filepath.Join(ws, ArchiveFile)) != before || !exists(filepath.Join(ws, "logs", "a.log")) {
		t.Errorf("refused archive changed files")
	}

	writeFiles(t, ws, map[string]string{ArchiveFile: "{bad"})

	if _, err := runArchive(t, Archive, ws, "c"); err == nil || !strings.Contains(err.Error(), "not valid JSON") {
		t.Errorf("bad JSON: %v", err)
	}

	if _, err := runArchive(t, Archive, filepath.Join(ws, "nope"), "c"); err == nil {
		t.Errorf("missing workspace accepted")
	}
}

func TestArchiveNameCollisions(t *testing.T) {
	ws := archiveWorkspace(t)
	arch := filepath.Join(ws, "logs", "archived")

	if _, err := runArchive(t, Archive, ws, "a"); err != nil {
		t.Fatal(err)
	}

	// A new session reuses the name, then is archived too: it becomes a~2.
	writeFiles(t, ws, map[string]string{"logs/a.log": "second\n[exit 0] x\n", "logs/a.cmd": "again\n"})

	out, err := runArchive(t, Archive, ws, "a")
	if err != nil || !strings.Contains(out, "a → archived/a~2") {
		t.Fatalf("%v\n%s", err, out)
	}

	if readFile(t, filepath.Join(arch, "a~2.log")) != "second\n[exit 0] x\n" {
		t.Errorf("a~2.log wrong")
	}

	// A third session named a blocks restoring both under the name "a".
	writeFiles(t, ws, map[string]string{"logs/a.log": "third\n"})

	out, err = runArchive(t, Unarchive, ws, "a")
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected a refusal: %v\n%s", err, out)
	}

	if err := os.Remove(filepath.Join(ws, "logs", "a.log")); err != nil {
		t.Fatal(err)
	}

	// Without it, the first gets "a" back and the second keeps "a~2".
	out, err = runArchive(t, Unarchive, ws, "a")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}

	if readFile(t, filepath.Join(ws, "logs", "a~2.log")) != "second\n[exit 0] x\n" ||
		!strings.Contains(readFile(t, filepath.Join(ws, "logs", "a.log")), "results/R1") {
		t.Errorf("restored names wrong:\n%s", out)
	}
}

func TestSetGroup(t *testing.T) {
	ws := archiveWorkspace(t)

	var out bytes.Buffer

	n, err := SetGroup(ws, "new group", []string{"a", "c", "nosuch"}, &out)
	if err != nil || n != 2 || !strings.Contains(out.String(), "no session nosuch") {
		t.Fatalf("%d %v %s", n, err, out.String())
	}

	if readFile(t, filepath.Join(ws, "logs", "c.group")) != "new group\n" {
		t.Errorf("group not written")
	}

	if n, err := SetGroup(ws, "-", []string{"a", "c"}, &out); err != nil || n != 2 {
		t.Fatalf("%d %v", n, err)
	}

	if exists(filepath.Join(ws, "logs", "a.group")) {
		t.Errorf("group not removed")
	}
}

func TestLoadArchivedResults(t *testing.T) {
	ws := archiveWorkspace(t)
	if got := LoadArchivedResults(ws); !got["R9"] || len(got) != 1 {
		t.Errorf("got %v", got)
	}
}

func TestArchiveFinishedSessionWithLiveJob(t *testing.T) {
	// The exit line wins over a .job whose pid is alive (e.g. reused).
	ws := archiveWorkspace(t)
	writeFiles(t, ws, map[string]string{"logs/b.job": strconv.Itoa(os.Getpid()) + "\npid\n"})

	out, err := runArchive(t, Archive, ws, "g")
	if err != nil || !strings.Contains(out, "Archiving 2 sessions") {
		t.Errorf("%v\n%s", err, out)
	}
}
