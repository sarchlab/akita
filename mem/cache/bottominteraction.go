package cache

// bottomInteraction is a middleware that handles the interaction between the
// cache and the memory below it.
type bottomInteraction struct {
	*Comp
}

func (b *bottomInteraction) Tick() bool {
	return false
}
