package datarecording

// Struct ExeInfo is feed to SQLiteWriter
type ExeInfo struct {
	ID         string
	command    string
	cwd        string
	start_time string
	end_time   string
}

/*
1. Whenever a program executes certain command, we get notified.
2. Convert the execution into ExeInfo object
3. Through SQLiteWriter API we create a table, and insert all following objects
4. Use atexit command to "automatically" flush all ExeInfom objects in a table eventually
*/

type ExeRecorder struct {
	// ExeRecorder use SQLiteWriter to log execution
	writer *SQLiteWriter

	// Store property of each command execution in string format
	prop []string

	// Store value of each command execution in string format
	value []string
}

func (e *ExeRecorder) Trigger() {
}

func (e *ExeRecorder) RecordStartTime() {}

func (e *ExeRecorder) RecordEndTime() {}

func (e *ExeRecorder) RecordCommand() {}

func (e *ExeRecorder) RecordCurrentDirectory() {}

func NewExeRecoerder(path string) *ExeRecorder {
	return nil
}
