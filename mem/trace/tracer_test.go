package trace

import (
	"database/sql"
	"log"
	"os"
	"testing"

	"github.com/sarchlab/akita/v4/datarecording"
	"github.com/sarchlab/akita/v4/mem/vm"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/tracing"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	// Need SQLite driver for tests
	_ "github.com/mattn/go-sqlite3"
)

type TracerTestSuite struct {
	suite.Suite

	dataRecorder datarecording.DataRecorder
	db           *sql.DB
	tracer       tracing.Tracer
	timeTeller   *MockTimeTeller
	mockCtrl     *gomock.Controller
	tempFileName string
}

func (suite *TracerTestSuite) SetupTest() {
	// Create gomock controller
	suite.mockCtrl = gomock.NewController(suite.T())

	// Create temporary file database for testing (instead of in-memory)
	tempFile, err := os.CreateTemp("", "tracer_test_*.db")
	suite.Require().NoError(err)
	suite.tempFileName = tempFile.Name()
	tempFile.Close()

	db, err := sql.Open("sqlite3", suite.tempFileName)
	suite.Require().NoError(err)

	suite.db = db
	suite.dataRecorder = datarecording.NewDataRecorderWithDB(db)
	suite.timeTeller = NewMockTimeTeller(suite.mockCtrl)
	suite.tracer = NewDBTracer(suite.dataRecorder, suite.timeTeller)
}

func (suite *TracerTestSuite) TearDownTest() {
	if suite.dataRecorder != nil {
		suite.dataRecorder.Close()
	}
	if suite.db != nil {
		suite.db.Close()
	}
	if suite.mockCtrl != nil {
		suite.mockCtrl.Finish()
	}
	if suite.tempFileName != "" {
		os.Remove(suite.tempFileName)
	}
}

// MockAccessReq implements mem.AccessReq for testing
type MockAccessReq struct {
	sim.MsgMeta

	address  uint64
	byteSize uint64
	pid      vm.PID
}

func (r *MockAccessReq) Meta() *sim.MsgMeta {
	return &r.MsgMeta
}

func (r *MockAccessReq) Clone() sim.Msg {
	return &MockAccessReq{
		MsgMeta:  r.MsgMeta,
		address:  r.address,
		byteSize: r.byteSize,
		pid:      r.pid,
	}
}

func (r *MockAccessReq) GetAddress() uint64 {
	return r.address
}

func (r *MockAccessReq) GetByteSize() uint64 {
	return r.byteSize
}

func (r *MockAccessReq) GetPID() vm.PID {
	return r.pid
}

func (suite *TracerTestSuite) TestStartAndEndTask() {
	// Create a mock access request
	req := &MockAccessReq{
		address:  0x1000,
		byteSize: 64,
		pid:      1,
	}

	// Create a task
	task := tracing.Task{
		ID:       "test_task_1",
		Location: "test_location",
		What:     "read",
		Detail:   req,
	}

	suite.runBasicTrace(task)
	suite.verifyBasicTransaction(task)
}

func (suite *TracerTestSuite) runBasicTrace(task tracing.Task) {
	// Set up mock expectations
	suite.timeTeller.EXPECT().CurrentTime().Return(sim.VTimeInSec(100.0)).Times(1) // StartTask
	suite.timeTeller.EXPECT().CurrentTime().Return(sim.VTimeInSec(200.0)).Times(1) // EndTask

	// Start the task
	suite.tracer.StartTask(task)

	// End the task
	suite.tracer.EndTask(task)

	// Flush data to ensure it's written
	suite.dataRecorder.Flush()
}

func (suite *TracerTestSuite) verifyBasicTransaction(task tracing.Task) {
	// Check row count first
	countRows, err := suite.db.Query("SELECT COUNT(*) FROM memory_transactions")
	suite.Require().NoError(err)
	defer countRows.Close()
	suite.Require().True(countRows.Next())
	var count int
	countRows.Scan(&count)

	suite.Require().Equal(1, count, "Should have exactly one transaction")

	// Verify the transaction was recorded
	query := "SELECT ID, Location, What, StartTime, EndTime, Address, ByteSize FROM memory_transactions"
	rows, err := suite.db.Query(query)
	suite.Require().NoError(err)
	defer rows.Close()

	suite.Require().True(rows.Next(), "Expected at least one row")

	var id, location, what string
	var startTime, endTime float64
	var address, byteSize uint64

	err = rows.Scan(&id, &location, &what, &startTime, &endTime, &address, &byteSize)
	suite.Require().NoError(err)

	suite.Equal("test_task_1", id)
	suite.Equal("test_location", location)
	suite.Equal("read", what)
	suite.Equal(100.0, startTime)
	suite.Equal(200.0, endTime)
	suite.Equal(uint64(0x1000), address)
	suite.Equal(uint64(64), byteSize)

	// Ensure no more rows
	suite.False(rows.Next(), "Expected only one row")
}

func (suite *TracerTestSuite) TestStepTask() {
	// Create a mock access request
	req := &MockAccessReq{
		address:  0x2000,
		byteSize: 32,
		pid:      2,
	}

	// Create a task with a step
	task := tracing.Task{
		ID:       "test_task_2",
		Location: "test_location",
		What:     "write",
		Detail:   req,
		Steps: []tracing.TaskStep{
			{What: "cache_hit"},
		},
	}

	// Set up mock expectations
	suite.timeTeller.EXPECT().CurrentTime().Return(sim.VTimeInSec(150.0)).Times(1)

	// Record the step
	suite.tracer.StepTask(task)

	// Flush data to ensure it's written
	suite.dataRecorder.Flush()

	// Verify the step was recorded
	rows, err := suite.db.Query("SELECT ID, TaskID, Time, What FROM memory_steps")
	suite.Require().NoError(err)
	defer rows.Close()

	suite.Require().True(rows.Next(), "Expected at least one row")

	var id, taskID, what string
	var time float64

	err = rows.Scan(&id, &taskID, &time, &what)
	suite.Require().NoError(err)

	suite.Equal("test_task_2_step_cache_hit", id)
	suite.Equal("test_task_2", taskID)
	suite.Equal(150.0, time)
	suite.Equal("cache_hit", what)

	// Ensure no more rows
	suite.False(rows.Next(), "Expected only one row")
}

func (suite *TracerTestSuite) TestCompleteMemoryTrace() {
	// Create a mock access request
	req := &MockAccessReq{
		address:  0x3000,
		byteSize: 128,
		pid:      3,
	}

	// Create a task
	task := tracing.Task{
		ID:       "test_task_3",
		Location: "memory_controller",
		What:     "read",
		Detail:   req,
		Steps: []tracing.TaskStep{
			{What: "cache_miss"},
		},
	}

	suite.runCompleteTrace(task)
	suite.verifyCompleteTransaction(task)
	suite.verifyCompleteStep(task)
}

func (suite *TracerTestSuite) runCompleteTrace(task tracing.Task) {
	// Set up mock expectations in order
	gomock.InOrder(
		suite.timeTeller.EXPECT().CurrentTime().Return(sim.VTimeInSec(50.0)).Times(1),  // StartTask
		suite.timeTeller.EXPECT().CurrentTime().Return(sim.VTimeInSec(75.0)).Times(1),  // StepTask
		suite.timeTeller.EXPECT().CurrentTime().Return(sim.VTimeInSec(100.0)).Times(1), // EndTask
	)

	// Start task at time 50
	suite.tracer.StartTask(task)

	// Record step at time 75
	suite.tracer.StepTask(task)

	// End task at time 100
	suite.tracer.EndTask(task)

	// Flush data
	suite.dataRecorder.Flush()
}

func (suite *TracerTestSuite) verifyCompleteTransaction(task tracing.Task) {
	transactionQuery := "SELECT ID, Location, What, StartTime, EndTime, Address, ByteSize FROM memory_transactions"
	transactionRows, err := suite.db.Query(transactionQuery)
	suite.Require().NoError(err)
	defer transactionRows.Close()

	suite.Require().True(transactionRows.Next())
	var id, location, what string
	var startTime, endTime float64
	var address, byteSize uint64

	err = transactionRows.Scan(&id, &location, &what, &startTime, &endTime, &address, &byteSize)
	suite.Require().NoError(err)

	suite.Equal("test_task_3", id)
	suite.Equal("memory_controller", location)
	suite.Equal("read", what)
	suite.Equal(50.0, startTime)
	suite.Equal(100.0, endTime)
	suite.Equal(uint64(0x3000), address)
	suite.Equal(uint64(128), byteSize)
}

func (suite *TracerTestSuite) verifyCompleteStep(task tracing.Task) {
	stepRows, err := suite.db.Query("SELECT ID, TaskID, Time, What FROM memory_steps")
	suite.Require().NoError(err)
	defer stepRows.Close()

	suite.Require().True(stepRows.Next())
	var stepID, taskID, stepWhat string
	var stepTime float64

	err = stepRows.Scan(&stepID, &taskID, &stepTime, &stepWhat)
	suite.Require().NoError(err)

	suite.Equal("test_task_3_step_cache_miss", stepID)
	suite.Equal("test_task_3", taskID)
	suite.Equal(75.0, stepTime)
	suite.Equal("cache_miss", stepWhat)
}

func (suite *TracerTestSuite) TestTaskWithoutAccessReq() {
	// Create a task without AccessReq detail
	task := tracing.Task{
		ID:       "test_task_4",
		Location: "test_location",
		What:     "non_memory",
		Detail:   "not_an_access_req",
	}

	// Set up mock expectations - CurrentTime should be called for StartTask but not EndTask since no AccessReq
	suite.timeTeller.EXPECT().CurrentTime().Return(sim.VTimeInSec(10.0)).Times(1)

	// Start the task (this will not create a pending transaction due to no AccessReq)
	suite.tracer.StartTask(task)
	
	// End the task (this should not call CurrentTime since no pending transaction exists)
	suite.tracer.EndTask(task)

	// Flush data
	suite.dataRecorder.Flush()

	// Verify no transaction was recorded (since Detail is not AccessReq)
	rows, err := suite.db.Query("SELECT COUNT(*) FROM memory_transactions")
	suite.Require().NoError(err)
	defer rows.Close()

	suite.Require().True(rows.Next())
	var count int
	err = rows.Scan(&count)
	suite.Require().NoError(err)

	suite.Equal(0, count, "Expected no transactions for non-AccessReq tasks")
}

func (suite *TracerTestSuite) TestAddMilestone() {
	// AddMilestone should do nothing for memory tracer
	milestone := tracing.Milestone{
		ID:       "milestone_1",
		TaskID:   "task_1",
		Kind:     tracing.MilestoneKindHardwareResource,
		What:     "resource_acquired",
		Location: "test_location",
	}

	// This should not panic and should do nothing
	suite.tracer.AddMilestone(milestone)

	// Flush and verify no data was written
	suite.dataRecorder.Flush()

	// Check that no milestone-related tables exist or are populated
	// Since AddMilestone does nothing, we just ensure it doesn't crash
}

func TestTracerTestSuite(t *testing.T) {
	suite.Run(t, new(TracerTestSuite))
}

// Test compatibility with existing logger-based tracer
func TestLoggerTracerStillWorks(t *testing.T) {
	// Create a simple time teller that returns the current time
	timeTeller := &fixedTimeTeller{time: 100.0}
	
	// Create logger-based tracer - use discard to avoid output during tests
	logger := log.New(os.Stdout, "", 0)
	tracer := NewTracer(logger, timeTeller)

	// Create a mock request
	req := &MockAccessReq{
		address:  0x4000,
		byteSize: 256,
		pid:      4,
	}

	// Test that the logger tracer still works
	task := tracing.Task{
		ID:       "logger_test",
		Location: "test_loc",
		What:     "test_what",
		Detail:   req,
		Steps: []tracing.TaskStep{
			{What: "test_step"},
		},
	}

	// These should not panic
	tracer.StartTask(task)

	timeTeller.time = 150.0
	tracer.StepTask(task)

	timeTeller.time = 200.0
	tracer.EndTask(task)

	// Add milestone (should do nothing)
	milestone := tracing.Milestone{
		ID:       "test_milestone",
		TaskID:   "logger_test",
		Kind:     tracing.MilestoneKindOther,
		What:     "test",
		Location: "test_loc",
	}
	tracer.AddMilestone(milestone)
}

// fixedTimeTeller is a simple implementation for testing
type fixedTimeTeller struct {
	time sim.VTimeInSec
}

func (f *fixedTimeTeller) CurrentTime() sim.VTimeInSec {
	return f.time
}