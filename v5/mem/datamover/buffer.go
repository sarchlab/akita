package datamover

type buffer struct {
	offset      uint64
	granularity uint64
	data        [][]byte
}

func (b *buffer) addData(addr uint64, data []byte) {
	addressMustBeAligned(addr, b.granularity)

	slot := (addr - b.offset) / b.granularity
	for i := uint64(len(b.data)); i <= slot; i++ {
		b.data = append(b.data, nil)
	}

	b.data[slot] = data
}

func (b *buffer) extractData(addr, size uint64) (data []byte, ok bool) {
	data = make([]byte, size)

	sizeLeft := size
	slot := (addr - b.offset) / b.granularity
	offset := addr - b.offset - slot*b.granularity

	for i := slot; i < uint64(len(b.data)); i++ {
		if b.data[i] == nil {
			return nil, false
		}

		bytesToCopy := b.granularity - offset
		if sizeLeft < bytesToCopy {
			bytesToCopy = sizeLeft
		}

		copy(data[size-sizeLeft:], b.data[i][offset:offset+bytesToCopy])
		sizeLeft -= bytesToCopy
		offset = 0

		if sizeLeft == 0 {
			return data, true
		}
	}

	return nil, false
}

func (b *buffer) moveOffsetForwardTo(newStart uint64) {
	alignedNewStart := (newStart / b.granularity) * b.granularity

	if alignedNewStart <= b.offset {
		return
	}

	discardChunks := (alignedNewStart - b.offset) / b.granularity
	if discardChunks > uint64(len(b.data)) {
		b.data = b.data[:0]
	} else {
		b.data = b.data[discardChunks:]
	}

	b.offset = alignedNewStart
}
