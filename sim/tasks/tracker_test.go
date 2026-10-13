package tasks_test

import (
	"fmt"
	"runtime"
	"sync"
	"testing"

	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
	"sync/atomic"
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
	o := newOwner("off", nil, nil)
	o.tracker.RegisterPort(nil)
	o.tracker.FreezeSubscriptions()
	require.Zero(t, tasks.Start(o, tasks.StartInfo{}))
	require.Zero(t, tasks.StartReqOut(o, messaging.Msg{}, 0))
	require.Zero(t, tasks.StartReqIn(o, messaging.Msg{}))
	tasks.End(o, tasks.EndInfo{ID: 1})
	tasks.EndReqIn(o, 1)
	tasks.EndReqOut(o, 1)
	tasks.AddTag(o, tasks.Tag{TaskID: 1})
	tasks.AddMilestone(o, tasks.Milestone{TaskID: 1})
	tasks.AddBufferMilestone(o, nil, messaging.Msg{}, tasks.Milestone{})
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
			ids := &atomic.Uint64{}
			o := newOwner("Owner", tr, ids)
			p := port(o)
			r := &recorder{}
			tr.Subscribe(r, o)
			enqueue := p.Send
			retrieve := p.RetrieveOutgoing
			if incoming {
				enqueue = p.Deliver
				retrieve = p.RetrieveIncoming
			}
			a := messaging.Msg{ID: o.NewID(), Src: p.AsRemote(), Dst: "Remote"}
			b := a
			b.ID = o.NewID()
			enqueue(a) // Equivalent to a restored, untracked buffered message.
			tr.RegisterPort(p)
			enqueue(b)
			require.Len(t, r.starts, 1)
			require.Empty(t, r.milestones)
			tasks.AddBufferMilestone(o, p, a, tasks.Milestone{What: "untracked"})
			require.Empty(t, r.milestones)
			got, ok := retrieve()
			require.True(t, ok)
			require.Equal(t, a.ID, got.ID)
			require.Empty(t, r.ends)
			require.Len(t, r.milestones, 1)
			require.Equal(t, r.starts[0].ID, r.milestones[0].TaskID)
			if incoming {
				tasks.AddBufferMilestone(o, p, b, tasks.Milestone{What: "admitted"})
				require.Len(t, r.milestones, 2)
			}
			retrieve()
			require.Len(t, r.ends, 1)
			require.Equal(t, b.ID, r.ends[0].MsgID)
		})
	}
}

func TestSubscriptionScopeAndFreeze(t *testing.T) {
	tr := tasks.NewTracker()
	other := tasks.NewTracker()
	ids := &atomic.Uint64{}
	require.Panics(t, func() { tr.Subscribe(&recorder{}, newOwner("foreign", other, ids)) })
	tr.FreezeSubscriptions()
	require.Panics(t, func() { tr.Subscribe(&recorder{}) })
	require.Panics(t, func() { tr.RegisterPort(port(newOwner("late", tr, ids))) })
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
		tr.RegisterPort(p)
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				msg := messaging.Msg{ID: o.NewID()}
				id := tasks.StartReqOut(o, msg, 0)
				p.Deliver(msg)
				tasks.AddBufferMilestone(o, p, msg, tasks.Milestone{What: "admission"})
				p.RetrieveIncoming()
				tasks.AddTag(o, tasks.Tag{TaskID: id, What: "done"})
				tasks.EndReqOut(o, id)
			}
		}()
	}
	wg.Wait()
	require.Len(t, r.ends, 1600)
	for _, order := range r.order {
		require.Contains(t, []string{"STE", "SMME"}, order)
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
	tasks.AddBufferMilestone(h.o, h.p, msg, tasks.Milestone{What: "admission"})
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
		tr.RegisterPort(p)
		engine.RegisterHandler(o.name, taskHandler{o, p})
		engine.Schedule(timing.MakeEventBase(o.NewID(), 10, o.name))
	}
	require.NoError(t, engine.Run())
	require.Len(t, r.starts, 64)
	require.Len(t, r.ends, 64)
	for _, order := range r.order {
		require.Contains(t, []string{"SE", "SMME"}, order)
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
	tr.RegisterPort(p)
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
		var msg messaging.Msg
		var ok bool
		for {
			msg, ok = p.PeekIncoming()
			if ok {
				break
			}
			runtime.Gosched()
		}
		tasks.AddBufferMilestone(o, p, msg, tasks.Milestone{What: "admission"})
		p.RetrieveIncoming()
	}
	wg.Wait()
	require.Len(t, r.ends, 1000)
	for _, order := range r.order {
		require.Equal(t, "SMME", order)
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

type factRecorder struct{ changes []messaging.PortBufferChange }

func (r *factRecorder) Func(ctx hooking.HookCtx) {
	r.changes = append(r.changes, ctx.Detail.(messaging.PortBufferChange))
}

func TestPortHookReportsActualHead(t *testing.T) {
	o := newOwner("Owner", nil, &atomic.Uint64{})
	p := port(o)
	facts := &factRecorder{}
	p.AcceptHook(facts)
	a, b := messaging.Msg{ID: 1}, messaging.Msg{ID: 2}
	p.Deliver(a)
	p.Deliver(b)
	p.RetrieveIncoming()
	p.RetrieveIncoming()
	require.Equal(t, []messaging.PortBufferChange{
		{Head: a, HasHead: true, HeadChanged: true},
		{Head: a, HasHead: true, HeadChanged: false},
		{Head: b, HasHead: true, HeadChanged: true},
		{HeadChanged: true},
	}, facts.changes)
}
