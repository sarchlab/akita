package workspace

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/sarchlab/akita/v5/experiment/internal/hostsys"
)

// Job kinds of a .job file.
const (
	JobPID   = "pid"
	JobSlurm = "slurm"
)

// maxJobBytes bounds the .job file read.
const maxJobBytes = 200

// Job is the content of logs/<name>.job: "<pid>\npid\n" for a session run
// without tmux, "<id>\nslurm\n" for a SLURM job.
type Job struct {
	ID   string // process id or SLURM job id
	Kind string // JobPID or JobSlurm
}

// parseJob parses a .job file ("<id>\n<kind>\n"; a bare id is a pid).
func parseJob(data string) (Job, error) {
	lines := strings.Fields(data)
	if len(lines) == 0 {
		return Job{}, errors.New("empty job file")
	}

	j := Job{ID: lines[0], Kind: JobPID}
	if len(lines) > 1 {
		j.Kind = lines[1]
	}

	if _, err := strconv.Atoi(j.ID); err != nil {
		return Job{}, fmt.Errorf("bad job id %q", j.ID)
	}

	return j, nil
}

// ReadJob reads the .job file of a session; nil when it has none or it
// cannot be parsed.
func ReadJob(dir, name string) *Job {
	j, err := parseJob(hostsys.ReadText(SessionFile(dir, name, JobExt), maxJobBytes))
	if err != nil {
		return nil
	}

	return &j
}
