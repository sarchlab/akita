package direct

import "github.com/sarchlab/akita/v5/sim/messaging"

func (c *Connection) TestPorts() []messaging.Port { return c.ports.ports }
