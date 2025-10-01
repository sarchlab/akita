package tracingv5

import "github.com/sarchlab/akita/v4/sim"

// SendReqTaskID generates the task ID for the sending side of a request.
//
// This ID is used for the task created by ReqInitiate and ended by ReqFinalize.
// It represents the lifetime of the request from the sender's perspective,
// from when it's sent until the response is received.
func SendReqTaskID(msg sim.Msg) string {
	return msg.Meta().ID + "_send"
}

// ReceiveReqTaskID generates the task ID for the receiving side of a request.
//
// This ID is used for the task created by ReqReceive and ended by ReqComplete.
// It represents the lifetime of the request from the receiver's perspective,
// from when it's received until handling is complete.
func ReceiveReqTaskID(msg sim.Msg) string {
	return msg.Meta().ID + "_recv"
}
