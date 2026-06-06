package checkpointresume

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/noc/directconnection"
	"github.com/sarchlab/akita/v5/simulation"
	"github.com/sarchlab/akita/v5/timing"
)

const numOps = 16

// addressForOp and valueForOp make the traffic fully deterministic: op i writes
// valueForOp(i) at addressForOp(i), then reads it back and checks it. No RNG, so
// a resumed run is bit-identical to the uninterrupted one.
func addressForOp(i int) uint64 { return uint64(i) * 64 }
func valueForOp(i int) uint32   { return uint32(i)*2654435761 + 12345 }

func uint32ToBytes(v uint32) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, v)
	return b
}

func bytesToUint32(b []byte) uint32 { return binary.LittleEndian.Uint32(b) }

// driverSpec / driverState are the immutable config and mutable runtime of the
// traffic driver. State is fully serialized; there is no hidden runtime field,
// so the checkpoint captures everything the driver needs to resume.
type driverSpec struct {
	Freq   timing.Freq `json:"freq"`
	NumOps int         `json:"num_ops"`
}

type driverState struct {
	WritesSent    int            `json:"writes_sent"`
	WritesAcked   int            `json:"writes_acked"`
	ReadsSent     int            `json:"reads_sent"`
	ReadsVerified int            `json:"reads_verified"`
	PendingWrite  map[uint64]int `json:"pending_write"` // req ID -> op index
	PendingRead   map[uint64]int `json:"pending_read"`  // req ID -> op index
	Mismatch      bool           `json:"mismatch"`
}

type driver struct {
	*modeling.Component[driverSpec, driverState, modeling.None]
	lowModule messaging.Port
}

func (d *driver) done() bool {
	return d.State.ReadsVerified == d.Spec().NumOps && !d.State.Mismatch
}

type driverMW struct {
	d *driver
}

func (m *driverMW) port() messaging.Port { return m.d.GetPortByName("Mem") }

func (m *driverMW) Tick() bool {
	progress := m.processResponse()
	progress = m.sendNext() || progress
	return progress
}

func (m *driverMW) processResponse() bool {
	msg := m.port().RetrieveIncoming()
	if msg == nil {
		return false
	}

	st := &m.d.State
	switch rsp := msg.(type) {
	case mem.WriteDoneRsp:
		if _, ok := st.PendingWrite[rsp.RspTo]; ok {
			delete(st.PendingWrite, rsp.RspTo)
			st.WritesAcked++
		}
	case mem.DataReadyRsp:
		if idx, ok := st.PendingRead[rsp.RspTo]; ok {
			delete(st.PendingRead, rsp.RspTo)
			if bytesToUint32(rsp.Data) != valueForOp(idx) {
				st.Mismatch = true
			}
			st.ReadsVerified++
		}
	}

	return true
}

func (m *driverMW) sendNext() bool {
	st := &m.d.State
	spec := m.d.Spec()
	port := m.port()

	// Phase 1: send every write.
	if st.WritesSent < spec.NumOps {
		if !port.CanSend() {
			return false
		}
		idx := st.WritesSent
		req := mem.WriteReq{}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = port.AsRemote()
		req.Dst = m.d.lowModule.AsRemote()
		req.Address = addressForOp(idx)
		req.PID = 1
		req.Data = uint32ToBytes(valueForOp(idx))
		req.TrafficBytes = len(req.Data) + 12
		req.TrafficClass = "mem.WriteReq"
		port.Send(req)
		st.PendingWrite[req.ID] = idx
		st.WritesSent++
		return true
	}

	// Drain all writes before reading, so every read observes its value.
	if st.WritesAcked < spec.NumOps {
		return false
	}

	// Phase 2: send every read.
	if st.ReadsSent < spec.NumOps {
		if !port.CanSend() {
			return false
		}
		idx := st.ReadsSent
		req := mem.ReadReq{}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = port.AsRemote()
		req.Dst = m.d.lowModule.AsRemote()
		req.Address = addressForOp(idx)
		req.AccessByteSize = 4
		req.PID = 1
		req.TrafficBytes = 12
		req.TrafficClass = "mem.ReadReq"
		port.Send(req)
		st.PendingRead[req.ID] = idx
		st.ReadsSent++
		return true
	}

	return false
}

func buildDriver(reg modeling.Registrar, lowModule messaging.Port) *driver {
	spec := driverSpec{Freq: 1 * timing.GHz, NumOps: numOps}
	modelComp := modeling.NewBuilder[driverSpec, driverState, modeling.None]().
		WithEngine(reg.GetEngine()).
		WithFreq(spec.Freq).
		WithSpec(spec).
		Build("Driver")
	modelComp.State = driverState{
		PendingWrite: make(map[uint64]int),
		PendingRead:  make(map[uint64]int),
	}

	d := &driver{Component: modelComp, lowModule: lowModule}
	modelComp.AddMiddleware(&driverMW{d: d})
	modelComp.AddPort("Mem", messaging.NewPort(d, 4, 4, "Driver.Mem"))
	reg.RegisterComponent(d)

	return d
}

// buildSim assembles an identical simulation each time: a deterministic driver
// and an ideal memory controller wired over a direct connection. The connection
// is registered so its round-robin cursor is checkpointed too.
func buildSim() (*simulation.Simulation, *driver) {
	sim := simulation.MakeBuilder().WithoutMonitoring().Build()

	dramSpec := idealmemcontroller.DefaultSpec()
	dramSpec.Capacity = 1 * mem.MB
	dramSpec.Width = 4
	dramSpec.Latency = 10
	dramSpec.TopPortBufferSize = 8
	dram := idealmemcontroller.MakeBuilder().
		WithRegistrar(sim).
		WithSpec(dramSpec).
		Build("DRAM")

	d := buildDriver(sim, dram.GetPortByName("Top"))

	conn := directconnection.MakeBuilder().WithRegistrar(sim).Build("Conn")
	conn.PlugIn(d.GetPortByName("Mem"))
	conn.PlugIn(dram.GetPortByName("Top"))

	return sim, d
}

func cleanup(sim *simulation.Simulation) {
	sim.Terminate()
	os.Remove("akita_sim_" + sim.ID() + ".sqlite3")
}

// runReference runs a full uninterrupted simulation and returns the oracle: the
// final reads-verified count and end time that every resumed run must match.
func runReference(t *testing.T) (wantVerified int, wantTime timing.VTimeInPicoSec) {
	t.Helper()

	refSim, refD := buildSim()
	defer cleanup(refSim)

	refEngine := refSim.GetEngine().(*timing.SerialEngine)
	refD.TickLater()
	if err := refEngine.Run(); err != nil {
		t.Fatalf("reference run: %v", err)
	}
	if !refD.done() {
		t.Fatalf("reference run did not finish: %+v", refD.State)
	}

	return refD.State.ReadsVerified, refEngine.CurrentTime()
}

// resumeAndVerify loads a checkpoint into a fresh simulation, runs to
// completion, and asserts it matches the uninterrupted reference exactly.
func resumeAndVerify(
	t *testing.T,
	path, buildID string,
	wantVerified int,
	wantTime timing.VTimeInPicoSec,
) {
	t.Helper()

	resSim, resD := buildSim()
	defer cleanup(resSim)

	resEngine := resSim.GetEngine().(*timing.SerialEngine)
	if err := resSim.LoadCheckpoint(path, buildID); err != nil {
		t.Fatalf("LoadCheckpoint: %v", err)
	}
	if err := resEngine.Run(); err != nil {
		t.Fatalf("resumed run: %v", err)
	}

	if !resD.done() {
		t.Fatalf("resumed run did not finish: %+v", resD.State)
	}
	if resD.State.Mismatch {
		t.Fatalf("resumed run read stale/incorrect data")
	}
	if resD.State.ReadsVerified != wantVerified {
		t.Fatalf("resumed verified %d, want %d", resD.State.ReadsVerified, wantVerified)
	}
	if resEngine.CurrentTime() != wantTime {
		t.Fatalf("resumed end time %d, want %d", resEngine.CurrentTime(), wantTime)
	}
}

// checkpointAtMidTransaction advances a fresh source sim to a genuinely
// mid-transaction boundary (requests in flight, found via RunUntil) and writes
// a checkpoint there.
func checkpointAtMidTransaction(t *testing.T, path, buildID string, wantTime timing.VTimeInPicoSec) {
	t.Helper()

	srcSim, srcD := buildSim()
	srcEngine := srcSim.GetEngine().(*timing.SerialEngine)
	srcD.TickLater()

	step := wantTime / 8
	if step == 0 {
		step = 1
	}
	for boundary := step; boundary < wantTime; boundary += step {
		if err := srcEngine.RunUntil(boundary); err != nil {
			t.Fatalf("RunUntil: %v", err)
		}
		if len(srcD.State.PendingWrite)+len(srcD.State.PendingRead) > 0 {
			break
		}
	}

	inFlight := len(srcD.State.PendingWrite) + len(srcD.State.PendingRead)
	if inFlight == 0 || srcD.done() {
		t.Fatalf("never reached a mid-transaction boundary: %+v", srcD.State)
	}
	t.Logf("checkpoint at t=%d: %d requests in flight, writesAcked=%d",
		srcEngine.CurrentTime(), inFlight, srcD.State.WritesAcked)

	if err := srcSim.SaveCheckpoint(path, buildID); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}
	cleanup(srcSim)
}

func TestMidTransactionResumeOracle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ck.tar.gz")
	const buildID = "resume-oracle"

	wantVerified, wantTime := runReference(t)
	checkpointAtMidTransaction(t, path, buildID, wantTime)
	resumeAndVerify(t, path, buildID, wantVerified, wantTime)
}

// TestResumeOracleDeterministicAcrossBoundaries checkpoints the same simulation
// at many boundaries spanning both the write and read phases, and confirms every
// one resumes to the identical final state and end time as the uninterrupted
// reference. A boundary that lands in a quiescent gap is as valid as one with
// traffic in flight; the point is that no boundary diverges.
func TestResumeOracleDeterministicAcrossBoundaries(t *testing.T) {
	wantVerified, wantTime := runReference(t)

	const slices = 8
	for i := 1; i < slices; i++ {
		boundary := wantTime * timing.VTimeInPicoSec(i) / slices
		t.Run(fmt.Sprintf("boundary_%d_of_%d", i, slices), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "ck.tar.gz")
			const buildID = "multi-boundary"

			srcSim, srcD := buildSim()
			srcEngine := srcSim.GetEngine().(*timing.SerialEngine)
			srcD.TickLater()
			if err := srcEngine.RunUntil(boundary); err != nil {
				t.Fatalf("RunUntil(%d): %v", boundary, err)
			}
			if err := srcSim.SaveCheckpoint(path, buildID); err != nil {
				t.Fatalf("SaveCheckpoint: %v", err)
			}
			cleanup(srcSim)

			resumeAndVerify(t, path, buildID, wantVerified, wantTime)
		})
	}
}
