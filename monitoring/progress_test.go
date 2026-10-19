package monitoring

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/sarchlab/akita/v5/sim"
)

func TestProgressSnapshotIsIndependent(t *testing.T) {
	bar := &ProgressBar{}
	bar.IncrementInProgress(7)
	bar.MoveInProgressToFinished(3)
	bar.IncrementFinished(2)
	copy := bar.Snapshot()
	if copy.Finished != 5 || copy.InProgress != 4 {
		t.Fatalf("unexpected counters: %+v", copy)
	}
	copy.Finished = 999
	if bar.Snapshot().Finished != 5 {
		t.Fatal("changing the snapshot changed live progress")
	}
}

func TestProgressUpdatesAndHTTPReadsAreConcurrent(t *testing.T) {
	m := MakeBuilder().Build()
	s := sim.MakeBuilder().WithMonitor(m).Build()
	t.Cleanup(func() { _ = s.Terminate() })
	if err := s.Initialize(); err != nil {
		t.Fatal(err)
	}
	const workers, iterations = 4, 200
	bar := m.CreateProgressBar("work", workers*iterations)
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for range iterations {
				bar.IncrementInProgress(1)
				bar.MoveInProgressToFinished(1)
			}
		})
	}
	wg.Go(func() {
		for range iterations {
			other := m.CreateProgressBar("other", 1)
			m.CompleteProgressBar(other)
		}
	})
	for range iterations {
		response := httptest.NewRecorder()
		m.listProgressBars(response, httptest.NewRequest(http.MethodGet, "/api/progress", nil))
		var snapshots []ProgressBarSnapshot
		if err := json.Unmarshal(response.Body.Bytes(), &snapshots); err != nil {
			t.Fatal(err)
		}
		for _, snapshot := range snapshots {
			if snapshot.StartTime.IsZero() || snapshot.Finished+snapshot.InProgress > snapshot.Total {
				t.Errorf("inconsistent progress snapshot: %+v", snapshot)
			}
		}
	}
	wg.Wait()
	snapshot := bar.Snapshot()
	if snapshot.Finished != workers*iterations || snapshot.InProgress != 0 {
		t.Fatalf("lost progress: %+v", snapshot)
	}
	m.CompleteProgressBar(bar)
	m.CompleteProgressBar(bar) // Already removed: the list is empty.
	m.CompleteProgressBar(nil)
	response := httptest.NewRecorder()
	m.listProgressBars(response, httptest.NewRequest(http.MethodGet, "/api/progress", nil))
	if response.Body.String() != "[]" {
		t.Fatalf("expected an empty JSON array, got %s", response.Body.String())
	}
	t.Logf("HTTP snapshots stayed consistent during updates; finished=%d; final response=[]", snapshot.Finished)
}
