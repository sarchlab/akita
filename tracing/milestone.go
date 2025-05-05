package tracing

// Milestone represents a point in time where a task's blocking status is
// resolved.
type Milestone struct {
	ID               string `json:"id"`
	TaskID           string `json:"task_id"`
	BlockingCategory string `json:"blocking_category"`
	BlockingReason   string `json:"blocking_reason"`
	BlockingLocation string `json:"blocking_location"`
}
