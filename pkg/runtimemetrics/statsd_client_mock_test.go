package runtimemetrics

import (
	"strings"
	"testing"
	"time"
)

// statsdClientMock is a hand-rolled mock for partialStatsdClientInterface. Not
// using any mocking library to reduce dependencies for a future move into
// dd-trace-go.
type statsdClientMock struct {
	// Discard causes all calls to be discarded rather than tracked.
	Discard bool

	gaugeCall              []statsdCall[float64]
	countCall              []statsdCall[int64]
	distributionSampleCall []statsdCall[[]float64]
}

// GaugeWithTimestamp implements partialStatsdClientInterface.
func (s *statsdClientMock) GaugeWithTimestamp(name string, value float64, tags []string, rate float64, _ time.Time) error {
	if s.Discard {
		return nil
	}
	s.gaugeCall = append(s.gaugeCall, statsdCall[float64]{
		name:  name,
		value: value,
		tags:  tags,
		rate:  rate,
	})
	return nil
}

// CountWithTimestamp implements partialStatsdClientInterface.
func (s *statsdClientMock) CountWithTimestamp(name string, value int64, tags []string, rate float64, _ time.Time) error {
	if s.Discard {
		return nil
	}
	s.countCall = append(s.countCall, statsdCall[int64]{
		name:  name,
		value: value,
		tags:  tags,
		rate:  rate,
	})
	return nil
}

func (s *statsdClientMock) DistributionSamples(name string, values []float64, tags []string, rate float64) error {
	if s.Discard {
		return nil
	}
	s.distributionSampleCall = append(s.distributionSampleCall, statsdCall[[]float64]{
		name:  name,
		value: values,
		tags:  tags,
		rate:  rate,
	})
	return nil
}

type statsdCall[T int64 | float64 | []float64] struct {
	name  string
	value T
	tags  []string
	rate  float64
}

func mockCallsWith[T int64 | float64 | []float64](calls []statsdCall[T], filter func(statsdCall[T]) bool) []statsdCall[T] {
	var results []statsdCall[T]
	for _, c := range calls {
		if filter(c) {
			results = append(results, c)
		}
	}
	return results
}

func mockCallsWithSuffix[T int64 | float64 | []float64](calls []statsdCall[T], suffix string) []statsdCall[T] {
	return mockCallsWith(calls, func(c statsdCall[T]) bool {
		return strings.HasSuffix(c.name, suffix)
	})
}

func mockCallWithSuffix[T int64 | float64 | []float64](t *testing.T, calls []statsdCall[T], suffix string) statsdCall[T] {
	t.Helper()
	candidates := mockCallsWithSuffix(calls, suffix)
	if len(candidates) != 1 {
		t.Fatalf("expected 1 call with suffix %s, got %d", suffix, len(candidates))
	}
	return candidates[0]
}
