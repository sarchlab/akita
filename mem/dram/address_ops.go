package dram

// convertExternalToInternal converts a global physical address to a
// DRAM-internal physical address using interleaving parameters from Spec.
func convertExternalToInternal(spec *Spec, addr uint64) uint64 {
	if !spec.HasAddrConverter {
		return addr
	}

	unitSize := spec.InterleavingSize
	totalUnits := uint64(spec.TotalNumOfElements)
	currentIdx := uint64(spec.CurrentElementIndex)
	offset := spec.Offset

	// InterleavingConverter logic:
	// externalAddr = offset + highBits*totalUnits*unitSize + currentIdx*unitSize + lowBits
	// internalAddr = highBits*unitSize + lowBits
	addrAfterOffset := addr - offset
	lowBits := addrAfterOffset % unitSize
	highBits := (addrAfterOffset - currentIdx*unitSize - lowBits) / (totalUnits * unitSize)

	return highBits*unitSize + lowBits
}

// mapAddress maps an internal address to a Location (channel, rank, etc.)
// using the address mapping parameters in Spec.
func mapAddress(spec *Spec, addr uint64) Location {
	l := Location{}

	l.Channel = (addr >> spec.ChannelPos) & spec.ChannelMask
	l.Rank = (addr >> spec.RankPos) & spec.RankMask
	l.BankGroup = (addr >> spec.BankGroupPos) & spec.BankGroupMask
	l.Bank = (addr >> spec.BankPos) & spec.BankMask
	l.Row = (addr >> spec.RowPos) & spec.RowMask
	l.Column = (addr >> spec.ColPos) & spec.ColMask

	return l
}
