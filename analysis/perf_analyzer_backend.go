package analysis

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"os"

	// Need to use SQLite connections.
	_ "github.com/mattn/go-sqlite3"

	"github.com/tebeka/atexit"
)

// PerfAnalyzerBackend is the interface that provides the service that can
// record performance data entries.
type PerfAnalyzerBackend interface {
	AddDataEntry(entry PerfAnalyzerEntry)
	Flush()
}

// CSVBackend is a PerfAnalyzerBackend that writes data entries to
// a CSV file.
type CSVBackend struct {
	dbFile    *os.File
	csvWriter *csv.Writer
}

// NewCSVPerfAnalyzerBackend creates a new CSVPerfAnalyzerBackend.
func NewCSVPerfAnalyzerBackend(dbFilename string) *CSVBackend {
	if dbFilename == "" {
		return nil
	}

	p := &CSVBackend{}

	var err error
	p.dbFile, err = os.Create(dbFilename + ".csv")
	if err != nil {
		panic(err)
	}

	dbFilename = dbFilename + ".csv"
	p.dbFile, err = os.OpenFile(dbFilename,
		os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		panic(err)
	}

	p.csvWriter = csv.NewWriter(p.dbFile)

	header := []string{"Start", "End", "Where", "What", "EntryType", "Value", "Unit"}
	err = p.csvWriter.Write(header)
	if err != nil {
		panic(err)
	}

	return p
}

// AddDataEntry adds a data entry to the CSV file.
func (p *CSVBackend) AddDataEntry(entry PerfAnalyzerEntry) {
	err := p.csvWriter.Write([]string{
		fmt.Sprintf("%.10f", entry.Start),
		fmt.Sprintf("%.10f", entry.End),
		entry.Where,
		entry.What,
		entry.EntryType,
		fmt.Sprintf("%.10f", entry.Value),
		entry.Unit,
	})
	if err != nil {
		panic(err)
	}
}

// Flush flushes the CSV writer.
func (p *CSVBackend) Flush() {
	p.csvWriter.Flush()
}

// SQLiteBackend is a PerfAnalyzerBackend that writes data entries
// to a SQLite database.
type SQLiteBackend struct {
	*sql.DB
	statement *sql.Stmt

	batchSize int
	entries   []PerfAnalyzerEntry
}

// NewSQLitePerfAnalyzerBackend creates a new SQLitePerfAnalyzerBackend.
func NewSQLitePerfAnalyzerBackend(
	dbFilename string,
) *SQLiteBackend {
	p := &SQLiteBackend{
		batchSize: 50000,
	}

	p.createDatabase(dbFilename + ".sqlite3")
	p.prepareStatement()

	atexit.Register(func() {
		p.Flush()
		err := p.Close()
		if err != nil {
			panic(err)
		}
	})

	return p
}

func (p *SQLiteBackend) AddDataEntry(entry PerfAnalyzerEntry) {
	p.entries = append(p.entries, entry)
	if len(p.entries) >= p.batchSize {
		p.Flush()
	}
}

func (p *SQLiteBackend) Flush() {
	if len(p.entries) == 0 {
		return
	}

	tx, err := p.Begin()
	if err != nil {
		panic(err)
	}

	defer func() {
		innerErr := tx.Commit()
		if innerErr != nil {
			panic(innerErr)
		}
	}()

	for _, entry := range p.entries {
		_, err = tx.Stmt(p.statement).Exec(
			entry.Start,
			entry.End,
			entry.Where,
			entry.What,
			entry.EntryType,
			entry.Value,
			entry.Unit,
		)
		if err != nil {
			panic(err)
		}
	}

	p.entries = p.entries[:0]
}

func (p *SQLiteBackend) createDatabase(dbFilename string) {
	var err error

	_, err = os.Stat(dbFilename)
	if err == nil {
		err = os.Remove(dbFilename)
		if err != nil {
			panic(err)
		}
	}

	p.DB, err = sql.Open("sqlite3", dbFilename)
	if err != nil {
		panic(err)
	}

	p.createTable()
}

func (p *SQLiteBackend) createTable() {
	sqlStmt := `
	create table perf (
		id integer not null primary key,
		start real,
		end real,
		where text,
		what text,
		entryType text,
		value real,
		unit text
	);
	`

	_, err := p.Exec(sqlStmt)
	if err != nil {
		panic(err)
	}
}

func (p *SQLiteBackend) prepareStatement() {
	var err error

	sqlStmt := `
	insert into perf(start, end, where_, what_, entryType, value, unit)
	values(?, ?, ?, ?, ?, ?, ?)
	`

	p.statement, err = p.Prepare(sqlStmt)
	if err != nil {
		panic(err)
	}
}
