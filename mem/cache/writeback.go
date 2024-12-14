package cache

type writeBackStrategy struct {
	*Comp
}

func (s *writeBackStrategy) Tick() bool {
	return false
}
