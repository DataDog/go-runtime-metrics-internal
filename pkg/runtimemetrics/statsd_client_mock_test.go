package runtimemetrics

import "time"

// statsdClientMock is a hand-rolled mock for partialStatsdClientInterface. Not
// using any mocking library to reduce dependencies for a future move into
// dd-trace-go.
type statsdClientMock struct {
	// Discard causes all calls to be discarded rather than tracked.
	Discard bool

	gaugeCall []statsdCall[float64]
	countCall []statsdCall[int64]
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

type statsdCall[T int64 | float64] struct {
	name  string
	value T
	tags  []string
	rate  float64
}
