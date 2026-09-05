package datarecording

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
)

var faultDriverID atomic.Uint64
var errInjectedIO = errors.New("injected recording I/O failure")

type faultState struct {
	phase  string
	closed bool
}
type faultDriver struct{ state *faultState }

func (d faultDriver) Open(string) (driver.Conn, error) { return &faultConn{state: d.state}, nil }

type faultConn struct{ state *faultState }

func (c *faultConn) Prepare(q string) (driver.Stmt, error) {
	return &faultStmt{state: c.state, query: q}, nil
}
func (c *faultConn) Close() error {
	c.state.closed = true
	if c.state.phase == "close" {
		return errInjectedIO
	}
	return nil
}
func (c *faultConn) Begin() (driver.Tx, error) {
	if c.state.phase == "begin" {
		return nil, errInjectedIO
	}
	return &faultTx{state: c.state}, nil
}
func (c *faultConn) ExecContext(_ context.Context, q string, _ []driver.NamedValue) (driver.Result, error) {
	if c.state.phase == "index" && strings.HasPrefix(q, "CREATE INDEX") {
		return nil, errInjectedIO
	}
	return driver.RowsAffected(1), nil
}

type faultStmt struct {
	state *faultState
	query string
}

func (*faultStmt) Close() error  { return nil }
func (*faultStmt) NumInput() int { return -1 }
func (s *faultStmt) Exec([]driver.Value) (driver.Result, error) {
	if s.state.phase == "write" {
		return nil, errInjectedIO
	}
	return driver.RowsAffected(1), nil
}
func (*faultStmt) Query([]driver.Value) (driver.Rows, error) {
	return nil, errors.New("unexpected query")
}

type faultTx struct{ state *faultState }

func (tx *faultTx) Commit() error {
	if tx.state.phase == "commit" {
		return errInjectedIO
	}
	return nil
}
func (*faultTx) Rollback() error { return nil }

func TestRecorderContainsIOFailuresAndClosesResources(t *testing.T) {
	for _, phase := range []string{"begin", "write", "commit", "index", "close"} {
		t.Run(phase, func(t *testing.T) {
			state := &faultState{}
			name := fmt.Sprintf("akita-fault-%d", faultDriverID.Add(1))
			sql.Register(name, faultDriver{state})
			db, err := sql.Open(name, "")
			if err != nil {
				t.Fatal(err)
			}
			db.SetMaxOpenConns(1)
			w := NewDataRecorderWithDB(db).(*sqliteWriter)
			type row struct {
				ID int `akita_data:"index"`
			}
			if err := w.CreateTable("records", row{}); err != nil {
				t.Fatal(err)
			}
			if err := w.InsertData("records", row{1}); err != nil {
				t.Fatal(err)
			}
			state.phase = phase
			if err := w.Close(); !errors.Is(err, errInjectedIO) {
				t.Fatalf("%s failure lost: %v", phase, err)
			}
			if !state.closed {
				t.Fatalf("%s failure leaked connection", phase)
			}
			if (phase == "begin" || phase == "write" || phase == "commit") && w.entryCount != 1 {
				t.Fatal("failed transaction discarded uncommitted batch")
			}
		})
	}
}
