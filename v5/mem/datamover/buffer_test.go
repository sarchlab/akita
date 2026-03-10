package datamover

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Buffer", func() {
	Describe("ExtractData", func() {
		tests := []struct {
			name        string
			initAddr    uint64
			granularity uint64
			data        [][]byte
			addr        uint64
			size        uint64
			expected    []byte
			ok          bool
		}{
			{
				name:        "Extract data within single chunk",
				initAddr:    0,
				granularity: 4,
				data:        [][]byte{{1, 2, 3, 4}, {5, 6, 7, 8}},
				addr:        0,
				size:        4,
				expected:    []byte{1, 2, 3, 4},
				ok:          true,
			},
			{
				name:        "Extract data spanning multiple chunks",
				initAddr:    0,
				granularity: 4,
				data:        [][]byte{{1, 2, 3, 4}, {5, 6, 7, 8}},
				addr:        2,
				size:        6,
				expected:    []byte{3, 4, 5, 6, 7, 8},
				ok:          true,
			},
			{
				name:        "Extract data with missing chunk",
				initAddr:    0,
				granularity: 4,
				data:        [][]byte{{1, 2, 3, 4}, nil, {9, 10, 11, 12}},
				addr:        0,
				size:        12,
				expected:    nil,
				ok:          false,
			},
		}

		for _, tt := range tests {
			tt := tt
			Context(tt.name, func() {
				It("should extract data correctly", func() {
					b := &buffer{
						offset:      tt.initAddr,
						granularity: tt.granularity,
						data:        tt.data,
					}

					data, ok := b.extractData(tt.addr, tt.size)

					Expect(ok).To(Equal(tt.ok))
					Expect(data).To(Equal(tt.expected))
				})
			})
		}
	})
})
