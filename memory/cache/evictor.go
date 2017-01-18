package cache

// A Evictor decides with block should be evicted
type Evictor interface {
	Evict(set Set) Block
}
