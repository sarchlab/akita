package base

import (
	"fmt"
	"reflect"
)

// InitializeElement initializes one element in place when it defines
// Initialize(C). Otherwise its zero value is retained. A misspelled signature
// must fail rather than silently skipping the element's initialization.
func InitializeElement[T, C any](element *T, component C) {
	if _, exists := reflect.TypeFor[T]().MethodByName("Initialize"); exists {
		panic(fmt.Sprintf("modeling: %T.Initialize must use a pointer receiver", element))
	}
	if initializer, ok := any(element).(interface{ Initialize(C) }); ok {
		initializer.Initialize(component)
		return
	}
	if _, exists := reflect.TypeOf(element).MethodByName("Initialize"); exists {
		panic(fmt.Sprintf("modeling: %T.Initialize must have signature Initialize(%T)", element, component))
	}
}
