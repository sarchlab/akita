package workspace

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/sarchlab/akita/v5/experiment"
	"github.com/sarchlab/akita/v5/experiment/internal/hostsys"
)

// Sections of .exp_archive.json:
//
//	{"version": 1,
//	 "groups":   {<group>: {"archived_at", "sessions": [...]}},
//	 "sessions": {<archived name>: {"name", "group", "archived_at", "results": [...]}},
//	 "results":  {<run>: {"archived_at", "by": ["session:<name>" | "group:<g>" | "manual"]}}}
const (
	secGroups   = "groups"
	secSessions = "sessions"
	secResults  = "results"
)

// archiveDB is .exp_archive.json, kept as generic JSON so that unknown
// keys survive.
type archiveDB struct {
	path   string
	doc    map[string]any
	exists bool // the file existed when loaded
}

func loadArchiveDB(path string) (*archiveDB, error) {
	db := &archiveDB{path: path, doc: map[string]any{}}

	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}

	db.exists = err == nil

	if len(bytes.TrimSpace(data)) > 0 {
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.UseNumber()

		var v any
		if err := dec.Decode(&v); err != nil {
			return nil, fmt.Errorf("%s is not valid JSON; fix it by hand first.%s", path, nothingChanged)
		}

		m, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s is not a JSON object; fix it by hand first.%s", path, nothingChanged)
		}

		db.doc = m
	}

	if _, ok := db.doc["version"]; !ok {
		db.doc["version"] = 1
	}

	for _, k := range []string{secGroups, secSessions, secResults} {
		if _, ok := db.doc[k].(map[string]any); !ok {
			db.doc[k] = map[string]any{}
		}
	}

	return db, nil
}

// LoadArchivedResults returns the result directories that a workspace's
// .exp_archive.json marks as archived (none when it cannot be read).
func LoadArchivedResults(workspace string) map[string]bool {
	out := map[string]bool{}

	db, err := loadArchiveDB(filepath.Join(experiment.ExpandHome(workspace), ArchiveFile))
	if err != nil {
		return out
	}

	for r := range db.section(secResults) {
		out[r] = true
	}

	return out
}

func (db *archiveDB) section(k string) map[string]any {
	return db.doc[k].(map[string]any)
}

func (db *archiveDB) has(sec, k string) bool {
	_, ok := db.section(sec)[k]
	return ok
}

// entry returns section[k], creating it from def.
func (db *archiveDB) entry(sec, k string, def map[string]any) map[string]any {
	if m, ok := db.section(sec)[k].(map[string]any); ok {
		return m
	}

	db.section(sec)[k] = def

	return def
}

// sessionName is the original name of an archived session ("" if unknown).
func (db *archiveDB) sessionName(k string) string {
	m, _ := db.section(secSessions)[k].(map[string]any)
	s, _ := m["name"].(string)

	return s
}

func (db *archiveDB) addRef(r, ref, now string) {
	e := db.entry(secResults, r, map[string]any{"archived_at": now, "by": []any{}})
	by := strList(e["by"])

	if !slices.Contains(by, ref) {
		by = append(by, ref)
	}

	e["by"] = toAny(by)
}

func (db *archiveDB) dropRef(ref string) {
	res := db.section(secResults)
	for r, v := range res {
		e, ok := v.(map[string]any)
		if !ok {
			continue
		}

		by := slices.DeleteFunc(strList(e["by"]), func(s string) bool { return s == ref })
		if len(by) == 0 {
			delete(res, r)
		} else {
			e["by"] = toAny(by)
		}
	}
}

// renameResult moves the marks of result directory from to to.
func (db *archiveDB) renameResult(from, to string) bool {
	changed := false

	if e, ok := db.section(secResults)[from]; ok {
		db.section(secResults)[to] = e
		delete(db.section(secResults), from)

		changed = true
	}

	for _, v := range db.section(secSessions) {
		m, ok := v.(map[string]any)
		if !ok {
			continue
		}

		rs := strList(m["results"])
		if i := slices.Index(rs, from); i >= 0 {
			rs[i] = to
			m["results"] = toAny(rs)
			changed = true
		}
	}

	return changed
}

// save writes the file atomically: sorted keys, one-space indent, like the
// original tool.
func (db *archiveDB) save() error {
	var buf bytes.Buffer

	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", " ")

	if err := enc.Encode(db.doc); err != nil {
		return err
	}

	return hostsys.WriteFileAtomic(db.path, buf.Bytes(), 0o644)
}

func strList(v any) []string {
	l, _ := v.([]any)
	out := make([]string, 0, len(l))

	for _, x := range l {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}

	return out
}

func toAny(s []string) []any {
	out := make([]any, len(s))
	for i, x := range s {
		out[i] = x
	}

	return out
}
