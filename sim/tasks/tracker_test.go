package tasks_test

import (
	"bytes"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/tasks/tracing"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

type owner struct {
	name    string
	tracker *tasks.Tracker
	ids     *atomic.Uint64
	now     timing.VTimeInPicoSec
}

func (o *owner) Name() string                { return o.name }
func (o *owner) TaskTracker() *tasks.Tracker { return o.tracker }
func (o *owner) NewID() uint64               { return o.ids.Add(1) }
func (o *owner) CurrentTime() timing.VTimeInPicoSec {
	if o.ids == nil {
		panic("disabled helper read clock")
	}
	return o.now
}
func (*owner) NotifyRecv(messaging.Port)     {}
func (*owner) NotifyPortFree(messaging.Port) {}

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
func (*connection) NotifySend()                    {}
func (*connection) NotifyAvailable(messaging.Port) {}
func (c *connection) BindPort(p messaging.Port)    { p.BindConnection(c) }

func newOwner(name string, tracker *tasks.Tracker, ids *atomic.Uint64) *owner {
	return &owner{name: name, tracker: tracker, ids: ids}
}
func port(o *owner) *twowaybuffered.Port {
	p := twowaybuffered.NewPort(8, 8)
	p.BindOwner(o, o.name+".Port")
	(&connection{}).BindPort(p)
	return p
}

func TestDisabledDoesNotAllocateOrReadClock(t *testing.T) {
	// nil IDs would panic if any helper attempted an allocation.
	o := newOwner("Off", nil, nil)
	require.Zero(t, tasks.Start(o, tasks.StartInfo{}))
	require.Zero(t, tasks.StartReqOut(o, messaging.Msg{}, 0))
	require.Zero(t, tasks.StartReqIn(o, messaging.Msg{}))
	tasks.End(o, tasks.EndInfo{ID: 1})
	tasks.EndReqIn(o, 1)
	tasks.EndReqOut(o, 1)
	tasks.AddTag(o, tasks.Tag{TaskID: 1})
	tasks.AddMilestone(o, tasks.Milestone{TaskID: 1})
	p := port(o)
	p.Deliver(messaging.Msg{ID: 1})
	p.AddIncomingMilestone(messaging.WaitReason{})
	p.RetrieveIncoming()
	p.Send(messaging.Msg{ID: 2, Src: p.AsRemote(), Dst: "Remote"})
	p.AddOutgoingMilestone(messaging.WaitReason{})
	p.RetrieveOutgoing()
}

func TestFullEndAndSharedParentResolution(t *testing.T) {
	tr := tasks.NewTracker()
	ids := &atomic.Uint64{}
	sender := newOwner("Sender", tr, ids)
	receiver := newOwner("Receiver", tr, ids)
	all, scoped := &recorder{}, &recorder{}
	tr.Subscribe(all)
	tr.Subscribe(scoped, receiver)
	msg := messaging.Msg{ID: ids.Add(1)}
	sender.now = 10
	send := tasks.StartReqOut(sender, msg, 0)
	receiver.now = 12
	recv := tasks.StartReqIn(receiver, msg)
	require.NotEqual(t, msg.ID, send)
	require.NotEqual(t, msg.ID, recv)
	require.Equal(t, send, all.starts[1].ParentID)
	require.Equal(t, all.starts[1], scoped.starts[0])
	// Cross-component end is legal; original scope, identity and clock survive.
	sender.now = 30
	tasks.End(sender, tasks.EndInfo{ID: recv})
	want := scoped.starts[0]
	want.EndTime = 30
	require.Equal(t, want, scoped.ends[0])
	require.Equal(t, want, all.ends[0])
	tasks.EndReqOut(sender, send)
	tasks.EndReqOut(sender, send)
	require.Len(t, all.ends, 2)
	orphan := tasks.StartReqIn(receiver, msg)
	require.Zero(t, all.starts[2].ParentID)
	tasks.EndReqIn(receiver, orphan)
	t.Logf("message %d, sender task %d, receiver task %d; both observers received full end [%d,%d] with parent %d",
		msg.ID, send, recv, want.StartTime, want.EndTime, want.ParentID)
}

func TestBufferIdentityWithPreexistingHead(t *testing.T) {
	for _, incoming := range []bool{true, false} {
		t.Run(fmt.Sprint(incoming), func(t *testing.T) {
			tr := tasks.NewTracker()
			o := newOwner("Owner", tr, &atomic.Uint64{})
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
			o.tracker = nil
			enqueue(a) // Equivalent to an untracked message restored with tracking on.
			o.tracker = tr
			enqueue(b)
			require.Len(t, r.starts, 1)
			o.now = 10
			milestone(tasks.Reason(tasks.MilestoneKindOther, "untracked"))
			require.Empty(t, r.milestones)
			got, ok := retrieve()
			require.True(t, ok)
			require.Equal(t, a.ID, got.ID)
			require.Empty(t, r.ends)
			require.Len(t, r.milestones, 1)
			require.Equal(t, r.starts[0].ID, r.milestones[0].TaskID)
			require.Equal(t, tasks.MilestoneKindQueue, r.milestones[0].Kind)
			o.now = 20
			milestone(tasks.Reason(tasks.MilestoneKindHardwareResource, "admitted"))
			retrieve()
			require.Len(t, r.milestones, 2, "retrieve must not duplicate an explicit milestone at the same time")
			require.Len(t, r.ends, 1)
			require.Equal(t, b.ID, r.ends[0].MsgID)
		})
	}
}

func TestSubscriptionScopes(t *testing.T) {
	tr := tasks.NewTracker()
	other := tasks.NewTracker()
	ids := &atomic.Uint64{}
	o := newOwner("Owner", tr, ids)
	p := port(o)
	sibling := twowaybuffered.NewPort(8, 8)
	sibling.BindOwner(o, "UnrelatedName") // Actual ownership, not a name prefix.
	(&connection{}).BindPort(sibling)
	foreign := newOwner("Owner.Port.PrefixOnly", tr, ids)
	all, component, justPort := &recorder{}, &recorder{}, &recorder{}
	tr.Subscribe(all)
	tr.Subscribe(component, o)
	var typedPort messaging.Port = p
	tr.Subscribe(justPort, typedPort)
	require.Panics(t, func() { tr.Subscribe(&recorder{}, newOwner("Foreign", other, ids)) })
	require.Panics(t, func() { tr.Subscribe(&recorder{}, port(newOwner("Foreign", other, ids))) })
	require.Panics(t, func() { tr.Subscribe(&recorder{}, nil) })
	id := tasks.Start(o, tasks.StartInfo{Kind: "work", What: "component"})
	tasks.Start(foreign, tasks.StartInfo{Kind: "work", What: "unrelated"})
	p.Deliver(messaging.Msg{ID: o.NewID()})
	sibling.Deliver(messaging.Msg{ID: o.NewID()})
	o.now = 10
	p.AddIncomingMilestone(tasks.Reason(tasks.MilestoneKindData, "data"))
	p.RetrieveIncoming()
	sibling.RetrieveIncoming()
	tasks.End(o, tasks.EndInfo{ID: id})
	require.Len(t, all.starts, 4)
	require.Len(t, component.starts, 3)
	require.Len(t, component.ends, 3)
	require.Len(t, justPort.starts, 1)
	require.Len(t, justPort.ends, 1)
	require.Len(t, justPort.milestones, 1)
	require.Zero(t, p.NumHooks(), "port tasks must work without a tracker hook")
}

func TestConcurrentDeliveryAndAdmission(t *testing.T) {
	tr := tasks.NewTracker()
	ids := &atomic.Uint64{}
	r := &recorder{}
	tr.Subscribe(r)
	var wg sync.WaitGroup
	for i := range 8 {
		o := newOwner(fmt.Sprintf("Owner%d", i), tr, ids)
		p := port(o)
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				msg := messaging.Msg{ID: o.NewID()}
				id := tasks.StartReqOut(o, msg, 0)
				p.Deliver(msg)
				p.AddIncomingMilestone(tasks.Reason(tasks.MilestoneKindHardwareResource, "admission"))
				p.RetrieveIncoming()
				tasks.AddTag(o, tasks.Tag{TaskID: id, What: "done"})
				tasks.EndReqOut(o, id)
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
	id := tasks.StartReqOut(h.o, msg, 0)
	h.p.Deliver(msg)
	h.p.AddIncomingMilestone(tasks.Reason(tasks.MilestoneKindHardwareResource, "admission"))
	h.p.RetrieveIncoming()
	tasks.EndReqOut(h.o, id)
}

func TestParallelEngineTaskLifecycles(t *testing.T) {
	tr := tasks.NewTracker()
	ids := &atomic.Uint64{}
	r := &recorder{}
	tr.Subscribe(r)
	engine := timing.NewParallelEngine()
	for i := range 32 {
		o := newOwner(fmt.Sprintf("Owner%d", i), tr, ids)
		o.now = 10
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
	tr := tasks.NewTracker()
	ids := &atomic.Uint64{}
	o := newOwner("Shared", tr, ids)
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
	tr := tasks.NewTracker()
	ids := &atomic.Uint64{}
	o := newOwner("owner", tr, ids)
	r := &recorder{}
	tr.Subscribe(r)
	msg := messaging.Msg{ID: o.NewID()}
	send := tasks.StartReqOut(o, msg, 0)
	rsp := messaging.Msg{ID: o.NewID(), RspTo: msg.ID}
	for _, kind := range []string{tasks.KindIncomingBuffer, tasks.KindOutgoingBuffer, tasks.KindMsgTransport} {
		tasks.Start(o, tasks.StartInfo{MsgID: rsp.ID, Kind: kind, What: "response", Detail: rsp})
		require.Equal(t, send, r.starts[len(r.starts)-1].ParentID)
	}
	flit := messaging.Msg{ID: o.NewID()}
	transport := tasks.Start(o, tasks.StartInfo{MsgID: flit.ID, Kind: tasks.KindFlitTransport, What: "flit"})
	tasks.Start(o, tasks.StartInfo{MsgID: flit.ID, Kind: tasks.KindIncomingBuffer, What: "flit"})
	require.Equal(t, transport, r.starts[len(r.starts)-1].ParentID)
	separate := tasks.NewTracker()
	other := newOwner("owner", separate, &atomic.Uint64{})
	second := &recorder{}
	separate.Subscribe(second)
	tasks.StartReqIn(other, msg)
	require.Zero(t, second.starts[0].ParentID)
}

func TestSubscribeBeforeRunCanObserveCompleteEnd(t *testing.T) {
	tr := tasks.NewTracker()
	o := newOwner("Owner", tr, &atomic.Uint64{})
	o.now = 10
	id := tasks.Start(o, tasks.StartInfo{Kind: "work", What: "seeded"})
	r := &recorder{}
	r.order = map[uint64]string{}
	tr.Subscribe(r, o)
	o.now = 20
	tasks.End(o, tasks.EndInfo{ID: id})
	require.Empty(t, r.starts)
	require.Len(t, r.ends, 1)
	require.Equal(t, timing.VTimeInPicoSec(10), r.ends[0].StartTime)
	require.Equal(t, timing.VTimeInPicoSec(20), r.ends[0].EndTime)
}

type factRecorder struct{ events []hooking.HookCtx }

func (r *factRecorder) Func(ctx hooking.HookCtx) { r.events = append(r.events, ctx) }

func TestPortHookReportsMessage(t *testing.T) {
	o := newOwner("Owner", nil, &atomic.Uint64{})
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
	tr := tasks.NewTracker()
	ids := &atomic.Uint64{}
	o := newOwner("Owner", tr, ids)
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
	o.now = 10
	a := messaging.Msg{ID: o.NewID(), Src: p.AsRemote(), Dst: "Remote"}
	parent := tasks.StartReqOut(o, a, 0)
	enqueue(a)
	require.Empty(t, r.milestones, "queue into an empty buffer takes zero time")
	first := r.starts[1]
	require.Equal(t, kind, first.Kind)
	require.Equal(t, p.Name()+direction, first.Location)
	require.Equal(t, parent, first.ParentID)
	o.now = 12
	b := a
	b.ID = o.NewID()
	enqueue(b)
	o.now = 15
	milestone(tasks.Reason(tasks.MilestoneKindHardwareResource, "slots"))
	o.now = 18
	milestone(tasks.Reason(tasks.MilestoneKindData, "data"))
	var trackerState, portState bytes.Buffer
	require.NoError(t, tr.SaveCheckpoint(&trackerState))
	require.NotContains(t, trackerState.String(), `"buffers"`)
	require.NoError(t, p.SaveCheckpoint(&portState))
	require.Contains(t, portState.String(), `"task_id":`)
	return bufferSnapshot{first, trackerState.Bytes(), portState.Bytes(), ids}
}

func TestBufferMilestonesAndCheckpoint(t *testing.T) {
	for _, incoming := range []bool{true, false} {
		t.Run(fmt.Sprint(incoming), func(t *testing.T) {
			saved := saveBufferWait(t, incoming)
			tr := tasks.NewTracker()
			o := newOwner("Owner", tr, saved.ids)
			o.now = 18
			p := port(o)
			require.NoError(t, tr.LoadCheckpoint(bytes.NewReader(saved.tracker)))
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
			o.now = 20
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
	tr := tasks.NewTracker()
	o := newOwner("Owner", tr, &atomic.Uint64{})
	o.now = 10
	id := tasks.Start(o, tasks.StartInfo{Kind: "work", What: "inflight"})
	tasks.AddTag(o, tasks.Tag{TaskID: id, What: "earlier"})
	tasks.AddMilestone(o, tasks.Milestone{TaskID: id, Kind: tasks.MilestoneKindData, What: "earlier"})
	o.now = 20
	events := &recorder{order: map[uint64]string{}}
	busy, avg, total := tracing.NewBusyTimeTracer(nil), tracing.NewAverageTimeTracer(nil), tracing.NewTotalTimeTracer(nil)
	tr.Subscribe(events, o)
	tr.Subscribe(busy, o)
	tr.Subscribe(avg, o)
	tr.Subscribe(total, o)
	o.now = 30
	tasks.End(o, tasks.EndInfo{ID: id})
	require.Empty(t, events.starts)
	require.Empty(t, events.tags)
	require.Empty(t, events.milestones)
	require.Equal(t, timing.VTimeInPicoSec(20), busy.BusyTime())
	require.Equal(t, timing.VTimeInPicoSec(20), avg.AverageTime())
	require.Equal(t, timing.VTimeInPicoSec(20), total.TotalTime())
	t.Log("Subscribed at 20 to task [10,30]: busy, average, and total each report 20; no earlier events replayed")
}
