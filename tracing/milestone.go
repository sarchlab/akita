package tracing

// Milestone represents a point in time where a task is blocked
type Milestone struct {
	ID               string  `json:"id"`
	TaskID           string  `json:"task_id"`
	BlockingCategory string  `json:"blocking_category"`
	BlockingReason   string  `json:"blocking_reason"`
	BlockingLocation string  `json:"blocking_location"`
	Time             float64 `json:"time"`
}
