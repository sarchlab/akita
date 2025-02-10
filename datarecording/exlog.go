package datarecording

type Exlog struct {
	// Exlog utilize DataRecorder to log execution
	DataRecorder

	// Store property of each command execution in string format
	prop []string

	// Store value of each command execution in string format
	value []string
}
