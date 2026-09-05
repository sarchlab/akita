package datarecording

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"

	// Need to use SQLite connections.
	_ "github.com/glebarez/go-sqlite"
	"github.com/rs/xid"
	"github.com/sarchlab/akita/v5/internal/sqlitefile"
)

// DataRecorder is a backend that can record and store data.
// The SQLite implementation supports concurrent CreateTable, InsertData,
// ListTables, and Flush calls. Stop producers before calling Close.
type DataRecorder interface {
	// CreateTable creates a new table with given filename
	CreateTable(tableName string, sampleEntry any) error

	// DataInsert writes a same-type task into table that already exists
	InsertData(tableName string, entry any) error

	// ListTable returns a slice containing names of all tables
	ListTables() []string

	// Flush flushes all the buffered task into database
	Flush() error

	// Close closes the recorder
	Close() error
}

// NewDataRecorder creates a new DataRecorder.
func NewDataRecorder(path string) (DataRecorder, error) {
	w := &sqliteWriter{
		dbName:    path,
		batchSize: 100000,
		tables:    make(map[string]*table),
	}

	if err := w.Init(); err != nil {
		return nil, err
	}
	return w, nil
}

// NewDataRecorderWithDB creates a recorder and takes responsibility for closing
// the database. The caller must not share it with another simulation.
func NewDataRecorderWithDB(db *sql.DB) DataRecorder {
	w := &sqliteWriter{
		DB:        db,
		batchSize: 100000,
		tables:    make(map[string]*table),
	}

	return w
}

// Feed to location table when inserting data
type location struct {
	ID     int
	Locale string
}

type table struct {
	structType reflect.Type
	entries    []any
	statement  *sql.Stmt
}

// sqliteWriter is the writer that writes data into SQLite database
type sqliteWriter struct {
	*sql.DB

	mu           sync.Mutex
	dbName       string
	tables       map[string]*table
	locationInfo map[string]int
	batchSize    int
	entryCount   int
	closed       bool
	closeErr     error
}

// Init establishes a connection to the database.
func (t *sqliteWriter) Init() (err error) {
	defer recoverIO(&err)
	if t.dbName == "" {
		t.dbName = "akita_data_recording_" + xid.New().String()
	}

	filename := t.dbName + ".sqlite3"
	// Exclusive creation prevents a second simulation from deleting a live
	// recorder's file. Output paths are owned by exactly one simulation.
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	// No recorder has been published yet. Release our unused path if opening
	// the database fails so a later attempt can claim it again.
	defer func() {
		if err != nil {
			err = errors.Join(err, os.Remove(filename))
		}
	}()
	if err = file.Close(); err != nil {
		return err
	}

	dsn, err := sqlitefile.DSN(filename)
	if err != nil {
		return err
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return err
	}

	if err = db.Ping(); err != nil {
		err = errors.Join(err, db.Close())
		return err
	}
	t.DB = db
	return nil
}

func (t *sqliteWriter) isAllowedType(kind reflect.Kind) bool {
	switch kind {
	case
		reflect.Bool,
		reflect.Int,
		reflect.Int8,
		reflect.Int16,
		reflect.Int32,
		reflect.Int64,
		reflect.Uint,
		reflect.Uint8,
		reflect.Uint16,
		reflect.Uint32,
		reflect.Uint64,
		reflect.Float32,
		reflect.Float64,
		reflect.Complex64,
		reflect.Complex128,
		reflect.String:
		return true
	default:
		return false
	}
}

func (t *sqliteWriter) checkStructFields(entry any) {
	types := reflect.TypeOf(entry)

	for i := 0; i < types.NumField(); i++ {
		field := types.Field(i)

		t.mustHaveAtMostOneTag(field)

		if t.fieldIgnored(field) {
			continue
		}

		fieldKind := field.Type.Kind()
		if !t.isAllowedType(fieldKind) {
			panic("entry is invalid")
		}
	}
}

func (t *sqliteWriter) mustHaveAtMostOneTag(field reflect.StructField) {
	tags, ok := field.Tag.Lookup("akita_data")
	if !ok {
		return // No tag is fine
	}

	if tags == "ignore" {
		return
	}

	if tags == "unique" {
		return
	}

	if tags == "index" {
		return
	}

	if tags == "location" {
		return
	}

	panic("akita_data tag can only be either " +
		"ignore, unique, index, or location")
}

func (t *sqliteWriter) CreateTable(tableName string, sampleEntry any) (err error) {
	defer recoverIO(&err)
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return sql.ErrConnDone
	}

	t.checkStructFields(sampleEntry)

	fieldNames := t.getFieldNames(sampleEntry)
	fields := strings.Join(fieldNames, ", \n\t")

	createTableSQL := `CREATE TABLE ` + tableName +
		` (` + "\n\t" + fields + "\n" + `);`
	t.mustExecute(createTableSQL)

	hasLocTag := t.checkLocationTag(sampleEntry)
	_, exists := t.tables["location"]

	if !exists && hasLocTag {
		t.createLocationTable()
	}

	tableInfo := &table{
		structType: reflect.TypeOf(sampleEntry),
		entries:    []any{},
	}
	t.tables[tableName] = tableInfo

	t.prepareStatement(tableName, sampleEntry)
	return nil
}

func (t *sqliteWriter) checkLocationTag(entry any) bool {
	if t.locationInfo == nil {
		t.locationInfo = make(map[string]int)
	}

	hasLocation := false

	sType := reflect.TypeOf(entry)

	for i := 0; i < sType.NumField(); i++ {
		field := sType.Field(i)

		dbTag, ok := field.Tag.Lookup("akita_data")
		if ok && dbTag == "location" {
			kind := field.Type.Kind()
			if kind != reflect.String {
				panic("location field type mismatch")
			}

			hasLocation = true
		}
	}

	return hasLocation
}

func (t *sqliteWriter) createLocationTable() {
	sampleLoc := location{1, "A"}

	fieldNames := t.getFieldNames(sampleLoc)
	fields := strings.Join(fieldNames, ", \n\t")

	createTableSQL := `CREATE TABLE ` + "location" +
		` (` + "\n\t" + fields + "\n" + `);`
	t.mustExecute(createTableSQL)

	tableInfo := &table{
		structType: reflect.TypeOf(sampleLoc),
		entries:    []any{},
	}
	t.tables["location"] = tableInfo

	t.prepareStatement("location", sampleLoc)
}

func (t *sqliteWriter) prepareStatement(table string, task any) {
	fieldNames := t.getFieldNames(task)
	placeholders := make([]string, len(fieldNames))

	for i := range placeholders {
		placeholders[i] = "?"
	}

	entryToFill := "(" + strings.Join(placeholders, ", ") + ")"
	sqlStr := "INSERT INTO " + table + " VALUES " + entryToFill

	stmt, err := t.PrepareContext(context.Background(), sqlStr)
	if err != nil {
		panic(recorderIOError{err})
	}

	t.tables[table].statement = stmt
}

func (t *sqliteWriter) getFieldNames(entry any) []string {
	sType := reflect.TypeOf(entry)

	var fieldNames []string

	for i := 0; i < sType.NumField(); i++ {
		field := sType.Field(i)

		if t.fieldIgnored(field) {
			continue
		}

		fieldNames = append(fieldNames, field.Name)
	}

	return fieldNames
}

func (t *sqliteWriter) createIndexesForTable(
	tableName string,
	sType reflect.Type,
) {
	for i := 0; i < sType.NumField(); i++ {
		field := sType.Field(i)

		if dbTag, ok := field.Tag.Lookup("akita_data"); ok {
			switch dbTag {
			case "unique":
				t.createIndex(tableName, field.Name, true)
			case "index":
				t.createIndex(tableName, field.Name, false)
			}
			// A "location" tag only interns the column into the shared location
			// table; it is intentionally not indexed here. The interned-id column
			// is filtered only alongside columns the reader covers with its own
			// (Location-led) indexes, so a standalone index on it is dead weight —
			// the reader builds whatever it actually needs.
		}
	}
}

func (t *sqliteWriter) createIndex(tableName, fieldName string, unique bool) {
	indexType := "INDEX"
	if unique {
		indexType = "UNIQUE INDEX"
	}

	indexSQL := fmt.Sprintf(
		"CREATE %s idx_%s_%s ON %s(%s);",
		indexType, tableName, fieldName, tableName, fieldName,
	)
	t.mustExecute(indexSQL)
}

func (t *sqliteWriter) InsertData(tableName string, entry any) (err error) {
	defer recoverIO(&err)
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return sql.ErrConnDone
	}

	table, exists := t.tables[tableName]
	if !exists {
		panic(fmt.Sprintf("table %s does not exist", tableName))
	}

	table.entries = append(table.entries, entry)

	t.entryCount += 1

	if t.entryCount >= t.batchSize {
		t.flushLocked()
	}
	return nil
}

func (t *sqliteWriter) ListTables() []string {
	t.mu.Lock()
	defer t.mu.Unlock()

	tables := make([]string, 0, len(t.tables))
	for table := range t.tables {
		tables = append(tables, table)
	}

	return tables
}

func (t *sqliteWriter) Flush() (err error) {
	defer recoverIO(&err)
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return sql.ErrConnDone
	}

	t.flushLocked()
	return nil
}

// flushLocked owns the buffers and location dictionary until the whole batch
// commits. The caller must hold mu, including for automatic flushes.
func (t *sqliteWriter) flushLocked() {
	if t.entryCount == 0 {
		return
	}

	tx, err := t.DB.BeginTx(context.Background(), nil)
	if err != nil {
		panic(recorderIOError{err})
	}
	defer tx.Rollback()

	for tableName, table := range t.tables {
		if len(table.entries) == 0 {
			continue
		}

		if tableName == "location" {
			continue
		}

		t.insertTableEntries(tx, table)
	}

	if locations, ok := t.tables["location"]; ok {
		t.insertTableEntries(tx, locations)
	}

	if err := tx.Commit(); err != nil {
		panic(recorderIOError{err})
	}

	for _, table := range t.tables {
		table.entries = nil
	}

	t.entryCount = 0
}

// insertTableEntries writes one table through the batch's connection. Location
// rows use the same insertion path after all other tables have interned theirs.
func (t *sqliteWriter) insertTableEntries(tx *sql.Tx, table *table) {
	if len(table.entries) == 0 {
		return
	}

	stmt := tx.StmtContext(context.Background(), table.statement)
	defer stmt.Close()

	for _, task := range table.entries {
		t.insertEntryForTable(task, table, stmt)
	}
}

func (t *sqliteWriter) insertEntryForTable(
	task any,
	table *table,
	stmt *sql.Stmt,
) {
	v := []any{}

	value := reflect.ValueOf(task)
	vType := value.Type()

	if vType != table.structType {
		panic("entry type mismatch")
	}

	for i := 0; i < value.NumField(); i++ {
		field := vType.Field(i)

		if t.fieldIgnored(field) {
			continue
		}

		if t.fieldLocation(field) {
			id := t.getLocationID(value, i)
			v = append(v, id)
		} else {
			v = append(v, value.Field(i).Interface())
		}
	}

	_, err := stmt.ExecContext(context.Background(), v...)
	if err != nil {
		panic(recorderIOError{err})
	}
}

func (t *sqliteWriter) getLocationID(
	value reflect.Value,
	i int,
) int {
	loc := value.Field(i).String()
	id, exists := t.locationInfo[loc]

	if !exists {
		id = len(t.locationInfo) + 1
		t.locationInfo[loc] = id

		newLocation := location{id, loc}
		locTable := t.tables["location"]
		locTable.entries = append(locTable.entries, newLocation)
		t.entryCount += 1
	}

	return id
}

func (t *sqliteWriter) fieldIgnored(field reflect.StructField) bool {
	tag, ok := field.Tag.Lookup("akita_data")
	return ok && strings.Contains(tag, "ignore")
}

func (t *sqliteWriter) fieldLocation(field reflect.StructField) bool {
	tag, ok := field.Tag.Lookup("akita_data")

	return ok && strings.Contains(tag, "location")
}

func (t *sqliteWriter) mustExecute(query string) sql.Result {
	res, err := t.ExecContext(context.Background(), query)
	if err != nil {
		fmt.Printf("Failed to execute: %s\n", query)
		panic(recorderIOError{err})
	}

	return res
}

// buildIndexes creates every recorded table's indices in one bulk pass after
// all rows have been written. Creating indices up front and maintaining them
// on every insert turns each of the millions of rows a trace records into a
// B-tree update (with random-ish key order causing page splits); a single
// CREATE INDEX over the finished table is a one-shot sorted build instead. The
// recorder never queries the data while writing, so nothing needs the indices
// until now.
func (t *sqliteWriter) buildIndexes() {
	for tableName, tbl := range t.tables {
		if tableName == "location" {
			// The interned-id dictionary key: unique so a reader can resolve an
			// id back to its string with an indexed lookup.
			t.createIndex("location", "ID", true)
			continue
		}

		t.createIndexesForTable(tableName, tbl.structType)
	}
}

// Close releases the owned database even when the final batch or index build
// fails. A failed Flush retains its batch for retry; Close is final and is not
// a retry operation. Do not resubmit entries after an automatic flush failure.
func (t *sqliteWriter) Close() (err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return t.closeErr
	}
	t.closed = true
	defer func() { err = errors.Join(err, t.DB.Close()); t.closeErr = err }()
	defer recoverIO(&err)
	t.flushLocked()
	t.buildIndexes()
	return nil
}
