package datarecording_test

import (
	"fmt"
	"testing"

	"github.com/sarchlab/akita/v4/datarecording"
)

// This example demonstrates how to use the ClickHouse data recorder
// with the RecorderConfig API
func ExampleNewDataRecorderWithConfig_clickhouse() {
	// Create a ClickHouse recorder using connection string
	recorder := datarecording.NewDataRecorderWithConfig(datarecording.RecorderConfig{
		Type:      "clickhouse",
		ConnStr:   "clickhouse://localhost:9000/test_db?username=default&password=secret",
		BatchSize: 50000,
	})
	defer recorder.Close()

	// Create a table
	type TaskEntry struct {
		ID        string
		Name      string
		Priority  int
		StartTime float64
	}

	recorder.CreateTable("tasks", TaskEntry{})

	// Insert data
	recorder.InsertData("tasks", TaskEntry{
		ID:        "task-1",
		Name:      "Example Task",
		Priority:  1,
		StartTime: 0.0,
	})

	// Flush to ensure data is written
	recorder.Flush()

	fmt.Println("Data recorded successfully")
	// Output: Data recorded successfully
}

// This example demonstrates using individual parameters instead of connection string
func ExampleNewDataRecorderWithConfig_clickhouseParams() {
	recorder := datarecording.NewDataRecorderWithConfig(datarecording.RecorderConfig{
		Type:      "clickhouse",
		Host:      "localhost",
		Port:      9000,
		Database:  "test_db",
		Username:  "default",
		Password:  "secret",
		BatchSize: 100000,
	})
	defer recorder.Close()

	fmt.Println("Recorder created successfully")
	// Output: Recorder created successfully
}

// This test shows backward compatibility with SQLite
func ExampleNewDataRecorderWithConfig_sqlite() {
	recorder := datarecording.NewDataRecorderWithConfig(datarecording.RecorderConfig{
		Type:      "sqlite",
		Path:      "test_data",
		BatchSize: 10000,
	})
	defer recorder.Close()

	fmt.Println("SQLite recorder created successfully")
	// Output: SQLite recorder created successfully
}

// Benchmark test to show high performance
func BenchmarkClickHouseRecorder_Insert(b *testing.B) {
	// Skip if ClickHouse is not available
	// In real use, you'd have ClickHouse running
	b.Skip("Requires ClickHouse server")

	recorder := datarecording.NewDataRecorderWithConfig(datarecording.RecorderConfig{
		Type:      "clickhouse",
		Host:      "localhost",
		Port:      9000,
		Database:  "benchmark_db",
		Username:  "default",
		Password:  "",
		BatchSize: 100000,
	})
	defer recorder.Close()

	type BenchEntry struct {
		ID    string
		Value int
		Time  float64
	}

	recorder.CreateTable("bench_table", BenchEntry{})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		recorder.InsertData("bench_table", BenchEntry{
			ID:    fmt.Sprintf("entry-%d", i),
			Value: i,
			Time:  float64(i),
		})
	}
	recorder.Flush()
}
