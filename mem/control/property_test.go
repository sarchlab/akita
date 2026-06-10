package control_test

import (
	"bytes"
	"math/rand"
	"testing"

	"github.com/sarchlab/akita/v5/mem/control"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
)

// This file is the Layer-4 property/fuzz test. It drives the write-back
// cache over a real backing memory (the cacheOverDRAM harness from
// checkpoint_test.go) through long, deterministic-but-pseudo-random
// interleavings of a read/write workload and control verbs, and checks
// invariants that no single hand-written case enumerates:
//
//   - data consistency: every read returns the value a reference model
//     predicts, across Pause/Drain/Enable/Flush/Invalidate/Reset cycles;
//   - liveness: no in-flight request is lost — after a final Enable every
//     outstanding request completes;
//   - protocol safety: every control ack matches a command we sent, with
//     the Success/Error the verb+state contract requires;
//   - quiescence: a Drain ack means the cache is actually quiescent.
//
// Pause/Enable may interleave with in-flight work (this is what would
// catch an Enable that discards frozen traffic, or a Drain that never
// quiesces). The data-mutating verbs (Flush/Invalidate/Reset) are only
// issued from a fully-drained paused state, which keeps the reference
// model sound: Flush makes the backing store match the model; Invalidate
// and Reset drop the cache, so reads then come from the backing store.

type fuzzer struct {
	t       *testing.T
	h       *cacheOverDRAM
	rng     *rand.Rand
	addrs   []uint64
	model   map[uint64][]byte // expected value a read should return
	flushed map[uint64][]byte // value currently in the backing memory
	pending map[uint64]fuzzReq
	busy    map[uint64]bool            // address has an outstanding request
	ctrlIDs map[uint64]control.Command // control reqs issued, awaiting ack
	paused  bool
}

type fuzzReq struct {
	addr    uint64
	isWrite bool
	data    []byte
}

func newFuzzer(t *testing.T, seed int64) *fuzzer {
	t.Helper()

	f := &fuzzer{
		t:       t,
		h:       buildCacheOverDRAM(t),
		rng:     rand.New(rand.NewSource(seed)),
		model:   map[uint64][]byte{},
		flushed: map[uint64][]byte{},
		pending: map[uint64]fuzzReq{},
		busy:    map[uint64]bool{},
		ctrlIDs: map[uint64]control.Command{},
	}
	for i := range 6 {
		addr := uint64(i) * cpBlockSize
		f.addrs = append(f.addrs, addr)
		f.model[addr] = []byte{0, 0, 0, 0}   // DRAM starts zeroed
		f.flushed[addr] = []byte{0, 0, 0, 0} // ... so the backing store does too
	}
	return f
}

// collect drains all responses/acks produced by the most recent tick and
// checks the invariants they carry.
func (f *fuzzer) collect() {
	for {
		out := f.h.top.RetrieveOutgoing()
		if out == nil {
			break
		}
		f.handleWorkloadRsp(out)
	}
	for {
		out := f.h.ctrl.RetrieveOutgoing()
		if out == nil {
			break
		}
		f.handleControlRsp(out)
	}
}

func (f *fuzzer) handleWorkloadRsp(out messaging.Msg) {
	rspTo := out.Meta().RspTo
	req, ok := f.pending[rspTo]
	if !ok {
		f.t.Fatalf("response %T for unknown request id %d", out, rspTo)
	}

	switch r := out.(type) {
	case memprotocol.DataReadyRsp:
		if !bytes.Equal(r.Data, f.model[req.addr]) {
			f.t.Fatalf("read %#x = %v, want %v",
				req.addr, r.Data, f.model[req.addr])
		}
	case memprotocol.WriteDoneRsp:
		// model[addr] was set to the written value at issue time.
	default:
		f.t.Fatalf("unexpected workload response %T", out)
	}

	delete(f.pending, rspTo)
	f.busy[req.addr] = false
}

func (f *fuzzer) handleControlRsp(out messaging.Msg) {
	rsp, ok := out.(control.Rsp)
	if !ok {
		f.t.Fatalf("non-ControlRsp %T on control port", out)
	}
	if _, ok := f.ctrlIDs[rsp.RspTo]; !ok {
		f.t.Fatalf("control ack for unknown request id %d", rsp.RspTo)
	}
	delete(f.ctrlIDs, rsp.RspTo)
	if !rsp.Success {
		f.t.Fatalf("control %v failed unexpectedly: %q", rsp.Command, rsp.Error)
	}
	if rsp.Command == control.CmdDrain && !f.cacheQuiescent() {
		f.t.Fatalf("Drain acked but cache is not quiescent")
	}

	// Track the lifecycle state from acks (not optimistically): Drain is
	// async, so the cache is only paused once its ack arrives.
	switch rsp.Command {
	case control.CmdPause, control.CmdDrain:
		f.paused = true
	case control.CmdEnable, control.CmdReset:
		f.paused = false
	}
}

// cacheQuiescent mirrors the cache's own quiescence definition using only
// exported state.
func (f *fuzzer) cacheQuiescent() bool {
	st := &f.h.cache.State
	for i := range st.Transactions {
		if !st.Transactions[i].Removed {
			return false
		}
	}
	if st.WriteBufferBuf.Size() > 0 {
		return false
	}
	for _, c := range st.BankInflightTransCounts {
		if c > 0 {
			return false
		}
	}
	for _, c := range st.BankDownwardInflightTransCounts {
		if c > 0 {
			return false
		}
	}
	return true
}

func (f *fuzzer) freeAddr() (uint64, bool) {
	order := f.rng.Perm(len(f.addrs))
	for _, i := range order {
		if !f.busy[f.addrs[i]] {
			return f.addrs[i], true
		}
	}
	return 0, false
}

func (f *fuzzer) issueWrite() {
	addr, ok := f.freeAddr()
	if !ok {
		return
	}
	data := []byte{
		byte(f.rng.Intn(256)), byte(f.rng.Intn(256)),
		byte(f.rng.Intn(256)), byte(f.rng.Intn(256)),
	}
	req := memprotocol.WriteReq{Address: addr, Data: data}
	req.ID = timing.GetIDGenerator().Generate()
	req.Src = f.h.agent
	req.Dst = f.h.top.AsRemote()
	req.TrafficClass = "memprotocol.WriteReq"
	f.h.top.Deliver(req)

	f.busy[addr] = true
	f.pending[req.ID] = fuzzReq{addr: addr, isWrite: true, data: data}
	f.model[addr] = data // the value this write will commit
}

func (f *fuzzer) issueRead() {
	addr, ok := f.freeAddr()
	if !ok {
		return
	}
	req := memprotocol.ReadReq{Address: addr, AccessByteSize: 4}
	req.ID = timing.GetIDGenerator().Generate()
	req.Src = f.h.agent
	req.Dst = f.h.top.AsRemote()
	req.TrafficClass = "memprotocol.ReadReq"
	f.h.top.Deliver(req)

	f.busy[addr] = true
	f.pending[req.ID] = fuzzReq{addr: addr}
}

func (f *fuzzer) issueControl(cmd control.Command) {
	req := control.Req{Command: cmd}
	req.ID = timing.GetIDGenerator().Generate()
	req.Src = f.h.agent
	req.Dst = f.h.ctrl.AsRemote()
	req.TrafficClass = "control.Req"
	f.h.ctrl.Deliver(req)
	f.ctrlIDs[req.ID] = cmd
}

// pickControl issues at most one control verb that is legal for the
// current ack-tracked state. Only one control verb is ever in flight at a
// time, and the data-mutating verbs (Flush/Invalidate/Reset) are issued
// only from a fully-drained paused state, which keeps the reference model
// sound. State transitions are applied when acks arrive, not here.
func (f *fuzzer) pickControl() {
	if len(f.ctrlIDs) != 0 {
		return // a control verb is already in flight
	}

	if !f.paused {
		if f.rng.Intn(2) == 0 {
			f.issueControl(control.CmdPause)
		} else {
			f.issueControl(control.CmdDrain)
		}
		return
	}

	// Paused. If workload is still outstanding, resume to let it drain
	// rather than mutating cache contents underneath it.
	if len(f.pending) != 0 {
		f.issueControl(control.CmdEnable)
		return
	}

	switch f.rng.Intn(3) {
	case 0:
		// Flush persists every dirty block: the backing store now matches
		// the model.
		f.issueControl(control.CmdFlush)
		for a := range f.model {
			f.flushed[a] = append([]byte(nil), f.model[a]...)
		}
	case 1:
		// Invalidate/Reset drop the cache without writeback: reads now come
		// from the backing store, so the model reverts to the flushed view.
		cmd := control.CmdInvalidate
		if f.rng.Intn(2) == 0 {
			cmd = control.CmdReset
		}
		f.issueControl(cmd)
		for a := range f.model {
			f.model[a] = append([]byte(nil), f.flushed[a]...)
		}
	default:
		f.issueControl(control.CmdEnable)
	}
}

// tickUntil ticks (collecting responses) until done() holds or a generous
// budget is exhausted.
func (f *fuzzer) tickUntil(done func() bool) {
	for range 200000 {
		if done() {
			return
		}
		f.h.tick()
		f.collect()
	}
}

func (f *fuzzer) run(iterations int) {
	for range iterations {
		// A disciplined caller does not inject new traffic while a control
		// verb is in flight (notably: feeding a Drain new work would keep it
		// from ever quiescing). Pause/Drain may still land on work that is
		// already in flight, which is the interesting interleaving.
		if len(f.ctrlIDs) == 0 {
			switch f.rng.Intn(4) {
			case 0:
				f.issueWrite()
			case 1:
				f.issueRead()
			case 2:
				f.pickControl()
			}
		}
		// Always make some progress and observe results.
		for range 1 + f.rng.Intn(8) {
			f.h.tick()
			f.collect()
		}
	}

	// Settle any in-flight control verb first. Control commands are
	// serialized, so a verb issued while a Drain is still in flight just
	// queues behind it; settling first keeps the wind-down bookkeeping simple.
	f.tickUntil(func() bool { return len(f.ctrlIDs) == 0 })

	// Resume so queued workload can finish, then confirm nothing was lost
	// or stuck.
	if f.paused {
		f.issueControl(control.CmdEnable)
	}
	f.tickUntil(func() bool {
		return len(f.pending) == 0 && len(f.ctrlIDs) == 0
	})
	if len(f.pending) != 0 {
		f.t.Fatalf("%d workload requests never completed", len(f.pending))
	}
	if len(f.ctrlIDs) != 0 {
		stuck := make([]control.Command, 0, len(f.ctrlIDs))
		for _, cmd := range f.ctrlIDs {
			stuck = append(stuck, cmd)
		}
		f.t.Fatalf("%d control requests never acked: %v", len(f.ctrlIDs), stuck)
	}

	// Final consistency sweep: every address reads back its model value.
	for _, addr := range f.addrs {
		got := f.h.read(f.t, addr, 4)
		if !bytes.Equal(got, f.model[addr]) {
			f.t.Fatalf("final read %#x = %v, want %v", addr, got, f.model[addr])
		}
	}
}

func TestProperty_ControlVerbsKeepCacheConsistent(t *testing.T) {
	for _, seed := range []int64{1, 2, 3, 4, 5} {
		seed := seed
		t.Run(seedName(seed), func(t *testing.T) {
			newFuzzer(t, seed).run(400)
		})
	}
}

func seedName(seed int64) string {
	return "seed-" + string(rune('0'+seed))
}
