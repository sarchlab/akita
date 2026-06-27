package main

import (
	"flag"
	"log"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/acceptancetests/memaccessagent"
	"github.com/sarchlab/akita/v5/mem/datamover"
	"github.com/sarchlab/akita/v5/mem/datamoverprotocol"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/noc/directconnection"
	"github.com/sarchlab/akita/v5/simulation"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
)

var migDebugFlag = flag.Bool(
	"migrate-debug", false, "Log migration controller phase transitions")

// migPhase is the state of the migration finite state machine.
//
// Quiescence is reached in dependency order to avoid a drain deadlock: the
// agents' reorder buffers (the only request sources) are drained first, which
// lets all downstream in-flight work flow to completion through the still-enabled
// translation and data path. Only then are the now-idle downstream components
// paused. Draining everything at once would deadlock, because a draining
// component refuses the new downstream requests that another component's
// in-flight work still needs to issue.
type migPhase int

const (
	// migIdle counts down to the next migration.
	migIdle migPhase = iota
	// migDrainROB drains the reorder buffers so no new requests enter the data
	// path and all in-flight work completes; the agents then stall on
	// backpressure.
	migDrainROB
	// migPauseRest pauses the now-idle translation and data path so it can be
	// flushed and invalidated.
	migPauseRest
	// migFlushing writes the write-back L2's dirty lines to memory so the copy
	// reads authoritative data.
	migFlushing
	// migCopying drives the data mover to copy the page to the destination
	// device, then repoints the page table.
	migCopying
	// migInvalidating drops now-stale cache lines and TLB translations.
	migInvalidating
	// migEnabling resumes every paused/drained component.
	migEnabling
)

// migSpec is the immutable configuration of the migration controller.
type migSpec struct {
	Freq         timing.Freq `json:"freq"`
	Interval     uint64      `json:"interval"`
	NumPages     uint64      `json:"num_pages"`
	DeviceStride uint64      `json:"device_stride"`
}

// migState is the mutable runtime data of the migration controller.
type migState struct {
	Phase         migPhase `json:"phase"`
	Countdown     uint64   `json:"countdown"`
	PageCursor    uint64   `json:"page_cursor"`
	CurPage       uint64   `json:"cur_page"`
	SrcAddr       uint64   `json:"src_addr"`
	DstAddr       uint64   `json:"dst_addr"`
	DstDevice     uint64   `json:"dst_device"`
	SendCursor    int      `json:"send_cursor"`
	PendingAcks   int      `json:"pending_acks"`
	MoveSent      bool     `json:"move_sent"`
	NumMigrations uint64   `json:"num_migrations"`

	// MigTaskID and PhaseTaskID are the tracing task IDs for the current
	// migration and its current phase, so the control sequence shows up as
	// labeled, nested tasks in Daisen.
	MigTaskID   uint64 `json:"mig_task_id"`
	PhaseTaskID uint64 `json:"phase_task_id"`
}

// phaseName is the human-readable label used for a phase's trace task.
func phaseName(phase migPhase) string {
	switch phase {
	case migDrainROB:
		return "drain_rob"
	case migPauseRest:
		return "pause_rest"
	case migFlushing:
		return "flush"
	case migCopying:
		return "copy"
	case migInvalidating:
		return "invalidate"
	case migEnabling:
		return "enable"
	default:
		return "idle"
	}
}

// migrationController periodically relocates a page from one device to another
// while keeping the data transparent to the agents. It owns no memory; it
// orchestrates the existing control protocol, page table, and data mover.
type migrationController struct {
	*modeling.Component[migSpec, migState, modeling.None]

	// pages is the shared page table; the controller mutates it directly.
	pages vm.PageTable
	// agents is used only to detect when the workload is finished so the
	// controller can stop ticking and let the simulation terminate.
	agents []*memaccessagent.MemAccessAgent
	// moverDst is the data mover's Top port.
	moverDst messaging.RemotePort

	// robTargets are the request sources, drained first.
	robTargets []messaging.RemotePort
	// restTargets are the translation and data path components, paused once the
	// reorder buffers are drained and everything downstream is idle.
	restTargets []messaging.RemotePort
	// allTargets is robTargets+restTargets, re-enabled together at the end.
	allTargets []messaging.RemotePort
	// flushTargets are the write-back caches whose dirty data must reach memory
	// before the copy.
	flushTargets []messaging.RemotePort
	// invalTargets are the caches and TLBs whose stale entries are dropped after
	// the page table is repointed.
	invalTargets []messaging.RemotePort
}

// migMW is the migration controller's behavior.
type migMW struct {
	ctrl *migrationController
}

func (m *migMW) ctrlPort() messaging.Port {
	return m.ctrl.GetPortByName("Ctrl")
}

func (m *migMW) moverPort() messaging.Port {
	return m.ctrl.GetPortByName("Mover")
}

// Tick advances the migration finite state machine.
func (m *migMW) Tick() bool {
	state := &m.ctrl.State
	progress := false

	progress = m.processAcks() || progress
	progress = m.processMoveRsp() || progress

	switch state.Phase {
	case migIdle:
		progress = m.tickIdle() || progress
	case migDrainROB:
		progress = m.runControlPhase(
			m.ctrl.robTargets, memcontrolprotocol.CmdDrain,
			func() { m.enterControlPhase(migPauseRest, len(m.ctrl.restTargets)) },
		) || progress
	case migPauseRest:
		progress = m.runControlPhase(
			m.ctrl.restTargets, memcontrolprotocol.CmdPause,
			func() { m.enterControlPhase(migFlushing, len(m.ctrl.flushTargets)) },
		) || progress
	case migFlushing:
		progress = m.runControlPhase(
			m.ctrl.flushTargets, memcontrolprotocol.CmdFlush,
			func() { m.enterCopyPhase() },
		) || progress
	case migCopying:
		progress = m.tickCopying() || progress
	case migInvalidating:
		progress = m.runControlPhase(
			m.ctrl.invalTargets, memcontrolprotocol.CmdInvalidate,
			func() { m.enterControlPhase(migEnabling, len(m.ctrl.allTargets)) },
		) || progress
	case migEnabling:
		progress = m.runControlPhase(
			m.ctrl.allTargets, memcontrolprotocol.CmdEnable,
			func() { m.finishMigration() },
		) || progress
	}

	return progress
}

// tickIdle counts down to the next migration. It returns false (stops ticking,
// letting the simulation terminate) once every agent has finished, since there
// is no reason to keep migrating.
func (m *migMW) tickIdle() bool {
	state := &m.ctrl.State

	if m.allAgentsDone() {
		return false
	}

	if state.Countdown > 0 {
		state.Countdown--
		return true
	}

	m.beginMigration()

	return true
}

func (m *migMW) allAgentsDone() bool {
	for _, a := range m.ctrl.agents {
		if a.State.ReadLeft > 0 || a.State.WriteLeft > 0 {
			return false
		}
		if len(a.State.PendingReadReq) > 0 || len(a.State.PendingWriteReq) > 0 {
			return false
		}
	}

	return true
}

// beginMigration picks the next page round-robin, flips its target device, and
// enters the drain phase.
func (m *migMW) beginMigration() {
	state := &m.ctrl.State
	spec := m.ctrl.Spec()

	page := state.PageCursor % spec.NumPages
	state.PageCursor = (state.PageCursor + 1) % spec.NumPages

	vAddr := page * pageSize
	pageEntry, found := m.ctrl.pages.Find(1, vAddr)
	if !found {
		log.Panicf("migration: page %d (vAddr 0x%x) not found", page, vAddr)
	}

	srcDevice := pageEntry.DeviceID
	dstDevice := (srcDevice + 1) % numDevices

	state.CurPage = page
	state.SrcAddr = pageEntry.PAddr
	state.DstAddr = pagePAddr(dstDevice, page, spec.DeviceStride)
	state.DstDevice = dstDevice

	pageEntry.IsMigrating = true
	m.ctrl.pages.Update(pageEntry)

	if *migDebugFlag {
		log.Printf("%d migration #%d: page %d dev %d->%d (0x%x->0x%x)",
			m.ctrl.CurrentTime(), state.NumMigrations, page,
			srcDevice, dstDevice, state.SrcAddr, state.DstAddr)
	}

	// Open a parent task spanning the whole migration so the control phases nest
	// under it in the trace.
	state.MigTaskID = timing.GetIDGenerator().Generate()
	tracing.StartTask(m.ctrl, tracing.TaskStart{
		ID:       state.MigTaskID,
		Kind:     "migration",
		What:     "migration",
		Location: m.ctrl.Name(),
	})

	m.enterControlPhase(migDrainROB, len(m.ctrl.robTargets))
}

// startPhaseTask ends the previous phase's trace task (if any) and opens one for
// the phase the controller is entering. Each phase kind gets its own location so
// it occupies its own, non-overlapping Daisen row.
func (m *migMW) startPhaseTask(phase migPhase) {
	state := &m.ctrl.State

	if state.PhaseTaskID != 0 {
		tracing.EndTask(m.ctrl, tracing.TaskEnd{ID: state.PhaseTaskID})
	}

	name := phaseName(phase)
	state.PhaseTaskID = timing.GetIDGenerator().Generate()
	tracing.StartTask(m.ctrl, tracing.TaskStart{
		ID:       state.PhaseTaskID,
		ParentID: state.MigTaskID,
		Kind:     "migration_step",
		What:     name,
		Location: m.ctrl.Name() + "." + name,
	})
}

// enterControlPhase resets the send/ack bookkeeping for a control phase that
// must reach n targets.
func (m *migMW) enterControlPhase(phase migPhase, n int) {
	state := &m.ctrl.State
	state.Phase = phase
	state.SendCursor = 0
	state.PendingAcks = n
	m.startPhaseTask(phase)

	if *migDebugFlag {
		log.Printf("%d   -> phase %d (%d targets)",
			m.ctrl.CurrentTime(), phase, n)
	}
}

func (m *migMW) enterCopyPhase() {
	state := &m.ctrl.State
	state.Phase = migCopying
	state.MoveSent = false
	m.startPhaseTask(migCopying)

	if *migDebugFlag {
		log.Printf("%d   -> phase copy", m.ctrl.CurrentTime())
	}
}

// runControlPhase sends cmd to every target (across ticks, respecting
// backpressure) and waits for one response per target. When all targets have
// acked it calls advance to move to the next phase.
func (m *migMW) runControlPhase(
	targets []messaging.RemotePort,
	cmd memcontrolprotocol.Command,
	advance func(),
) bool {
	state := &m.ctrl.State
	progress := false

	for state.SendCursor < len(targets) {
		if !m.ctrlPort().CanSend() {
			break
		}

		req := memcontrolprotocol.Req{Command: cmd}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = m.ctrlPort().AsRemote()
		req.Dst = targets[state.SendCursor]
		req.TrafficClass = "memcontrolprotocol.Req"
		m.ctrlPort().Send(req)

		// Open a req_out task under the current phase so the receiver's req_in
		// task (parented by the message ID) has a parent; closed in processAcks.
		tracing.TraceReqInitiate(m.ctrl, req, state.PhaseTaskID)

		state.SendCursor++
		progress = true
	}

	if state.SendCursor == len(targets) && state.PendingAcks == 0 {
		advance()
		progress = true
	}

	return progress
}

// processAcks consumes control responses and decrements the outstanding-ack
// count. A failing response aborts the run loudly: every target the controller
// addresses is expected to support the verb it is sent.
func (m *migMW) processAcks() bool {
	progress := false

	for {
		msgI := m.ctrlPort().RetrieveIncoming()
		if msgI == nil {
			break
		}

		rsp, ok := msgI.(memcontrolprotocol.Rsp)
		if !ok {
			log.Panicf("migration: unexpected control msg %T", msgI)
		}

		if !rsp.Success {
			log.Panicf("migration: control command %d failed: %s",
				rsp.Command, rsp.Error)
		}

		// Close the req_out task opened for this command (keyed by request ID).
		tracing.EndTask(m.ctrl, tracing.TaskEnd{ID: rsp.RspTo})

		m.ctrl.State.PendingAcks--
		progress = true
	}

	return progress
}

// tickCopying issues a single data-move request and then waits for its
// response (handled in processMoveRsp).
func (m *migMW) tickCopying() bool {
	state := &m.ctrl.State

	if state.MoveSent {
		return false
	}

	if !m.moverPort().CanSend() {
		return false
	}

	req := datamoverprotocol.DataMoveRequest{
		SrcAddress: state.SrcAddr,
		DstAddress: state.DstAddr,
		ByteSize:   pageSize,
		SrcSide:    "inside",
		DstSide:    "outside",
	}
	req.ID = timing.GetIDGenerator().Generate()
	req.Src = m.moverPort().AsRemote()
	req.Dst = m.ctrl.moverDst
	req.TrafficClass = "datamoverprotocol.DataMoveRequest"
	m.moverPort().Send(req)

	// Open a req_out task under the copy phase so the data mover's req_in task
	// has a parent; closed in processMoveRsp.
	tracing.TraceReqInitiate(m.ctrl, req, state.PhaseTaskID)

	state.MoveSent = true

	if *migDebugFlag {
		log.Printf("%d   move req sent to %s (0x%x->0x%x %d bytes)",
			m.ctrl.CurrentTime(), m.ctrl.moverDst,
			state.SrcAddr, state.DstAddr, pageSize)
	}

	return true
}

// processMoveRsp handles the data mover's completion: it repoints the page to
// the destination device and moves on to invalidation.
func (m *migMW) processMoveRsp() bool {
	msgI := m.moverPort().RetrieveIncoming()
	if msgI == nil {
		return false
	}

	rsp, ok := msgI.(datamoverprotocol.DataMoveResponse)
	if !ok {
		log.Panicf("migration: unexpected mover msg %T", msgI)
	}

	state := &m.ctrl.State
	if state.Phase != migCopying {
		log.Panicf("migration: move response in phase %d", state.Phase)
	}

	// Close the req_out task opened for the data-move request.
	tracing.EndTask(m.ctrl, tracing.TaskEnd{ID: rsp.RspTo})

	vAddr := state.CurPage * pageSize
	pageEntry, found := m.ctrl.pages.Find(1, vAddr)
	if !found {
		log.Panicf("migration: page %d vanished mid-migration", state.CurPage)
	}

	pageEntry.PAddr = state.DstAddr
	pageEntry.DeviceID = state.DstDevice
	pageEntry.IsMigrating = false
	m.ctrl.pages.Update(pageEntry)

	m.enterControlPhase(migInvalidating, len(m.ctrl.invalTargets))

	return true
}

func (m *migMW) finishMigration() {
	state := &m.ctrl.State

	// Close the final phase task and the parent migration task.
	if state.PhaseTaskID != 0 {
		tracing.EndTask(m.ctrl, tracing.TaskEnd{ID: state.PhaseTaskID})
		state.PhaseTaskID = 0
	}
	if state.MigTaskID != 0 {
		tracing.EndTask(m.ctrl, tracing.TaskEnd{ID: state.MigTaskID})
		state.MigTaskID = 0
	}

	state.NumMigrations++
	state.Phase = migIdle
	state.Countdown = m.ctrl.Spec().Interval
	state.MoveSent = false
	state.SendCursor = 0
	state.PendingAcks = 0

	if *migDebugFlag {
		log.Printf("%d migration complete (total %d)",
			m.ctrl.CurrentTime(), state.NumMigrations)
	}
}

// setupMigrationController builds the data mover and the migration controller,
// wires their control/data connections, and starts the controller ticking. It
// returns the controller so the caller can report the migration count.
func setupMigrationController(
	s *simulation.Simulation,
	shared sharedHierarchy,
	chains []agentChain,
	memConn *directconnection.Comp,
) *migrationController {
	mover := buildDataMover(s, shared)

	// The data mover reads and writes memory over the same fabric as the L2.
	memConn.PlugIn(mover.GetPortByName("Inside"))
	memConn.PlugIn(mover.GetPortByName("Outside"))

	ctrl := buildMigrationController(s, shared, chains, mover)

	// One control connection carries the controller plus every component it
	// drains/pauses/flushes/invalidates/enables; directconnection routes by Dst.
	ctrlConn := directconnection.MakeBuilder().
		WithRegistrar(s).
		Build("ConnControl")
	ctrlConn.PlugIn(ctrl.GetPortByName("Ctrl"))
	for _, c := range chains {
		ctrlConn.PlugIn(c.rob.GetPortByName("Control"))
		ctrlConn.PlugIn(c.at.GetPortByName("Control"))
		ctrlConn.PlugIn(c.l1Cache.GetPortByName("Control"))
		ctrlConn.PlugIn(c.l1TLB.GetPortByName("Control"))
	}
	ctrlConn.PlugIn(shared.l2Cache.GetPortByName("Control"))
	ctrlConn.PlugIn(shared.l2TLB.GetPortByName("Control"))
	ctrlConn.PlugIn(shared.ioMMU.GetPortByName("Control"))

	connect(s, "ConnMover",
		ctrl.GetPortByName("Mover"),
		mover.GetPortByName("Top"),
	)

	ctrl.TickLater()

	return ctrl
}

func buildDataMover(
	s *simulation.Simulation,
	shared sharedHierarchy,
) *datamover.Comp {
	memCtrlPorts := make([]messaging.RemotePort, len(shared.memCtrls))
	for d, mc := range shared.memCtrls {
		memCtrlPorts[d] = mc.GetPortByName("Top").AsRemote()
	}

	dmSpec := datamover.DefaultSpec()
	dmSpec.BufferSize = pageSize
	dmSpec.InsideByteGranularity = 64
	dmSpec.OutsideByteGranularity = 64
	mover := datamover.MakeBuilder().
		WithRegistrar(s).
		WithSpec(dmSpec).
		WithResources(datamover.Resources{
			InsideMapper: &mem.InterleavedAddressPortMapper{
				InterleavingSize: shared.deviceStride,
				LowModules:       memCtrlPorts,
			},
			OutsideMapper: &mem.InterleavedAddressPortMapper{
				InterleavingSize: shared.deviceStride,
				LowModules:       memCtrlPorts,
			},
		}).
		Build("DataMover")
	assignPorts(s, mover, "Top", "Inside", "Outside", "Control")

	return mover
}

func buildMigrationController(
	s *simulation.Simulation,
	shared sharedHierarchy,
	chains []agentChain,
	mover *datamover.Comp,
) *migrationController {
	spec := migSpec{
		Freq:         1 * timing.GHz,
		Interval:     *migrateIntervalFlag,
		NumPages:     shared.numPages,
		DeviceStride: shared.deviceStride,
	}

	modelComp := modeling.NewBuilder[migSpec, migState, modeling.None]().
		WithEngine(s.GetEngine()).
		WithFreq(spec.Freq).
		WithSpec(spec).
		Build("MigrationController")
	modelComp.State = migState{
		Phase:     migIdle,
		Countdown: spec.Interval,
	}

	ctrl := &migrationController{
		Component: modelComp,
		pages:     shared.pageTable,
		moverDst:  mover.GetPortByName("Top").AsRemote(),
	}
	collectControlTargets(ctrl, shared, chains)

	mw := &migMW{ctrl: ctrl}
	modelComp.AddMiddleware(mw)

	modelComp.DeclarePort("Ctrl", memcontrolprotocol.Requester)
	modelComp.DeclarePort("Mover", datamoverprotocol.Requester)

	s.RegisterComponent(ctrl)
	assignPorts(s, ctrl, "Ctrl", "Mover")

	return ctrl
}

// collectControlTargets gathers the control-port references the migration FSM
// addresses: the reorder buffers (drained first), the translation and data path
// (paused after), the write-back L2 (flushed), and the caches and TLBs
// (invalidated). allTargets is the union re-enabled at the end.
func collectControlTargets(
	ctrl *migrationController,
	shared sharedHierarchy,
	chains []agentChain,
) {
	control := func(c messaging.Component) messaging.RemotePort {
		return c.GetPortByName("Control").AsRemote()
	}

	for _, c := range chains {
		ctrl.agents = append(ctrl.agents, c.agent)
		ctrl.robTargets = append(ctrl.robTargets, control(c.rob))
		ctrl.restTargets = append(ctrl.restTargets,
			control(c.at), control(c.l1Cache), control(c.l1TLB))
		ctrl.invalTargets = append(ctrl.invalTargets,
			control(c.l1Cache), control(c.l1TLB))
	}

	ctrl.restTargets = append(ctrl.restTargets,
		control(shared.l2Cache), control(shared.l2TLB), control(shared.ioMMU))
	ctrl.flushTargets = append(ctrl.flushTargets, control(shared.l2Cache))
	ctrl.invalTargets = append(ctrl.invalTargets,
		control(shared.l2Cache), control(shared.l2TLB))

	ctrl.allTargets = append(ctrl.allTargets, ctrl.robTargets...)
	ctrl.allTargets = append(ctrl.allTargets, ctrl.restTargets...)
}
