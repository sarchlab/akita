package tracingv5

import "github.com/sarchlab/akita/v4/v5/instrumentation/hooking"

// Hook positions for request lifecycle events
var (
	// HookPosReqInitiate is triggered when a component sends a request
	HookPosReqInitiate = &hooking.HookPos{Name: "ReqInitiate"}

	// HookPosReqReceive is triggered when a component receives a request
	HookPosReqReceive = &hooking.HookPos{Name: "ReqReceive"}

	// HookPosReqComplete is triggered when a component finishes handling a request
	HookPosReqComplete = &hooking.HookPos{Name: "ReqComplete"}

	// HookPosReqFinalize is triggered when the sender receives the response
	HookPosReqFinalize = &hooking.HookPos{Name: "ReqFinalize"}
)
