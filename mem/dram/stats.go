package dram

// RowBufferHitRate returns the row-buffer hit rate of c (0.0 to 1.0).
func RowBufferHitRate(c *Comp) float64 {
	s := &c.State
	total := s.RowBufferHits + s.RowBufferMisses
	if total == 0 {
		return 0
	}
	return float64(s.RowBufferHits) / float64(total)
}

// AverageReadLatency returns the average read latency of c in cycles.
func AverageReadLatency(c *Comp) float64 {
	s := &c.State
	if s.CompletedReads == 0 {
		return 0
	}
	return float64(s.TotalReadLatencyCycles) / float64(s.CompletedReads)
}

// AverageWriteLatency returns the average write latency of c in cycles.
func AverageWriteLatency(c *Comp) float64 {
	s := &c.State
	if s.CompletedWrites == 0 {
		return 0
	}
	return float64(s.TotalWriteLatencyCycles) / float64(s.CompletedWrites)
}

// ReadBandwidth returns the read bandwidth of c in bytes per cycle.
func ReadBandwidth(c *Comp) float64 {
	s := &c.State
	if s.TotalCycles == 0 {
		return 0
	}
	return float64(s.BytesRead) / float64(s.TotalCycles)
}

// WriteBandwidth returns the write bandwidth of c in bytes per cycle.
func WriteBandwidth(c *Comp) float64 {
	s := &c.State
	if s.TotalCycles == 0 {
		return 0
	}
	return float64(s.BytesWritten) / float64(s.TotalCycles)
}
