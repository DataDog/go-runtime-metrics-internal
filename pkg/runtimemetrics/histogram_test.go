package runtimemetrics

import (
	"math"
	"runtime/metrics"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHistogramToDistributionSamples(t *testing.T) {
	t.Run("should correctly convert a given histogram to distribution samples", func(t *testing.T) {
		h := &metrics.Float64Histogram{
			Counts:  []uint64{1, 0, 10},
			Buckets: []float64{0, 10, 20, 30},
		}
		s := distributionSamplesFromHist(h, nil)
		assert.Len(t, s, 2)
		assert.Equal(t, s, []distributionSample{
			{Value: 5, Rate: 1},
			{Value: 25, Rate: 0.1},
		})
	})
}

func TestHistogramSub(t *testing.T) {
	t.Run("should correctly compute the substraction of two given histograms", func(t *testing.T) {
		a := &metrics.Float64Histogram{
			Counts:  []uint64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			Buckets: []float64{0, 10, 20, 30, 40, 50, 60, 70, 80, 90, 100},
		}

		b := &metrics.Float64Histogram{
			Counts:  []uint64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9},
			Buckets: []float64{0, 10, 20, 30, 40, 50, 60, 70, 80, 90, 100},
		}

		s, eq := sub(a, b)
		for i := range s.Counts {
			assert.False(t, eq)
			assert.Equal(t, a.Counts[i]-b.Counts[i], s.Counts[i])
		}
	})

	t.Run("should return 0 when the substraction is empty", func(t *testing.T) {
		a := &metrics.Float64Histogram{
			Counts:  []uint64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			Buckets: []float64{0, 10, 20, 30, 40, 50, 60, 70, 80, 90, 100},
		}

		b := &metrics.Float64Histogram{
			Counts:  []uint64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			Buckets: []float64{0, 10, 20, 30, 40, 50, 60, 70, 80, 90, 100},
		}

		s, eq := sub(a, b)
		for i := range s.Counts {
			assert.True(t, eq)
			assert.Equal(t, a.Counts[i]-b.Counts[i], uint64(0))
		}
	})
}

func TestHistogramAvg(t *testing.T) {
	t.Run("should correctly compute the average of a given histogram", func(t *testing.T) {
		h := &metrics.Float64Histogram{
			Counts:  []uint64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			Buckets: []float64{0, 10, 20, 30, 40, 50, 60, 70, 80, 90, 100},
		}
		a := avg(h)
		assert.Equal(t, 65.0, a)
	})

	t.Run("should correctly compute the average of a histogram containing buckets spanning from -inf to +inf", func(t *testing.T) {
		h := &metrics.Float64Histogram{
			Counts:  []uint64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 5},
			Buckets: []float64{math.Inf(-1), 0, 10, 20, 30, 40, 50, 60, 70, 80, 90, math.Inf(+1)},
		}
		a := avg(h)
		assert.Equal(t, 61.5, a)
	})

	t.Run("return 0 when there is a single -inf, +inf bucket", func(t *testing.T) {
		h := &metrics.Float64Histogram{
			Counts:  []uint64{10},
			Buckets: []float64{math.Inf(-1), math.Inf(+1)},
		}
		a := avg(h)
		assert.Equal(t, 0.0, a)
	})

	t.Run("return 0 when the histogram is empty", func(t *testing.T) {
		h := &metrics.Float64Histogram{
			Counts:  []uint64{0, 0, 0},
			Buckets: []float64{1, 2, 3, 4},
		}
		a := avg(h)
		assert.Equal(t, 0.0, a)
	})
}

func TestHistogramPercentiles(t *testing.T) {
	t.Run("should correctly compute the percentiles of a given histogram", func(t *testing.T) {
		h := &metrics.Float64Histogram{
			Counts:  []uint64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			Buckets: []float64{0, 10, 20, 30, 40, 50, 60, 70, 80, 90, 100},
		}
		p := percentiles(h, []float64{0, 0.5, 0.95, 0.99, 1})
		assert.InDeltaSlice(t, []float64{0, 69.2, 97.25, 99.45, 100}, p, 0.1)
	})

	t.Run("should correctly compute the percentiles of a given histogram in the correct order", func(t *testing.T) {
		h := &metrics.Float64Histogram{
			Counts:  []uint64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			Buckets: []float64{0, 10, 20, 30, 40, 50, 60, 70, 80, 90, 100},
		}
		p := percentiles(h, []float64{0.5, 0.95, 1, 0, 0.99})
		assert.InDeltaSlice(t, []float64{69.2, 97.25, 100, 0, 99.45}, p, 0.1)
	})

	t.Run("should correctly compute the percentiles of a histogram containing buckets spanning from -inf to +inf", func(t *testing.T) {
		h := &metrics.Float64Histogram{
			Counts:  []uint64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			Buckets: []float64{math.Inf(-1), 10, 20, 30, 40, 50, 60, 70, 80, 90, math.Inf(+1)},
		}
		p := percentiles(h, []float64{0, 0.5, 0.95, 0.99, 1})
		assert.InDeltaSlice(t, []float64{10, 69.2, 90, 90, 90}, p, 0.1)
	})

	t.Run("should panic when given <0 percentiles to compute", func(t *testing.T) {
		h := &metrics.Float64Histogram{
			Counts:  []uint64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			Buckets: []float64{0, 10, 20, 30, 40, 50, 60, 70, 80, 90, 100},
		}
		assert.Panics(t, func() { percentiles(h, []float64{-1}) })
	})

	t.Run("should panic when given >1 percentiles to compute", func(t *testing.T) {
		h := &metrics.Float64Histogram{
			Counts:  []uint64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			Buckets: []float64{0, 10, 20, 30, 40, 50, 60, 70, 80, 90, 100},
		}
		assert.Panics(t, func() { percentiles(h, []float64{25}) })
	})

	t.Run("return 0 when there is a single -inf, +inf bucket", func(t *testing.T) {
		h := &metrics.Float64Histogram{
			Counts:  []uint64{10},
			Buckets: []float64{math.Inf(-1), math.Inf(+1)},
		}
		a := percentiles(h, []float64{0, 0.5, 0.95, 0.99, 1})
		assert.Equal(t, []float64{0, 0, 0, 0, 0}, a)
	})

	t.Run("return 0 when the histogram is empty", func(t *testing.T) {
		h := &metrics.Float64Histogram{
			Counts:  []uint64{0, 0, 0},
			Buckets: []float64{1, 2, 3, 4},
		}
		a := percentiles(h, []float64{0, 0.5, 0.95, 0.99, 1})
		assert.Equal(t, []float64{0, 0, 0, 0, 0}, a)
	})
}
