package cmdq

// GetNextQueueIndex returns the current next-queue index.
func (q *CommandQueueImpl) GetNextQueueIndex() int {
	return q.nextQueueIndex
}

// SetNextQueueIndex sets the current next-queue index.
func (q *CommandQueueImpl) SetNextQueueIndex(idx int) {
	q.nextQueueIndex = idx
}
