package memcontroltest

import "github.com/sarchlab/akita/v5/mem/memcontrolprotocol"

// VerbSupport declares which control verbs a component implements.
// Verbs left false must respond with ControlRsp{Success: false,
// Error: memcontrolprotocol.ErrUnsupported}.
type VerbSupport struct {
	Pause      bool
	Drain      bool
	Enable     bool
	Reset      bool
	Invalidate bool
	Flush      bool
	Hold       bool
	Release    bool
}

// Supports reports whether the given command is in the support set.
func (v VerbSupport) Supports(cmd memcontrolprotocol.Command) bool {
	switch cmd {
	case memcontrolprotocol.CmdPause:
		return v.Pause
	case memcontrolprotocol.CmdDrain:
		return v.Drain
	case memcontrolprotocol.CmdEnable:
		return v.Enable
	case memcontrolprotocol.CmdReset:
		return v.Reset
	case memcontrolprotocol.CmdInvalidate:
		return v.Invalidate
	case memcontrolprotocol.CmdFlush:
		return v.Flush
	case memcontrolprotocol.CmdHold:
		return v.Hold
	case memcontrolprotocol.CmdRelease:
		return v.Release
	default:
		return false
	}
}

// Universal is the matrix every memory agent must satisfy: the four
// universal verbs (Pause, Drain, Enable, Reset). The two conditional
// verbs (Invalidate, Flush) are off.
func Universal() VerbSupport {
	return VerbSupport{
		Pause:  true,
		Drain:  true,
		Enable: true,
		Reset:  true,
	}
}

// CacheLike is the support matrix for components that hold dirty
// private cache-of-memory state (write-back style caches): all four
// universal verbs plus Invalidate and Flush.
func CacheLike() VerbSupport {
	v := Universal()
	v.Invalidate = true
	v.Flush = true
	return v
}

// TranslationCacheLike is the support matrix for components that hold
// a private cache of translations (TLB, MMU cache): the four universal
// verbs plus Invalidate. Flush is not meaningful because translations
// are never dirty.
func TranslationCacheLike() VerbSupport {
	v := Universal()
	v.Invalidate = true
	return v
}

// TranslatorLike is the support matrix for components that translate
// accesses and hold them while their pages migrate (address translators):
// the four universal verbs plus Hold and Release.
func TranslatorLike() VerbSupport {
	v := Universal()
	v.Hold = true
	v.Release = true
	return v
}

// IsSyncVerb reports whether the verb is acknowledged synchronously
// (Pause, Enable, Reset, Invalidate, Release) versus asynchronously on
// completion (Drain, Flush, Hold). Used by the contract harness to know
// whether to expect a memcontrolprotocol.Rsp within one tick or to keep ticking.
func IsSyncVerb(cmd memcontrolprotocol.Command) bool {
	switch cmd {
	case memcontrolprotocol.CmdPause, memcontrolprotocol.CmdEnable,
		memcontrolprotocol.CmdReset, memcontrolprotocol.CmdInvalidate,
		memcontrolprotocol.CmdRelease:
		return true
	case memcontrolprotocol.CmdDrain, memcontrolprotocol.CmdFlush,
		memcontrolprotocol.CmdHold:
		return false
	default:
		return true
	}
}
