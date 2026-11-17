package pagetablemmu

import "github.com/sarchlab/akita/v5/mem/vm"

const (
	maxLevels = 8 // ceil(64 / 9)
	nodeBytes = 4096
	pteBytes  = 8
	validBit  = 1
	leafBit   = 2
	frameMask = (uint64(1) << 43) - 1
)

func (s Spec) prefix(vpn uint64, level int) uint64 {
	return vpn >> (9 * (len(s.Banks) - level))
}

func (s Spec) pteAddress(table, vpn uint64, level int) uint64 {
	return table + (s.prefix(vpn, level)&511)*pteBytes
}

func (s Spec) inPool(addr uint64) bool {
	return addr >= s.PoolBase && (addr-s.PoolBase)/nodeBytes < s.PoolNodes
}

func (s Spec) pageSize() uint64       { return uint64(1) << s.Log2PageSize }
func (s Spec) validVA(va uint64) bool { return va>>s.VABits == 0 }

// Leaves store frame numbers to keep device identity and vm.Page attributes in
// the same 8-byte memory word. Non-leaf pointers use the full physical address.
func (s Spec) encodeLeaf(p vm.Page) uint64 {
	word := (p.PAddr>>s.Log2PageSize)<<5 | p.DeviceID<<48 | validBit | leafBit
	if p.Unified {
		word |= 4
	}
	if p.IsMigrating {
		word |= 8
	}
	if p.IsPinned {
		word |= 16
	}
	return word
}

func (s Spec) decodeLeaf(word uint64, pid vm.PID, vpn uint64) vm.Page {
	return vm.Page{
		PID:         pid,
		VAddr:       vpn << s.Log2PageSize,
		PAddr:       ((word >> 5) & frameMask) << s.Log2PageSize,
		Valid:       true,
		PageSize:    s.pageSize(),
		DeviceID:    word >> 48,
		Unified:     word&4 != 0,
		IsMigrating: word&8 != 0,
		IsPinned:    word&16 != 0,
	}
}

type prefixEntry struct {
	PID    vm.PID
	Prefix uint64
	Table  uint64
}

// Each set stores MRU first. A bank contains only non-leaf pointers.
type prefixBank [][]prefixEntry

func (b prefixBank) lookup(pid vm.PID, prefix uint64) (uint64, bool) {
	if len(b) == 0 {
		return 0, false
	}
	set := b[prefix%uint64(len(b))]
	for i, e := range set {
		if e.PID == pid && e.Prefix == prefix {
			copy(set[1:i+1], set[:i])
			set[0] = e
			return e.Table, true
		}
	}
	return 0, false
}

func (b prefixBank) fill(pid vm.PID, prefix, table uint64, ways int) {
	if len(b) == 0 {
		return
	}
	index := prefix % uint64(len(b))
	if _, ok := b.lookup(pid, prefix); ok {
		b[index][0].Table = table
		return
	}
	set := b[index]
	if len(set) < ways {
		set = append(set, prefixEntry{})
	}
	copy(set[1:], set[:len(set)-1])
	set[0] = prefixEntry{PID: pid, Prefix: prefix, Table: table}
	b[index] = set
}

func (b prefixBank) invalidate(pid vm.PID, first, last uint64) {
	for i, set := range b {
		keep := set[:0]
		for _, entry := range set {
			if entry.PID != pid || entry.Prefix < first || entry.Prefix > last {
				keep = append(keep, entry)
			}
		}
		b[i] = keep
	}
}
