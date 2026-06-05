// Command tasktree shows how request tasks chain into a tree across a
// component hierarchy.
//
// A Client sends one request down a memory hierarchy: Client -> L1 -> L2 ->
// Memory. Each cache "misses" and forwards the request one level down. The
// trick is the parent task id: when a cache initiates its downstream request,
// it passes tracing.MsgIDAtReceiver(upstreamReq, comp) as the parent — the id
// of the req_in task it is currently handling. That makes every downstream
// task a child of the task that caused it, so the whole hierarchy forms one
// task tree. A custom tracer attached to every component prints that tree.
package main

import (
	"fmt"
	"strings"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/noc/directconnection"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
)

// --- Messages ---

type readReq struct {
	messaging.MsgMeta
}

type readRsp struct {
	messaging.MsgMeta
}

func newReq(src, dst messaging.RemotePort) *readReq {
	return &readReq{MsgMeta: messaging.MsgMeta{
		ID: timing.GetIDGenerator().Generate(), Src: src, Dst: dst}}
}

func newRsp(src, dst messaging.RemotePort, rspTo uint64) *readRsp {
	return &readRsp{MsgMeta: messaging.MsgMeta{
		ID: timing.GetIDGenerator().Generate(), Src: src, Dst: dst, RspTo: rspTo}}
}

// --- Client ---

type clientState struct {
	ReqsToSend int                  `json:"reqs_to_send"`
	Dst        messaging.RemotePort `json:"dst"`
}

type ClientComp = modeling.Component[modeling.None, clientState, modeling.None]

type clientMW struct {
	comp     *ClientComp
	inFlight map[uint64]*readReq
}

func (m *clientMW) Tick() bool {
	p := false
	p = m.receive() || p
	p = m.send() || p
	return p
}

func (m *clientMW) send() bool {
	s := &m.comp.State
	port := m.comp.GetPortByName("Out")
	if s.ReqsToSend == 0 || len(m.inFlight) > 0 || !port.CanSend() {
		return false
	}

	req := newReq(port.AsRemote(), s.Dst)
	tracing.TraceReqInitiate(req, m.comp, 0) // root task, no parent
	port.Send(req)
	m.inFlight[req.ID] = req
	s.ReqsToSend--
	return true
}

func (m *clientMW) receive() bool {
	port := m.comp.GetPortByName("Out")
	msg := port.PeekIncoming()
	if msg == nil {
		return false
	}
	rsp := msg.(*readRsp)
	if req, ok := m.inFlight[rsp.RspTo]; ok {
		tracing.TraceReqFinalize(req, m.comp)
		delete(m.inFlight, rsp.RspTo)
	}
	port.RetrieveIncoming()
	return true
}

// --- Cache (reused for L1 and L2) ---

type cacheState struct {
	DownstreamDst messaging.RemotePort `json:"downstream_dst"`
}

type CacheComp = modeling.Component[modeling.None, cacheState, modeling.None]

type cacheTxn struct {
	upReq   *readReq
	downReq *readReq
}

type cacheMW struct {
	comp *CacheComp
	txns map[uint64]cacheTxn // keyed by downstream request id
}

func (m *cacheMW) Tick() bool {
	p := false
	p = m.forwardDown() || p
	p = m.respondUp() || p
	return p
}

// forwardDown takes an upstream request and, on a "miss", initiates a child
// request to the next level down.
func (m *cacheMW) forwardDown() bool {
	top := m.comp.GetPortByName("Top")
	bottom := m.comp.GetPortByName("Bottom")

	if !bottom.CanSend() {
		return false
	}
	msg := top.PeekIncoming()
	if msg == nil {
		return false
	}
	upReq := msg.(*readReq)

	// Open the handling task for the request we received.
	tracing.TraceReqReceive(upReq, m.comp) // req_in @ this cache

	// Miss: send a request one level down, parented to the task above.
	downReq := newReq(bottom.AsRemote(), m.comp.State.DownstreamDst)
	tracing.TraceReqInitiate(downReq, m.comp, tracing.MsgIDAtReceiver(upReq, m.comp))
	bottom.Send(downReq)

	m.txns[downReq.ID] = cacheTxn{upReq: upReq, downReq: downReq}
	top.RetrieveIncoming()
	return true
}

// respondUp takes a downstream response and answers the original requester.
func (m *cacheMW) respondUp() bool {
	top := m.comp.GetPortByName("Top")
	bottom := m.comp.GetPortByName("Bottom")

	if !top.CanSend() {
		return false
	}
	msg := bottom.PeekIncoming()
	if msg == nil {
		return false
	}
	downRsp := msg.(*readRsp)
	txn := m.txns[downRsp.RspTo]

	tracing.TraceReqFinalize(txn.downReq, m.comp) // close the downstream task

	upRsp := newRsp(top.AsRemote(), txn.upReq.Src, txn.upReq.ID)
	top.Send(upRsp)
	tracing.TraceReqComplete(txn.upReq, m.comp) // close the handling task

	delete(m.txns, downRsp.RspTo)
	bottom.RetrieveIncoming()
	return true
}

// --- Memory (leaf) ---

type MemComp = modeling.Component[modeling.None, modeling.None, modeling.None]

type memMW struct {
	comp *MemComp
}

func (m *memMW) Tick() bool {
	port := m.comp.GetPortByName("Top")
	msg := port.PeekIncoming()
	if msg == nil {
		return false
	}
	if !port.CanSend() {
		return false
	}
	req := msg.(*readReq)

	tracing.TraceReqReceive(req, m.comp) // req_in @ Memory — a leaf task
	port.Send(newRsp(port.AsRemote(), req.Src, req.ID))
	tracing.TraceReqComplete(req, m.comp)
	port.RetrieveIncoming()
	return true
}

// --- A custom tracer that prints the task tree ---

type taskNode struct {
	parent uint64
	kind   string
	loc    string
}

type treeTracer struct {
	order []uint64
	nodes map[uint64]taskNode
}

func (t *treeTracer) StartTask(task tracing.Task) {
	if _, seen := t.nodes[task.ID]; seen {
		return
	}
	t.nodes[task.ID] = taskNode{parent: task.ParentID, kind: task.Kind, loc: task.Location}
	t.order = append(t.order, task.ID)
}

func (t *treeTracer) EndTask(_ tracing.Task)           {}
func (t *treeTracer) StepTask(_ tracing.Task)          {}
func (t *treeTracer) AddMilestone(_ tracing.Milestone) {}

func (t *treeTracer) print() {
	children := make(map[uint64][]uint64)
	for _, id := range t.order {
		children[t.nodes[id].parent] = append(children[t.nodes[id].parent], id)
	}

	var rec func(id uint64, depth int)
	rec = func(id uint64, depth int) {
		n := t.nodes[id]
		fmt.Printf("%s%s @ %s\n", strings.Repeat("  ", depth), n.kind, n.loc)
		for _, c := range children[id] {
			rec(c, depth+1)
		}
	}

	for _, id := range t.order {
		if _, hasParent := t.nodes[t.nodes[id].parent]; !hasParent {
			rec(id, 0) // a root: its parent is not itself a recorded task
		}
	}
}

// --- Wiring ---

func main() {
	engine := timing.NewSerialEngine()
	reg := modeling.NewStandaloneRegistrar(engine)

	client := modeling.NewBuilder[modeling.None, clientState, modeling.None]().
		WithEngine(engine).WithFreq(1 * timing.GHz).Build("Client")
	client.AddMiddleware(&clientMW{comp: client, inFlight: map[uint64]*readReq{}})
	client.AddPort("Out", messaging.NewPort(client, 4, 4, "Client.Out"))
	reg.RegisterComponent(client)

	newCache := func(name string) *CacheComp {
		c := modeling.NewBuilder[modeling.None, cacheState, modeling.None]().
			WithEngine(engine).WithFreq(1 * timing.GHz).Build(name)
		c.AddMiddleware(&cacheMW{comp: c, txns: map[uint64]cacheTxn{}})
		c.AddPort("Top", messaging.NewPort(c, 4, 4, name+".Top"))
		c.AddPort("Bottom", messaging.NewPort(c, 4, 4, name+".Bottom"))
		reg.RegisterComponent(c)
		return c
	}
	l1 := newCache("L1")
	l2 := newCache("L2")

	mem := modeling.NewBuilder[modeling.None, modeling.None, modeling.None]().
		WithEngine(engine).WithFreq(1 * timing.GHz).Build("Memory")
	mem.AddMiddleware(&memMW{comp: mem})
	mem.AddPort("Top", messaging.NewPort(mem, 4, 4, "Memory.Top"))
	reg.RegisterComponent(mem)

	connect := func(name string, a, b messaging.Port) {
		conn := directconnection.MakeBuilder().WithRegistrar(reg).Build(name)
		conn.PlugIn(a)
		conn.PlugIn(b)
	}
	connect("ConnClientL1", client.GetPortByName("Out"), l1.GetPortByName("Top"))
	connect("ConnL1L2", l1.GetPortByName("Bottom"), l2.GetPortByName("Top"))
	connect("ConnL2Mem", l2.GetPortByName("Bottom"), mem.GetPortByName("Top"))

	l1.State.DownstreamDst = l2.GetPortByName("Top").AsRemote()
	l2.State.DownstreamDst = mem.GetPortByName("Top").AsRemote()

	tracer := &treeTracer{nodes: map[uint64]taskNode{}}
	for _, c := range []tracing.NamedHookable{client, l1, l2, mem} {
		tracing.CollectTrace(c, tracer)
	}

	cs := client.State
	cs.Dst = l1.GetPortByName("Top").AsRemote()
	cs.ReqsToSend = 1
	client.State = cs

	client.TickLater()
	if err := engine.Run(); err != nil {
		panic(err)
	}

	tracer.print()
}
