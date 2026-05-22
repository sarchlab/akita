package simulation

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/sarchlab/akita/v5/datarecording"
	"github.com/sarchlab/akita/v5/timing"
)

type simulationInfo struct {
	Property string
	Value    string
}

// metaRecorder records simulation-level metadata into exec_info.
type metaRecorder struct {
	tableName  string
	recorder   datarecording.DataRecorder
	timeTeller timing.TimeTeller
	entries    []simulationInfo
	ended      bool
}

func newMetaRecorder(
	recorder datarecording.DataRecorder,
	timeTeller timing.TimeTeller,
) *metaRecorder {
	r := &metaRecorder{
		tableName:  "exec_info",
		recorder:   recorder,
		timeTeller: timeTeller,
	}
	r.recorder.CreateTable(r.tableName, simulationInfo{})
	r.Start()
	return r
}

func (r *metaRecorder) Start() {
	currentTime := time.Now()
	r.entries = append(r.entries, simulationInfo{
		Property: "Start Time",
		Value:    currentTime.Format("2006-01-02 15:04:05.000000000"),
	})

	r.entries = append(r.entries, simulationInfo{
		Property: "Command",
		Value:    strings.Join(os.Args, " "),
	})

	ex, err := os.Executable()
	if err != nil {
		panic(err)
	}

	r.entries = append(r.entries, simulationInfo{
		Property: "Working Directory",
		Value:    filepath.Dir(ex),
	})

	if r.timeTeller != nil {
		r.entries = append(r.entries, simulationInfo{
			Property: "Start Virtual Time",
			Value:    strconv.FormatUint(uint64(r.timeTeller.CurrentTime()), 10),
		})
	}
}

func (r *metaRecorder) End() {
	if r.ended {
		return
	}
	r.ended = true

	for _, entry := range r.entries {
		r.recorder.InsertData(r.tableName, entry)
	}

	r.recorder.InsertData(r.tableName, simulationInfo{
		Property: "End Time",
		Value:    time.Now().Format("2006-01-02 15:04:05.000000000"),
	})

	if r.timeTeller != nil {
		r.recorder.InsertData(r.tableName, simulationInfo{
			Property: "End Virtual Time",
			Value:    strconv.FormatUint(uint64(r.timeTeller.CurrentTime()), 10),
		})
	}

	r.entries = nil
	r.recorder.Flush()
}
