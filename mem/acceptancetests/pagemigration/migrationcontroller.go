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
	"github.com/sarchlab/akita/v5/noc/directconnection"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/akita/v5/sim/tracing"
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

// migResources holds the references the migration controller works on,
// supplied by the system builder. It owns no memory; it orchestrates the
// existing control protocol, page table, and data mover.
type migResources struct {
	// Pages is the shared page table; the controller mutates it directly.
	Pages vm.PageTable
	// Agents is used only to detect when the workload is finished so the
	// controller can stop ticking and let the simulation terminate.
	Agents []*memaccessagent.MemAccessAgent
	// MoverDst is the data mover's Top port.
	MoverDst messaging.RemotePort

	// ROBTargets are the request sources, drained first.
	ROBTargets []messaging.RemotePort
	// RestTargets are the translation and data path components, paused once the
	// reorder buffers are drained and everything downstream is idle.
	RestTargets []messaging.RemotePort
	// AllTargets is ROBTargets+RestTargets, re-enabled together at the end.
	AllTargets []messaging.RemotePort
	// FlushTargets are the write-back caches whose dirty data must reach memory
	// before the copy.
	FlushTargets []messaging.RemotePort
	// InvalTargets are the caches and TLBs whose stale entries are dropped after
	// the page table is repointed.
	InvalTargets []messaging.RemotePort
}

// migPorts holds the migration controller's ports.
type migPorts struct {
	// Ctrl sends the drain, pause, flush, invalidate, and enable commands and
	// receives their responses.
	Ctrl messaging.Port `akita:"role=github.com/sarchlab/akita/v5/mem/memcontrolprotocol.requester"`

	// Mover sends the page-copy request to the data mover and receives its
	// completion.
	Mover messaging.Port `akita:"role=github.com/sarchlab/akita/v5/mem/datamoverprotocol.requester"`
}

// migMiddlewares holds the migration controller's behavior.
type migMiddlewares struct {
	// Migration advances the migration finite state machine.
	Migration *migMW
}

// migrationController periodically relocates a page from one device to another
// while keeping the data transparent to the agents.
type migrationController = ticking.Component[
	migSpec, migState, migResources, migPorts, migMiddlewares]

// Definition declares the migration controller, a ticking component.
var Definition = ticking.Definition[
	migSpec, migState, migResources, migPorts, migMiddlewares]{
	DefaultSpec: migSpec{
		Freq: 1 * timing.GHz,
	},
	NewState:       newMigState,
	NewMiddlewares: newMigMiddlewares,
}

func newMigState(c *migrationController) migState {
	return migState{
		Phase:     migIdle,
		Countdown: c.Spec.Interval,
	}
}

func newMigMiddlewares(c *migrationController) migMiddlewares {
	return migMiddlewares{
		Migration: &migMW{ctrl: c, res: c.Resources},
	}
}

// migMW is the migration controller's behavior.
type migMW struct {
	ctrl *migrationController
	res  migResources
}

func (m *migMW) ctrlPort() messaging.Port {
	return m.ctrl.Ports.Ctrl
}

func (m *migMW) moverPort() messaging.Port {
	return m.ctrl.Ports.Mover
}

// Handle advances the migration finite state machine by one cycle.
func (m *migMW) Handle(_ timing.Event) bool {
	state := &m.ctrl.State
	progress := false

	progress = m.processAcks() || progress
	progress = m.processMoveRsp() || progress

	switch state.Phase {
	case migIdle:
		progress = m.tickIdle() || progress
	case migDrainROB:
		progress = m.runControlPhase(
			m.res.ROBTargets, memcontrolprotocol.CmdDrain,
			func() { m.enterControlPhase(migPauseRest, len(m.res.RestTargets)) },
		) || progress
	case migPauseRest:
		progress = m.runControlPhase(
			m.res.RestTargets, memcontrolprotocol.CmdPause,
			func() { m.enterControlPhase(migFlushing, len(m.res.FlushTargets)) },
		) || progress
	case migFlushing:
		progress = m.runControlPhase(
			m.res.FlushTargets, memcontrolprotocol.CmdFlush,
			func() { m.enterCopyPhase() },
		) || progress
	case migCopying:
		progress = m.tickCopying() || progress
	case migInvalidating:
		progress = m.runControlPhase(
			m.res.InvalTargets, memcontrolprotocol.CmdInvalidate,
			func() { m.enterControlPhase(migEnabling, len(m.res.AllTargets)) },
		) || progress
	case migEnabling:
		progress = m.runControlPhase(
			m.res.AllTargets, memcontrolprotocol.CmdEnable,
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
	for _, a := range m.res.Agents {
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
	spec := m.ctrl.Spec

	page := state.PageCursor % spec.NumPages
	state.PageCursor = (state.PageCursor + 1) % spec.NumPages

	vAddr := page * pageSize
	pageEntry, found := m.res.Pages.Find(1, vAddr)
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
	m.res.Pages.Update(pageEntry)

	if *migDebugFlag {
		log.Printf("%d migration #%d: page %d dev %d->%d (0x%x->0x%x)",
			m.ctrl.CurrentTime(), state.NumMigrations, page,
			srcDevice, dstDevice, state.SrcAddr, state.DstAddr)
	}

	// Open a parent task spanning the whole migration so the control phases nest
	// under it in the trace.
	state.MigTaskID = m.ctrl.NewID()
	tracing.StartTask(m.ctrl, tracing.TaskStart{
		ID:       state.MigTaskID,
		Kind:     "migration",
		What:     "migration",
		Location: m.ctrl.Name(),
	})

	m.enterControlPhase(migDrainROB, len(m.res.ROBTargets))
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
	state.PhaseTaskID = m.ctrl.NewID()
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

		req := messaging.Msg{Payload: memcontrolprotocol.Req{Command: cmd},
			ID:           m.ctrl.NewID(),
			Src:          m.ctrlPort().AsRemote(),
			Dst:          targets[state.SendCursor],
			TrafficClass: "memcontrolprotocol.Req"}

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
		msgI, ok := m.ctrlPort().RetrieveIncoming()
		if !ok {
			break
		}
		_, ok = msgI.Payload.(memcontrolprotocol.Rsp)
		rsp := msgI
		if !ok {
			log.Panicf("migration: unexpected control msg %T", msgI.Payload)
		}

		if !rsp.Payload.(memcontrolprotocol.Rsp).Success {
			log.Panicf("migration: control command %d failed: %s",
				rsp.Payload.(memcontrolprotocol.Rsp).Command, rsp.Payload.(memcontrolprotocol.Rsp).Error)
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

	req := messaging.Msg{Payload: datamoverprotocol.DataMoveReq{
		SrcAddress: state.SrcAddr,
		DstAddress: state.DstAddr,
		ByteSize:   pageSize,
		SrcSide:    "inside",
		DstSide:    "outside",
	},
		ID:           m.ctrl.NewID(),
		Src:          m.moverPort().AsRemote(),
		Dst:          m.res.MoverDst,
		TrafficClass: "datamoverprotocol.DataMoveReq"}

	m.moverPort().Send(req)

	// Open a req_out task under the copy phase so the data mover's req_in task
	// has a parent; closed in processMoveRsp.
	tracing.TraceReqInitiate(m.ctrl, req, state.PhaseTaskID)

	state.MoveSent = true

	if *migDebugFlag {
		log.Printf("%d   move req sent to %s (0x%x->0x%x %d bytes)",
			m.ctrl.CurrentTime(), m.res.MoverDst,
			state.SrcAddr, state.DstAddr, pageSize)
	}

	return true
}

// processMoveRsp handles the data mover's completion: it repoints the page to
// the destination device and moves on to invalidation.
func (m *migMW) processMoveRsp() bool {
	msgI, ok := m.moverPort().RetrieveIncoming()
	if !ok {
		return false
	}
	_, ok = msgI.Payload.(datamoverprotocol.DataMoveRsp)
	rsp := msgI
	if !ok {
		log.Panicf("migration: unexpected mover msg %T", msgI.Payload)
	}

	state := &m.ctrl.State
	if state.Phase != migCopying {
		log.Panicf("migration: move response in phase %d", state.Phase)
	}

	// Close the req_out task opened for the data-move request.
	tracing.EndTask(m.ctrl, tracing.TaskEnd{ID: rsp.RspTo})

	vAddr := state.CurPage * pageSize
	pageEntry, found := m.res.Pages.Find(1, vAddr)
	if !found {
		log.Panicf("migration: page %d vanished mid-migration", state.CurPage)
	}

	pageEntry.PAddr = state.DstAddr
	pageEntry.DeviceID = state.DstDevice
	pageEntry.IsMigrating = false
	m.res.Pages.Update(pageEntry)

	m.enterControlPhase(migInvalidating, len(m.res.InvalTargets))

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
	state.Countdown = m.ctrl.Spec.Interval
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
	s *sim.Simulation,
	shared sharedHierarchy,
	chains []agentChain,
	memConn *directconnection.Comp,
) *migrationController {
	mover := buildDataMover(s, shared)

	// The data mover reads and writes memory over the same fabric as the L2.
	memConn.PlugIn(mover.Ports.Inside)
	memConn.PlugIn(mover.Ports.Outside)

	ctrl := buildMigrationController(s, shared, chains, mover)

	// One control connection carries the controller plus every component it
	// drains/pauses/flushes/invalidates/enables; directconnection routes by Dst.
	ctrlConn := directconnection.MakeBuilder().
		WithSimulation(s).
		Build("ConnControl")
	ctrlConn.PlugIn(ctrl.Ports.Ctrl)
	for _, c := range chains {
		ctrlConn.PlugIn(c.rob.Ports.Control)
		ctrlConn.PlugIn(c.at.Ports.Control)
		ctrlConn.PlugIn(c.l1Cache.Ports.Control)
		ctrlConn.PlugIn(c.l1TLB.Ports.Control)
	}
	ctrlConn.PlugIn(shared.l2Cache.Ports.Control)
	ctrlConn.PlugIn(shared.l2TLB.Ports.Control)
	ctrlConn.PlugIn(shared.ioMMU.Ports.Control)

	connect(s, "ConnMover",
		ctrl.Ports.Mover,
		mover.Ports.Top,
	)

	ctrl.TickLater()

	return ctrl
}

func buildDataMover(
	s *sim.Simulation,
	shared sharedHierarchy,
) *datamover.Comp {
	memCtrlPorts := make([]messaging.RemotePort, len(shared.memCtrls))
	for d, mc := range shared.memCtrls {
		memCtrlPorts[d] = mc.Ports.Top.AsRemote()
	}

	dmSpec := datamover.Definition.DefaultSpec
	dmSpec.BufferSize = pageSize
	dmSpec.InsideByteGranularity = 64
	dmSpec.OutsideByteGranularity = 64
	mover := datamover.Definition.Builder().
		WithSimulation(s).
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
		WithPorts(datamover.Ports{
			Top:     newPort("DataMover.Top"),
			Inside:  newPort("DataMover.Inside"),
			Outside: newPort("DataMover.Outside"),
			Control: newPort("DataMover.Control"),
		}).
		Build("DataMover")

	return mover
}

func buildMigrationController(
	s *sim.Simulation,
	shared sharedHierarchy,
	chains []agentChain,
	mover *datamover.Comp,
) *migrationController {
	spec := Definition.DefaultSpec
	spec.Interval = *migrateIntervalFlag
	spec.NumPages = shared.numPages
	spec.DeviceStride = shared.deviceStride

	res := migResources{
		Pages:    shared.pageTable,
		MoverDst: mover.Ports.Top.AsRemote(),
	}
	collectControlTargets(&res, shared, chains)

	return Definition.Builder().
		WithSimulation(s).
		WithSpec(spec).
		WithResources(res).
		WithPorts(migPorts{
			Ctrl:  newPort("MigrationController.Ctrl"),
			Mover: newPort("MigrationController.Mover"),
		}).
		Build("MigrationController")
}

// collectControlTargets gathers the control-port references the migration FSM
// addresses: the reorder buffers (drained first), the translation and data path
// (paused after), the write-back L2 (flushed), and the caches and TLBs
// (invalidated). AllTargets is the union re-enabled at the end.
func collectControlTargets(
	res *migResources,
	shared sharedHierarchy,
	chains []agentChain,
) {
	for _, c := range chains {
		res.Agents = append(res.Agents, c.agent)
		res.ROBTargets = append(res.ROBTargets, c.rob.Ports.Control.AsRemote())
		res.RestTargets = append(res.RestTargets,
			c.at.Ports.Control.AsRemote(),
			c.l1Cache.Ports.Control.AsRemote(),
			c.l1TLB.Ports.Control.AsRemote())
		res.InvalTargets = append(res.InvalTargets,
			c.l1Cache.Ports.Control.AsRemote(),
			c.l1TLB.Ports.Control.AsRemote())
	}

	res.RestTargets = append(res.RestTargets,
		shared.l2Cache.Ports.Control.AsRemote(),
		shared.l2TLB.Ports.Control.AsRemote(),
		shared.ioMMU.Ports.Control.AsRemote())
	res.FlushTargets = append(res.FlushTargets,
		shared.l2Cache.Ports.Control.AsRemote())
	res.InvalTargets = append(res.InvalTargets,
		shared.l2Cache.Ports.Control.AsRemote(),
		shared.l2TLB.Ports.Control.AsRemote())

	res.AllTargets = append(res.AllTargets, res.ROBTargets...)
	res.AllTargets = append(res.AllTargets, res.RestTargets...)
}
