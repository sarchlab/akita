package tasks

import "github.com/sarchlab/akita/v5/sim/messaging"

// Reason describes a resolved port wait using a typed milestone kind.
func Reason(kind MilestoneKind, what string) messaging.WaitReason {
	return messaging.WaitReason{Kind: string(kind), What: what}
}
