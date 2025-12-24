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

	if tags == "location" {
		return
	}

	panic("akita_data tag can only be either " +
		"ignore, unique, index, or location")
}

func (t *sqliteWriter) CreateTable(tableName string, sampleEntry any) {
	t.mu.Lock()
	defer t.mu.Unlock()

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

	table, exists := t.tables[tableName]
	if !exists {
		t.mu.Unlock()
		panic(fmt.Sprintf("table %s does not exist", tableName))
	}

	table.entries = append(table.entries, entry)

	t.entryCount += 1

	if t.entryCount >= t.batchSize {
		t.mu.Unlock()
		t.Flush()

		return
	}

	t.mu.Unlock()
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
		if len(table.entries) == 0 {
			continue
		}

		if tableName == "location" {
			continue
		}

		for _, task := range table.entries {
			t.insertEntryForTable(task, table)
		}

		table.entries = nil
	}

	t.flushLocationTable()

	t.entryCount = 0
}

func (t *sqliteWriter) insertEntryForTable(
	task any,
	table *table,
) {
	t.mu.Lock()
	defer t.mu.Unlock()

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

	_, err := table.statement.ExecContext(context.Background(), v...)
	if err != nil {
		panic(err)
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

func (t *sqliteWriter) flushLocationTable() {
	table, exists := t.tables["location"]
	if !exists {
		return
	}

	if len(table.entries) == 0 {
		return
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

		_, err := table.statement.ExecContext(context.Background(), v...)
		if err != nil {
			panic(err)
		}
	}

	table.entries = nil
}

func (t *sqliteWriter) mustExecute(query string) sql.Result {
	res, err := t.ExecContext(context.Background(), query)
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

// RecorderConfig holds configuration for creating a DataRecorder
type RecorderConfig struct {
	// Type specifies the backend: "sqlite", "clickhouse"
	Type string

	// Path is used for SQLite (database file path)
	Path string

	// ConnStr is used for ClickHouse (connection string)
	// ClickHouse format: "clickhouse://host:port/database?username=user&password=pass"
	// Example: "clickhouse://localhost:9000/akita_sim?username=default&password=secret"
	ConnStr string

	// Host, Database, Username, Password - alternative to ConnStr for easier configuration
	Host     string
	Port     int
	Database string
	Username string
	Password string

	// BatchSize controls how many entries to buffer before flushing (default: 100000)
	// For ClickHouse with 300M records, consider 500000-1000000
	BatchSize int
}

// NewDataRecorderWithConfig creates a DataRecorder based on the provided configuration.
// This is the recommended way to create recorders for production use.
func NewDataRecorderWithConfig(config RecorderConfig) DataRecorder {
	// Set default batch size if not specified
	if config.BatchSize == 0 {
		config.BatchSize = 100000
	}

	switch strings.ToLower(config.Type) {
	case "sqlite", "":
		// Default to SQLite for backward compatibility
		if config.Path == "" {
			return NewDataRecorder("")
		}
		return NewDataRecorder(config.Path)

	case "clickhouse":
		// Use FAST implementation (no reflection!)
		if config.Host == "" && config.ConnStr != "" {
			// Parse connection string
			host, port, database, username, password := parseClickHouseConnStr(config.ConnStr)
			return NewFastClickHouseRecorder(host, port, database, username, password, config.BatchSize)
		}

		// Use individual parameters
		if config.Host == "" {
			panic("ClickHouse host is required")
		}

		port := config.Port
		if port == 0 {
			port = 9000
		}

		username := config.Username
		if username == "" {
			username = "default"
		}

		database := config.Database
		if database == "" {
			database = "default"
		}

		return NewFastClickHouseRecorder(config.Host, port, database, username, config.Password, config.BatchSize)

	default:
		panic(fmt.Sprintf("unknown recorder type: %s (valid: sqlite, clickhouse)", config.Type))
	}
}

// parseClickHouseConnStr parses a ClickHouse connection string
// Format: clickhouse://host:port/database?username=user&password=pass
func parseClickHouseConnStr(connStr string) (host string, port int, database string, username string, password string) {
	// Default values
	port = 9000
	username = "default"
	database = "default"

	// Remove protocol prefix
	connStr = strings.TrimPrefix(connStr, "clickhouse://")
	connStr = strings.TrimPrefix(connStr, "tcp://")

	// Split by ? to separate host/db from parameters
	parts := strings.SplitN(connStr, "?", 2)
	hostPart := parts[0]

	// Parse host:port/database
	if strings.Contains(hostPart, "/") {
		hostPort := strings.Split(hostPart, "/")
		if len(hostPort) >= 2 {
			database = hostPort[1]
		}
		hostPart = hostPort[0]
	}

	// Parse host:port
	if strings.Contains(hostPart, ":") {
		hostPortParts := strings.Split(hostPart, ":")
		host = hostPortParts[0]
		if len(hostPortParts) >= 2 {
			fmt.Sscanf(hostPortParts[1], "%d", &port)
		}
	} else {
		host = hostPart
	}

	// Parse parameters
	if len(parts) >= 2 {
		params := strings.Split(parts[1], "&")
		for _, param := range params {
			kv := strings.SplitN(param, "=", 2)
			if len(kv) == 2 {
				key := kv[0]
				value := kv[1]
				switch key {
				case "username", "user":
					username = value
				case "password", "pass":
					password = value
				case "database", "db":
					database = value
				}
			}
		}
	}

	return
}
