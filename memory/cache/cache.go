package cache

// A Cache is a storage that is that is managed in sets and blocks.
//
// The Cache class does not responsible for the cache timing and the coherency
// of the cache. It only responsible for what data is stored in the cache.
type Cache struct {
	NumSets   uint
	NumWays   uint
	BlockSize uint
}
