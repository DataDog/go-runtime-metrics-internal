//go:build go1.26

package runtimemetrics

import (
	"log/slog"
	"runtime"
	"runtime/metrics"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGo126Metrics verifies that the new Go 1.26 goroutine and thread metrics
// are present in the runtime and correctly handled by the library.
func TestGo126Metrics(t *testing.T) {
	newMetrics := []string{
		"/sched/goroutines-created:goroutines",
		"/sched/goroutines/not-in-go:goroutines",
		"/sched/goroutines/runnable:goroutines",
		"/sched/goroutines/running:goroutines",
		"/sched/goroutines/waiting:goroutines",
		"/sched/threads/total:threads",
	}

	t.Run("new metrics exist in runtime", func(t *testing.T) {
		allMetrics := metrics.All()
		metricMap := make(map[string]bool)
		for _, m := range allMetrics {
			metricMap[m.Name] = true
		}

		for _, name := range newMetrics {
			assert.True(t, metricMap[name], "metric %s should exist in Go 1.26+", name)
		}
	})

	t.Run("new metrics are in supportedMetricsTable", func(t *testing.T) {
		for _, name := range newMetrics {
			_, ok := supportedMetricsTable[name]
			assert.True(t, ok, "metric %s should be in supportedMetricsTable", name)
		}
	})

	t.Run("new metrics have correct types", func(t *testing.T) {
		for _, name := range newMetrics {
			desc := metricDesc(name, metrics.KindUint64)
			assert.Equal(t, metrics.KindUint64, desc.Kind, "metric %s should be KindUint64", name)
		}
	})

	t.Run("new metrics have correct cumulative flag", func(t *testing.T) {
		// goroutines-created is cumulative
		desc := metricDesc("/sched/goroutines-created:goroutines", metrics.KindUint64)
		assert.True(t, desc.Cumulative, "goroutines-created should be cumulative")

		// All other new metrics are gauges (non-cumulative)
		gaugeMetrics := []string{
			"/sched/goroutines/not-in-go:goroutines",
			"/sched/goroutines/runnable:goroutines",
			"/sched/goroutines/running:goroutines",
			"/sched/goroutines/waiting:goroutines",
			"/sched/threads/total:threads",
		}
		for _, name := range gaugeMetrics {
			desc := metricDesc(name, metrics.KindUint64)
			assert.False(t, desc.Cumulative, "metric %s should not be cumulative", name)
		}
	})

	t.Run("new metrics generate correct Datadog names", func(t *testing.T) {
		expectedNames := map[string]string{
			"/sched/goroutines-created:goroutines":   "runtime.go.metrics.sched_goroutines_created.goroutines",
			"/sched/goroutines/not-in-go:goroutines": "runtime.go.metrics.sched_goroutines_not_in_go.goroutines",
			"/sched/goroutines/runnable:goroutines":  "runtime.go.metrics.sched_goroutines_runnable.goroutines",
			"/sched/goroutines/running:goroutines":   "runtime.go.metrics.sched_goroutines_running.goroutines",
			"/sched/goroutines/waiting:goroutines":   "runtime.go.metrics.sched_goroutines_waiting.goroutines",
			"/sched/threads/total:threads":           "runtime.go.metrics.sched_threads_total.threads",
		}

		for runtimeName, expectedDDName := range expectedNames {
			ddName, err := datadogMetricName(runtimeName)
			require.NoError(t, err, "should generate Datadog name for %s", runtimeName)
			assert.Equal(t, expectedDDName, ddName, "Datadog name for %s", runtimeName)
		}
	})

	t.Run("goroutines-created cumulative metric", func(t *testing.T) {
		// Note: This test could fail if background goroutines are created
		// by the test framework or race detector between measurements.
		mock, rms := reportMetric("/sched/goroutines-created:goroutines", metrics.KindUint64)

		// Create some goroutines to increment the counter
		done := make(chan struct{})
		for i := 0; i < 10; i++ {
			go func() {
				<-done
			}()
		}
		defer close(done)

		// Report again - should show the delta from goroutines created
		rms.report()

		calls := mockCallsWithSuffix(mock.GaugeCalls(), ".sched_goroutines_created.goroutines")
		// Cumulative metrics are only reported when they change, so we should have at least 1 call
		require.GreaterOrEqual(t, len(calls), 1, "goroutines-created should be reported at least once")
		// The last reported value should be > 0
		require.Greater(t, calls[len(calls)-1].value, 0.0, "goroutines-created should be > 0")
	})

	t.Run("gauge metrics report current state", func(t *testing.T) {
		descs := supportedMetrics()
		mock := &statsdClientMock{}
		rms := newRuntimeMetricStore(descs, mock, slog.Default(), nil)

		// Create some goroutines to ensure we have interesting values
		done := make(chan struct{})
		for i := 0; i < 10; i++ {
			go func() {
				<-done
			}()
		}
		defer close(done)

		// Trigger a GC to populate metrics
		runtime.GC()
		rms.report()

		// threads/total should always be reported and > 0
		threadCalls := mockCallsWithSuffix(mock.GaugeCalls(), ".sched_threads_total.threads")
		require.Equal(t, 1, len(threadCalls), "threads/total should be reported")
		require.Greater(t, threadCalls[0].value, 0.0, "threads/total should be > 0")

		// At least one of the goroutine state metrics should be reported
		runningCalls := mockCallsWithSuffix(mock.GaugeCalls(), ".sched_goroutines_running.goroutines")
		waitingCalls := mockCallsWithSuffix(mock.GaugeCalls(), ".sched_goroutines_waiting.goroutines")
		require.Greater(t, len(runningCalls)+len(waitingCalls), 0, "at least one goroutine state metric should be reported")
	})
}
