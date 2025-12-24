package datarecording

// This file verifies that FastClickHouseRecorder implements DataRecorder interface
// If this compiles, the interface is correctly implemented

var _ DataRecorder = (*FastClickHouseRecorder)(nil)
var _ DataRecorder = (*sqliteWriter)(nil)
