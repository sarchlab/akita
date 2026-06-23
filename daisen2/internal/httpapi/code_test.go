package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sarchlab/akita/v5/sourcefs"
)

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
