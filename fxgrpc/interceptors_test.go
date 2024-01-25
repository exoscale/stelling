package fxgrpc

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSortInterceptors(t *testing.T) {
	cases := []struct {
		name     string
		input    []WeightedInterceptor
		expected []WeightedInterceptor
	}{
		{
			name:     "Should work with emty input",
			input:    []WeightedInterceptor{},
			expected: []WeightedInterceptor{},
		},
		{
			name: "Should sort by ascending weight",
			input: []WeightedInterceptor{
				&StreamServerInterceptor{Weight: 100},
				&StreamServerInterceptor{Weight: 1},
				&StreamServerInterceptor{Weight: 75},
				&StreamServerInterceptor{Weight: 22},
				&StreamServerInterceptor{Weight: 12},
			},
			expected: []WeightedInterceptor{
				&StreamServerInterceptor{Weight: 1},
				&StreamServerInterceptor{Weight: 12},
				&StreamServerInterceptor{Weight: 22},
				&StreamServerInterceptor{Weight: 75},
				&StreamServerInterceptor{Weight: 100},
			},
		},
		{
			name: "Should not change list if it's already sorted",
			input: []WeightedInterceptor{
				&StreamServerInterceptor{Weight: 1},
				&StreamServerInterceptor{Weight: 12},
				&StreamServerInterceptor{Weight: 22},
				&StreamServerInterceptor{Weight: 75},
				&StreamServerInterceptor{Weight: 100},
			},
			expected: []WeightedInterceptor{
				&StreamServerInterceptor{Weight: 1},
				&StreamServerInterceptor{Weight: 12},
				&StreamServerInterceptor{Weight: 22},
				&StreamServerInterceptor{Weight: 75},
				&StreamServerInterceptor{Weight: 100},
			},
		},
		{
			name: "Should remove nil slice elements",
			input: []WeightedInterceptor{
				nil,
				&StreamServerInterceptor{Weight: 100},
				&StreamServerInterceptor{Weight: 1},
				&StreamServerInterceptor{Weight: 75},
				(*StreamServerInterceptor)(nil),
				&StreamServerInterceptor{Weight: 22},
				&StreamServerInterceptor{Weight: 12},
				nil,
			},
			expected: []WeightedInterceptor{
				&StreamServerInterceptor{Weight: 1},
				&StreamServerInterceptor{Weight: 12},
				&StreamServerInterceptor{Weight: 22},
				&StreamServerInterceptor{Weight: 75},
				&StreamServerInterceptor{Weight: 100},
			},
		},
		{
			name: "Should remove nil interfaces slice elements",
			input: []WeightedInterceptor{
				(*UnaryClientInterceptor)(nil),
				&StreamServerInterceptor{Weight: 100},
				&StreamServerInterceptor{Weight: 1},
				&StreamServerInterceptor{Weight: 75},
				(*StreamServerInterceptor)(nil),
				&StreamServerInterceptor{Weight: 22},
				&StreamServerInterceptor{Weight: 12},
				(*UnaryServerInterceptor)(nil),
			},
			expected: []WeightedInterceptor{
				&StreamServerInterceptor{Weight: 1},
				&StreamServerInterceptor{Weight: 12},
				&StreamServerInterceptor{Weight: 22},
				&StreamServerInterceptor{Weight: 75},
				&StreamServerInterceptor{Weight: 100},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			output := SortInterceptors(tc.input)
			require.EqualValues(t, tc.expected, output)
		})
	}
}
