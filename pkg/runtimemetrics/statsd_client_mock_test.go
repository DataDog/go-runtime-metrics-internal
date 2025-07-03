package runtimemetrics

import (
	"strings"
	"sync"
	"testing"
	"time"
)

// statsdClientMock is a hand-rolled mock for partialStatsdClientInterface. Not
// using any mocking library to reduce dependencies for a future move into
// dd-trace-go.
type statsdClientMock struct {
	// Discard causes all calls to be discarded rather than tracked.
	Discard bool

	mu                     sync.RWMutex
	gaugeCall              []statsdCall[float64]
	countCall              []statsdCall[int64]
	distributionSampleCall []statsdCall[[]float64]
}

// GaugeWithTimestamp implements partialStatsdClientInterface.
func (s *statsdClientMock) GaugeWithTimestamp(name string, value float64, tags []string, rate float64, _ time.Time) error {
	if s.Discard {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
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
	s.mu.Lock()
	defer s.mu.Unlock()
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
	s.mu.Lock()
	defer s.mu.Unlock()
	s.distributionSampleCall = append(s.distributionSampleCall, statsdCall[[]float64]{
		name:  name,
		value: values,
		tags:  tags,
		rate:  rate,
	})
	return nil
}

// Thread-safe accessors for the call slices
func (s *statsdClientMock) GaugeCalls() []statsdCall[float64] {
	s.mu.RLock()
	defer s.mu.RUnlock()
	calls := make([]statsdCall[float64], len(s.gaugeCall))
	copy(calls, s.gaugeCall)
	return calls
}

func (s *statsdClientMock) CountCalls() []statsdCall[int64] {
	s.mu.RLock()
	defer s.mu.RUnlock()
	calls := make([]statsdCall[int64], len(s.countCall))
	copy(calls, s.countCall)
	return calls
}

func (s *statsdClientMock) DistributionSampleCalls() []statsdCall[[]float64] {
	s.mu.RLock()
	defer s.mu.RUnlock()
	calls := make([]statsdCall[[]float64], len(s.distributionSampleCall))
	copy(calls, s.distributionSampleCall)
	return calls
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
