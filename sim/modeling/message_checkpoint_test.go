package modeling_test

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/noc/packetization"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
)

func TestCheckpointPreservesMessagePayloadTypes(t *testing.T) {
	type state struct{ Buf []messaging.Msg }
	request := messaging.Msg{
		ID: 1, Src: "cpu", Dst: "mem",
		Payload: memprotocol.WriteReq{Address: 64, Data: []byte{1, 2, 3}},
	}
	src := state{Buf: []messaging.Msg{
		request,
		{ID: 2, Src: "a", Dst: "b"},
		{ID: 3, Src: "network.a", Dst: "network.b", Payload: packetization.Flit{
			MsgID: request.ID, Dst: request.Dst, Msg: request, SeqID: 0, NumFlitInMsg: 1}},
	}}
	if err := modeling.ValidateState(src); err != nil {
		t.Fatal(err)
	}
	var checkpoint bytes.Buffer
	if err := modeling.WriteCheckpoint(&checkpoint, struct{}{}, src, nil); err != nil {
		t.Fatal(err)
	}
	t.Logf("checkpoint with ordinary, nil, and nested payloads: %s", checkpoint.String())
	var dst state
	if err := modeling.ReadCheckpoint(&checkpoint, struct{}{}, &dst, nil); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(src, dst) {
		t.Fatalf("checkpoint changed message types or values: %#v", dst)
	}
	flit, ok := dst.Buf[2].Payload.(packetization.Flit)
	if !ok {
		t.Fatalf("restored flit as %T", dst.Buf[2].Payload)
	}
	write, ok := flit.Msg.Payload.(memprotocol.WriteReq)
	if !ok || write.Address != 64 || !bytes.Equal(write.Data, []byte{1, 2, 3}) {
		t.Fatalf("inner request: %#v", flit.Msg)
	}
}
