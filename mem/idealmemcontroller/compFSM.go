package idealmemcontroller

func (c *Comp) Tick() bool {
	madeProgress := false

	// get ctrl msg from ctrl port
	madeProgress = c.handleCtrlSignals() || madeProgress

	for i := 0; i < c.width; i++ {
		madeProgress = c.updateFSM(i) || madeProgress
	}

	return madeProgress
}

func (c *Comp) updateFSM(i int) bool {
	madeProgress := false

	switch state := c.state; state {
	case "enable":
		madeProgress = c.handleMemReqs()
	case "pause":
		madeProgress = false
	case "drain":
		madeProgress = c.handleDrainReq(i)
	}

	return madeProgress
}

func (c *Comp) handleDrainReq(i int) bool {
	if c.fullyDrained(i) {
		if !c.handleMemReqs() {
			return false
		}

		if !c.setState("pause", c.respondReq) {
			return false
		}
		c.isDraining = false

		return true
	}

	return c.handleMemReqs()
}

func (c *Comp) fullyDrained(i int) bool {
	if (i == c.width-1) || c.topPort.PeekIncoming() == nil {
		return true
	}

	return false
}
