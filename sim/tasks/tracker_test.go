package tasks_test

import (
	"bytes"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"

	akitasim "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/modeling"

	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/naming"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/tasks/tracing"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

type testSimulation struct {
	*akitasim.Simulation
	*tasks.Tracker
	ids    *atomic.Uint64
	now    timing.VTimeInPicoSec
	owners map[naming.Named]bool
}

func (s *testSimulation) TaskTracker() *tasks.Tracker        { return s.Tracker }
func (s *testSimulation) CurrentTime() timing.VTimeInPicoSec { return s.now }
func (s *testSimulation) NewID() uint64                      { return s.ids.Add(1) }
func newTestSimulation() *testSimulation {
	s := &testSimulation{
		Simulation: akitasim.MakeBuilder().Build(), ids: &atomic.Uint64{}, owners: make(map[naming.Named]bool),
	}
	s.Tracker = tasks.NewTracker(s, s.NewID, func(n naming.Named) bool {
		if p, ok := n.(messaging.Port); ok {
			n = p.Owner()
		}
		return s.owners[n]
	})
	return s
}

type owner struct {
	name       string
	tracker    *tasks.Tracker
	simulation *testSimulation
}

func (o *owner) Name() string                       { return o.name }
func (o *owner) TaskTracker() *tasks.Tracker        { return o.tracker }
func (o *owner) NewID() uint64                      { return o.simulation.NewID() }
func (o *owner) CurrentTime() timing.VTimeInPicoSec { return o.simulation.CurrentTime() }
func (*owner) NotifyRecv(messaging.Port)            {}
func (*owner) NotifyPortFree(messaging.Port)        {}

type recorder struct {
	tasks.NopObserver
	mu           sync.Mutex
	starts, ends []tasks.Task
	tags         []tasks.Tag
	milestones   []tasks.Milestone
	order        map[uint64]string
}

func (r *recorder) TaskStarted(v tasks.Task) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.starts = append(r.starts, v)
	if r.order == nil {
		r.order = map[uint64]string{}
	}
	r.order[v.ID] += "S"
}
func (r *recorder) TaskEnded(v tasks.Task) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ends = append(r.ends, v)
	r.order[v.ID] += "E"
}
func (r *recorder) TagAdded(v tasks.Tag) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tags = append(r.tags, v)
	r.order[v.TaskID] += "T"
}
func (r *recorder) MilestoneAdded(v tasks.Milestone) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.milestones = append(r.milestones, v)
	r.order[v.TaskID] += "M"
}

type connection struct{ hooking.HookableBase }

func (*connection) Name() string                   { return "Link" }
func (*connection) NotifySend(_ messaging.Port)    {}
func (*connection) NotifyAvailable(messaging.Port) {}
func (c *connection) BindPort(p messaging.Port)    { p.BindConnection(c) }

func newOwner(name string, simulation *testSimulation) *owner {
	o := &owner{name: name, simulation: simulation}
	if simulation != nil {
		o.tracker = simulation.Tracker
		simulation.owners[o] = true
	}
	return o
}
func port(o *owner) *twowaybuffered.Port {
	var sim modeling.Simulation = o.simulation
	if o.simulation == nil {
		sim = akitasim.MakeBuilder().Build()
	}
	p := twowaybuffered.MakeBuilder().
		WithSimulation(sim).
		WithSpec(twowaybuffered.Spec{IncomingCapacity: 8, OutgoingCapacity: 8}).
		Build()
	p.BindOwner(o, o.name+".Port")
	(&connection{}).BindPort(p)
	return p
}

func TestDisabledDoesNotAllocateOrReadClock(t *testing.T) {
	// nil IDs would panic if any helper attempted an allocation.
	o := newOwner("Off", nil)
	require.Zero(t, o.TaskTracker().StartTask(o, tasks.StartInfo{}))
	require.Zero(t, o.TaskTracker().StartReqOut(o, messaging.Msg{}, 0))
	require.Zero(t, o.TaskTracker().StartReqIn(o, messaging.Msg{}))
	o.TaskTracker().EndTask(1)
	o.TaskTracker().EndReqIn(1)
	o.TaskTracker().EndReqOut(1)
	o.TaskTracker().AddTag(tasks.Tag{TaskID: 1})
	o.TaskTracker().AddMilestone(tasks.Milestone{TaskID: 1})
	p := port(o)
	p.Deliver(messaging.Msg{ID: 1})
	p.AddIncomingMilestone(messaging.WaitReason{})
	p.RetrieveIncoming()
	p.Send(messaging.Msg{ID: 2, Src: p.AsRemote(), Dst: "Remote"})
	p.AddOutgoingMilestone(messaging.WaitReason{})
	p.RetrieveOutgoing()
}

func TestFullEndAndSharedParentResolution(t *testing.T) {
	tr := newTestSimulation()
	sender := newOwner("Sender", tr)
	receiver := newOwner("Receiver", tr)
	all, scoped := &recorder{}, &recorder{}
	tr.Subscribe(all)
	tr.Subscribe(scoped, receiver)
	msg := messaging.Msg{ID: tr.NewID()}
	tr.now = 10
	send := sender.TaskTracker().StartReqOut(sender, msg, 0)
	tr.now = 12
	recv := receiver.TaskTracker().StartReqIn(receiver, msg)
	require.NotEqual(t, msg.ID, send)
	require.NotEqual(t, msg.ID, recv)
	require.Equal(t, send, all.starts[1].ParentID)
	require.Equal(t, all.starts[1], scoped.starts[0])
	// Cross-component end is legal; original scope, identity and clock survive.
	tr.now = 30
	sender.TaskTracker().EndTask(recv)
	want := scoped.starts[0]
	want.EndTime = 30
	require.Equal(t, want, scoped.ends[0])
	require.Equal(t, want, all.ends[0])
	sender.TaskTracker().EndReqOut(send)
	sender.TaskTracker().EndReqOut(send)
	require.Len(t, all.ends, 2)
	orphan := receiver.TaskTracker().StartReqIn(receiver, msg)
	require.Zero(t, all.starts[2].ParentID)
	receiver.TaskTracker().EndReqIn(orphan)
	t.Logf("message %d, sender task %d, receiver task %d; both observers received full end [%d,%d] with parent %d",
		msg.ID, send, recv, want.StartTime, want.EndTime, want.ParentID)
}

func TestBufferIdentityWithPreexistingHead(t *testing.T) {
	for _, incoming := range []bool{true, false} {
		t.Run(fmt.Sprint(incoming), func(t *testing.T) {
			tr := newTestSimulation()
			o := newOwner("Owner", tr)
			p := port(o)
			r := &recorder{}
			tr.Subscribe(r, o)
			enqueue, retrieve, milestone := p.Send, p.RetrieveOutgoing, p.AddOutgoingMilestone
			if incoming {
				enqueue, retrieve, milestone = p.Deliver, p.RetrieveIncoming, p.AddIncomingMilestone
			}
			a := messaging.Msg{ID: o.NewID(), Src: p.AsRemote(), Dst: "Remote"}
			b := a
			b.ID = o.NewID()
			// Restore an untracked head into a port assembled with tracking on.
			untracked := port(newOwner("Owner", nil))
			if incoming {
				untracked.Deliver(a)
			} else {
				untracked.Send(a)
			}
			var snapshot bytes.Buffer
			require.NoError(t, untracked.SaveCheckpoint(&snapshot))
			require.NoError(t, p.LoadCheckpoint(&snapshot))
			enqueue(b)
			require.Len(t, r.starts, 1)
			tr.now = 10
			milestone(tasks.Reason(tasks.MilestoneKindOther, "untracked"))
			require.Empty(t, r.milestones)
			got, ok := retrieve()
			require.True(t, ok)
			require.Equal(t, a.ID, got.ID)
			require.Empty(t, r.ends)
			require.Len(t, r.milestones, 1)
			require.Equal(t, r.starts[0].ID, r.milestones[0].TaskID)
			require.Equal(t, tasks.MilestoneKindQueue, r.milestones[0].Kind)
			tr.now = 20
			milestone(tasks.Reason(tasks.MilestoneKindHardwareResource, "admitted"))
			retrieve()
			require.Len(t, r.milestones, 2, "retrieve must not duplicate an explicit milestone at the same time")
			require.Len(t, r.ends, 1)
			require.Equal(t, b.ID, r.ends[0].MsgID)
		})
	}
}

func TestSubscriptionScopes(t *testing.T) {
	tr := newTestSimulation()
	other := newTestSimulation()
	o := newOwner("Owner", tr)
	p := port(o)
	sibling := twowaybuffered.MakeBuilder().
		WithSimulation(tr).
		WithSpec(twowaybuffered.Spec{IncomingCapacity: 8, OutgoingCapacity: 8}).
		Build()
	sibling.BindOwner(o, "UnrelatedName") // Actual ownership, not a name prefix.
	(&connection{}).BindPort(sibling)
	foreign := newOwner("Owner.Port.PrefixOnly", tr)
	all, component, justPort := &recorder{}, &recorder{}, &recorder{}
	tr.Subscribe(all)
	tr.Subscribe(component, o)
	var typedPort messaging.Port = p
	tr.Subscribe(justPort, typedPort)
	require.Panics(t, func() { tr.Subscribe(&recorder{}, newOwner("Foreign", other)) })
	require.Panics(t, func() { tr.Subscribe(&recorder{}, port(newOwner("Foreign", other))) })
	require.Panics(t, func() { tr.Subscribe(&recorder{}, nil) })
	id := o.TaskTracker().StartTask(o, tasks.StartInfo{Kind: "work", What: "component"})
	foreign.TaskTracker().StartTask(foreign, tasks.StartInfo{Kind: "work", What: "unrelated"})
	p.Deliver(messaging.Msg{ID: o.NewID()})
	sibling.Deliver(messaging.Msg{ID: o.NewID()})
	tr.now = 10
	p.AddIncomingMilestone(tasks.Reason(tasks.MilestoneKindData, "data"))
	p.RetrieveIncoming()
	sibling.RetrieveIncoming()
	o.TaskTracker().EndTask(id)
	require.Len(t, all.starts, 4)
	require.Len(t, component.starts, 3)
	require.Len(t, component.ends, 3)
	require.Len(t, justPort.starts, 1)
	require.Len(t, justPort.ends, 1)
	require.Len(t, justPort.milestones, 1)
	require.Zero(t, p.NumHooks(), "port tasks must work without a tracker hook")
}

func TestConcurrentDeliveryAndAdmission(t *testing.T) {
	tr := newTestSimulation()
	r := &recorder{}
	tr.Subscribe(r)
	var wg sync.WaitGroup
	for i := range 8 {
		o := newOwner(fmt.Sprintf("Owner%d", i), tr)
		p := port(o)
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				msg := messaging.Msg{ID: o.NewID()}
				id := o.TaskTracker().StartReqOut(o, msg, 0)
				p.Deliver(msg)
				p.AddIncomingMilestone(tasks.Reason(tasks.MilestoneKindHardwareResource, "admission"))
				p.RetrieveIncoming()
				o.TaskTracker().AddTag(tasks.Tag{TaskID: id, What: "done"})
				o.TaskTracker().EndReqOut(id)
			}
		}()
	}
	wg.Wait()
	require.Len(t, r.ends, 1600)
	for _, order := range r.order {
		require.Contains(t, []string{"STE", "SE"}, order)
	}
	t.Log("800 concurrent message deliveries retained distinct buffer IDs and ordered lifecycle callbacks")
}

type taskHandler struct {
	o *owner
	p messaging.Port
}

func (h taskHandler) Handle(timing.Event) {
	msg := messaging.Msg{ID: h.o.NewID()}
	id := h.o.TaskTracker().StartReqOut(h.o, msg, 0)
	h.p.Deliver(msg)
	h.p.AddIncomingMilestone(tasks.Reason(tasks.MilestoneKindHardwareResource, "admission"))
	h.p.RetrieveIncoming()
	h.o.TaskTracker().EndReqOut(id)
}

func TestParallelEngineTaskLifecycles(t *testing.T) {
	tr := newTestSimulation()
	r := &recorder{}
	tr.Subscribe(r)
	engine := timing.NewParallelEngine()
	for i := range 32 {
		o := newOwner(fmt.Sprintf("Owner%d", i), tr)
		tr.now = 10
		p := port(o)
		engine.RegisterHandler(o.name, taskHandler{o, p})
		engine.Schedule(timing.MakeEventBase(o.NewID(), 10, o.name))
	}
	require.NoError(t, engine.Run())
	require.Len(t, r.starts, 64)
	require.Len(t, r.ends, 64)
	for _, order := range r.order {
		require.Contains(t, []string{"SE"}, order)
	}
	t.Log("32 parallel engine handlers completed request and buffer lifecycles")
}

func TestSamePortConcurrentProducerConsumer(t *testing.T) {
	tr := newTestSimulation()
	o := newOwner("Shared", tr)
	p := port(o)
	r := &recorder{}
	tr.Subscribe(r)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 1000 {
			for !p.CanDeliver() {
				runtime.Gosched()
			}
			p.Deliver(messaging.Msg{ID: o.NewID()})
		}
	}()
	for range 1000 {
		var ok bool
		for {
			_, ok = p.PeekIncoming()
			if ok {
				break
			}
			runtime.Gosched()
		}
		p.AddIncomingMilestone(tasks.Reason(tasks.MilestoneKindHardwareResource, "admission"))
		p.RetrieveIncoming()
	}
	wg.Wait()
	require.Len(t, r.ends, 1000)
	for _, order := range r.order {
		require.Equal(t, "SE", order)
	}
}

func TestParentKindsAndSimulationIsolation(t *testing.T) {
	tr := newTestSimulation()
	o := newOwner("owner", tr)
	r := &recorder{}
	tr.Subscribe(r)
	msg := messaging.Msg{ID: o.NewID()}
	send := o.TaskTracker().StartReqOut(o, msg, 0)
	rsp := messaging.Msg{ID: o.NewID(), RspTo: msg.ID}
	for _, kind := range []string{tasks.KindIncomingBuffer, tasks.KindOutgoingBuffer, tasks.KindMsgTransport} {
		o.TaskTracker().StartTask(o, tasks.StartInfo{MsgID: rsp.ID, Kind: kind, What: "response", Detail: rsp})
		require.Equal(t, send, r.starts[len(r.starts)-1].ParentID)
	}
	flit := messaging.Msg{ID: o.NewID()}
	transport := o.TaskTracker().StartTask(o, tasks.StartInfo{MsgID: flit.ID, Kind: tasks.KindFlitTransport, What: "flit"})
	o.TaskTracker().StartTask(o, tasks.StartInfo{MsgID: flit.ID, Kind: tasks.KindIncomingBuffer, What: "flit"})
	require.Equal(t, transport, r.starts[len(r.starts)-1].ParentID)
	separate := newTestSimulation()
	other := newOwner("owner", separate)
	second := &recorder{}
	separate.Subscribe(second)
	other.TaskTracker().StartReqIn(other, msg)
	require.Zero(t, second.starts[0].ParentID)
}

func TestSubscribeBeforeRunCanObserveCompleteEnd(t *testing.T) {
	tr := newTestSimulation()
	o := newOwner("Owner", tr)
	tr.now = 10
	id := o.TaskTracker().StartTask(o, tasks.StartInfo{Kind: "work", What: "seeded"})
	r := &recorder{}
	r.order = map[uint64]string{}
	tr.Subscribe(r, o)
	tr.now = 20
	o.TaskTracker().EndTask(id)
	require.Empty(t, r.starts)
	require.Len(t, r.ends, 1)
	require.Equal(t, timing.VTimeInPicoSec(10), r.ends[0].StartTime)
	require.Equal(t, timing.VTimeInPicoSec(20), r.ends[0].EndTime)
}

type factRecorder struct{ events []hooking.HookCtx }

func (r *factRecorder) Func(ctx hooking.HookCtx) { r.events = append(r.events, ctx) }

func TestPortHookReportsMessage(t *testing.T) {
	o := newOwner("Owner", nil)
	p := port(o)
	facts := &factRecorder{}
	p.AcceptHook(facts)
	a, b := messaging.Msg{ID: 1}, messaging.Msg{ID: 2}
	p.Deliver(a)
	p.Deliver(b)
	p.RetrieveIncoming()
	p.RetrieveIncoming()
	require.Len(t, facts.events, 4)
	for i, msg := range []messaging.Msg{a, b, a, b} {
		require.Equal(t, msg, facts.events[i].Item)
		require.Nil(t, facts.events[i].Detail)
	}
}

type bufferSnapshot struct {
	task          tasks.Task
	tracker, port []byte
	ids           *atomic.Uint64
}

func saveBufferWait(t *testing.T, incoming bool) bufferSnapshot {
	t.Helper()
	tr := newTestSimulation()
	o := newOwner("Owner", tr)
	p := port(o)
	r := &recorder{}
	tr.Subscribe(r)
	enqueue, milestone := p.Send, p.AddOutgoingMilestone
	kind, direction := tasks.KindOutgoingBuffer, ".outgoing"
	if incoming {
		enqueue, milestone = p.Deliver, p.AddIncomingMilestone
		kind, direction = tasks.KindIncomingBuffer, ".incoming"
	}
	milestone(messaging.WaitReason{})
	tr.now = 10
	a := messaging.Msg{ID: o.NewID(), Src: p.AsRemote(), Dst: "Remote"}
	parent := o.TaskTracker().StartReqOut(o, a, 0)
	enqueue(a)
	require.Empty(t, r.milestones, "queue into an empty buffer takes zero time")
	first := r.starts[1]
	require.Equal(t, kind, first.Kind)
	require.Equal(t, p.Name()+direction, first.Location)
	require.Equal(t, parent, first.ParentID)
	tr.now = 12
	b := a
	b.ID = o.NewID()
	enqueue(b)
	tr.now = 15
	milestone(tasks.Reason(tasks.MilestoneKindHardwareResource, "slots"))
	tr.now = 18
	milestone(tasks.Reason(tasks.MilestoneKindData, "data"))
	var trackerState, portState bytes.Buffer
	require.NoError(t, tr.Tracker.SaveCheckpoint(&trackerState))
	require.NotContains(t, trackerState.String(), `"buffers"`)
	require.NoError(t, p.SaveCheckpoint(&portState))
	require.Contains(t, portState.String(), `"task_id":`)
	return bufferSnapshot{first, trackerState.Bytes(), portState.Bytes(), tr.ids}
}

func TestBufferMilestonesAndCheckpoint(t *testing.T) {
	for _, incoming := range []bool{true, false} {
		t.Run(fmt.Sprint(incoming), func(t *testing.T) {
			saved := saveBufferWait(t, incoming)
			tr := newTestSimulation()
			tr.ids = saved.ids
			o := newOwner("Owner", tr)
			tr.now = 18
			p := port(o)
			require.NoError(t, tr.Tracker.LoadCheckpoint(bytes.NewReader(saved.tracker)))
			require.NoError(t, p.LoadCheckpoint(bytes.NewReader(saved.port)))
			scoped, portScoped := &recorder{order: map[uint64]string{}}, &recorder{order: map[uint64]string{}}
			tr.Subscribe(scoped, o)
			tr.Subscribe(portScoped, messaging.Port(p))
			retrieve, milestone := p.RetrieveOutgoing, p.AddOutgoingMilestone
			fallback, what := tasks.MilestoneKindNetworkBusy, "Link"
			if incoming {
				retrieve, milestone = p.RetrieveIncoming, p.AddIncomingMilestone
				fallback, what = tasks.MilestoneKindConsumer, "Owner"
			}
			milestone(tasks.Reason(tasks.MilestoneKindOther, "same-time"))
			require.Empty(t, scoped.milestones, "checkpoint must preserve the previous milestone boundary")
			tr.now = 20
			retrieve()
			require.Len(t, scoped.milestones, 2)
			require.Equal(t, fallback, scoped.milestones[0].Kind)
			require.Equal(t, what, scoped.milestones[0].What)
			require.Equal(t, tasks.MilestoneKindQueue, scoped.milestones[1].Kind)
			require.Equal(t, p.Name(), scoped.milestones[1].What)
			saved.task.Detail = nil
			saved.task.EndTime = 20
			require.Equal(t, saved.task, scoped.ends[0])
			retrieve()
			require.Len(t, scoped.milestones, 2, "same-time head retrieval adds no milestone")
			require.Len(t, scoped.ends, 2)
			require.Equal(t, scoped.ends, portScoped.ends)
			require.Empty(t, scoped.starts)
			t.Logf("%s: restored task %d kept parent %d and [10,20]; %s and queue at 20, no duplicate",
				saved.task.Kind, saved.task.ID, saved.task.ParentID, fallback)
		})
	}
}

func TestLateSubscribersCountFullIntervals(t *testing.T) {
	tr := newTestSimulation()
	o := newOwner("Owner", tr)
	tr.now = 10
	id := o.TaskTracker().StartTask(o, tasks.StartInfo{Kind: "work", What: "inflight"})
	o.TaskTracker().AddTag(tasks.Tag{TaskID: id, What: "earlier"})
	o.TaskTracker().AddMilestone(tasks.Milestone{TaskID: id, Kind: tasks.MilestoneKindData, What: "earlier"})
	tr.now = 20
	events := &recorder{order: map[uint64]string{}}
	busy, avg, total := tracing.NewBusyTimeTracer(nil), tracing.NewAverageTimeTracer(nil), tracing.NewTotalTimeTracer(nil)
	tr.Subscribe(events, o)
	tr.Subscribe(busy, o)
	tr.Subscribe(avg, o)
	tr.Subscribe(total, o)
	tr.now = 30
	o.TaskTracker().EndTask(id)
	require.Empty(t, events.starts)
	require.Empty(t, events.tags)
	require.Empty(t, events.milestones)
	require.Equal(t, timing.VTimeInPicoSec(20), busy.BusyTime())
	require.Equal(t, timing.VTimeInPicoSec(20), avg.AverageTime())
	require.Equal(t, timing.VTimeInPicoSec(20), total.TotalTime())
	t.Log("Subscribed at 20 to task [10,30]: busy, average, and total each report 20; no earlier events replayed")
}

func (o *owner) BindPort(slot string, p messaging.Port) { p.BindOwner(o, o.Name()+"."+slot) }
