package xslices

import (
	"cmp"
	"testing"

	"gotest.tools/v3/assert"
)

func TestChunkBy(t *testing.T) {
	tests := []struct {
		name    string
		input   []int
		output  [][]int
		chunker func(int) int
	}{
		{
			name:    "PositiveNegative",
			input:   []int{-1, -2, -3, 1, 2, 3, -1, -2, 3},
			output:  [][]int{{-1, -2, -3}, {1, 2, 3}, {-1, -2}, {3}},
			chunker: func(v int) int { return cmp.Compare(v, 0) },
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := ChunkBy(test.input, test.chunker)
			assert.DeepEqual(t, test.output, result)
		})
	}
}
