package datamover

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
)

//go:generate mockgen -destination "mock_sim_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v5/sim Port

func TestDataMover(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "DataMover Suite")
}

func TestValidateState(t *testing.T) {
	if err := modeling.ValidateState(State{}); err != nil {
		t.Fatalf("State failed validation: %v", err)
	}
}

func TestSnapshotStateEmpty(t *testing.T) {
	// A Comp with no active transaction should snapshot cleanly.
	engine := sim.NewSerialEngine()
	insidePort := sim.NewPort(nil, 16, 16, "test.Inside")
	outsidePort := sim.NewPort(nil, 16, 16, "test.Outside")
	ctrlPort := sim.NewPort(nil, 16, 16, "test.Ctrl")

	comp := MakeBuilder().
		WithEngine(engine).
		WithBufferSize(2048).
		WithInsidePortMapper(&mem.SinglePortMapper{Port: "dummy.inside"}).
		WithOutsidePortMapper(&mem.SinglePortMapper{Port: "dummy.outside"}).
		WithInsideByteGranularity(64).
		WithOutsideByteGranularity(256).
		WithCtrlPort(ctrlPort).
		WithInsidePort(insidePort).
		WithOutsidePort(outsidePort).
		Build("TestDM")

	s := comp.snapshotState()
	if s.CurrentTransaction.Active {
		t.Fatal("expected no active transaction")
	}

	// Restore should not panic on an empty state.
	comp.restoreFromState(s)
}

func buildTestComp(engine sim.Engine, name string) *Comp {
	return MakeBuilder().
		WithEngine(engine).
		WithBufferSize(2048).
		WithInsidePortMapper(&mem.SinglePortMapper{Port: "dummy.inside"}).
		WithOutsidePortMapper(&mem.SinglePortMapper{Port: "dummy.outside"}).
		WithInsideByteGranularity(64).
		WithOutsideByteGranularity(256).
		WithCtrlPort(sim.NewPort(nil, 16, 16, name+".Ctrl")).
		WithInsidePort(sim.NewPort(nil, 16, 16, name+".Inside")).
		WithOutsidePort(sim.NewPort(nil, 16, 16, name+".Outside")).
		Build(name)
}

func setupTransactionState(comp *Comp) {
	comp.srcPort = comp.insidePort
	comp.dstPort = comp.outsidePort
	comp.srcPortMapper = comp.insidePortMapper
	comp.dstPortMapper = comp.outsidePortMapper
	comp.srcByteGranularity = 64
	comp.dstByteGranularity = 256

	payload := &DataMoveRequestPayload{
		SrcAddress: 0, DstAddress: 4096,
		ByteSize: 4096, SrcSide: "inside", DstSide: "outside",
	}
	req := &sim.Msg{
		MsgMeta: sim.MsgMeta{ID: "test-req-1", Src: "src-port", Dst: "dst-port"},
		Payload: payload,
	}
	comp.currentTransaction = &dataMoverTransaction{
		req: req, reqPayload: payload,
		nextReadAddr: 128, nextWriteAddr: 4352,
		pendingRead: make(map[string]*sim.Msg),
		pendingWrite: make(map[string]*sim.Msg),
	}

	readReq := &sim.Msg{
		MsgMeta: sim.MsgMeta{ID: "read-1", Src: "dm.inside", Dst: "mem.top"},
		Payload: &mem.ReadReqPayload{Address: 64, AccessByteSize: 64},
	}
	comp.currentTransaction.pendingRead["read-1"] = readReq

	comp.buffer = &buffer{
		offset: 0, granularity: 64,
		data: [][]byte{{1, 2, 3}, nil, {4, 5, 6}},
	}
}

func verifySnapshot(t *testing.T, s State) {
	t.Helper()

	if !s.CurrentTransaction.Active {
		t.Fatal("expected active transaction")
	}
	if s.CurrentTransaction.ReqID != "test-req-1" {
		t.Fatalf("wrong req id: %s", s.CurrentTransaction.ReqID)
	}
	if s.SrcSide != "inside" || s.DstSide != "outside" {
		t.Fatal("wrong sides")
	}
	if len(s.Buffer.Chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(s.Buffer.Chunks))
	}
	if s.Buffer.Chunks[1].Valid {
		t.Fatal("expected chunk 1 to be invalid (nil)")
	}
}

func verifyRestore(t *testing.T, comp *Comp) {
	t.Helper()

	if comp.currentTransaction == nil {
		t.Fatal("expected transaction after restore")
	}
	if comp.currentTransaction.nextReadAddr != 128 {
		t.Fatalf("wrong nextReadAddr: %d", comp.currentTransaction.nextReadAddr)
	}
	if comp.srcPort != comp.insidePort {
		t.Fatal("srcPort not restored to insidePort")
	}
	if comp.dstPort != comp.outsidePort {
		t.Fatal("dstPort not restored to outsidePort")
	}
	if len(comp.buffer.data) != 3 {
		t.Fatalf("wrong buffer data length: %d", len(comp.buffer.data))
	}
	if comp.buffer.data[1] != nil {
		t.Fatal("expected nil slot at index 1")
	}
}

func TestSnapshotStateWithTransaction(t *testing.T) {
	engine := sim.NewSerialEngine()
	comp := buildTestComp(engine, "TestDM")
	setupTransactionState(comp)

	s := comp.snapshotState()
	verifySnapshot(t, s)

	comp2 := buildTestComp(engine, "TestDM2")
	comp2.restoreFromState(s)
	verifyRestore(t, comp2)
}
