package sourcefs

import (
	"database/sql"
	"encoding/base64"
	"fmt"
	"io/fs"
	"sort"
	"testing/fstest"
)

// traceSourceTable is the trace table that holds recorded source archives,
// written by the simulation source recorder.
const traceSourceTable = "source"

// Source is the simulator source recorded in a trace, exposed as a read-only
// file tree. Paths are "<root>/<file>" so multiple recorded roots (e.g. Akita
// plus a simulator's own module) coexist without collision. A nil or zero
// Source means no source was recorded.
type Source struct {
	fsys  fs.FS
	Roots []string // recorded module roots, sorted (e.g. "github.com/sarchlab/akita/v5")
	Files int      // total file count across all roots
}

// FS returns the recorded source as a read-only fs.FS, or nil when empty. The
// code-reading tools walk and read it; a future on-disk override can supply a
// different fs.FS behind the same interface.
func (s *Source) FS() fs.FS {
	if s == nil {
		return nil
	}
	return s.fsys
}

// IsEmpty reports whether no source is available to read.
func (s *Source) IsEmpty() bool {
	return s == nil || s.Files == 0
}

// OpenTraceSource reads the recorded `source` table from a trace database and
// returns it as a read-only file tree. A missing or empty table is not an
// error — it returns an empty Source — so traces recorded before source
// recording existed (or with it disabled) are handled gracefully.
func OpenTraceSource(db *sql.DB) (*Source, error) {
	if db == nil {
		return &Source{}, nil
	}

	present, err := hasTable(db, traceSourceTable)
	if err != nil {
		return nil, err
	}
	if !present {
		return &Source{}, nil
	}

	rows, err := db.Query("SELECT Root, Content FROM " + traceSourceTable)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	mapFS := fstest.MapFS{}
	rootSet := map[string]struct{}{}
	for rows.Next() {
		var root, content string
		if err := rows.Scan(&root, &content); err != nil {
			return nil, err
		}

		gztar, err := base64.StdEncoding.DecodeString(content)
		if err != nil {
			return nil, fmt.Errorf("decoding source for %q: %w", root, err)
		}
		files, err := ReadArchive(gztar)
		if err != nil {
			return nil, fmt.Errorf("reading source archive for %q: %w", root, err)
		}

		for p, data := range files {
			mapFS[root+"/"+p] = &fstest.MapFile{Data: data}
		}
		rootSet[root] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	roots := make([]string, 0, len(rootSet))
	for r := range rootSet {
		roots = append(roots, r)
	}
	sort.Strings(roots)

	return &Source{fsys: mapFS, Roots: roots, Files: len(mapFS)}, nil
}

func hasTable(db *sql.DB, name string) (bool, error) {
	var found string
	err := db.QueryRow(
		"SELECT name FROM sqlite_master WHERE type='table' AND name=? LIMIT 1",
		name,
	).Scan(&found)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
