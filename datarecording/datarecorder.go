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
	// Init establishes a connection to the database
	Init()

	// CreateTable creates a new table with given filename
	CreateTable(table string, sampleEntry any)

	//CreateIndex accelerates data r/w performance on certain fieldname
	CreateIndex(table string, fieldName string)

	// DataInsert writes a same-type task into table that already exists
	InsertData(table string, entry any)

	// // ListTable returns a slice containing names of all tables
	// ListTables() []string

	// Flush flushes all the baffered task into database
	Flush()
}

// SQLiteWriter is the writer that writes data into SQLite database
type SQLiteWriter struct {
	*sql.DB
	statement map[string]*sql.Stmt

	dbName     string
	tables     map[string][]any
	batchSize  int
	tableCount int
	entryCount int
}

// NewSQLiteWriter creates a new SQLiteWriter.
func NewSQLiteWriter(path string) *SQLiteWriter {
	w := &SQLiteWriter{
		dbName:    path,
		batchSize: 100000,
		tables:    make(map[string][]any),
		statement: make(map[string]*sql.Stmt),
	}

	atexit.Register(func() { w.Flush() })

	return w
}

// Init establishes a connection to the databse
func (t *SQLiteWriter) Init() {
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

func (t *SQLiteWriter) isAllowedType(kind reflect.Kind) bool {
	switch kind {
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128,
		reflect.String, reflect.UnsafePointer:
		return true
	default:
		return false
	}
}

func (t *SQLiteWriter) checkStructFields(entry any) error {
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

func (t *SQLiteWriter) CreateTable(table string, sampleEntry any) {
	err := t.checkStructFields(sampleEntry)
	if err != nil {
		panic(err)
	}

	t.tableCount++
	n := structs.Names(sampleEntry)
	for i := range n {
        n[i] = fmt.Sprintf(`"%s"`, n[i]) 
    }
	fields := strings.Join(n, ", \n\t")
	tableName := table
	createTableSQL := `CREATE TABLE ` + tableName + ` (` + "\n\t" + fields + "\n" + `);`

	t.mustExecute(createTableSQL)
	fmt.Printf("Table %s created successfully\n", tableName)

	storedTasks := []any{sampleEntry}
	t.tables[tableName] = storedTasks
	t.entryCount++
	t.Flush()
}

// CreateIndex creates an index on the specified field in the given table
func (t *SQLiteWriter) CreateIndex(table string, fieldName string) {
    indexName := fmt.Sprintf("idx_%s_%s", table, fieldName)
    createIndexSQL := fmt.Sprintf("CREATE INDEX IF NOT EXISTS %s ON %s(%s);", 
        indexName, table, fieldName)
    
    t.mustExecute(createIndexSQL)
    fmt.Printf("Index %s created successfully on table %s\n", indexName, table)
}

func (t *SQLiteWriter) InsertData(table string, entry any) {
	if _, exists := t.tables[table]; !exists {
        panic(fmt.Errorf("table %s does not exist, please create it first", table))
    }
	err := t.checkStructFields(entry)
	if err != nil {
		panic(err)
	}

	storedTasks, exists := t.tables[table]
	if !exists {
		panic(fmt.Errorf("table %s does not exist", table))
	}

	stdTask := storedTasks[0]
	if reflect.TypeOf(stdTask) != reflect.TypeOf(entry) {
		panic(fmt.Errorf("task %s can't be written into table %s", entry, table))
	}
	// fmt.Println("Data is successfully inserted")

	storedTasks = append(storedTasks, entry)
	t.tables[table] = storedTasks
	t.entryCount += 1
	if t.entryCount >= t.batchSize {
		t.Flush()
	}
}

func (t *SQLiteWriter) Flush() {
	if t.entryCount == 0 {
		return
	}

	t.mustExecute("BEGIN TRANSACTION")
	defer t.mustExecute("COMMIT TRANSACTION")

	for tableName, storedEntries := range t.tables {
		sampleEntry := storedEntries[0]
		t.prepareStatement(tableName, sampleEntry)
		for _, task := range storedEntries {
			v := structs.Values(task)
			_, err := t.statement[tableName].Exec(v...)
			if err != nil {
				panic(err)
			}
		}
	}

	t.tables[tableName] = []any{storedEntries[0]}
	t.entryCount = 0
}

func (t *SQLiteWriter) mustExecute(query string) sql.Result {
	res, err := t.Exec(query)
	if err != nil {
		fmt.Printf("Failed to execute: %s\n", query)
		panic(err)
	}
	return res
}

func (t *SQLiteWriter) prepareStatement(table string, task any) {
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

	t.statement[table] = stmt
}

// SQLiteReader is a reader that reads trace data from a SQLite database.
type SQLiteReader struct {
	*sql.DB

	filename string
}

// NewSQLiteReader creates a new SQLiteTraceReader.
func NewSQLiteReader(filename string) *SQLiteReader {
	r := &SQLiteReader{
		filename: filename + ".sqlite3",
	}

	return r
}

// Init establishes a connection to the database.
func (r *SQLiteReader) Init() {
	db, err := sql.Open("sqlite3", r.filename)
	if err != nil {
		panic(err)
	}

	r.DB = db
}

// ListTables returns a slice containing names of all tables
func (r *SQLiteReader) ListTables() []string {
	tableNames := make([]string, 0)
	query := `SELECT name FROM sqlite_master WHERE type='table';`

	rows, err := r.Query(query)
	if err != nil {
		panic(err)
	}

	close := func() {
		err := rows.Close()
		if err != nil {
			panic(err)
		}
	}
	defer close()

	for rows.Next() {
		var tableName string
		err := rows.Scan(&tableName)
		if err != nil {
			panic(err)
		}
		tableNames = append(tableNames, tableName)
	}

	return tableNames
}

