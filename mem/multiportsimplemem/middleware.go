package multiportsimplemem

import (
	"log"
	"sort"

	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/tracing"
)

type pendingRequest struct {
	req          mem.AccessReq
	port         sim.Port
	arrivalOrder uint64
}

type activeRequest struct {
	*pendingRequest
	remainingCycles int
	serviceIndex    uint64
}

type pendingResponse struct {
	port      sim.Port
	msg       sim.Msg
	original  mem.AccessReq
	committed bool
}

type middleware struct {
	*Comp
}

func (m *middleware) Tick() bool {
	madeProgress := false

	madeProgress = m.sendPendingResponses() || madeProgress
	madeProgress = m.advanceActiveRequests() || madeProgress
	madeProgress = m.collectIncomingRequests() || madeProgress
	madeProgress = m.dispatchWaitingRequests() || madeProgress

	return madeProgress
}

func (m *middleware) sendPendingResponses() bool {
	if len(m.pendingResponses) == 0 {
		return false
	}

	remaining := m.pendingResponses[:0]
	madeProgress := false

	for _, item := range m.pendingResponses {
		if err := item.port.Send(item.msg); err != nil {
			remaining = append(remaining, item)
			continue
		}

		tracing.TraceReqComplete(item.original, m)
		madeProgress = true
	}

	m.pendingResponses = remaining

	return madeProgress
}

func (m *middleware) advanceActiveRequests() bool {
	if len(m.activeRequests) == 0 {
		return false
	}

	completed := make([]*activeRequest, 0)

	for _, req := range m.activeRequests {
		req.remainingCycles--
		if req.remainingCycles <= 0 {
			completed = append(completed, req)
		}
	}

	if len(completed) == 0 {
		return false
	}

	remaining := m.activeRequests[:0]
	for _, req := range m.activeRequests {
		if req.remainingCycles > 0 {
			remaining = append(remaining, req)
		}
	}
	m.activeRequests = remaining

	sort.Slice(completed, func(i, j int) bool {
		if completed[i].serviceIndex == completed[j].serviceIndex {
			return completed[i].arrivalOrder < completed[j].arrivalOrder
		}

		return completed[i].serviceIndex < completed[j].serviceIndex
	})

	for _, req := range completed {
		m.finishRequest(req)
	}

	return true
}

func (m *middleware) finishRequest(req *activeRequest) {
	switch typed := req.req.(type) {
	case *mem.ReadReq:
		m.handleReadCompletion(req.port, typed)
	case *mem.WriteReq:
		m.handleWriteCompletion(req.port, typed)
	default:
		log.Panicf("multiportsimplemem: unsupported request type %T", req.req)
	}
}

func (m *middleware) handleReadCompletion(port sim.Port, req *mem.ReadReq) {
	addr := req.Address
	if m.addressConverter != nil {
		addr = m.addressConverter.ConvertExternalToInternal(addr)
	}

	data, err := m.Storage.Read(addr, req.AccessByteSize)
	if err != nil {
		log.Panic(err)
	}

	rsp := mem.DataReadyRspBuilder{}.
		WithSrc(port.AsRemote()).
		WithDst(req.Src).
		WithRspTo(req.ID).
		WithData(data).
		Build()

	if err := port.Send(rsp); err != nil {
		m.pendingResponses = append(m.pendingResponses, &pendingResponse{
			port:     port,
			msg:      rsp,
			original: req,
		})
		return
	}

	tracing.TraceReqComplete(req, m)
}

func (m *middleware) handleWriteCompletion(port sim.Port, req *mem.WriteReq) {
	addr := req.Address
	if m.addressConverter != nil {
		addr = m.addressConverter.ConvertExternalToInternal(addr)
	}

	if req.DirtyMask == nil {
		if err := m.Storage.Write(addr, req.Data); err != nil {
			log.Panic(err)
		}
	} else {
		existing, err := m.Storage.Read(addr, uint64(len(req.Data)))
		if err != nil {
			log.Panic(err)
		}

		for i := 0; i < len(req.Data); i++ {
			if req.DirtyMask[i] {
				existing[i] = req.Data[i]
			}
		}

		if err := m.Storage.Write(addr, existing); err != nil {
			log.Panic(err)
		}
	}

	rsp := mem.WriteDoneRspBuilder{}.
		WithSrc(port.AsRemote()).
		WithDst(req.Src).
		WithRspTo(req.ID).
		Build()

	if err := port.Send(rsp); err != nil {
		m.pendingResponses = append(m.pendingResponses, &pendingResponse{
			port:     port,
			msg:      rsp,
			original: req,
		})
		return
	}

	tracing.TraceReqComplete(req, m)
}

func (m *middleware) collectIncomingRequests() bool {
	madeProgress := false

	for _, port := range m.topPorts {
		for {
			item := port.PeekIncoming()
			if item == nil {
				break
			}

			req, ok := item.(mem.AccessReq)
			if !ok {
				log.Panicf("multiportsimplemem: unsupported message type %T", item)
			}

			port.RetrieveIncoming()

			order := m.arrivalOrderOf(req)
			m.removeArrivalRecord(req)

			m.waitingRequests = append(m.waitingRequests, &pendingRequest{
				req:          req,
				port:         port,
				arrivalOrder: order,
			})

			madeProgress = true
		}
	}

	return madeProgress
}

func (m *middleware) dispatchWaitingRequests() bool {
	if len(m.waitingRequests) == 0 {
		return false
	}

	availableSlots := m.ConcurrentSlots - len(m.activeRequests)
	if availableSlots <= 0 {
		return false
	}

	madeProgress := false

	for availableSlots > 0 && len(m.waitingRequests) > 0 {
		index := m.nextWaitingRequestIndex()
		pending := m.waitingRequests[index]
		m.waitingRequests = append(m.waitingRequests[:index],
			m.waitingRequests[index+1:]...)

		active := &activeRequest{
			pendingRequest: pending,
			remainingCycles: func() int {
				if m.Latency <= 0 {
					return 0
				}
				return m.Latency
			}(),
			serviceIndex: m.nextServiceIndex,
		}
		m.nextServiceIndex++

		m.activeRequests = append(m.activeRequests, active)
		availableSlots--
		madeProgress = true

		tracing.TraceReqReceive(pending.req, m)
	}

	return madeProgress
}

func (m *middleware) nextWaitingRequestIndex() int {
	bestIdx := 0
	bestOrder := m.waitingRequests[0].arrivalOrder

	for i := 1; i < len(m.waitingRequests); i++ {
		if m.waitingRequests[i].arrivalOrder < bestOrder {
			bestIdx = i
			bestOrder = m.waitingRequests[i].arrivalOrder
		}
	}

	return bestIdx
}
