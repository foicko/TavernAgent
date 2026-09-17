package domain

import (
	"math"
	"testing"
)

func TestAggregateDeltasDoesNotOverflowOrLoseCancellation(t *testing.T) {
	for _, tc := range []struct {
		values []int
		want   int
	}{
		{[]int{math.MaxInt, math.MaxInt}, 10},
		{[]int{math.MinInt, math.MinInt}, -10},
		{[]int{math.MaxInt, math.MaxInt, math.MinInt, math.MinInt}, -2},
		{[]int{math.MaxInt, 5, -math.MaxInt}, 5},
		{[]int{math.MinInt, -5, math.MaxInt}, -6},
	} {
		if got := AggregateDeltas(tc.values); got != tc.want {
			t.Errorf("AggregateDeltas(%v)=%d, want %d", tc.values, got, tc.want)
		}
	}
}
