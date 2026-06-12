package tracing

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/sarchlab/akita/v5/datarecording"
)

var _ = Describe("DBTracer Termination", func() {
	var (
		timeTeller   *testTimeTeller
		dataRecorder datarecording.DataRecorder
		tracer       *DBTracer
		dbPath       string
	)

	BeforeEach(func() {
		timeTeller = &testTimeTeller{}
		dbPath = "test_trace_terminate"
		os.Remove(dbPath + ".sqlite3")
		dataRecorder = datarecording.NewDataRecorder(dbPath)
		tracer = NewDBTracer(timeTeller, dataRecorder)
	})

	AfterEach(func() {
		if dataRecorder != nil {
			dataRecorder.Close()
			os.Remove(dbPath + ".sqlite3")
		}
	})

	// Regression test: Terminate() sets tracingTasks to nil. Any task activity
	// arriving afterwards (e.g. an in-flight request completing during teardown)
	// must be ignored rather than panic with "assignment to entry in nil map".
	It("should ignore task activity that arrives after Terminate", func() {
		tracer.StartTracing()
		tracer.Terminate()

		Expect(func() {
			tracer.StartTask(TaskStart{
				ID:       1,
				Kind:     "req",
				What:     "read",
				Location: "L1",
				Time:     100,
			})
			tracer.AddTaskTag(TaskTag{ID: 2, TaskID: 1, What: "tag", Time: 100})
			tracer.AddMilestone(Milestone{
				ID:     3,
				TaskID: 1,
				Time:   100,
				Kind:   MilestoneKindHardwareResource,
				What:   "resource_acquired",
			})
			tracer.EndTask(TaskEnd{ID: 1, Time: 200})
		}).ToNot(Panic())
	})
})
