package mshr

import (
	"fmt"

	"github.com/sarchlab/akita/v4/mem"
	"github.com/sarchlab/akita/v4/mem/vm"
)

// MSHREntry is an entry in MSHR.
type mshrEntry struct {
	PID      vm.PID
	Address  uint64
	Requests []mem.AccessReq
	ReadReq  mem.ReadReq
}

// MSHR is an interface that controls MSHR entries
type MSHR struct {
	Capacity int
	Entries  []mshrEntry
}

func (m *MSHR) Lookup(pid vm.PID, addr uint64) bool {
	for _, e := range m.Entries {
		if e.PID == pid && e.Address == addr {
			return true
		}
	}

	return false
}

func (m *MSHR) AddEntry(readToBottom mem.ReadReq) error {
	if m.Lookup(readToBottom.PID, readToBottom.Address) {
		return fmt.Errorf("trying to add an address that is already in MSHR")
	}

	if m.IsFull() {
		return fmt.Errorf("trying to add to a full MSHR")
	}

	entry := mshrEntry{
		PID:     readToBottom.PID,
		Address: readToBottom.Address,
		ReadReq: readToBottom,
	}

	m.Entries = append(m.Entries, entry)

	return nil
}

func (m *MSHR) RemoveEntry(pid vm.PID, addr uint64) error {
	for i, e := range m.Entries {
		if e.PID == pid && e.Address == addr {
			m.Entries = append(m.Entries[:i], m.Entries[i+1:]...)
			return nil
		}
	}

	return fmt.Errorf("trying to remove an non-exist entry")
}

func (m *MSHR) AddReqToEntry(req mem.AccessReq) error {
	for i, e := range m.Entries {
		if e.PID == req.GetPID() && e.Address == req.GetAddress() {
			e.Requests = append(e.Requests, req)
			m.Entries[i] = e

			return nil
		}
	}

	return fmt.Errorf("trying to add a request to an non-exist entry")
}

func (m *MSHR) RemoveReqFromEntry(reqID string) error {
	for i, e := range m.Entries {
		for j, req := range e.Requests {
			if req.Meta().ID == reqID {
				e.Requests = append(e.Requests[:j], e.Requests[j+1:]...)
				m.Entries[i] = e

				return nil
			}
		}
	}

	return fmt.Errorf("request %s not found", reqID)
}

func (m *MSHR) GetNextReqInEntry(
	pid vm.PID,
	addr uint64,
) (mem.AccessReq, error) {
	for _, e := range m.Entries {
		if e.PID == pid && e.Address == addr {
			if len(e.Requests) == 0 {
				return nil, fmt.Errorf(
					"no request found for pid %d and addr 0x%x", pid, addr,
				)
			}

			return e.Requests[0], nil
		}
	}

	return nil, fmt.Errorf(
		"no entry found for pid %d and addr 0x%x", pid, addr,
	)
}

func (m *MSHR) IsFull() bool {
	return len(m.Entries) >= m.Capacity
}

func (m *MSHR) Reset() {
	m.Entries = nil
}
