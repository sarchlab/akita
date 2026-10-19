package herd

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	// Pure-Go SQLite driver "sqlite" (the one monitoring and datarecording
	// use; importing modernc.org/sqlite as well would register the driver
	// name twice).
	_ "github.com/glebarez/go-sqlite"
)

// sqliteTimeout bounds reading one database.
const sqliteTimeout = 2 * time.Second

// ExecInfo reads the exec_info table of every *.sqlite3 file among the
// names of directory dir and returns its rows as properties. Databases are
// opened read-only and immutable (nothing is written, no locks are taken);
// missing, locked, or malformed databases are skipped.
func ExecInfo(ctx context.Context, dir string, names []string) []Property {
	var dbs []string

	for _, n := range names {
		if strings.HasSuffix(n, ".sqlite3") {
			dbs = append(dbs, n)
		}
	}

	sort.Strings(dbs)

	var props []Property
	for _, n := range dbs {
		props = MergeProperties(props, readExecInfo(ctx, filepath.Join(dir, n)), false)
	}

	return props
}

// readExecInfo returns the exec_info rows of one SQLite file, or nil.
func readExecInfo(ctx context.Context, path string) []Property {
	st, err := os.Stat(path)
	if err != nil || !st.Mode().IsRegular() || st.Size() == 0 {
		return nil
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return nil
	}

	dsn := (&url.URL{
		Scheme:   "file",
		Path:     abs,
		RawQuery: "mode=ro&immutable=1",
	}).String()

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(ctx, sqliteTimeout)
	defer cancel()

	rows, err := db.QueryContext(ctx, "SELECT Property, Value FROM exec_info")
	if err != nil {
		return nil
	}
	defer rows.Close()

	var props []Property

	for rows.Next() {
		var k, v sql.NullString
		if rows.Scan(&k, &v) != nil || !k.Valid || k.String == "" {
			continue
		}

		props = append(props, Property{Property: k.String, Value: v.String})
	}

	return props
}
