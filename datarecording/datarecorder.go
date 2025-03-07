package datarecording

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/fatih/structs"

	// Need to use SQLite connections.
	_ "github.com/mattn/go-sqlite3"
	"github.com/rs/xid"
	"github.com/tebeka/atexit"
)

// DataRecorder is a backend that can record and store data
type DataRecorder interface {
	// CreateTable creates a new table with given filename
	CreateTable(tableName string, sampleEntry any)

	// DataInsert writes a same-type task into table that already exists
	InsertData(tableName string, entry any)

	// ListTable returns a slice containing names of all tables
	ListTables() []string

	// Flush flushes all the buffered task into database
	Flush()
}

// New creates a new DataRecorder.
func New(path string) DataRecorder {
	w := &sqliteWriter{
		dbName:    path,
		batchSize: 100000,
		tables:    make(map[string]*table),
	}

	w.Init()

	atexit.Register(func() { w.Flush() })

	return w
}

// NewWithDB creates a new DataRecorder with a given database.
func NewWithDB(db *sql.DB) DataRecorder {
	w := &sqliteWriter{
		DB:        db,
		batchSize: 100000,
		tables:    make(map[string]*table),
	}

	atexit.Register(func() { w.Flush() })

	return w
}

type table struct {
	structType reflect.Type
	entries    []any
}

// sqliteWriter is the writer that writes data into SQLite database
type sqliteWriter struct {
	*sql.DB
	statement *sql.Stmt

	dbName     string
	tables     map[string]*table
	batchSize  int
	tableCount int
	entryCount int
}

// Init establishes a connection to the database.
func (t *sqliteWriter) Init() {
	if t.dbName == "" {
		t.dbName = "akita_data_recording_" + xid.New().String()
	}

	filename := t.dbName + ".sqlite3"

	_, err := os.Stat(filename)
	if err == nil {
		panic(fmt.Errorf("file %s already exists", filename))
	}

	fmt.Fprintf(os.Stderr, "Database created for recording: %s\n", filename)

	db, err := sql.Open("sqlite3", filename)
	if err != nil {
		panic(err)
	}

	t.DB = db
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
		reflect.Uintptr,
		reflect.Float32,
		reflect.Float64,
		reflect.Complex64,
		reflect.Complex128,
		reflect.String,
		reflect.UnsafePointer:
		return true
	default:
		return false
	}
}

func (t *sqliteWriter) checkStructFields(entry any) error {
	types := reflect.TypeOf(entry)

	for i := 0; i < types.NumField(); i++ {
		field := types.Field(i)

		fieldKind := field.Type.Kind()
		if !t.isAllowedType(fieldKind) {
			return errors.New("entry is invalid")
		}
	}

	return nil
}

func (t *sqliteWriter) CreateTable(tableName string, sampleEntry any) {
	err := t.checkStructFields(sampleEntry)
	if err != nil {
		panic(err)
	}

	t.tableCount++
	n := structs.Names(sampleEntry)
	fields := strings.Join(n, ", \n\t")

	createTableSQL := `CREATE TABLE ` + tableName +
		` (` + "\n\t" + fields + "\n" + `);`
	t.mustExecute(createTableSQL)

	tableInfo := &table{
		structType: reflect.TypeOf(sampleEntry),
		entries:    []any{},
	}
	t.tables[tableName] = tableInfo
}

func (t *sqliteWriter) InsertData(tableName string, entry any) {
	table, exists := t.tables[tableName]
	if !exists {
		panic(fmt.Sprintf("table %s does not exist", tableName))
	}

	table.entries = append(table.entries, entry)

	t.entryCount += 1
	if t.entryCount >= t.batchSize {
		t.Flush()
	}
}

func (t *sqliteWriter) ListTables() []string {
	tables := make([]string, 0, len(t.tables))
	for table := range t.tables {
		tables = append(tables, table)
	}

	return tables
}

func (t *sqliteWriter) Flush() {
	if t.entryCount == 0 {
		return
	}

	t.mustExecute("BEGIN TRANSACTION")
	defer t.mustExecute("COMMIT TRANSACTION")

	for tableName, table := range t.tables {
		sampleEntry := table.entries[0]
		t.prepareStatement(tableName, sampleEntry)

		for _, task := range table.entries {
			v := []any{}

			types := reflect.ValueOf(task)
			for i := 0; i < types.NumField(); i++ {
				v = append(v, types.Field(i).Interface())
			}

			_, err := t.statement.Exec(v...)
			if err != nil {
				panic(err)
			}
		}

		table.entries = nil

		t.statement.Close()
		t.statement = nil
	}

	t.entryCount = 0
}

func (t *sqliteWriter) mustExecute(query string) sql.Result {
	res, err := t.Exec(query)
	if err != nil {
		fmt.Printf("Failed to execute: %s\n", query)
		panic(err)
	}

	return res
}

func (t *sqliteWriter) prepareStatement(table string, task any) {
	n := structs.Names(task)
	for i := 0; i < len(n); i++ {
		n[i] = "?"
	}

	entryToFill := "(" + strings.Join(n, ", ") + ")"
	sqlStr := "INSERT INTO " + table + " VALUES " + entryToFill

	stmt, err := t.Prepare(sqlStr)
	if err != nil {
		panic(err)
	}

	t.statement = stmt
}
