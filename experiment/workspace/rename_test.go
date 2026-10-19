package workspace

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarchlab/akita/v5/experiment"
)

func renameNaming() *experiment.Naming {
	return &experiment.Naming{
		Separator: "_",
		Fields: []experiment.NamingField{
			{Name: "date", Pattern: `\d{4}`},
			{Name: "category", Values: map[string]string{"abl": "ablation", "diag": "diagnosis"}},
			{Name: "item", Pattern: `[A-Za-z0-9][A-Za-z0-9.-]*`},
			{Name: "version", Pattern: `[0-9a-f]{7}`},
		},
		// Old names "<date>_<item>_old_<version>" become "<date>_abl_<item>_<version>".
		Legacy: `sed -n 's/^\([0-9]*\)_\([a-z0-9]*\)_old_\([0-9a-f]*\)$/&	\1_abl_\2_\3/p; ` +
			`s/^\(bad.*\)$/\1	not_valid/p'`,
	}
}

// resultsTree makes <ws>/results with old, new, and unknown names.
func resultsTree(t *testing.T) (ws, results string) {
	t.Helper()

	ws = filepath.Join(t.TempDir(), "proj")
	results = filepath.Join(ws, "results")
	old := filepath.Join(results, "1013_wg32k_old_5d72b18")
	writeFiles(t, results, map[string]string{
		"1013_wg32k_old_5d72b18/meta.json":         `{"out": "` + old + `/cell1", "commit": "5d72b18"}`,
		"1013_wg32k_old_5d72b18/cell1/stdout.log":  "writing " + old + "/cell1/metrics.csv\n",
		"1013_wg32k_old_5d72b18/cell1/metrics.csv": "where,what,value\n" + old + ",x,1\n",
		"1013_wg32k_old_5d72b18/b_metrics.csv":     old + "\n",
		"1013_wg32k_old_5d72b18/blob.bin":          old,
		"1014_diag_hbm_3e9a41c/run.json":           `{"schema": "exp-run/1"}`,
		"mystery/x":                                "?",
		"bad-one/x":                                "legacy maps it to an invalid name",
		"file.txt":                                 "not a directory",
	})
	writeFiles(t, ws, map[string]string{
		ArchiveFile: `{"results": {"1013_wg32k_old_5d72b18": {"archived_at": "t", "by": ["session:s"]}},` +
			` "sessions": {"s": {"name": "s", "results": ["1013_wg32k_old_5d72b18"]}}}`,
	})

	return ws, results
}

func TestPlanRenames(t *testing.T) {
	ws, results := resultsTree(t)

	plan, err := PlanRenames(context.Background(), results, renameNaming(), ws)
	if err != nil {
		t.Fatal(err)
	}

	got := map[string]string{}
	for _, r := range plan.Renames {
		got[r.Old] = r.New
	}

	want := map[string]string{
		"1013_wg32k_old_5d72b18": "1013_abl_wg32k_5d72b18",
		"1014_diag_hbm_3e9a41c":  "1014_diag_hbm_3e9a41c",
		"mystery":                "",
		"bad-one":                "",
	}
	if len(got) != len(want) {
		t.Errorf("got %v", got)
	}

	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s -> %q, want %q", k, got[k], v)
		}
	}

	if len(plan.Warnings) != 1 || !strings.Contains(plan.Warnings[0], "bad-one") {
		t.Errorf("warnings %v", plan.Warnings)
	}

	if _, err := PlanRenames(context.Background(), results, nil, ws); err == nil {
		t.Errorf("plan without a naming rule accepted")
	}
}

func TestApplyRenames(t *testing.T) {
	ws, results := resultsTree(t)
	naming := renameNaming()
	ctx := context.Background()
	oldDir := filepath.Join(results, "1013_wg32k_old_5d72b18")
	newDir := filepath.Join(results, "1013_abl_wg32k_5d72b18")

	plan, err := PlanRenames(ctx, results, naming, ws)
	if err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := ApplyRenames(results, naming, plan, true, &out); err != nil {
		t.Fatal(err)
	}

	if !exists(oldDir) || !strings.Contains(out.String(), "1013_wg32k_old_5d72b18\t1013_abl_wg32k_5d72b18\n") ||
		!strings.Contains(out.String(), "mystery\t?\n") {
		t.Fatalf("dry run: %s", out.String())
	}

	if err := ApplyRenames(results, naming, plan, false, &out); err != nil {
		t.Fatal(err)
	}

	if exists(oldDir) || !exists(newDir) {
		t.Fatalf("not renamed")
	}

	checkRenamedTree(t, ws, results, oldDir, newDir)

	// A second plan has nothing left to rename.
	plan, _ = PlanRenames(ctx, results, naming, ws)
	if len(plan.changes()) != 0 {
		t.Errorf("changes left: %v", plan.changes())
	}
}

func checkRenamedTree(t *testing.T, ws, results, oldDir, newDir string) {
	t.Helper()

	if s := readFile(t, filepath.Join(newDir, "meta.json")); !strings.Contains(s, newDir+"/cell1") {
		t.Errorf("meta.json not rewritten: %s", s)
	}

	if s := readFile(t, filepath.Join(newDir, "cell1", "stdout.log")); !strings.Contains(s, newDir) {
		t.Errorf("stdout.log not rewritten: %s", s)
	}

	for _, f := range []string{"cell1/metrics.csv", "b_metrics.csv", "blob.bin"} {
		if s := readFile(t, filepath.Join(newDir, f)); !strings.Contains(s, oldDir) {
			t.Errorf("%s rewritten: %s", f, s)
		}
	}

	rec, err := experiment.ReadRecord(newDir)
	if err != nil || rec.Property("Renamed From") != "1013_wg32k_old_5d72b18" || rec.Fields["category"] != "abl" {
		t.Errorf("run.json: %+v %v", rec, err)
	}

	db := loadDB(t, ws)
	if _, ok := db["results"].(map[string]any)["1013_abl_wg32k_5d72b18"]; !ok {
		t.Errorf("archive mark not moved: %v", db["results"])
	}

	s := db["sessions"].(map[string]any)["s"].(map[string]any)
	if strList(s["results"])[0] != "1013_abl_wg32k_5d72b18" {
		t.Errorf("session results not updated: %v", s)
	}

	tsv := readFile(t, filepath.Join(results, renamesFile))
	if fields := strings.Split(strings.TrimSpace(tsv), "\t"); len(fields) != 3 || fields[1] != "1013_wg32k_old_5d72b18" {
		t.Errorf("RENAMES.tsv: %q", tsv)
	}
}

func TestApplyRenamesRefuses(t *testing.T) {
	ws, results := resultsTree(t)
	naming := renameNaming()

	// Collision: the legacy name of one directory is another's name.
	writeFiles(t, results, map[string]string{"1013_abl_wg32k_5d72b18/x": "taken"})

	plan, err := PlanRenames(context.Background(), results, naming, ws)
	if err != nil {
		t.Fatal(err)
	}

	err = ApplyRenames(results, naming, plan, false, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "collide") {
		t.Errorf("collision: %v", err)
	}

	if err := os.RemoveAll(filepath.Join(results, "1013_abl_wg32k_5d72b18")); err != nil {
		t.Fatal(err)
	}

	// Busy: a process names the directory on its command line.
	cmd := exec.Command("sh", "-c", "sleep 30; : 1013_wg32k_old_5d72b18")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()

	plan, _ = PlanRenames(context.Background(), results, naming, ws)

	err = ApplyRenames(results, naming, plan, false, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "processes still use") {
		t.Errorf("busy: %v", err)
	}

	if !exists(filepath.Join(results, "1013_wg32k_old_5d72b18")) || exists(filepath.Join(results, renamesFile)) {
		t.Errorf("refused apply changed files")
	}
}

func TestRewritePathsSkipsLargeFiles(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"a.json": `{"p": "/old/x"}`, "b.txt": "nothing"})

	big := filepath.Join(dir, "big.log")
	if err := os.WriteFile(big, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.Truncate(big, maxRewriteBytes+1); err != nil {
		t.Fatal(err)
	}

	n, err := rewritePaths(dir, "/old/x", "/new/y")
	if err != nil || n != 1 {
		t.Fatalf("%d %v", n, err)
	}

	var m map[string]string
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(dir, "a.json"))), &m); err != nil || m["p"] != "/new/y" {
		t.Errorf("a.json: %v %v", m, err)
	}
}

func TestRewritePathsWholePathsOnly(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"a.json": `{"p": "/results/foo", "q": "/results/foo/x", "r": "/results/foo_v2/x", ` +
			`"s": "/results/foo+m1", "t": "/results/foo.bak", "u": "/results/foobar"}`,
		"b.txt": "/results/foo\n/results/foo,1\n'/results/foo' /results/foo2 /results/foo-x\n",
		"c.log": "/results/foo_v2 only\n",
	})

	n, err := rewritePaths(dir, "/results/foo", "/results/bar")
	if err != nil || n != 2 {
		t.Fatalf("%d %v", n, err)
	}

	if got, want := readFile(t, filepath.Join(dir, "a.json")),
		`{"p": "/results/bar", "q": "/results/bar/x", "r": "/results/foo_v2/x", `+
			`"s": "/results/foo+m1", "t": "/results/foo.bak", "u": "/results/foobar"}`; got != want {
		t.Errorf("a.json:\n%s\nwant\n%s", got, want)
	}

	if got, want := readFile(t, filepath.Join(dir, "b.txt")),
		"/results/bar\n/results/bar,1\n'/results/bar' /results/foo2 /results/foo-x\n"; got != want {
		t.Errorf("b.txt: %q, want %q", got, want)
	}

	if got := readFile(t, filepath.Join(dir, "c.log")); got != "/results/foo_v2 only\n" {
		t.Errorf("c.log changed: %q", got)
	}
}

func TestContainsPath(t *testing.T) {
	cases := map[string]bool{
		"sleep 30; : foo":         true,
		"run --out=results/foo/x": true,
		"run results/foo_v2":      false,
		"run results/foo_v2 foo":  true,
		"run results/foo+cfg":     false,
		`run "results/foo"`:       true,
		"foo":                     true,
		"foobar":                  false,
	}
	for in, want := range cases {
		if got := containsPath([]byte(in), []byte("foo")); got != want {
			t.Errorf("containsPath(%q) = %v, want %v", in, got, want)
		}
	}
}
