package datarecording

// Struct ExeInfo is feed to SQLiteWriter
type ExeInfo struct {
	property string
	value    string
}

/*
1. Log command through init() function
2. Convert the execution into ExeInfo object
3. Through SQLiteWriter API we create a table, and insert all following objects
4. Use atexit command to "automatically" flush all ExeInfom objects in a table eventually
*/

type ExeRecorder struct {
	// ExeRecorder use SQLiteWriter to log execution
	writer *SQLiteWriter

	// Stores all the tasks as command
	tasks ExeInfo
}

// Write keeps track of current execution and writes it into SQLiteWriter
func (e *ExeRecorder) Write() {
}

// Flush writes data into SQLite along with program exit time
func (e *ExeRecorder) Flush() {
}

// Sets SQLiteWriter
func (e *ExeRecorder) SetWriter(inputWriter *SQLiteWriter) {
	e.writer = inputWriter
}

func NewExeRecoerder(
	path string,
) *ExeRecorder {
	return nil
}

func init() {}
