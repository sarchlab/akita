package twowaybuffered

import "github.com/sarchlab/akita/v5/sim/messaging"

// Test-only accessors preserve low-level checkpoint and send-guard coverage
// while external tests use the real Simulation without an import cycle.
func (p *Port) TestPopOutgoing()                   { p.outgoingBuf.Pop() }
func (p *Port) TestClearConnection()               { p.conn = nil }
func (p *Port) TestPushIncoming(m messaging.Msg)   { p.incomingBuf.Push(bufferEntry{Message: m}) }
func (p *Port) TestPushOutgoing(m messaging.Msg)   { p.outgoingBuf.Push(bufferEntry{Message: m}) }
func (p *Port) TestIncomingEntries() []bufferEntry { return p.incomingBuf.Elements() }
func (p *Port) TestOutgoingEntries() []bufferEntry { return p.outgoingBuf.Elements() }
