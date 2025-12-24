package datarecording

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/tebeka/atexit"
)

// FastClickHouseRecorder is a high-performance ClickHouse data recorder
// that avoids reflection and uses type-specific batch handlers
type FastClickHouseRecorder struct {
	conn      clickhouse.Conn
	mu        sync.Mutex
	batchSize int

	// Table-specific batches (zero-allocation, type-safe)
	execInfoBatch             []execInfoEntry
	taskTableBatch            []taskTableEntryDB
	milestoneBatch            []milestoneTableEntryDB
	segmentBatch              []segmentTableEntryDB
	memoryTransactionBatch    []memoryTransactionEntryDB
	memoryStepBatch           []memoryStepEntryDB
	locationBatch             []locationEntry

	// Track which tables exist
	tables map[string]tableType

	// Location tracking (for location tag support)
	locationInfo map[string]int

	// Entry counter
	entryCount int

	// For execRecorder
	execRecorder *execRecorder
}

type tableType int

const (
	tableTypeExecInfo tableType = iota
	tableTypeTask
	tableTypeMilestone
	tableTypeSegment
	tableTypeMemoryTransaction
	tableTypeMemoryStep
	tableTypeLocation
)

// Internal struct types that match the external ones
type execInfoEntry struct {
	Property string
	Value    string
}

type taskTableEntryDB struct {
	ID        string
	ParentID  string
	Kind      string
	What      string
	Location  string
	StartTime float64
	EndTime   float64
}

type milestoneTableEntryDB struct {
	ID       string
	TaskID   string
	Time     float64
	Kind     string
	What     string
	Location string
}

type segmentTableEntryDB struct {
	StartTime float64
	EndTime   float64
}

type memoryTransactionEntryDB struct {
	ID        string
	Location  string
	What      string
	StartTime float64
	EndTime   float64
	Address   uint64
	ByteSize  uint64
}

type memoryStepEntryDB struct {
	ID     string
	TaskID string
	Time   float64
	What   string
}

type locationEntry struct {
	ID     int
	Locale string
}

// NewFastClickHouseRecorder creates a new high-performance ClickHouse recorder
func NewFastClickHouseRecorder(host string, port int, database string, username string, password string, batchSize int) DataRecorder {
	if batchSize == 0 {
		batchSize = 100000
	}

	// Create ClickHouse connection using native protocol
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{fmt.Sprintf("%s:%d", host, port)},
		Auth: clickhouse.Auth{
			Database: database,
			Username: username,
			Password: password,
		},
		Settings: clickhouse.Settings{
			"max_execution_time": 60,
		},
		DialTimeout:      time.Second * 30,
		MaxOpenConns:     5,
		MaxIdleConns:     5,
		ConnMaxLifetime:  time.Hour,
		ConnOpenStrategy: clickhouse.ConnOpenInOrder,
		BlockBufferSize:  10,
	})
	if err != nil {
		panic(fmt.Errorf("failed to connect to ClickHouse: %w", err))
	}

	// Verify connection
	if err := conn.Ping(context.Background()); err != nil {
		panic(fmt.Errorf("failed to ping ClickHouse: %w", err))
	}

	recorder := &FastClickHouseRecorder{
		conn:         conn,
		batchSize:    batchSize,
		tables:       make(map[string]tableType),
		locationInfo: make(map[string]int),
	}

	// Register atexit handler
	atexit.Register(func() {
		recorder.Flush()
	})

	// Create exec recorder
	execRecorder := newExecRecorderWithWriter(recorder)
	execRecorder.Start()
	recorder.execRecorder = execRecorder

	return recorder
}

// CreateTable creates a table with ClickHouse-optimized schema
func (r *FastClickHouseRecorder) CreateTable(tableName string, sampleEntry any) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Determine table type and create appropriate schema
	var createSQL string
	var tType tableType

	// Type switch to avoid reflection
	switch sampleEntry.(type) {
	case execInfoEntry, execInfo:
		tType = tableTypeExecInfo
		createSQL = fmt.Sprintf(`
			CREATE TABLE IF NOT EXISTS %s (
				Property String,
				Value String
			) ENGINE = MergeTree()
			ORDER BY Property
		`, tableName)

	default:
		// Check by field matching for external types
		createSQL, tType = r.detectTableTypeAndCreateSQL(tableName, sampleEntry)
	}

	// Execute table creation
	err := r.conn.Exec(context.Background(), createSQL)
	if err != nil {
		panic(fmt.Errorf("failed to create table %s: %w", tableName, err))
	}

	r.tables[tableName] = tType
}

func (r *FastClickHouseRecorder) detectTableTypeAndCreateSQL(tableName string, sample any) (string, tableType) {
	// Use type assertion to detect table type without reflection
	sampleStr := fmt.Sprintf("%T", sample)

	if strings.Contains(sampleStr, "taskTableEntry") {
		return fmt.Sprintf(`
			CREATE TABLE IF NOT EXISTS %s (
				ID String,
				ParentID String,
				Kind String,
				What String,
				Location String,
				StartTime Float64,
				EndTime Float64
			) ENGINE = MergeTree()
			ORDER BY (ID, StartTime)
		`, tableName), tableTypeTask
	}

	if strings.Contains(sampleStr, "milestoneTableEntry") {
		return fmt.Sprintf(`
			CREATE TABLE IF NOT EXISTS %s (
				ID String,
				TaskID String,
				Time Float64,
				Kind String,
				What String,
				Location String
			) ENGINE = MergeTree()
			ORDER BY (ID, Time)
		`, tableName), tableTypeMilestone
	}

	if strings.Contains(sampleStr, "segmentTableEntry") {
		return fmt.Sprintf(`
			CREATE TABLE IF NOT EXISTS %s (
				StartTime Float64,
				EndTime Float64
			) ENGINE = MergeTree()
			ORDER BY StartTime
		`, tableName), tableTypeSegment
	}

	if strings.Contains(sampleStr, "memoryTransactionEntry") {
		return fmt.Sprintf(`
			CREATE TABLE IF NOT EXISTS %s (
				ID String,
				Location String,
				What String,
				StartTime Float64,
				EndTime Float64,
				Address UInt64,
				ByteSize UInt64
			) ENGINE = MergeTree()
			ORDER BY (ID, StartTime)
		`, tableName), tableTypeMemoryTransaction
	}

	if strings.Contains(sampleStr, "memoryStepEntry") {
		return fmt.Sprintf(`
			CREATE TABLE IF NOT EXISTS %s (
				ID String,
				TaskID String,
				Time Float64,
				What String
			) ENGINE = MergeTree()
			ORDER BY (ID, Time)
		`, tableName), tableTypeMemoryStep
	}

	if strings.Contains(sampleStr, "location") {
		return fmt.Sprintf(`
			CREATE TABLE IF NOT EXISTS %s (
				ID Int32,
				Locale String
			) ENGINE = MergeTree()
			ORDER BY ID
		`, tableName), tableTypeLocation
	}

	panic(fmt.Sprintf("unknown table type: %T", sample))
}

// InsertData inserts data using type-specific fast paths (no reflection!)
func (r *FastClickHouseRecorder) InsertData(tableName string, entry any) {
	r.mu.Lock()

	tType, exists := r.tables[tableName]
	if !exists {
		r.mu.Unlock()
		panic(fmt.Sprintf("table %s does not exist", tableName))
	}

	// Type-specific batch append (zero reflection!)
	switch tType {
	case tableTypeExecInfo:
		if e, ok := entry.(execInfoEntry); ok {
			r.execInfoBatch = append(r.execInfoBatch, e)
		} else if e, ok := entry.(execInfo); ok {
			r.execInfoBatch = append(r.execInfoBatch, execInfoEntry{
				Property: e.Property,
				Value:    e.Value,
			})
		} else {
			r.mu.Unlock()
			panic(fmt.Sprintf("invalid entry type for execInfo: %T", entry))
		}

	case tableTypeTask:
		// Convert from external type using type assertion
		converted := r.convertToTaskEntry(entry)
		r.taskTableBatch = append(r.taskTableBatch, converted)

	case tableTypeMilestone:
		converted := r.convertToMilestoneEntry(entry)
		r.milestoneBatch = append(r.milestoneBatch, converted)

	case tableTypeSegment:
		converted := r.convertToSegmentEntry(entry)
		r.segmentBatch = append(r.segmentBatch, converted)

	case tableTypeMemoryTransaction:
		converted := r.convertToMemoryTransactionEntry(entry)
		r.memoryTransactionBatch = append(r.memoryTransactionBatch, converted)

	case tableTypeMemoryStep:
		converted := r.convertToMemoryStepEntry(entry)
		r.memoryStepBatch = append(r.memoryStepBatch, converted)

	case tableTypeLocation:
		if e, ok := entry.(locationEntry); ok {
			r.locationBatch = append(r.locationBatch, e)
		} else if e, ok := entry.(location); ok {
			r.locationBatch = append(r.locationBatch, locationEntry{
				ID:     e.ID,
				Locale: e.Locale,
			})
		}

	default:
		r.mu.Unlock()
		panic(fmt.Sprintf("unknown table type: %d", tType))
	}

	r.entryCount++

	if r.entryCount >= r.batchSize {
		r.mu.Unlock()
		r.Flush()
		return
	}

	r.mu.Unlock()
}

// Type conversion helpers - these use interface extraction without reflection
func (r *FastClickHouseRecorder) convertToTaskEntry(entry any) taskTableEntryDB {
	// This uses interface type assertion, not reflection
	type taskInterface interface {
		GetID() string
		GetParentID() string
		GetKind() string
		GetWhat() string
		GetLocation() string
		GetStartTime() float64
		GetEndTime() float64
	}

	// Try direct type assertion first
	if t, ok := entry.(taskTableEntryDB); ok {
		return t
	}

	// Fall back to interface-based extraction
	if t, ok := entry.(taskInterface); ok {
		return taskTableEntryDB{
			ID:        t.GetID(),
			ParentID:  t.GetParentID(),
			Kind:      t.GetKind(),
			What:      t.GetWhat(),
			Location:  t.GetLocation(),
			StartTime: t.GetStartTime(),
			EndTime:   t.GetEndTime(),
		}
	}

	// Use direct field access via type assertion on struct
	return extractTaskTableEntry(entry)
}

func (r *FastClickHouseRecorder) convertToMilestoneEntry(entry any) milestoneTableEntryDB {
	if m, ok := entry.(milestoneTableEntryDB); ok {
		return m
	}
	return extractMilestoneTableEntry(entry)
}

func (r *FastClickHouseRecorder) convertToSegmentEntry(entry any) segmentTableEntryDB {
	if s, ok := entry.(segmentTableEntryDB); ok {
		return s
	}
	return extractSegmentTableEntry(entry)
}

func (r *FastClickHouseRecorder) convertToMemoryTransactionEntry(entry any) memoryTransactionEntryDB {
	if m, ok := entry.(memoryTransactionEntryDB); ok {
		return m
	}
	return extractMemoryTransactionEntry(entry)
}

func (r *FastClickHouseRecorder) convertToMemoryStepEntry(entry any) memoryStepEntryDB {
	if m, ok := entry.(memoryStepEntryDB); ok {
		return m
	}
	return extractMemoryStepEntry(entry)
}

// ListTables returns all table names
func (r *FastClickHouseRecorder) ListTables() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	tables := make([]string, 0, len(r.tables))
	for name := range r.tables {
		tables = append(tables, name)
	}
	return tables
}

// Flush writes all batched data to ClickHouse using bulk inserts
func (r *FastClickHouseRecorder) Flush() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.entryCount == 0 {
		return
	}

	ctx := context.Background()

	// Flush each table batch
	for tableName, tType := range r.tables {
		switch tType {
		case tableTypeExecInfo:
			if len(r.execInfoBatch) > 0 {
				r.flushExecInfo(ctx, tableName)
			}
		case tableTypeTask:
			if len(r.taskTableBatch) > 0 {
				r.flushTaskTable(ctx, tableName)
			}
		case tableTypeMilestone:
			if len(r.milestoneBatch) > 0 {
				r.flushMilestoneTable(ctx, tableName)
			}
		case tableTypeSegment:
			if len(r.segmentBatch) > 0 {
				r.flushSegmentTable(ctx, tableName)
			}
		case tableTypeMemoryTransaction:
			if len(r.memoryTransactionBatch) > 0 {
				r.flushMemoryTransaction(ctx, tableName)
			}
		case tableTypeMemoryStep:
			if len(r.memoryStepBatch) > 0 {
				r.flushMemoryStep(ctx, tableName)
			}
		case tableTypeLocation:
			if len(r.locationBatch) > 0 {
				r.flushLocation(ctx, tableName)
			}
		}
	}

	r.entryCount = 0
}

// High-performance bulk insert functions (no reflection!)
func (r *FastClickHouseRecorder) flushExecInfo(ctx context.Context, tableName string) {
	batch, err := r.conn.PrepareBatch(ctx, fmt.Sprintf("INSERT INTO %s", tableName))
	if err != nil {
		panic(fmt.Errorf("failed to prepare batch for %s: %w", tableName, err))
	}

	for _, entry := range r.execInfoBatch {
		err = batch.Append(entry.Property, entry.Value)
		if err != nil {
			panic(fmt.Errorf("failed to append to batch: %w", err))
		}
	}

	err = batch.Send()
	if err != nil {
		panic(fmt.Errorf("failed to send batch: %w", err))
	}

	r.execInfoBatch = r.execInfoBatch[:0] // Reset slice, keep capacity
}

func (r *FastClickHouseRecorder) flushTaskTable(ctx context.Context, tableName string) {
	batch, err := r.conn.PrepareBatch(ctx, fmt.Sprintf("INSERT INTO %s", tableName))
	if err != nil {
		panic(fmt.Errorf("failed to prepare batch for %s: %w", tableName, err))
	}

	for _, entry := range r.taskTableBatch {
		err = batch.Append(
			entry.ID,
			entry.ParentID,
			entry.Kind,
			entry.What,
			entry.Location,
			entry.StartTime,
			entry.EndTime,
		)
		if err != nil {
			panic(fmt.Errorf("failed to append to batch: %w", err))
		}
	}

	err = batch.Send()
	if err != nil {
		panic(fmt.Errorf("failed to send batch: %w", err))
	}

	r.taskTableBatch = r.taskTableBatch[:0]
}

func (r *FastClickHouseRecorder) flushMilestoneTable(ctx context.Context, tableName string) {
	batch, err := r.conn.PrepareBatch(ctx, fmt.Sprintf("INSERT INTO %s", tableName))
	if err != nil {
		panic(fmt.Errorf("failed to prepare batch for %s: %w", tableName, err))
	}

	for _, entry := range r.milestoneBatch {
		err = batch.Append(
			entry.ID,
			entry.TaskID,
			entry.Time,
			entry.Kind,
			entry.What,
			entry.Location,
		)
		if err != nil {
			panic(fmt.Errorf("failed to append to batch: %w", err))
		}
	}

	err = batch.Send()
	if err != nil {
		panic(fmt.Errorf("failed to send batch: %w", err))
	}

	r.milestoneBatch = r.milestoneBatch[:0]
}

func (r *FastClickHouseRecorder) flushSegmentTable(ctx context.Context, tableName string) {
	batch, err := r.conn.PrepareBatch(ctx, fmt.Sprintf("INSERT INTO %s", tableName))
	if err != nil {
		panic(fmt.Errorf("failed to prepare batch for %s: %w", tableName, err))
	}

	for _, entry := range r.segmentBatch {
		err = batch.Append(entry.StartTime, entry.EndTime)
		if err != nil {
			panic(fmt.Errorf("failed to append to batch: %w", err))
		}
	}

	err = batch.Send()
	if err != nil {
		panic(fmt.Errorf("failed to send batch: %w", err))
	}

	r.segmentBatch = r.segmentBatch[:0]
}

func (r *FastClickHouseRecorder) flushMemoryTransaction(ctx context.Context, tableName string) {
	batch, err := r.conn.PrepareBatch(ctx, fmt.Sprintf("INSERT INTO %s", tableName))
	if err != nil {
		panic(fmt.Errorf("failed to prepare batch for %s: %w", tableName, err))
	}

	for _, entry := range r.memoryTransactionBatch {
		err = batch.Append(
			entry.ID,
			entry.Location,
			entry.What,
			entry.StartTime,
			entry.EndTime,
			entry.Address,
			entry.ByteSize,
		)
		if err != nil {
			panic(fmt.Errorf("failed to append to batch: %w", err))
		}
	}

	err = batch.Send()
	if err != nil {
		panic(fmt.Errorf("failed to send batch: %w", err))
	}

	r.memoryTransactionBatch = r.memoryTransactionBatch[:0]
}

func (r *FastClickHouseRecorder) flushMemoryStep(ctx context.Context, tableName string) {
	batch, err := r.conn.PrepareBatch(ctx, fmt.Sprintf("INSERT INTO %s", tableName))
	if err != nil {
		panic(fmt.Errorf("failed to prepare batch for %s: %w", tableName, err))
	}

	for _, entry := range r.memoryStepBatch {
		err = batch.Append(
			entry.ID,
			entry.TaskID,
			entry.Time,
			entry.What,
		)
		if err != nil {
			panic(fmt.Errorf("failed to append to batch: %w", err))
		}
	}

	err = batch.Send()
	if err != nil {
		panic(fmt.Errorf("failed to send batch: %w", err))
	}

	r.memoryStepBatch = r.memoryStepBatch[:0]
}

func (r *FastClickHouseRecorder) flushLocation(ctx context.Context, tableName string) {
	batch, err := r.conn.PrepareBatch(ctx, fmt.Sprintf("INSERT INTO %s", tableName))
	if err != nil {
		panic(fmt.Errorf("failed to prepare batch for %s: %w", tableName, err))
	}

	for _, entry := range r.locationBatch {
		err = batch.Append(entry.ID, entry.Locale)
		if err != nil {
			panic(fmt.Errorf("failed to append to batch: %w", err))
		}
	}

	err = batch.Send()
	if err != nil {
		panic(fmt.Errorf("failed to send batch: %w", err))
	}

	r.locationBatch = r.locationBatch[:0]
}

// Close flushes remaining data and closes the connection
func (r *FastClickHouseRecorder) Close() error {
	if r.execRecorder != nil {
		r.execRecorder.End()
	}

	r.Flush()

	err := r.conn.Close()
	if err != nil {
		return fmt.Errorf("failed to close ClickHouse connection: %w", err)
	}

	return nil
}

