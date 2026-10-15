// Package simconfig supplies internal simulation construction options for tests.
package simconfig

import "github.com/sarchlab/akita/v5/sim/timing"

// EngineOption can be embedded privately by sim.Builder. Its private methods
// keep engine injection unavailable outside Akita's internal packages.
type EngineOption struct{ engine timing.Engine }

func (o *EngineOption) setEngine(e timing.Engine) { o.engine = e }
func (o *EngineOption) getEngine() timing.Engine  { return o.engine }

// SetEngine selects the engine used by all simulation services.
func SetEngine(b interface{ setEngine(timing.Engine) }, e timing.Engine) { b.setEngine(e) }

// Engine returns the internal override, or nil for normal engine construction.
func Engine(b interface{ getEngine() timing.Engine }) timing.Engine { return b.getEngine() }
