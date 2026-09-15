package containers

import "github.com/sarchlab/akita/v5/modeling"

type Choice string

const Escaped Choice = "quote\"\n\t\\中文"

type Spec struct {
	Array        [3]int
	Sparse       []int
	Matrix       [][]int
	Maps         map[int64][]int
	Unsigned     map[uint64][2]int
	Nil          []int
	Bytes        []byte
	StringNumber uint64 `json:",string"`
	Fraction     float32
	EmptySlice   []int          `json:",omitempty"`
	EmptyMap     map[string]int `json:",omitempty"`
	Choice       Choice
}

var Definition = modeling.DefineComponent(modeling.ComponentDef[Spec, modeling.None]{Name: "Containers", DefaultSpec: Spec{
	Array: [3]int{1: 2}, Sparse: []int{uint64(2): 4, 5}, Matrix: [][]int{{1}, nil},
	Maps: map[int64][]int{-1: {2}}, Unsigned: map[uint64][2]int{18446744073709551615: {3}},
	Nil: nil, Bytes: []byte{65, 66}, StringNumber: 18446744073709551615, Fraction: 0.1,
	EmptySlice: []int{}, EmptyMap: map[string]int{}, Choice: Escaped,
}})
