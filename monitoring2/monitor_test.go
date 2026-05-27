package monitoring2

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestExecutionInfoReadsExecInfoTable(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "trace.sqlite3")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec(`CREATE TABLE exec_info (Property TEXT, Value TEXT)`)
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec(`INSERT INTO exec_info (Property, Value) VALUES (?, ?)`,
		"Command", "akita")
	if err != nil {
		t.Fatal(err)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	monitor := NewMonitor()
	monitor.SetTraceDBPath(dbPath)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/execution/info", nil)

	monitor.apiExecutionInfo(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}

	var response []executionInfoEntry
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}

	if len(response) != 1 {
		t.Fatalf("expected 1 execution info entry, got %d", len(response))
	}

	if response[0].Property != "Command" || response[0].Value != "akita" {
		t.Fatalf("unexpected execution info entry: %#v", response[0])
	}
}

func TestTraceStorageReportsDatabaseAndDiskSpace(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "trace.sqlite3")

	if err := os.WriteFile(dbPath, make([]byte, 7), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(dbPath+"-wal", make([]byte, 5), 0o600); err != nil {
		t.Fatal(err)
	}

	monitor := NewMonitor()
	monitor.SetTraceDBPath(dbPath)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/trace/storage", nil)

	monitor.apiTraceStorage(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}

	var response traceStorageRsp
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}

	if response.Path != dbPath {
		t.Fatalf("expected path %q, got %q", dbPath, response.Path)
	}

	if response.FileSizeBytes != 7 {
		t.Fatalf("expected file size 7, got %d", response.FileSizeBytes)
	}

	if response.SidecarSizeBytes != 5 {
		t.Fatalf("expected sidecar size 5, got %d", response.SidecarSizeBytes)
	}

	if response.TotalSizeBytes != 12 {
		t.Fatalf("expected total size 12, got %d", response.TotalSizeBytes)
	}

	if response.DiskAvailableBytes == 0 {
		t.Fatal("expected available disk bytes")
	}

	if response.DiskTotalBytes == 0 {
		t.Fatal("expected total disk bytes")
	}
}
