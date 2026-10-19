package writethrough

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/stretchr/testify/require"
)

func TestRespondStage(t *testing.T) {
	var (
		mw      *pipelineMW
		topPort messaging.Port
		s       *respondStage
	)

	setup1 := func(t *testing.T) {
		t.Helper()

		// topPort is a real, single-slot port (owned by the component) so the
		// "cannot send" cases can be forced by pre-filling its outgoing buffer.
		ports := makePorts("Cache", 4)
		ports.Top = twowaybuffered.NewPort(1, 1)

		spec := DefaultSpec()
		spec.Bottom.Destinations = []messaging.RemotePort{"Memory.Top"}
		mw = buildStageTestCache(
			spec,
			Resources{Storage: mem.NewStorage(4 * mem.KB)},
			ports,
			state{},
		)

		topPort = ports.Top

		s = &respondStage{cache: mw}
	}

	// fillOutgoing pre-fills topPort's single outgoing slot so the next
	// CanSend returns false, simulating a busy port.
	fillOutgoing := func(t *testing.T) {
		t.Helper()

		dummy := messaging.Msg{ID: 1, Payload: memprotocol.DataReadyRsp{},
			Src: topPort.AsRemote(),
			Dst: messaging.RemotePort("SomeSrc"),
		}

		require.True(t, topPort.CanSend())
		topPort.Send(dummy)
	}

	t.Run("read", func(t *testing.T) {
		var readMeta messaging.Msg

		setup2 := func(t *testing.T) {
			t.Helper()

			next := &mw.comp.State

			readMeta = messaging.Msg{
				ID:           mw.comp.NewID(),
				Src:          "SomeSrc",
				TrafficBytes: 12,
			}

			next.Transactions = append(next.Transactions,
				transactionState{
					HasRead:            true,
					ReadMeta:           readMeta,
					ReadAddress:        0x100,
					ReadAccessByteSize: 4,
					ReadPID:            1,
				},
			)
		}

		t.Run("should stall if cannot send to top", func(t *testing.T) {
			setup1(t)
			setup2(t)

			next := &mw.comp.State
			next.Transactions[0].Data = []byte{1, 2, 3, 4}
			next.Transactions[0].Done = true
			fillOutgoing(t)

			madeProgress := s.Tick()

			require.False(t, madeProgress)
		})

		t.Run("should send data ready to top", func(t *testing.T) {
			setup1(t)
			setup2(t)

			next := &mw.comp.State
			next.Transactions[0].Data = []byte{1, 2, 3, 4}
			next.Transactions[0].Done = true

			madeProgress := s.Tick()

			require.True(t, madeProgress)
			require.True(t, next.Transactions[0].Removed)

			out, _ := topPort.RetrieveOutgoing()
			dr := out
			require.Equal(t, readMeta.ID, dr.RspTo)
			require.Equal(t, []byte{1, 2, 3, 4}, dr.Payload.(memprotocol.DataReadyRsp).Data)
		})
	})

	t.Run("write", func(t *testing.T) {
		var writeMeta messaging.Msg

		setup3 := func(t *testing.T) {
			t.Helper()

			next := &mw.comp.State

			writeMeta = messaging.Msg{
				ID:           mw.comp.NewID(),
				Src:          "SomeSrc",
				TrafficBytes: 12,
			}

			next.Transactions = append(next.Transactions,
				transactionState{
					HasWrite:     true,
					WriteMeta:    writeMeta,
					WriteAddress: 0x100,
					WritePID:     1,
				},
			)
		}

		t.Run("should stall if cannot send to top", func(t *testing.T) {
			setup1(t)
			setup3(t)

			next := &mw.comp.State
			next.Transactions[0].Done = true
			fillOutgoing(t)

			madeProgress := s.Tick()

			require.False(t, madeProgress)
		})

		t.Run("should send data ready to top", func(t *testing.T) {
			setup1(t)
			setup3(t)

			next := &mw.comp.State
			next.Transactions[0].Data = []byte{1, 2, 3, 4}
			next.Transactions[0].Done = true

			madeProgress := s.Tick()

			require.True(t, madeProgress)
			require.True(t, next.Transactions[0].Removed)

			out, _ := topPort.RetrieveOutgoing()
			require.Equal(t, writeMeta.ID, out.RspTo)
		})
	})
}
