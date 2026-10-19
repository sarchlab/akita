package workspace

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/sarchlab/akita/v5/experiment"
)

// maxRewriteBytes is the largest file whose recorded paths are rewritten.
const maxRewriteBytes = 64 << 20

// rewriteSuffixes are the text files whose recorded paths are rewritten.
var rewriteSuffixes = []string{".json", ".stdout", ".log", ".txt", ".csv", ".tsv"}

// Rename is the planned new name of a result directory; New is "" when
// neither the naming rule nor the legacy command knows the directory.
type Rename struct {
	Old string
	New string
}

// RenamePlan is the plan for a results directory.
type RenamePlan struct {
	Renames  []Rename // every directory, sorted by old name
	Warnings []string
}

// PlanRenames maps every directory of resultsDir to its name under the
// naming rule: names that follow it are kept, the others are given to the
// project's naming.legacy command (run in projectDir, names on stdin,
// "old<TAB>new" lines on stdout).
func PlanRenames(ctx context.Context, resultsDir string, naming *experiment.Naming, projectDir string,
) (*RenamePlan, error) {
	if naming == nil {
		return nil, errors.New("the project has no naming rule (exp.json \"naming\")")
	}

	if _, err := os.ReadDir(resultsDir); err != nil {
		return nil, err
	}

	plan := &RenamePlan{}

	var legacy []string

	for _, d := range ResultDirs(resultsDir) {
		r := Rename{Old: d.Name}
		if experiment.CheckName(r.Old, naming) == nil {
			r.New = r.Old
		} else {
			legacy = append(legacy, r.Old)
		}

		plan.Renames = append(plan.Renames, r)
	}

	sort.Slice(plan.Renames, func(i, j int) bool { return plan.Renames[i].Old < plan.Renames[j].Old })

	if len(legacy) == 0 || naming.Legacy == "" {
		return plan, nil
	}

	mapped, err := runLegacy(ctx, naming.Legacy, projectDir, legacy)
	if err != nil {
		return nil, err
	}

	for i, r := range plan.Renames {
		n, ok := mapped[r.Old]
		if !ok || n == "" {
			continue
		}

		if err := experiment.CheckName(n, naming); err != nil {
			plan.Warnings = append(plan.Warnings, fmt.Sprintf("legacy name of %s ignored: %v", r.Old, err))
			continue
		}

		plan.Renames[i].New = n
	}

	return plan, nil
}

// runLegacy runs the naming.legacy command on names.
func runLegacy(ctx context.Context, command, dir string, names []string) (map[string]string, error) {
	var stdout, stderr bytes.Buffer

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(strings.Join(names, "\n") + "\n")
	cmd.Stdout, cmd.Stderr = &stdout, &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("naming.legacy %q: %w: %s", command, err, strings.TrimSpace(stderr.String()))
	}

	out := map[string]string{}

	sc := bufio.NewScanner(&stdout)
	for sc.Scan() {
		old, n, ok := strings.Cut(sc.Text(), "\t")
		if ok {
			out[old] = strings.TrimSpace(n)
		}
	}

	return out, sc.Err()
}

// changes are the renames that change a name.
func (p *RenamePlan) changes() []Rename {
	var out []Rename

	for _, r := range p.Renames {
		if r.New != "" && r.New != r.Old {
			out = append(out, r)
		}
	}

	return out
}

// unknown are the directories without a new name.
func (p *RenamePlan) unknown() []string {
	var out []string

	for _, r := range p.Renames {
		if r.New == "" {
			out = append(out, r.Old)
		}
	}

	return out
}

// collisions are the new names that more than one directory would get.
func (p *RenamePlan) collisions() []string {
	count := map[string]int{}
	for _, r := range p.Renames {
		if r.New != "" {
			count[r.New]++
		}
	}

	var dups []string

	for n, c := range count {
		if c > 1 {
			dups = append(dups, n)
		}
	}

	sort.Strings(dups)

	return dups
}

// Print writes "old<TAB>new" for each change and "old<TAB>?" for each
// unknown directory.
func (p *RenamePlan) Print(w io.Writer) {
	for _, r := range p.changes() {
		fmt.Fprintf(w, "%s\t%s\n", r.Old, r.New)
	}

	for _, o := range p.unknown() {
		fmt.Fprintf(w, "%s\t?\n", o)
	}
}

// ApplyRenames renames the directories of a plan in resultsDir: it
// rewrites the absolute paths recorded in their small text files, moves
// their marks in ../.exp_archive.json, records "Renamed From" in their
// run.json, and appends to RENAMES.tsv. It refuses (changing nothing) when
// names collide or a process names a directory on its command line. With
// dryRun it only prints the plan.
func ApplyRenames(resultsDir string, naming *experiment.Naming, plan *RenamePlan, dryRun bool,
	out io.Writer,
) error {
	results, err := filepath.Abs(resultsDir)
	if err != nil {
		return err
	}

	if err := checkRenames(results, plan); err != nil {
		return err
	}

	plan.Print(out)

	if dryRun {
		return nil
	}

	db, err := loadArchiveDB(filepath.Join(filepath.Dir(results), ArchiveFile))
	if err != nil {
		return err
	}

	err = applyRenames(results, naming, plan.changes(), db)
	if db.exists {
		if serr := db.save(); err == nil {
			err = serr
		}
	}

	return err
}

func checkRenames(results string, plan *RenamePlan) error {
	if dups := plan.collisions(); len(dups) > 0 {
		return fmt.Errorf("the new names collide: %s%s", strings.Join(dups, ", "), nothingChanged)
	}

	changes := plan.changes()
	for _, r := range changes {
		if _, err := os.Lstat(filepath.Join(results, r.New)); err == nil {
			return fmt.Errorf("results/%s already exists%s", r.New, nothingChanged)
		}
	}

	busy, err := dirsInUse(changes)
	if err != nil {
		return err
	}

	if len(busy) > 0 {
		return fmt.Errorf("processes still use these directories:\n  %s%s",
			strings.Join(busy, "\n  "), nothingChanged)
	}

	return nil
}

// dirsInUse are the old names that some process names on its command line
// (by name, since scripts pass relative paths such as results/<name>).
func dirsInUse(changes []Rename) ([]string, error) {
	if len(changes) == 0 {
		return nil, nil
	}

	out, err := exec.Command("ps", "-eo", "args").Output()
	if err != nil {
		return nil, fmt.Errorf("cannot list processes (%w)%s", err, nothingChanged)
	}

	var busy []string

	for _, r := range changes {
		if containsPath(out, []byte(r.Old)) {
			busy = append(busy, r.Old)
		}
	}

	sort.Strings(busy)

	return busy, nil
}

func applyRenames(results string, naming *experiment.Naming, changes []Rename, db *archiveDB) error {
	now := time.Now().Format(recordTime)

	for _, r := range changes {
		src, dst := filepath.Join(results, r.Old), filepath.Join(results, r.New)
		if err := os.Rename(src, dst); err != nil {
			return err
		}

		if _, err := rewritePaths(dst, src, dst); err != nil {
			return err
		}

		if _, err := experiment.WriteRecord(dst, naming,
			[]experiment.Property{{Property: "Renamed From", Value: r.Old}}); err != nil {
			return err
		}

		db.renameResult(r.Old, r.New)

		if err := appendLine(filepath.Join(results, renamesFile),
			now+"\t"+r.Old+"\t"+r.New+"\n"); err != nil {
			return err
		}
	}

	return nil
}

func appendLine(path, line string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}

	_, err = f.WriteString(line)
	if cerr := f.Close(); err == nil {
		err = cerr
	}

	return err
}

// rewritePaths replaces oldPath with newPath in the small text files under
// dir (statistics files metrics.csv and *_metrics.csv are skipped) and
// returns how many files changed. Only whole paths are replaced: oldPath
// followed by a path boundary (see containsPath), so that renaming
// /results/foo leaves /results/foo_v2 alone.
func rewritePaths(dir, oldPath, newPath string) (int, error) {
	changed := 0
	oldB, newB := []byte(oldPath), []byte(newPath)

	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !rewritable(d) {
			return err
		}

		data, err := os.ReadFile(p)
		if err != nil || !containsPath(data, oldB) {
			return err
		}

		fi, err := d.Info()
		if err != nil {
			return err
		}

		changed++

		return os.WriteFile(p, replacePath(data, oldB, newB), fi.Mode().Perm())
	})

	return changed, err
}

// nameByte tells whether c can be part of a file name as result directory
// names are written: letters, digits, "._-+~", and bytes of UTF-8
// sequences. Any other byte (end, '/', quotes, whitespace, ',', '=', ...)
// ends a path.
func nameByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' ||
		strings.IndexByte("._-+~", c) >= 0 || c >= utf8.RuneSelf
}

// pathEnds tells whether data[i:] starts at a path boundary.
func pathEnds(data []byte, i int) bool {
	return i >= len(data) || !nameByte(data[i])
}

// containsPath tells whether data holds old as a whole path: followed by
// the end, '/', a quote, whitespace, or another byte that is not part of a
// name.
func containsPath(data, old []byte) bool {
	for i := 0; ; {
		j := bytes.Index(data[i:], old)
		if j < 0 {
			return false
		}

		if pathEnds(data, i+j+len(old)) {
			return true
		}

		i += j + 1
	}
}

// replacePath replaces old by new where containsPath would find it.
func replacePath(data, old, new []byte) []byte {
	var out bytes.Buffer

	for {
		j := bytes.Index(data, old)
		if j < 0 {
			out.Write(data)
			return out.Bytes()
		}

		end := j + len(old)
		if pathEnds(data, end) {
			out.Write(data[:j])
			out.Write(new)
		} else {
			out.Write(data[:end])
		}

		data = data[end:]
	}
}

func rewritable(d os.DirEntry) bool {
	name := d.Name()
	if !d.Type().IsRegular() || name == "metrics.csv" || strings.HasSuffix(name, "_metrics.csv") {
		return false
	}

	if fi, err := d.Info(); err != nil || fi.Size() > maxRewriteBytes {
		return false
	}

	for _, s := range rewriteSuffixes {
		if strings.HasSuffix(name, s) {
			return true
		}
	}

	return false
}
