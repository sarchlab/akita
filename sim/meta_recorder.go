package sim

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/sarchlab/akita/v5/sim/datarecording"
	"github.com/sarchlab/akita/v5/sim/timing"
)

type simulationInfo struct {
	Property string
	Value    string
}

// execInfoTableName holds simulation-level metadata as property/value rows.
const execInfoTableName = "akita$exec_info"

// metaRecorder records simulation-level metadata into akita$exec_info.
type metaRecorder struct {
	recorder   datarecording.DataRecorder
	timeTeller timing.TimeTeller
	ended      bool
}

func newMetaRecorder(
	recorder datarecording.DataRecorder,
	timeTeller timing.TimeTeller,
) *metaRecorder {
	r := &metaRecorder{
		recorder:   recorder,
		timeTeller: timeTeller,
	}
	r.recorder.CreateTable(execInfoTableName, simulationInfo{})
	r.Start()
	return r
}

func (r *metaRecorder) Start() {
	currentTime := time.Now()
	r.insertEntry("Start Time", currentTime.Format("2006-01-02 15:04:05.000000000"))

	r.insertEntry("Command", strings.Join(os.Args, " "))

	ex, err := os.Executable()
	if err != nil {
		panic(err)
	}

	r.insertEntry("Working Directory", filepath.Dir(ex))

	if r.timeTeller != nil {
		r.insertEntry("Start Virtual Time",
			strconv.FormatUint(uint64(r.timeTeller.CurrentTime()), 10))
	}

	r.recorder.Flush()
}

func (r *metaRecorder) End() {
	if r.ended {
		return
	}
	r.ended = true

	r.insertEntry("End Time", time.Now().Format("2006-01-02 15:04:05.000000000"))

	if r.timeTeller != nil {
		r.insertEntry("End Virtual Time",
			strconv.FormatUint(uint64(r.timeTeller.CurrentTime()), 10))
	}

	r.recorder.Flush()
}

func (r *metaRecorder) insertEntry(property, value string) {
	r.recorder.InsertData(execInfoTableName, simulationInfo{
		Property: property,
		Value:    value,
	})
}
