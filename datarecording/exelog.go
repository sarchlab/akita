package datarecording

// Struct ExeInfo is feed to SQLiteWriter
type ExeInfo struct {
	property string
	value    string
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

	// Stores all the tasks as command
	tasks []ExeInfo
}

// Write keeps track of current execution and writes it into SQLiteWriter
func (e *ExeRecorder) Write() {
}

// Flush wash all the data into SQLite
func (e *ExeRecorder) Flush() {
}

func NewExeRecoerder(path string) *ExeRecorder {
	return nil
}

func init() {}
