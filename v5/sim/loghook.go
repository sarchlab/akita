package sim

import (
	"log"
)

// LogHookBase proovides the common logic for all LogHooks
type LogHookBase struct {
	*log.Logger
}
