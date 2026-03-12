package datamover

import (
	"log"

	"github.com/sarchlab/akita/v5/sim"
)

// Spec contains immutable configuration for the data mover.
type Spec struct {
	BufferSize             uint64 `json:"buffer_size"`
	InsideByteGranularity  uint64 `json:"inside_byte_granularity"`
	OutsideByteGranularity uint64 `json:"outside_byte_granularity"`

	InsideMapperKind             string           `json:"inside_mapper_kind"`
	InsideMapperPorts            []sim.RemotePort `json:"inside_mapper_ports"`
	InsideMapperInterleavingSize uint64           `json:"inside_mapper_interleaving_size"`

	OutsideMapperKind             string           `json:"outside_mapper_kind"`
	OutsideMapperPorts            []sim.RemotePort `json:"outside_mapper_ports"`
	OutsideMapperInterleavingSize uint64           `json:"outside_mapper_interleaving_size"`
}

// dataChunk wraps a single []byte slot. This avoids [][]byte which fails
// ValidateState. Valid distinguishes a nil slot from an empty one.
type dataChunk struct {
	Data  []byte `json:"data"`
	Valid bool   `json:"valid"`
}

// bufferState is the serializable representation of a buffer.
type bufferState struct {
	Offset      uint64      `json:"offset"`
	Granularity uint64      `json:"granularity"`
	Chunks      []dataChunk `json:"chunks"`
}

// pendingReadState captures the serializable fields of a pending read request.
type pendingReadState struct {
	ID      string         `json:"id"`
	Src     sim.RemotePort `json:"src"`
	Dst     sim.RemotePort `json:"dst"`
	Address uint64         `json:"address"`
}

// pendingWriteState captures the serializable fields of a pending write request.
type pendingWriteState struct {
	ID      string         `json:"id"`
	Src     sim.RemotePort `json:"src"`
	Dst     sim.RemotePort `json:"dst"`
	Address uint64         `json:"address"`
	Data    []byte         `json:"data"`
}

// dataMoverTransactionState is the serializable representation of a
// dataMoverTransaction.
type dataMoverTransactionState struct {
	ReqID         string                       `json:"req_id"`
	ReqSrc        sim.RemotePort               `json:"req_src"`
	ReqDst        sim.RemotePort               `json:"req_dst"`
	SrcAddress    uint64                       `json:"src_address"`
	DstAddress    uint64                       `json:"dst_address"`
	ByteSize      uint64                       `json:"byte_size"`
	SrcSide       string                       `json:"src_side"`
	DstSide       string                       `json:"dst_side"`
	NextReadAddr  uint64                       `json:"next_read_addr"`
	NextWriteAddr uint64                       `json:"next_write_addr"`
	PendingRead   map[string]pendingReadState  `json:"pending_read"`
	PendingWrite  map[string]pendingWriteState `json:"pending_write"`
	Active        bool                         `json:"active"`
}

// State contains mutable runtime data for the data mover.
type State struct {
	CurrentTransaction dataMoverTransactionState `json:"current_transaction"`
	Buffer             bufferState               `json:"buffer"`
	SrcByteGranularity uint64                    `json:"src_byte_granularity"`
	DstByteGranularity uint64                    `json:"dst_byte_granularity"`
	SrcSide            string                    `json:"src_side"`
	DstSide            string                    `json:"dst_side"`
}

func alignAddress(addr, granularity uint64) uint64 {
	return addr / granularity * granularity
}

func addressMustBeAligned(addr, granularity uint64) {
	if addr%granularity != 0 {
		log.Panicf("address %d must be aligned to %d", addr, granularity)
	}
}

// findPort resolves a port mapper lookup from Spec fields.
func findPort(
	kind string,
	ports []sim.RemotePort,
	interleavingSize uint64,
	addr uint64,
) sim.RemotePort {
	switch kind {
	case "single":
		return ports[0]
	case "interleaved":
		number := addr / interleavingSize % uint64(len(ports))
		return ports[number]
	default:
		log.Panicf("unknown mapper kind %q", kind)
		return ""
	}
}

// bufferAddData adds data to the buffer at the given offset.
func bufferAddData(bs *bufferState, offset uint64, data []byte) {
	addressMustBeAligned(offset, bs.Granularity)

	slot := (offset - bs.Offset) / bs.Granularity
	for i := uint64(len(bs.Chunks)); i <= slot; i++ {
		bs.Chunks = append(bs.Chunks, dataChunk{Valid: false})
	}

	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)
	bs.Chunks[slot] = dataChunk{Data: dataCopy, Valid: true}
}

// bufferExtractData extracts data from the buffer at the given offset.
func bufferExtractData(
	bs *bufferState, offset, size uint64,
) ([]byte, bool) {
	data := make([]byte, size)

	sizeLeft := size
	relOffset := offset - bs.Offset
	slot := relOffset / bs.Granularity
	slotOffset := relOffset - slot*bs.Granularity

	for i := slot; i < uint64(len(bs.Chunks)); i++ {
		if !bs.Chunks[i].Valid {
			return nil, false
		}

		bytesToCopy := min(bs.Granularity-slotOffset, sizeLeft)

		copy(data[size-sizeLeft:],
			bs.Chunks[i].Data[slotOffset:slotOffset+bytesToCopy])
		sizeLeft -= bytesToCopy
		slotOffset = 0

		if sizeLeft == 0 {
			return data, true
		}
	}

	return nil, false
}

// bufferMoveOffsetForwardTo moves the buffer offset forward, discarding chunks.
func bufferMoveOffsetForwardTo(bs *bufferState, newOffset uint64) {
	alignedNewStart := (newOffset / bs.Granularity) * bs.Granularity

	if alignedNewStart <= bs.Offset {
		return
	}

	discardChunks := (alignedNewStart - bs.Offset) / bs.Granularity
	if discardChunks > uint64(len(bs.Chunks)) {
		bs.Chunks = bs.Chunks[:0]
	} else {
		bs.Chunks = bs.Chunks[discardChunks:]
	}

	bs.Offset = alignedNewStart
}

// resolveByteGranularity returns the byte granularity for a given port side.
func resolveByteGranularity(spec Spec, side DateMovePort) uint64 {
	switch side {
	case "inside":
		return spec.InsideByteGranularity
	case "outside":
		return spec.OutsideByteGranularity
	default:
		log.Panicf("can't process port side %s", side)
		return 0
	}
}

// transactionAsMsg creates a temporary DataMoveRequest for tracing purposes.
func transactionAsMsg(
	trans *dataMoverTransactionState,
) *DataMoveRequest {
	req := &DataMoveRequest{
		SrcAddress: trans.SrcAddress,
		DstAddress: trans.DstAddress,
		ByteSize:   trans.ByteSize,
		SrcSide:    DateMovePort(trans.SrcSide),
		DstSide:    DateMovePort(trans.DstSide),
	}
	req.ID = trans.ReqID
	req.Src = trans.ReqSrc
	req.Dst = trans.ReqDst
	return req
}
