package cache

// A Block of a cache is the information that is associated with a cache line
type Block struct {
	Tag     uint64
	WayID   uint
	IsValid bool
}

// A Set is a list of blocks where a certain piece memory be stored at
type Set struct {
	blocks []Block
}
