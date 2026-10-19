package workspace

import (
	"io"
	"os"
	"strings"

	"github.com/sarchlab/akita/v5/experiment"
)

// Log is what is read of a log file: its head and its tail.
type Log struct {
	Size  int64
	MTime float64 // Unix seconds
	Head  string
	Tail  string
	Cut   bool // the tail does not start at the beginning of the file
}

// ReadLog reads the first headBytes and the last tailBytes of a log
// (invalid UTF-8 replaced).
func ReadLog(path string, headBytes, tailBytes int64) (*Log, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return nil, err
	}

	head := make([]byte, min(headBytes, st.Size()))
	n, _ := io.ReadFull(f, head)

	off := max(0, st.Size()-tailBytes)
	if _, err := f.Seek(off, io.SeekStart); err != nil {
		return nil, err
	}

	tail, err := io.ReadAll(io.LimitReader(f, tailBytes))
	if err != nil {
		return nil, err
	}

	return &Log{
		Size:  st.Size(),
		MTime: experiment.UnixSeconds(st.ModTime()),
		Head:  strings.ToValidUTF8(string(head[:n]), "�"),
		Tail:  strings.ToValidUTF8(string(tail), "�"),
		Cut:   off > 0,
	}, nil
}

// Markers parses the log's markers.
func (l *Log) Markers() LogState { return parseLogMarkers(l.Head, l.Tail) }

// TailLines returns the last n complete lines of the tail, without "\r"
// and without Go's "GOPATH set to GOROOT" warnings.
func (l *Log) TailLines(n int) []string {
	lines := strings.Split(l.Tail, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	if l.Cut && len(lines) > 0 {
		lines = lines[1:]
	}

	out := make([]string, 0, len(lines))

	for _, line := range lines {
		if !strings.Contains(line, "GOPATH set to GOROOT") {
			out = append(out, strings.TrimSuffix(line, "\r"))
		}
	}

	if len(out) > n {
		out = out[len(out)-n:]
	}

	return out
}
