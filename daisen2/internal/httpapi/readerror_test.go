package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

func TestTraceReaderReturnsFailureInsteadOfPartialResults(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	r := NewSQLiteTraceReader("")
	r.DB = db
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if result, err := r.ListTasks(context.Background(), TaskQuery{}); err == nil || result != nil {
		t.Fatalf("task query failure lost: %v %v", result, err)
	}
	if result, err := r.ListComponents(context.Background()); err == nil || result != nil {
		t.Fatalf("component query failure lost: %v %v", result, err)
	}
	if _, _, err := r.TimeRange(context.Background()); err == nil {
		t.Fatal("time range query failure lost")
	}
	if _, err := r.ListSegments(context.Background()); err == nil {
		t.Fatal("query failure reported as absent optional segments")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.ListTasks(ctx, TaskQuery{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
}

func TestProviderFallbackOnlyForUnsupportedTools(t *testing.T) {
	for _, status := range []int{401, 429, 500, 503} {
		if errors.Is(providerResponseError(status, "tools are unsupported", true), errToolsUnsupported) {
			t.Fatalf("retried operational failure %d", status)
		}
	}
	if errors.Is(providerResponseError(400, "invalid temperature", true), errToolsUnsupported) {
		t.Fatal("unrelated validation error triggered tool fallback")
	}
	if !errors.Is(providerResponseError(400, "tools are unsupported", true), errToolsUnsupported) {
		t.Fatal("missing explicit capability fallback")
	}
}
