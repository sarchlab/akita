package cache

type writeThroughStrategy struct {
	*Comp
}

func (s *writeThroughStrategy) Tick() bool {
	return false
}

func (s *writeThroughStrategy) ParseTop() (madeProgress bool) {
	return false
}
