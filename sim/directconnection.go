package sim

type directConnectionEnd struct {
	port    Port
	buf     []Msg
	bufSize int
	busy    bool
}

// DirectConnection connects two components without latency
type DirectConnection struct {
	*TickingComponent

	nextPortID int
	ports      []Port
	ends       map[Port]*directConnectionEnd
}

// PlugIn marks the port connects to this DirectConnection.
func (c *DirectConnection) PlugIn(port Port, sourceSideBufSize int) {
	c.Lock()
	defer c.Unlock()

	c.ports = append(c.ports, port)
	end := &directConnectionEnd{}
	end.port = port
	end.bufSize = sourceSideBufSize
	c.ends[port] = end

	port.SetConnection(c)
}

// Unplug marks the port no longer connects to this DirectConnection.
func (c *DirectConnection) Unplug(_ Port) {
	panic("not implemented")
}

// NotifyAvailable is called by a port to notify that the connection can
// deliver to the port again.
func (c *DirectConnection) NotifyAvailable(now VTimeInSec, _ Port) {
	c.TickNow(now)
}

// CanSend checks if the direct message can send a message from the port.
func (c *DirectConnection) CanSend(src Port) bool {
	c.Lock()
	defer c.Unlock()

	end := c.ends[src]

	canSend := len(end.buf) < end.bufSize

	if !canSend {
		end.busy = true
	}

	return canSend
}

// Send of a DirectConnection schedules a DeliveryEvent immediately
func (c *DirectConnection) Send(msg Msg) *SendError {
	c.Lock()
	defer c.Unlock()

	c.msgMustBeValid(msg)

	srcEnd := c.ends[msg.Meta().Src]

	if len(srcEnd.buf) >= srcEnd.bufSize {
		srcEnd.busy = true
		return NewSendError()
	}

	srcEnd.buf = append(srcEnd.buf, msg)

	c.TickNow(msg.Meta().SendTime)

	return nil
}

func (c *DirectConnection) msgMustBeValid(msg Msg) {
	c.portMustNotBeNil(msg.Meta().Src)
	c.portMustNotBeNil(msg.Meta().Dst)
	c.portMustBeConnected(msg.Meta().Src)
	c.portMustBeConnected(msg.Meta().Dst)
	c.srcDstMustNotBeTheSame(msg)
}

func (c *DirectConnection) portMustNotBeNil(port Port) {
	if port == nil {
		panic("src or dst is not given")
	}
}

func (c *DirectConnection) portMustBeConnected(port Port) {
	if _, connected := c.ends[port]; !connected {
		panic("src or dst is not connected")
	}
}

func (c *DirectConnection) srcDstMustNotBeTheSame(msg Msg) {
	if msg.Meta().Src == msg.Meta().Dst {
		panic("sending back to src")
	}
}

// Tick updates the states of the connection and delivers messages.
func (c *DirectConnection) Tick(now VTimeInSec) bool {
	madeProgress := false
	for i := 0; i < len(c.ports); i++ {
		portID := (i + c.nextPortID) % len(c.ports)
		port := c.ports[portID]
		end := c.ends[port]
		madeProgress = c.forwardMany(end, now) || madeProgress
	}

	c.nextPortID = (c.nextPortID + 1) % len(c.ports)
	return madeProgress
}

func (c *DirectConnection) forwardMany(
	end *directConnectionEnd,
	now VTimeInSec,
) bool {
	madeProgress := false
	for {
		if len(end.buf) == 0 {
			break
		}

		head := end.buf[0]
		head.Meta().RecvTime = now

		err := head.Meta().Dst.Recv(head)
		if err != nil {
			break
		}

		madeProgress = true
		end.buf = end.buf[1:]

		if end.busy {
			end.port.NotifyAvailable(now)
			end.busy = false
		}
	}

	return madeProgress
}

// NewDirectConnection creates a new DirectConnection object
func NewDirectConnection(
	name string,
	engine Engine,
	freq Freq,
) *DirectConnection {
	c := new(DirectConnection)
	c.TickingComponent = NewSecondaryTickingComponent(name, engine, freq, c)
	c.ends = make(map[Port]*directConnectionEnd)
	return c
}
