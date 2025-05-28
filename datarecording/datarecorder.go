package datarecording

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"

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

	// Close closes the recorder
	Close() error
}

// NewDataRecorder creates a new DataRecorder.
func NewDataRecorder(path string) DataRecorder {
	w := &sqliteWriter{
		dbName:    path,
		batchSize: 100000,
		tables:    make(map[string]*table),
	}

	w.Init()

	createExecRecorder(w)

	atexit.Register(func() {
		w.Flush()
	})

	return w
}

// NewDataRecorderWithDB creates a new DataRecorder with a given database.
func NewDataRecorderWithDB(db *sql.DB) DataRecorder {
	w := &sqliteWriter{
		DB:        db,
		batchSize: 100000,
		tables:    make(map[string]*table),
	}

	createExecRecorder(w)

	return w
}

func createExecRecorder(w *sqliteWriter) {
	execRecorder := newExecRecorderWithWriter(w)
	execRecorder.Start()

	w.execRecorder = execRecorder
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
	batchSize    int
	entryCount   int
	execRecorder *execRecorder
}

// Init establishes a connection to the database.
func (t *sqliteWriter) Init() {
	if t.dbName == "" {
		t.dbName = "akita_data_recording_" + xid.New().String()
	}

	filename := t.dbName + ".sqlite3"
	os.Remove(filename)

	_, err := os.Stat(filename)
	if err == nil {
		panic(fmt.Errorf("file %s already exists", filename))
	}

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

func (t *sqliteWriter) checkStructFields(entry any) error {
	types := reflect.TypeOf(entry)

	for i := 0; i < types.NumField(); i++ {
		field := types.Field(i)

		t.mustHaveAtMostOneTag(field)

		if t.fieldIgnored(field) {
			continue
		}

		fieldKind := field.Type.Kind()
		if !t.isAllowedType(fieldKind) {
			return errors.New("entry is invalid")
		}
	}

	return nil
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

	panic("akita_data tag can only be either ignore, unique, or index")
}

func (t *sqliteWriter) CreateTable(tableName string, sampleEntry any) {
	err := t.checkStructFields(sampleEntry)
	if err != nil {
		panic(err)
	}

	fieldNames := t.getFieldNames(sampleEntry)
	fields := strings.Join(fieldNames, ", \n\t")

	createTableSQL := `CREATE TABLE ` + tableName +
		` (` + "\n\t" + fields + "\n" + `);`
	t.mustExecute(createTableSQL)

	t.createIndexesForTable(tableName, sampleEntry)

	tableInfo := &table{
		structType: reflect.TypeOf(sampleEntry),
		entries:    []any{},
	}
	t.tables[tableName] = tableInfo

	t.prepareStatement(tableName, sampleEntry)
}

func (t *sqliteWriter) prepareStatement(table string, task any) {
	fieldNames := t.getFieldNames(task)
	placeholders := make([]string, len(fieldNames))

	for i := range placeholders {
		placeholders[i] = "?"
	}

	entryToFill := "(" + strings.Join(placeholders, ", ") + ")"
	sqlStr := "INSERT INTO " + table + " VALUES " + entryToFill

	stmt, err := t.Prepare(sqlStr)
	if err != nil {
		panic(err)
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
	sampleEntry any,
) {
	sType := reflect.TypeOf(sampleEntry)

	for i := 0; i < sType.NumField(); i++ {
		field := sType.Field(i)

		if dbTag, ok := field.Tag.Lookup("akita_data"); ok {
			switch dbTag {
			case "unique":
				t.createIndex(tableName, field.Name, true)
			case "index":
				t.createIndex(tableName, field.Name, false)
			}
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

func (t *sqliteWriter) InsertData(tableName string, entry any) {
	t.mu.Lock()
	defer t.mu.Unlock()

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

	for _, table := range t.tables {
		if len(table.entries) == 0 {
			continue
		}

		for _, task := range table.entries {
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

				v = append(v, value.Field(i).Interface())
			}

			_, err := table.statement.Exec(v...)
			if err != nil {
				panic(err)
			}
		}

		table.entries = nil
	}

	t.entryCount = 0
}

func (t *sqliteWriter) fieldIgnored(field reflect.StructField) bool {
	tag, ok := field.Tag.Lookup("akita_data")
	return ok && strings.Contains(tag, "ignore")
}

func (t *sqliteWriter) mustExecute(query string) sql.Result {
	res, err := t.Exec(query)
	if err != nil {
		fmt.Printf("Failed to execute: %s\n", query)
		panic(err)
	}

	return res
}

func (t *sqliteWriter) Close() error {
	if t.execRecorder != nil {
		t.execRecorder.End()
	}

	t.Flush()

	err := t.DB.Close()
	if err != nil {
		return fmt.Errorf("failed to close database connection: %w", err)
	}

	return nil
}
