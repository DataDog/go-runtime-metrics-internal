package runtimemetrics

import (
	"fmt"
	"log/slog"
	"runtime"
	"runtime/debug"
	"runtime/metrics"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmitter(t *testing.T) {
	// TODO: Use testing/synctest in go1.25 for this in the future.
	t.Run("should emit metrics", func(t *testing.T) {
		// Start the emitter and wait until some metrics are submitted.
		statsd := &statsdClientMock{}
		emitter := NewEmitter(statsd, &Options{Logger: slog.Default(), Period: 1 * time.Millisecond})
		require.NotNil(t, emitter)
		require.Eventually(t, func() bool {
			return len(statsd.GaugeCalls()) > 0
		}, time.Second, 1*time.Millisecond)

		// After Stop, no more metrics should be submitted.
		emitter.Stop()
		calls := statsd.GaugeCalls()
		time.Sleep(10 * time.Millisecond)
		finalCalls := statsd.GaugeCalls()
		require.Equal(t, len(calls), len(finalCalls))

		// Stop should be idempotent.
		emitter.Stop()
	})

	t.Run("should not panic on nil options", func(t *testing.T) {
		emitter := NewEmitter(&statsdClientMock{}, nil)
		require.NotNil(t, emitter)
		emitter.Stop()
	})
}

func TestDatadogMetricName(t *testing.T) {
	t.Run("should return a metric name without any error for all runtime metrics", func(t *testing.T) {
		for _, m := range metrics.All() {
			ddMetricName, err := datadogMetricName(m.Name)
			require.NoError(t, err)
			assert.NotEmpty(t, ddMetricName)
			assert.True(t, strings.HasPrefix(ddMetricName, "runtime.go.metrics."))
			assert.False(t, strings.HasSuffix(ddMetricName, "."))
		}
	})

	t.Run("should return an error for an unsupported metric name", func(t *testing.T) {
		ddMetricName, err := datadogMetricName("Lorem Ipsum")
		require.Error(t, err)
		assert.Empty(t, ddMetricName)
	})
}

// TestMetricKinds is an integration test that tests one metric for each
// metrics.ValueKind that exists.
func TestMetricKinds(t *testing.T) {
	descs := metrics.All()
	t.Run("KindUint64", func(t *testing.T) {
		t.Run("Non-Cumulative", func(t *testing.T) {
			old := debug.SetGCPercent(123)
			defer debug.SetGCPercent(old)
			mock, _ := reportMetric("/gc/gogc:percent", metrics.KindUint64)
			require.Equal(t, 123.0, mockCallWithSuffix(t, mock.GaugeCalls(), ".gc_gogc.percent").value)
		})

		t.Run("Cumulative", func(t *testing.T) {
			// Note: This test could fail if an unexpected GC occurs. This
			// should be extremely unlikely.
			mock, rms := reportMetric("/gc/cycles/total:gc-cycles", metrics.KindUint64)
			require.GreaterOrEqual(t, mockCallWithSuffix(t, mock.GaugeCalls(), ".gc_cycles_total.gc_cycles").value, 1.0)
			// Note: Only these two GC cycles are expected to occur here
			runtime.GC()
			runtime.GC()
			rms.report()
			calls := mockCallsWithSuffix(mock.GaugeCalls(), ".gc_cycles_total.gc_cycles")
			require.Equal(t, 2, len(calls))
			require.Greater(t, calls[1].value, calls[0].value)
		})
	})

	t.Run("KindFloat64", func(t *testing.T) {
		t.Run("Non-Cumulative", func(t *testing.T) {
			// There are no non-cumulative float64 metrics right now. So let's
			// just print a log message in case this changes in the future.
			for _, d := range descs {
				if d.Kind == metrics.KindFloat64 && !d.Cumulative {
					t.Logf("unexpected non-cumulative float64 metric: %s", d.Name)
				}
			}
		})

		t.Run("Cumulative", func(t *testing.T) {
			// Note: This test could fail if we get extremely unlucky with the
			// scheduling. This should be extremely unlikely.
			mock, rms := reportMetric("/sync/mutex/wait/total:seconds", metrics.KindFloat64)

			// With Go 1.22: mutex wait sometimes increments when calling runtime.GC().
			// This does not seem to happen with Go <= 1.21
			beforeCalls := mockCallsWithSuffix(mock.GaugeCalls(), ".sync_mutex_wait_total.seconds")
			require.LessOrEqual(t, len(beforeCalls), 1)
			createLockContention(100 * time.Millisecond)
			rms.report()
			afterCalls := mockCallsWithSuffix(mock.GaugeCalls(), ".sync_mutex_wait_total.seconds")
			require.Equal(t, len(beforeCalls)+1, len(afterCalls))
			require.Greater(t, afterCalls[len(afterCalls)-1].value, 0.0)
		})
	})

	t.Run("KindFloat64Histogram", func(t *testing.T) {
		t.Run("Non-Cumulative", func(t *testing.T) {
			// There are no non-cumulative float64 histogram metrics right now.
			// So let's just print a log message in case this changes in the
			// future.
			for _, d := range descs {
				if d.Kind == metrics.KindFloat64Histogram && !d.Cumulative {
					t.Logf("unexpected non-cumulative float64 metric: %s", d.Name)
				}
			}
		})

		t.Run("Cumulative", func(t *testing.T) {
			summaries := []string{"avg", "min", "max", "median", "p95", "p99"}
			// Note: This test could fail if an unexpected GC occurs. This
			// should be extremely unlikely.
			mock, rms := reportMetric("/gc/pauses:seconds", metrics.KindFloat64Histogram)
			calls1 := mockCallsWith(mock.GaugeCalls(), func(c statsdCall[float64]) bool {
				return strings.Contains(c.name, ".gc_pauses.seconds.")
			})
			require.Equal(t, len(summaries), len(calls1))
			for _, summary := range summaries {
				want := ".gc_pauses.seconds." + summary
				found := false
				for _, call := range mock.GaugeCalls() {
					if strings.HasSuffix(call.name, want) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("missing %s metric", want)
				}
			}
			found := false
			want := ".gc_pauses.seconds"
			for _, call := range mock.DistributionSampleCalls() {
				if strings.HasSuffix(call.name, want) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("missing %s metric", want)
			}
			rms.report()
			// Note: No GC cycle is expected to occur here
			calls2 := mockCallsWith(mock.GaugeCalls(), func(c statsdCall[float64]) bool {
				return strings.Contains(c.name, ".gc_pauses.seconds.")
			})
			require.Equal(t, len(summaries), len(calls2))
			// Note: Only this GC cycle is expected to occur here
			runtime.GC()
			rms.report()
			calls3 := mockCallsWith(mock.GaugeCalls(), func(c statsdCall[float64]) bool {
				return strings.Contains(c.name, ".gc_pauses.seconds.")
			})
			require.Equal(t, len(summaries)*2, len(calls3))
		})
	})
}

// TestSmoke is an integration test that is trying to read and report most
// metrics and check that we don't crash or produce a very unexpected number of
// metrics.
func TestSmoke(t *testing.T) {
	// Initialize store for all metrics with a mocked statsd client.
	descs := metrics.All()
	mock := &statsdClientMock{}
	rms := newRuntimeMetricStore(descs, mock, slog.Default(), []string{})

	// This poulates most runtime/metrics.
	runtime.GC()

	// But nothing should be sent to statsd yet.
	assert.Equal(t, 0, len(mock.GaugeCalls()))

	// Flush the current metrics to our statsd mock.
	rms.report()

	// The exact number of statsd calls depends on the metric values and may
	// also change as new version of Go are being released. So we assert that we
	// get roughly the expected number of statsd calls (+/- 50%). This is meant
	// to catch severe regression. Might need to be updated in the future if
	// lots of new metrics are added.
	assert.InDelta(t, 87, len(mock.GaugeCalls()), 87/2) // typically 87

	assert.Positive(t, len(mock.DistributionSampleCalls()))
}

// BenchmarkReport is used to determine the overhead of collecting all metrics
// and discarding them in a statsd mock. This can be used as a stress test,
// identify regressions and to inform decisions about pollFrequency.
func BenchmarkReport(b *testing.B) {
	// Initialize store for all metrics with a mocked statsd client.
	descs := metrics.All()
	mock := &statsdClientMock{Discard: true}
	rms := newRuntimeMetricStore(descs, mock, slog.Default(), []string{})

	// Benchmark report method
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rms.report()
	}
}

// reportMetric creates a metrics store for the given metric, hooks it up to a
// mock statsd client, triggers a GC cycle, calls report, and then returns
// both. Callers are expected to observe the calls recorded by the mock and/or
// trigger more activity.
func reportMetric(name string, kind metrics.ValueKind) (*statsdClientMock, runtimeMetricStore) {
	desc := metricDesc(name, kind)
	mock := &statsdClientMock{}
	rms := newRuntimeMetricStore([]metrics.Description{desc}, mock, slog.Default(), []string{})
	// Populate Metrics. Test implicitly expect this to be the only GC cycle to happen before report is finished.
	runtime.GC()
	rms.report()
	return mock, rms
}

// metricDesc looks up a metric by name and kind. The name alone should be
// unique, but we're trying to be extra precise here to ensure that our tests
// are doing exactly what we expect them to do.
func metricDesc(name string, kind metrics.ValueKind) metrics.Description {
	descs := metrics.All()
	for _, d := range descs {
		if d.Name == name && d.Kind == kind {
			return d
		}
	}
	panic(fmt.Sprintf("unknown metric: %s", name))
}

// createLockContention attempts to create a lot of lock contention during the
// given time window d. The runtime samples and upscales lock contention even
// for metrics, so we need to produce up to 8 (gTrackingPeriod) contention
// events per goroutine. If we get really unlucky with scheduling, we might fail
// to achieve this, causing test flakiness. But this should be extremely
// unlikely.
func createLockContention(d time.Duration) {
	var mu sync.Mutex
	start := time.Now()
	var wg sync.WaitGroup
	for i := 0; i < runtime.GOMAXPROCS(0)*10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for time.Since(start) < d {
				mu.Lock()
				time.Sleep(d / 100)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
}
