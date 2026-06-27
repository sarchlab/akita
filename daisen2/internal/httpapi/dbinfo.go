package httpapi

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"
)

// ---- DB activity tracking ----------------------------------------------------

// DBActivity describes one in-flight database operation — an index build or a
// heavy query — so the frontend can show what the database is actually doing
// instead of a bare spinner.
type DBActivity struct {
	ID      uint64  `json:"id"`
	Op      string  `json:"op"`              // "index" or "query"
	Name    string  `json:"name"`            // human-readable label
	Detail  string  `json:"detail"`          // columns or query summary
	Elapsed float64 `json:"elapsed_seconds"` // seconds since the op started

	startTime time.Time
}

// DBActivityTracker is a concurrency-safe registry of in-flight DB operations.
type DBActivityTracker struct {
	mu     sync.Mutex
	nextID uint64
	active map[uint64]*DBActivity
}

// NewDBActivityTracker creates an empty tracker.
func NewDBActivityTracker() *DBActivityTracker {
	return &DBActivityTracker{active: map[uint64]*DBActivity{}}
}

// Begin records the start of a DB operation and returns its id; pass that id to
// End when the operation finishes. A nil tracker is a no-op returning 0, so call
// sites need no nil checks.
func (t *DBActivityTracker) Begin(op, name, detail string) uint64 {
	if t == nil {
		return 0
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	t.nextID++
	id := t.nextID
	t.active[id] = &DBActivity{
		ID:        id,
		Op:        op,
		Name:      name,
		Detail:    detail,
		startTime: time.Now(),
	}

	return id
}

// End removes a previously-begun operation from the active set.
func (t *DBActivityTracker) End(id uint64) {
	if t == nil || id == 0 {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	delete(t.active, id)
}

// Snapshot returns the currently-active operations, oldest first, with their
// elapsed times filled in.
func (t *DBActivityTracker) Snapshot() []DBActivity {
	out := []DBActivity{}
	if t == nil {
		return out
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	for _, a := range t.active {
		c := *a
		c.Elapsed = now.Sub(a.startTime).Seconds()
		out = append(out, c)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })

	return out
}

// ---- DB schema-and-size overview --------------------------------------------

// DBIndexInfo is the on-disk size of one index.
type DBIndexInfo struct {
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
}

// DBTableInfo summarizes one table: its row count, the bytes its own b-tree
// occupies, and the bytes its secondary indexes occupy.
type DBTableInfo struct {
	Name       string        `json:"name"`
	Rows       int64         `json:"rows"`
	DataBytes  int64         `json:"data_bytes"`
	IndexBytes int64         `json:"index_bytes"`
	Indexes    []DBIndexInfo `json:"indexes"`
}

// DBInfo is the schema-and-size overview of the loaded trace database. Sizes are
// best-effort: when the SQLite build lacks the dbstat virtual table, HasSizes is
// false and the byte fields are zero while row counts and the file size remain
// valid.
type DBInfo struct {
	File       string        `json:"file"`
	FileBytes  int64         `json:"file_bytes"`
	TotalRows  int64         `json:"total_rows"`
	DataBytes  int64         `json:"data_bytes"`
	IndexBytes int64         `json:"index_bytes"`
	HasSizes   bool          `json:"has_sizes"`
	Tables     []DBTableInfo `json:"tables"`
}

// dbInfoCache memoizes the (expensive) dbstat scan so /api/db_info stays cheap to
// poll. The first request kicks off a background computation and returns
// {computing:true}; subsequent requests return the cached overview.
type dbInfoCache struct {
	mu        sync.Mutex
	info      *DBInfo
	computing bool
	// dirty is set when invalidate() lands while a scan is in flight: that scan's
	// result predates the change, so it is discarded rather than cached.
	dirty bool
}

// invalidate drops the cached overview so the next /api/db_info recomputes it. A
// scan already in flight is marked dirty so its (now-stale) result is discarded
// when it finishes instead of being cached.
func (c *dbInfoCache) invalidate() {
	if c == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.info = nil
	if c.computing {
		c.dirty = true
	}
}

// dbFileBytes is the database's live on-disk footprint: the main SQLite file
// plus its WAL and shared-memory sidecars. In WAL mode a fresh, multi-GB index
// can sit in <db>-wal until a checkpoint folds it into the main file, so stating
// only the main file would report a size far smaller than the table/index bytes.
func dbFileBytes(filename string) int64 {
	var total int64
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if st, err := os.Stat(filename + suffix); err == nil {
			total += st.Size()
		}
	}

	return total
}

// CollectDBInfo computes the schema-and-size overview directly (row counts via
// COUNT(*), per-object sizes via the dbstat virtual table). It is best-effort:
// any query that fails is skipped rather than failing the whole overview.
func (r *SQLiteTraceReader) CollectDBInfo(ctx context.Context) DBInfo {
	info := DBInfo{Tables: []DBTableInfo{}}

	if r.DB == nil {
		return info
	}

	info.File = r.filename
	info.FileBytes = dbFileBytes(r.filename)

	// table name -> its index names, from the schema.
	tableIndexes := r.tableIndexNames(ctx)

	// object name -> on-disk bytes, from dbstat (best-effort).
	sizes, hasSizes := r.objectSizes(ctx)
	info.HasSizes = hasSizes

	tableNames := make([]string, 0, len(tableIndexes))
	for name := range tableIndexes {
		tableNames = append(tableNames, name)
	}
	sort.Strings(tableNames)

	for _, name := range tableNames {
		t := DBTableInfo{Name: name, Indexes: []DBIndexInfo{}}

		var rows int64
		// #nosec G202 -- name comes from sqlite_master, not user input.
		if err := r.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM "`+name+`"`).Scan(&rows); err == nil {
			t.Rows = rows
		}
		info.TotalRows += t.Rows

		t.DataBytes = sizes[name]
		for _, idxName := range tableIndexes[name] {
			b := sizes[idxName]
			t.IndexBytes += b
			t.Indexes = append(t.Indexes, DBIndexInfo{Name: idxName, Bytes: b})
		}
		sort.Slice(t.Indexes, func(i, j int) bool {
			return t.Indexes[i].Bytes > t.Indexes[j].Bytes
		})

		info.DataBytes += t.DataBytes
		info.IndexBytes += t.IndexBytes
		info.Tables = append(info.Tables, t)
	}

	// Largest tables (by total footprint) first.
	sort.Slice(info.Tables, func(i, j int) bool {
		return info.Tables[i].DataBytes+info.Tables[i].IndexBytes >
			info.Tables[j].DataBytes+info.Tables[j].IndexBytes
	})

	return info
}

// tableIndexNames returns, for each table, the names of its indexes (including
// the implicit autoindex entries dbstat reports separately).
func (r *SQLiteTraceReader) tableIndexNames(ctx context.Context) map[string][]string {
	out := map[string][]string{}

	rows, err := r.QueryContext(ctx,
		`SELECT name, type, tbl_name FROM sqlite_master
		 WHERE type IN ('table','index') AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		return out
	}
	defer rows.Close()

	for rows.Next() {
		var name, typ, tbl string
		if err := rows.Scan(&name, &typ, &tbl); err != nil {
			continue
		}
		if typ == "table" {
			if _, ok := out[name]; !ok {
				out[name] = []string{}
			}
			continue
		}
		out[tbl] = append(out[tbl], name)
	}

	return out
}

// objectSizes maps every table/index name to its on-disk byte size via the
// dbstat virtual table. The second return is false when dbstat is unavailable
// (SQLite built without SQLITE_ENABLE_DBSTAT_VTAB), letting callers degrade to
// row-counts-only rather than reporting zero sizes as if they were real.
func (r *SQLiteTraceReader) objectSizes(ctx context.Context) (map[string]int64, bool) {
	sizes := map[string]int64{}

	rows, err := r.QueryContext(ctx,
		`SELECT name, SUM(pgsize) FROM dbstat GROUP BY name`)
	if err != nil {
		return sizes, false
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		var bytes sql.NullInt64
		if err := rows.Scan(&name, &bytes); err != nil {
			continue
		}
		sizes[name] = bytes.Int64
	}

	return sizes, true
}

// get returns the cached overview if present. When absent, it starts a
// background computation (tracked as a DB activity) and reports computing=true so
// the caller can return immediately. refresh forces a recompute.
func (r *SQLiteTraceReader) dbInfoGet(ctx context.Context, refresh bool) (*DBInfo, bool) {
	c := r.dbInfo

	c.mu.Lock()
	defer c.mu.Unlock()

	if !refresh && c.info != nil {
		return c.info, false
	}
	if c.computing {
		return c.info, true
	}

	c.computing = true
	go func() {
		id := r.activity.Begin("info",
			"Measuring table & index sizes", "scanning dbstat")
		info := r.CollectDBInfo(context.WithoutCancel(ctx))
		r.activity.End(id)

		c.mu.Lock()
		if c.dirty {
			// An index build landed mid-scan; this result may already be stale, so
			// drop it and let the next poll recompute rather than caching it.
			c.info = nil
			c.dirty = false
		} else {
			c.info = &info
		}
		c.computing = false
		c.mu.Unlock()
	}()

	return c.info, true
}

// ---- HTTP handlers -----------------------------------------------------------

// httpDBInfo serves the schema-and-size overview of the loaded trace database.
// The expensive dbstat scan runs once in the background; while it runs the
// response carries computing:true so the frontend can show a "measuring…" state
// and poll. Pass ?refresh=1 to recompute.
func (s *Server) httpDBInfo(w http.ResponseWriter, r *http.Request) {
	if s.traceReader == nil {
		http.Error(w, "trace data not available", http.StatusServiceUnavailable)
		return
	}

	info, computing := s.traceReader.dbInfoGet(r.Context(), r.URL.Query().Get("refresh") == "1")

	writeJSON(w, struct {
		Computing bool    `json:"computing"`
		Info      *DBInfo `json:"info"`
	}{Computing: computing, Info: info})
}

// httpDBActivity serves the set of in-flight database operations (index builds,
// heavy queries, the dbstat scan) so the frontend can show what the database is
// doing in real time.
func (s *Server) httpDBActivity(w http.ResponseWriter, _ *http.Request) {
	if s.traceReader == nil {
		writeJSON(w, []DBActivity{})
		return
	}

	writeJSON(w, s.traceReader.activity.Snapshot())
}
