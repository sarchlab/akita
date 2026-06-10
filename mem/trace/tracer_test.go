package trace

import (
	"database/sql"
	"os"
	"testing"

	"github.com/sarchlab/akita/v5/datarecording"
	"github.com/sarchlab/akita/v5/mem/vm"

	"github.com/sarchlab/akita/v5/tracing"
	"github.com/stretchr/testify/suite"

	// Need SQLite driver for tests
	_ "github.com/glebarez/go-sqlite"
	"github.com/sarchlab/akita/v5/messaging"
)

type TracerTestSuite struct {
	suite.Suite

	dataRecorder datarecording.DataRecorder
	db           *sql.DB
	tracer       tracing.Tracer
	tempFileName string
}

func (suite *TracerTestSuite) SetupTest() {
	// Create temporary file database for testing (instead of in-memory)
	tempFile, err := os.CreateTemp("", "tracer_test_*.db")
	suite.Require().NoError(err)
	suite.tempFileName = tempFile.Name()
	tempFile.Close()

	db, err := sql.Open("sqlite", suite.tempFileName)
	suite.Require().NoError(err)

	suite.db = db
	suite.dataRecorder = datarecording.NewDataRecorderWithDB(db)
	suite.tracer = NewDBTracer(suite.dataRecorder)
}

func (suite *TracerTestSuite) TearDownTest() {
	if suite.dataRecorder != nil {
		suite.dataRecorder.Close()
	}
	if suite.db != nil {
		suite.db.Close()
	}
	if suite.tempFileName != "" {
		os.Remove(suite.tempFileName)
	}
}

// MockAccessReq implements memprotocol.AccessReq for testing
type MockAccessReq struct {
	messaging.MsgMeta
	address  uint64
	byteSize uint64
	pid      vm.PID
}

func (r MockAccessReq) GetAddress() uint64 {
	return r.address
}

func (r MockAccessReq) GetByteSize() uint64 {
	return r.byteSize
}

func (r MockAccessReq) GetPID() vm.PID {
	return r.pid
}

func (suite *TracerTestSuite) TestStartAndEndTask() {
	req := MockAccessReq{
		address:  0x1000,
		byteSize: 64,
		pid:      1,
	}

	suite.tracer.StartTask(tracing.TaskStart{
		ID:       1,
		Location: "test_location",
		What:     "read",
		Detail:   req,
		Time:     100,
	})

	suite.tracer.EndTask(tracing.TaskEnd{ID: 1, Time: 200})

	suite.dataRecorder.Flush()

	suite.verifyBasicTransaction()
}

func (suite *TracerTestSuite) verifyBasicTransaction() {
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

	var id uint64
	var location, what string
	var startTime, endTime float64
	var address, byteSize uint64

	err = rows.Scan(
		&id,
		&location,
		&what,
		&startTime,
		&endTime,
		&address,
		&byteSize,
	)
	suite.Require().NoError(err)

	suite.Equal(uint64(1), id)
	suite.Equal("test_location", location)
	suite.Equal("read", what)
	suite.Equal(100.0, startTime)
	suite.Equal(200.0, endTime)
	suite.Equal(uint64(0x1000), address)
	suite.Equal(uint64(64), byteSize)

	// Ensure no more rows
	suite.False(rows.Next(), "Expected only one row")
}

func (suite *TracerTestSuite) TestTaskTag() {
	suite.tracer.AddTaskTag(tracing.TaskTag{
		TaskID: 2,
		What:   "cache_hit",
		Time:   150,
	})

	suite.dataRecorder.Flush()

	// Verify the tag was recorded
	rows, err := suite.db.Query(
		"SELECT ID, TaskID, Time, What FROM memory_tags",
	)
	suite.Require().NoError(err)
	defer rows.Close()

	suite.Require().True(rows.Next(), "Expected at least one row")

	var id, taskID uint64
	var what string
	var time float64

	err = rows.Scan(&id, &taskID, &time, &what)
	suite.Require().NoError(err)

	suite.Equal(uint64(2), taskID)
	suite.Equal(150.0, time)
	suite.Equal("cache_hit", what)

	// Ensure no more rows
	suite.False(rows.Next(), "Expected only one row")
}

func (suite *TracerTestSuite) TestCompleteMemoryTrace() {
	req := MockAccessReq{
		address:  0x3000,
		byteSize: 128,
		pid:      3,
	}

	suite.tracer.StartTask(tracing.TaskStart{
		ID:       3,
		Location: "memory_controller",
		What:     "read",
		Detail:   req,
		Time:     50,
	})

	suite.tracer.AddTaskTag(tracing.TaskTag{
		TaskID: 3,
		What:   "cache_miss",
		Time:   75,
	})

	suite.tracer.EndTask(tracing.TaskEnd{ID: 3, Time: 100})

	suite.dataRecorder.Flush()

	suite.verifyCompleteTransaction()
	suite.verifyCompleteTag()
}

func (suite *TracerTestSuite) verifyCompleteTransaction() {
	transactionQuery := "SELECT ID, Location, What, StartTime, EndTime, Address, ByteSize FROM memory_transactions"
	transactionRows, err := suite.db.Query(transactionQuery)
	suite.Require().NoError(err)
	defer transactionRows.Close()

	suite.Require().True(transactionRows.Next())
	var id uint64
	var location, what string
	var startTime, endTime float64
	var address, byteSize uint64

	err = transactionRows.Scan(
		&id,
		&location,
		&what,
		&startTime,
		&endTime,
		&address,
		&byteSize,
	)
	suite.Require().NoError(err)

	suite.Equal(uint64(3), id)
	suite.Equal("memory_controller", location)
	suite.Equal("read", what)
	suite.Equal(50.0, startTime)
	suite.Equal(100.0, endTime)
	suite.Equal(uint64(0x3000), address)
	suite.Equal(uint64(128), byteSize)
}

func (suite *TracerTestSuite) verifyCompleteTag() {
	tagRows, err := suite.db.Query(
		"SELECT ID, TaskID, Time, What FROM memory_tags",
	)
	suite.Require().NoError(err)
	defer tagRows.Close()

	suite.Require().True(tagRows.Next())
	var tagID, taskID uint64
	var tagWhat string
	var tagTime float64

	err = tagRows.Scan(&tagID, &taskID, &tagTime, &tagWhat)
	suite.Require().NoError(err)

	suite.Equal(uint64(3), taskID)
	suite.Equal(75.0, tagTime)
	suite.Equal("cache_miss", tagWhat)
}

func (suite *TracerTestSuite) TestTaskWithoutAccessReq() {
	suite.tracer.StartTask(tracing.TaskStart{
		ID:       4,
		Location: "test_location",
		What:     "non_memory",
		Detail:   "not_an_access_req",
		Time:     10,
	})

	suite.tracer.EndTask(tracing.TaskEnd{ID: 4, Time: 20})

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
	// AddMilestone should do nothing for the memory tracer.
	milestone := tracing.Milestone{
		ID:     10,
		TaskID: 11,
		Kind:   tracing.MilestoneKindHardwareResource,
		What:   "resource_acquired",
	}

	// This should not panic and should do nothing.
	suite.tracer.AddMilestone(milestone)

	suite.dataRecorder.Flush()
}

func TestTracerTestSuite(t *testing.T) {
	suite.Run(t, new(TracerTestSuite))
}

// TestDBTracerEndToEnd exercises the tracer through a full task lifecycle.
func TestDBTracerEndToEnd(t *testing.T) {
	// Create a temp DB file
	tmpFile, err := os.CreateTemp(t.TempDir(), "tracer-compat-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	recorder := datarecording.NewDataRecorder(tmpFile.Name())
	defer recorder.Close()
	tracer := NewDBTracer(recorder)

	req := MockAccessReq{
		address:  0x4000,
		byteSize: 256,
		pid:      4,
	}

	// These should not panic.
	tracer.StartTask(tracing.TaskStart{
		ID:       20,
		Location: "test_loc",
		What:     "test_what",
		Detail:   req,
		Time:     100,
	})

	tracer.AddTaskTag(tracing.TaskTag{TaskID: 20, What: "test_tag", Time: 150})

	tracer.EndTask(tracing.TaskEnd{ID: 20, Time: 200})

	// Add milestone (should do nothing).
	tracer.AddMilestone(tracing.Milestone{
		ID:     30,
		TaskID: 20,
		Kind:   tracing.MilestoneKindOther,
		What:   "test",
	})
}
