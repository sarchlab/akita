package idealmemcontroller
import "github.com/sarchlab/akita/v5/timing"
func (m *MockEngine) IDGenerator() timing.IDGenerator { return mockIDs }
var mockIDs = timing.NewIDGenerator()
