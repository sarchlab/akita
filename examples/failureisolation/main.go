// Command failureisolation runs two simulations in one process. A model panic
// fails one instance while its peer completes. Run with go run ./examples/failureisolation.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sarchlab/akita/v5/simulation"
	"github.com/sarchlab/akita/v5/timing"
)

type counter struct {
	engine timing.Engine
	count  int
	fail   bool
}

func (c *counter) Handle(evt timing.Event) {
	c.count++
	if c.fail {
		panic("injected model failure")
	}
	if c.count < 3 {
		c.engine.Schedule(timing.MakeEventBase(timing.IDsFor(c.engine), evt.Time()+1, "counter"))
	}
}
func build(path string, fail bool) (*simulation.Simulation, *counter, error) {
	c := &counter{fail: fail}
	s, err := simulation.MakeBuilder().WithoutMonitoring().WithOutputFileName(path).
		Build(func(s *simulation.Simulation) error {
			c.engine = s.GetEngine()
			c.engine.(timing.HandlerRegistrar).RegisterHandler("counter", c)
			c.engine.Schedule(timing.MakeEventBase(s.GetIDGenerator(), 1, "counter"))
			return nil
		})
	return s, c, err
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	dir, err := os.MkdirTemp("", "akita-isolation-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	bad, badCount, err := build(filepath.Join(dir, "failed"), true)
	if err != nil {
		return err
	}
	defer bad.Terminate()
	good, goodCount, err := build(filepath.Join(dir, "healthy"), false)
	if err != nil {
		return err
	}
	defer good.Terminate()
	done := make(chan error, 1)
	go func() { done <- bad.Run() }()
	goodErr := good.Run()
	badErr := <-done
	if goodErr != nil {
		return goodErr
	}
	if badErr == nil || badCount.count != 1 || goodCount.count != 3 {
		return fmt.Errorf("isolation contract not met")
	}
	if err := good.Terminate(); err != nil {
		return err
	}
	if err := bad.Terminate(); err == nil {
		return fmt.Errorf("failure lost during finalization")
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{
		"failure_contained": true, "failed_state": bad.State(), "unaffected_state": good.State(),
		"failed_events": badCount.count, "completed_events": goodCount.count,
	})
}
