package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sarchlab/akita/v5/sourcefs"
)

func TestMostBlockedRanksByBlockedTime(t *testing.T) {
	reader := newTestReader(t)
	exec := func(q string) {
		if _, err := reader.Exec(q); err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
	}
	exec(`CREATE TABLE location (ID INTEGER, Locale TEXT)`)
	exec(`INSERT INTO location VALUES (1, 'A'), (2, 'B')`)
	exec(`CREATE TABLE trace (
		ID INTEGER, ParentID INTEGER, Kind TEXT, What TEXT,
		Location INTEGER, StartTime REAL, EndTime REAL)`)
	exec(`INSERT INTO trace VALUES (1, 0, 'req_in', 'R', 1, 0, 100)`)
	exec(`INSERT INTO trace VALUES (2, 0, 'req_in', 'R', 2, 0, 100)`)
	exec(`CREATE TABLE milestone (ID INTEGER, TaskID INTEGER, Time REAL, Kind TEXT, What TEXT)`)
	// Task 1 (component A): blocked [0,30] then a 'work' interval that must NOT count.
	exec(`INSERT INTO milestone VALUES (10, 1, 30, 'network_busy', 'x')`)
	exec(`INSERT INTO milestone VALUES (11, 1, 50, 'work', 'x')`)
	// Task 2 (component B): blocked [0,40] then [40,90] -> 90 total.
	exec(`INSERT INTO milestone VALUES (20, 2, 40, 'hardware_resource', 'y')`)
	exec(`INSERT INTO milestone VALUES (21, 2, 90, 'network_busy', 'y')`)

	ranked := reader.MostBlocked(context.Background())
	if len(ranked) != 2 {
		t.Fatalf("expected 2 components, got %d (%+v)", len(ranked), ranked)
	}
	if ranked[0].Component != "B" || ranked[0].BlockedTime != 90 {
		t.Fatalf("expected B=90 first, got %+v", ranked[0])
	}
	if ranked[1].Component != "A" || ranked[1].BlockedTime != 30 {
		t.Fatalf("expected A=30 second (work excluded), got %+v", ranked[1])
	}
}

func TestMostBlockedMissingTables(t *testing.T) {
	reader := newTestReader(t)
	ranked := reader.MostBlocked(context.Background())
	if ranked == nil || len(ranked) != 0 {
		t.Fatalf("expected empty slice for missing tables, got %#v", ranked)
	}
}

func TestHTTPCodeLsEmptySource(t *testing.T) {
	s := &Server{codeSource: &sourcefs.Source{}}
	rec := httptest.NewRecorder()
	s.httpCodeLs(rec, httptest.NewRequest(http.MethodGet, "/api/code/ls", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var resp CodeLsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Entries) != 0 {
		t.Fatalf("expected no entries for empty source, got %+v", resp.Entries)
	}
}

func TestHTTPCodeReadEmptySource(t *testing.T) {
	s := &Server{codeSource: &sourcefs.Source{}}
	rec := httptest.NewRecorder()
	s.httpCodeRead(
		rec, httptest.NewRequest(http.MethodGet, "/api/code/read?path=x.go", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for empty source, got %d", rec.Code)
	}
}

func TestHTTPCodeReadRejectsBadPath(t *testing.T) {
	s := &Server{codeSource: &sourcefs.Source{}}
	rec := httptest.NewRecorder()
	s.httpCodeRead(
		rec, httptest.NewRequest(http.MethodGet, "/api/code/read?path=../escape", nil))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for path traversal, got %d", rec.Code)
	}
}
