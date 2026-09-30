package dram

// addrMapping is the bit layout that decodes a global physical address into a
// location: the position and mask of each field. It is derived from the
// geometry in Spec by newAddrMapping.
type addrMapping struct {
	channelPos    int
	channelMask   uint64
	rankPos       int
	rankMask      uint64
	bankGroupPos  int
	bankGroupMask uint64
	bankPos       int
	bankMask      uint64
	rowPos        int
	rowMask       uint64
	colPos        int
	colMask       uint64
}

// newAddrMapping derives the address mapping from the geometry in spec
// (NumChannel, NumRank, NumBankGroup, NumBank, NumRow, NumCol, BusWidth, and
// BurstLength). The access unit occupies the lowest bits; above it, from low
// to high: column, bank group, bank, rank, channel, row.
func newAddrMapping(spec *Spec) addrMapping {
	channelBit, _ := log2(uint64(spec.NumChannel))
	rankBit, _ := log2(uint64(spec.NumRank))
	bankGroupBit, _ := log2(uint64(spec.NumBankGroup))
	bankBit, _ := log2(uint64(spec.NumBank))
	rowBit, _ := log2(uint64(spec.NumRow))
	colBit, _ := log2(uint64(spec.NumCol))
	colLoBit, _ := log2(uint64(spec.BurstLength))
	colHiBit := colBit - colLoBit

	m := addrMapping{
		channelMask:   (1 << channelBit) - 1,
		rankMask:      (1 << rankBit) - 1,
		bankGroupMask: (1 << bankGroupBit) - 1,
		bankMask:      (1 << bankBit) - 1,
		rowMask:       (1 << rowBit) - 1,
		colMask:       (1 << colHiBit) - 1,
	}

	bitWidths := addrBitWidths{
		channel:   channelBit,
		rank:      rankBit,
		bankGroup: bankGroupBit,
		bank:      bankBit,
		row:       rowBit,
		colHi:     colHiBit,
	}

	m.assignBitPositions(spec.log2AccessUnitSize(), bitWidths)

	return m
}

type addrBitWidths struct {
	channel, rank, bankGroup, bank, row, colHi uint64
}

func (m *addrMapping) assignBitPositions(
	startPos uint64, w addrBitWidths,
) {
	// Default bit order high→low: Row, Channel, Rank, Bank, BankGroup, Column
	type locItem int
	const (
		liChannel locItem = iota
		liRank
		liBankGroup
		liBank
		liRow
		liColumn
	)

	bitOrder := []locItem{
		liRow, liChannel, liRank, liBank, liBankGroup, liColumn,
	}

	pos := startPos
	for i := len(bitOrder) - 1; i >= 0; i-- {
		switch bitOrder[i] {
		case liChannel:
			m.channelPos = int(pos)
			pos += w.channel
		case liRank:
			m.rankPos = int(pos)
			pos += w.rank
		case liBankGroup:
			m.bankGroupPos = int(pos)
			pos += w.bankGroup
		case liBank:
			m.bankPos = int(pos)
			pos += w.bank
		case liRow:
			m.rowPos = int(pos)
			pos += w.row
		case liColumn:
			m.colPos = int(pos)
			pos += w.colHi
		}
	}
}

// mapAddress decomposes a global physical address into a location (channel,
// rank, bank group, bank, row, column). Storage is global, so this operates
// directly on the request address.
func (m addrMapping) mapAddress(addr uint64) location {
	l := location{}

	l.Channel = (addr >> m.channelPos) & m.channelMask
	l.Rank = (addr >> m.rankPos) & m.rankMask
	l.BankGroup = (addr >> m.bankGroupPos) & m.bankGroupMask
	l.Bank = (addr >> m.bankPos) & m.bankMask
	l.Row = (addr >> m.rowPos) & m.rowMask
	l.Column = (addr >> m.colPos) & m.colMask

	return l
}

// log2 returns the log2 of a number. It also returns false if it is not a log2
// number.
func log2(n uint64) (uint64, bool) {
	oneCount := 0
	onePos := uint64(0)

	for i := range uint64(64) {
		if n&(1<<i) > 0 {
			onePos = i
			oneCount++
		}
	}

	return onePos, oneCount == 1
}
