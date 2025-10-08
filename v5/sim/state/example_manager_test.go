package state

import "fmt"

type ExampleState struct {
	Items []string
}

// ExampleManager_round demonstrates a simple round-style workflow.
func ExampleManager_round() {
	manager := NewManager()
	_ = manager.Register("queue", &ExampleState{Items: []string{"a"}})

	staged, _ := manager.Stage("queue")
	queue := staged.(*ExampleState)
	queue.Items = append(queue.Items, "b")

	manager.CommitAll()

	loaded, _ := manager.Load("queue")
	fmt.Println(loaded.(*ExampleState).Items)
	// Output: [a b]
}
