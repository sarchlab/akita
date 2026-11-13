// Package tracecompare compares two vis-trace databases written by Akita's
// task tracer. It backs the determinism CI job: two serial runs of the same
// program must produce identical traces, and a parallel run must produce the
// same trace as a serial run once task and milestone IDs are removed.
package tracecompare

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"

	_ "github.com/glebarez/go-sqlite" // registers the "sqlite" driver

	"github.com/sarchlab/akita/v5/internal/sqlitepath"
)

// Mode selects how strictly two traces are compared.
type Mode int

const (
	// Exact requires the same tasks, milestones, and tags with the same IDs.
	Exact Mode = iota

	// IgnoreIDs requires the same task forest up to renaming IDs: each task is
	// identified by its content (kind, what, location, start and end time,
	// milestones, and tags) together with the structure and content of its
	// whole subtree.
	IgnoreIDs
)

// maxReportedDiffs bounds how many differences a Result lists.
const maxReportedDiffs = 20

// Result reports the outcome of a comparison.
type Result struct {
	// NumTasks holds the number of tasks in the first and the second trace.
	NumTasks [2]int

	// NumDiffs counts the differing entries. Zero means the traces match.
	NumDiffs int

	// Diffs lists up to maxReportedDiffs differing entries, for diagnosis.
	Diffs []string
}

// Equal reports whether the two traces matched.
func (r Result) Equal() bool {
	return r.NumDiffs == 0
}

// Compare loads the traces at pathA and pathB and compares them.
func Compare(pathA, pathB string, mode Mode) (Result, error) {
	a, err := load(pathA)
	if err != nil {
		return Result{}, err
	}

	b, err := load(pathB)
	if err != nil {
		return Result{}, err
	}

	var keysA, keysB map[string]int

	switch mode {
	case Exact:
		keysA, keysB = a.exactKeys(), b.exactKeys()
	case IgnoreIDs:
		keysA, keysB = a.idFreeKeys(), b.idFreeKeys()
	default:
		return Result{}, fmt.Errorf("unknown comparison mode %d", mode)
	}

	result := Result{NumTasks: [2]int{len(a.tasks), len(b.tasks)}}
	diffCounts(keysA, keysB, "only in first: ", &result)
	diffCounts(keysB, keysA, "only in second: ", &result)
	sort.Strings(result.Diffs)

	return result, nil
}

func diffCounts(from, other map[string]int, prefix string, result *Result) {
	for key, n := range from {
		extra := n - other[key]
		if extra <= 0 {
			continue
		}

		result.NumDiffs += extra
		if len(result.Diffs) < maxReportedDiffs {
			result.Diffs = append(result.Diffs,
				fmt.Sprintf("%s%s (x%d)", prefix, key, extra))
		}
	}
}

type task struct {
	id, parentID uint64
	kind, what   string
	location     string
	start, end   float64
}

type milestone struct {
	id, taskID uint64
	time       float64
	kind, what string
}

type tag struct {
	id, taskID uint64
	time       float64
	what       string
}

type trace struct {
	tasks      map[uint64]task
	milestones map[uint64][]milestone
	tags       map[uint64][]tag
}

func load(path string) (*trace, error) {
	db, err := sql.Open("sqlite", sqlitepath.URI(path, "ro"))
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer db.Close()

	t := &trace{
		tasks:      make(map[uint64]task),
		milestones: make(map[uint64][]milestone),
		tags:       make(map[uint64][]tag),
	}

	loaders := []func(*sql.DB) error{t.loadTasks, t.loadMilestones, t.loadTags}
	for _, loadTable := range loaders {
		if err := loadTable(db); err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
	}

	return t, nil
}

func (t *trace) loadTasks(db *sql.DB) error {
	rows, err := db.Query(`
		SELECT t.ID, t.ParentID, t.Kind, t.What, COALESCE(l.Locale, ''),
			t.StartTime, t.EndTime
		FROM "akita$trace" t
		LEFT JOIN "akita$location" l ON t.Location = l.ID`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var tk task
		if err := rows.Scan(&tk.id, &tk.parentID, &tk.kind, &tk.what,
			&tk.location, &tk.start, &tk.end); err != nil {
			return err
		}

		t.tasks[tk.id] = tk
	}

	return rows.Err()
}

func (t *trace) loadMilestones(db *sql.DB) error {
	rows, err := db.Query(
		`SELECT ID, TaskID, Time, Kind, What FROM "akita$milestone"`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var m milestone
		if err := rows.Scan(&m.id, &m.taskID, &m.time, &m.kind, &m.what); err != nil {
			return err
		}

		t.milestones[m.taskID] = append(t.milestones[m.taskID], m)
	}

	return rows.Err()
}

func (t *trace) loadTags(db *sql.DB) error {
	rows, err := db.Query(`SELECT ID, TaskID, Time, What FROM "akita$tag"`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var g tag
		if err := rows.Scan(&g.id, &g.taskID, &g.time, &g.what); err != nil {
			return err
		}

		t.tags[g.taskID] = append(t.tags[g.taskID], g)
	}

	return rows.Err()
}

func formatTime(v float64) string {
	return strconv.FormatFloat(v, 'g', -1, 64)
}

// exactKeys describes every task, milestone, and tag including its IDs.
func (t *trace) exactKeys() map[string]int {
	keys := make(map[string]int)

	for _, tk := range t.tasks {
		keys[fmt.Sprintf("task id=%d parent=%d %s", tk.id, tk.parentID,
			t.content(tk))]++
	}

	for _, ms := range t.milestones {
		for _, m := range ms {
			keys[fmt.Sprintf("milestone id=%d task=%d t=%s %s %s",
				m.id, m.taskID, formatTime(m.time), m.kind, m.what)]++
		}
	}

	for _, gs := range t.tags {
		for _, g := range gs {
			keys[fmt.Sprintf("tag id=%d task=%d t=%s %s",
				g.id, g.taskID, formatTime(g.time), g.what)]++
		}
	}

	return keys
}

// idFreeKeys describes every task by its content and a hash of its whole
// subtree, so the comparison is a rooted-forest isomorphism check: two traces
// match exactly when they are the same task forest up to renaming IDs.
// Milestones and tags are folded into their task's content.
func (t *trace) idFreeKeys() map[string]int {
	children := make(map[uint64][]uint64)
	for id, tk := range t.tasks {
		if _, ok := t.tasks[tk.parentID]; ok && tk.parentID != id {
			children[tk.parentID] = append(children[tk.parentID], id)
		}
	}

	hashes := make(map[uint64]string, len(t.tasks))
	keys := make(map[string]int, len(t.tasks))

	for id, tk := range t.tasks {
		h := t.subtreeHash(id, children, hashes, map[uint64]bool{})
		keys[fmt.Sprintf("task {%s} subtree %s",
			t.content(tk)+t.children(id), h[:16])]++
	}

	return keys
}

// subtreeHash returns a hash of a task's content and the sorted hashes of its
// children, so it identifies the subtree up to ID renaming. It memoizes in
// hashes; visiting guards against malformed traces with parent cycles.
func (t *trace) subtreeHash(
	id uint64,
	children map[uint64][]uint64,
	hashes map[uint64]string,
	visiting map[uint64]bool,
) string {
	if h, ok := hashes[id]; ok {
		return h
	}

	if visiting[id] {
		return "cycle"
	}
	visiting[id] = true

	childHashes := make([]string, 0, len(children[id]))
	for _, c := range children[id] {
		childHashes = append(childHashes, t.subtreeHash(c, children, hashes, visiting))
	}
	sort.Strings(childHashes)

	sum := sha256.New()
	sum.Write([]byte(t.content(t.tasks[id]) + t.children(id)))
	for _, c := range childHashes {
		sum.Write([]byte{0})
		sum.Write([]byte(c))
	}

	h := hex.EncodeToString(sum.Sum(nil))
	hashes[id] = h
	delete(visiting, id)

	return h
}

func (t *trace) content(tk task) string {
	return fmt.Sprintf("%s %s @%s [%s,%s]", tk.kind, tk.what, tk.location,
		formatTime(tk.start), formatTime(tk.end))
}

// children describes a task's milestones and tags, sorted so that the order
// in which they were recorded does not matter.
func (t *trace) children(id uint64) string {
	parts := make([]string, 0, len(t.milestones[id])+len(t.tags[id]))

	for _, m := range t.milestones[id] {
		parts = append(parts, fmt.Sprintf("m(%s %s %s)",
			formatTime(m.time), m.kind, m.what))
	}

	for _, g := range t.tags[id] {
		parts = append(parts, fmt.Sprintf("g(%s %s)", formatTime(g.time), g.what))
	}

	if len(parts) == 0 {
		return ""
	}

	sort.Strings(parts)

	return " " + strings.Join(parts, " ")
}
