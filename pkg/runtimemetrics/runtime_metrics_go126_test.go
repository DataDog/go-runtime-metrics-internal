//go:build go1.26

package runtimemetrics

import (
	"runtime/metrics"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestGo126Metrics verifies that the new Go 1.26 goroutine and thread metrics
// flow through the reporting pipeline and produce statsd output.
func TestGo126Metrics(t *testing.T) {
	t.Run("Non-Cumulative", func(t *testing.T) {
		mock, _ := reportMetric("/sched/threads/total:threads", metrics.KindUint64)
		require.Greater(t, mockCallWithSuffix(t, mock.GaugeCalls(), ".sched_threads_total.threads").value, 0.0)
	})

	t.Run("Cumulative", func(t *testing.T) {
		mock, rms := reportMetric("/sched/goroutines-created:goroutines", metrics.KindUint64)

		// Create goroutines to increment the counter
		done := make(chan struct{})
		for i := 0; i < 10; i++ {
			go func() {
				<-done
			}()
		}
		defer close(done)

		rms.report()
		calls := mockCallsWithSuffix(mock.GaugeCalls(), ".sched_goroutines_created.goroutines")
		require.GreaterOrEqual(t, len(calls), 1)
		require.Greater(t, calls[len(calls)-1].value, 0.0)
	})
}
