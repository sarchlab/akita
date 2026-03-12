package sim

import (
	"log"
)

// A LogHook is a hook that is resonsible for recording information from the
// simulation
type LogHook interface {
	Hook
}

// LogHookBase proovides the common logic for all LogHooks
type LogHookBase struct {
	*log.Logger
}
