// Command reqtracing shows the four-stage request lifecycle.
//
// A Client sends requests to a Server, one at a time, over a direct
// connection; the Server replies after a fixed latency. Both sides annotate
// the request with the tracing.TraceReq* helpers:
//
//	Client send : TraceReqInitiate  -> opens a "req_out" task (round trip)
//	Server recv : TraceReqReceive   -> opens a "req_in"  task (handling)
//	Server done : TraceReqComplete  -> ends the "req_in"  task
//	Client recv : TraceReqFinalize  -> ends the "req_out" task
//
// Two AverageTimeTracers — one filtering "req_out" on the client, one
// filtering "req_in" on the server — then report the round-trip latency and
// the server handling time from the same run.
package main

import (
	"fmt"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/noc/directconnection"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
)

// --- Messages ---

type readReq struct {
	messaging.MsgMeta
	Seq int
}

type readRsp struct {
	messaging.MsgMeta
	Seq int
}

// readProtocol is the read protocol: the client requests reads and the server
// responds. Defining the protocol registers the message types with the
// checkpoint codec.
var (
	readProtocol = messaging.DefineProtocol("examples.reqtracing",
		messaging.RoleDef{Name: "requester",
			Sends: []messaging.Msg{readReq{}}},
		messaging.RoleDef{Name: "responder",
			Sends: []messaging.Msg{readRsp{}}},
	)
	readRequester = readProtocol.Role("requester")
	readResponder = readProtocol.Role("responder")
)

// --- Client ---

type clientSpec struct {
	Freq timing.Freq `json:"freq"`
}

type clientState struct {
	ReqsToSend int                  `json:"reqs_to_send"`
	NextSeq    int                  `json:"next_seq"`
	Dst        messaging.RemotePort `json:"dst"`
}

// ClientComp is the requesting component.
type ClientComp = modeling.Component[clientSpec, clientState, modeling.None]

type clientMW struct {
	comp     *ClientComp
	inFlight map[uint64]readReq
}

func (m *clientMW) Tick() bool {
	progress := false
	progress = m.receive() || progress
	progress = m.send() || progress
	return progress
}

func (m *clientMW) send() bool {
	s := &m.comp.State
	port := m.comp.GetPortByName("Out")

	// Send one request at a time: wait for the response before the next.
	if s.ReqsToSend == 0 || len(m.inFlight) > 0 || !port.CanSend() {
		return false
	}

	req := readReq{
		MsgMeta: messaging.MsgMeta{
			ID:  timing.GetIDGenerator().Generate(),
			Src: port.AsRemote(),
			Dst: s.Dst,
		},
		Seq: s.NextSeq,
	}

	// The req_out task is keyed by the request's own message ID.
	tracing.TraceReqInitiate(m.comp, req, 0)
	port.Send(req)

	m.inFlight[req.ID] = req
	s.ReqsToSend--
	s.NextSeq++

	return true
}

func (m *clientMW) receive() bool {
	port := m.comp.GetPortByName("Out")

	msg := port.PeekIncoming()
	if msg == nil {
		return false
	}

	rsp := msg.(readRsp)
	if req, ok := m.inFlight[rsp.RspTo]; ok {
		tracing.TraceReqFinalize(m.comp, req)
		delete(m.inFlight, rsp.RspTo)
	}
	port.RetrieveIncoming()

	return true
}

// --- Server ---

type serverSpec struct {
	Freq    timing.Freq `json:"freq"`
	Latency int         `json:"latency"`
}

type serverState struct{}

// ServerComp is the responding component.
type ServerComp = modeling.Component[serverSpec, serverState, modeling.None]

type serverTxn struct {
	req  readReq
	left int
}

type serverMW struct {
	comp    *ServerComp
	pending []serverTxn
}

func (m *serverMW) Tick() bool {
	progress := false
	progress = m.respond() || progress
	progress = m.countDown() || progress
	progress = m.receive() || progress
	return progress
}

func (m *serverMW) receive() bool {
	port := m.comp.GetPortByName("Out")

	msg := port.PeekIncoming()
	if msg == nil {
		return false
	}

	req := msg.(readReq)
	tracing.TraceReqReceive(m.comp, req)
	m.pending = append(m.pending, serverTxn{req: req, left: m.comp.Spec().Latency})
	port.RetrieveIncoming()

	return true
}

func (m *serverMW) countDown() bool {
	progress := false
	for i := range m.pending {
		if m.pending[i].left > 0 {
			m.pending[i].left--
			progress = true
		}
	}
	return progress
}

func (m *serverMW) respond() bool {
	if len(m.pending) == 0 || m.pending[0].left > 0 {
		return false
	}

	port := m.comp.GetPortByName("Out")
	if !port.CanSend() {
		return false
	}

	txn := m.pending[0]
	port.Send(readRsp{
		MsgMeta: messaging.MsgMeta{
			ID:    timing.GetIDGenerator().Generate(),
			Src:   port.AsRemote(),
			Dst:   txn.req.Src,
			RspTo: txn.req.ID,
		},
		Seq: txn.req.Seq,
	})

	tracing.TraceReqComplete(m.comp, txn.req)
	m.pending = m.pending[1:]

	return true
}

// --- Wiring ---

func main() {
	engine := timing.NewSerialEngine()
	registrar := modeling.NewStandaloneRegistrar(engine)

	client := modeling.NewBuilder[clientSpec, clientState, modeling.None]().
		WithEngine(engine).
		WithFreq(1 * timing.GHz).
		WithSpec(clientSpec{Freq: 1 * timing.GHz}).
		Build("Client")
	client.AddMiddleware(&clientMW{comp: client, inFlight: make(map[uint64]readReq)})
	client.DeclarePort("Out", readRequester)
	client.AssignPort("Out", messaging.NewPort(client, 4, 4, "Client.Out"))
	registrar.RegisterComponent(client)

	server := modeling.NewBuilder[serverSpec, serverState, modeling.None]().
		WithEngine(engine).
		WithFreq(1 * timing.GHz).
		WithSpec(serverSpec{Freq: 1 * timing.GHz, Latency: 4}).
		Build("Server")
	server.AddMiddleware(&serverMW{comp: server})
	server.DeclarePort("Out", readResponder)
	server.AssignPort("Out", messaging.NewPort(server, 4, 4, "Server.Out"))
	registrar.RegisterComponent(server)

	conn := directconnection.MakeBuilder().WithRegistrar(registrar).Build("Conn")
	conn.PlugIn(client.GetPortByName("Out"))
	conn.PlugIn(server.GetPortByName("Out"))

	// The filter is how each tracer selects the tasks it cares about.
	roundTrip := tracing.NewAverageTimeTracer(
		func(t tracing.TaskStart) bool { return t.Kind == "req_out" })
	handling := tracing.NewAverageTimeTracer(
		func(t tracing.TaskStart) bool { return t.Kind == "req_in" })
	tracing.CollectTrace(client, roundTrip)
	tracing.CollectTrace(server, handling)

	cs := client.State
	cs.Dst = server.GetPortByName("Out").AsRemote()
	cs.ReqsToSend = 3
	client.State = cs

	client.TickLater()

	if err := engine.Run(); err != nil {
		panic(err)
	}

	fmt.Printf("requests completed:           %d\n", roundTrip.TotalCount())
	fmt.Printf("avg round trip (req_out):     %d ps\n", roundTrip.AverageTime())
	fmt.Printf("avg server handling (req_in): %d ps\n", handling.AverageTime())
}
