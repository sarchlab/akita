package virtualmemcheckpoint

import (
	"encoding/binary"
	"os"
	"strconv"

	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling/ticking"
	"github.com/sarchlab/akita/v5/timing"
)

// numOps is the number of distinct virtual addresses written then read back. It
// defaults small for fast unit runs; the acceptance suite sets
// CHECKPOINT_NUM_OPS to a larger value to stress the hierarchy harder across the
// checkpoint — more in-flight state and TLB eviction pressure (the LRU whose
// serialization this test guards).
var numOps = numOpsFromEnv()

func numOpsFromEnv() int {
	if v := os.Getenv("CHECKPOINT_NUM_OPS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 48
}

// pid is the process the driver issues traffic for; it must match the page
// table entries the test installs.
const pid = 1

// addressForOp / valueForOp make the traffic fully deterministic. Addresses are
// spread 512 bytes apart so they span several cache lines and several pages
// (exercising the TLB and both caches); every address stays within the mapped
// virtual range.
func addressForOp(i int) uint64 { return uint64(i) * 512 }
func valueForOp(i int) uint32   { return uint32(i)*2654435761 + 12345 }

func uint32ToBytes(v uint32) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, v)
	return b
}

func bytesToUint32(b []byte) uint32 { return binary.LittleEndian.Uint32(b) }

// driverSpec / driverState are the immutable config and mutable runtime of the
// traffic driver. State is fully serialized — there is no hidden runtime field
// (unlike MemAccessAgent's RNG), so the checkpoint captures everything the
// driver needs to resume.
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

// driverResources holds the driver's external wiring.
type driverResources struct {
	// LowModule is the memory port the driver sends requests to.
	LowModule messaging.Port
}

// driverPorts holds the driver's ports.
type driverPorts struct {
	// Mem sends the writes and reads and receives their responses.
	Mem messaging.Port `akita:"role=mem/requester"`
}

// driverMiddlewares holds the driver's behavior.
type driverMiddlewares struct {
	// Drive checks responses and issues the next request.
	Drive *driverMW
}

type driver = ticking.Component[
	driverSpec, driverState, driverResources, driverPorts, driverMiddlewares]

// Definition declares the traffic driver, a ticking component. Build sets
// Spec.NumOps from numOps.
var Definition = ticking.Definition[
	driverSpec, driverState, driverResources, driverPorts, driverMiddlewares]{
	DefaultSpec: driverSpec{
		Freq: 1 * timing.GHz,
	},
	NewState:       newDriverState,
	NewMiddlewares: newDriverMiddlewares,
}

func newDriverState(_ *driver) driverState {
	return driverState{
		PendingWrite: make(map[uint64]int),
		PendingRead:  make(map[uint64]int),
	}
}

func newDriverMiddlewares(d *driver) driverMiddlewares {
	return driverMiddlewares{Drive: &driverMW{d: d}}
}

func done(d *driver) bool {
	return d.State.ReadsVerified == d.Spec().NumOps && !d.State.Mismatch
}

func inFlight(d *driver) int {
	return len(d.State.PendingWrite) + len(d.State.PendingRead)
}

type driverMW struct {
	d *driver
}

func (m *driverMW) port() messaging.Port { return m.d.Ports.Mem }

func (m *driverMW) Handle(_ timing.Event) bool {
	progress := m.processResponse()
	progress = m.sendNext() || progress
	return progress
}

func (m *driverMW) processResponse() bool {
	msg, ok := m.port().RetrieveIncoming()
	if !ok {
		return false
	}

	st := &m.d.State
	switch rsp := msg.(type) {
	case memprotocol.WriteDoneRsp:
		if _, ok := st.PendingWrite[rsp.RspTo]; ok {
			delete(st.PendingWrite, rsp.RspTo)
			st.WritesAcked++
		}
	case memprotocol.DataReadyRsp:
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
		req := memprotocol.WriteReq{}
		req.ID = m.d.NewID()
		req.Src = port.AsRemote()
		req.Dst = m.d.Resources().LowModule.AsRemote()
		req.Address = addressForOp(idx)
		req.PID = pid
		req.Data = uint32ToBytes(valueForOp(idx))
		req.TrafficBytes = len(req.Data) + 12
		req.TrafficClass = "memprotocol.WriteReq"
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
		req := memprotocol.ReadReq{}
		req.ID = m.d.NewID()
		req.Src = port.AsRemote()
		req.Dst = m.d.Resources().LowModule.AsRemote()
		req.Address = addressForOp(idx)
		req.AccessByteSize = 4
		req.PID = pid
		req.TrafficBytes = 12
		req.TrafficClass = "memprotocol.ReadReq"
		port.Send(req)
		st.PendingRead[req.ID] = idx
		st.ReadsSent++
		return true
	}

	return false
}

func buildDriver(sim timing.Simulation, lowModule messaging.Port) *driver {
	spec := Definition.DefaultSpec
	spec.NumOps = numOps

	return Definition.Builder().
		WithSimulation(sim).
		WithSpec(spec).
		WithResources(driverResources{LowModule: lowModule}).
		WithPorts(driverPorts{
			Mem: messaging.NewPort(nil, 4, 4, "Driver.Mem"),
		}).
		Build("Driver")
}
