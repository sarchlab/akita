package cache

// A Block of a cache is the information that is associated with a cache line
type Block struct {
	Tag               uint64
	WayID             uint
	SetID             uint
	CacheAddress      uint64
	IsValid           bool
	MostRecentUseTime float64
}

// A Set is a list of blocks where a certain piece memory can be stored at
type Set struct {
	Blocks []*Block
}

// NewSet create a new Set object
func NewSet() *Set {
	set := new(Set)
	set.Blocks = make([]*Block, 0, 0)
	return set
}
