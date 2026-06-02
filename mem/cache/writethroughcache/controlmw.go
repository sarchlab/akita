package writethroughcache

import (
	"github.com/sarchlab/akita/v5/modeling"
)

// controlMW runs the control stage (flush/invalidate/restart).
type controlMW struct {
	comp         *modeling.Component[Spec, State, Resources]
	controlStage *controlStage
}

// Tick runs the control stage.
func (m *controlMW) Tick() bool {
	return m.controlStage.Tick()
}
