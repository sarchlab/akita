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

// RebindState lets a State that was just decoded from a checkpoint reattach the
// runtime-only dependencies it does not serialize, when it defines
// AfterCheckpointLoad(C). A misspelled signature must fail rather than leave
// them unbound.
func RebindState[T, C any](state *T, component C) {
	if _, exists := reflect.TypeFor[T]().MethodByName("AfterCheckpointLoad"); exists {
		panic(fmt.Sprintf("modeling: %T.AfterCheckpointLoad must use a pointer receiver", state))
	}
	if rebinder, ok := any(state).(interface{ AfterCheckpointLoad(C) }); ok {
		rebinder.AfterCheckpointLoad(component)
		return
	}
	if _, exists := reflect.TypeOf(state).MethodByName("AfterCheckpointLoad"); exists {
		panic(fmt.Sprintf("modeling: %T.AfterCheckpointLoad must have signature AfterCheckpointLoad(%T)",
			state, component))
	}
}
