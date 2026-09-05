package mem_test

import (
	"errors"
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/timing"
)

type storagePanicHandler func(timing.Event)

func (h storagePanicHandler) Handle(evt timing.Event) { h(evt) }

func TestStoragePreconditionPanicBecomesRunError(t *testing.T) {
	for _, operation := range []string{"read", "write"} {
		t.Run(operation, func(t *testing.T) {
			e := timing.NewSerialEngine()
			storage := mem.NewStorage(16)
			e.RegisterHandler("memory", storagePanicHandler(func(timing.Event) {
				if operation == "read" {
					storage.Read(16, 1)
				} else {
					storage.Write(16, []byte{1})
				}
			}))
			e.Schedule(timing.EventBase{Time_: 1, HandlerID_: "memory"})
			var failure *timing.PanicError
			if err := e.Run(); !errors.As(err, &failure) || failure.Handler != "memory" || failure.Time != 1 {
				t.Fatalf("storage panic escaped its owning engine: %v", err)
			}
			t.Logf("invalid storage %s returned a PanicError from Run at memory event time 1", operation)
		})
	}
}
