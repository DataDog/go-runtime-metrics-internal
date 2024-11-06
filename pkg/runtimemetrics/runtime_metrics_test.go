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

func TestStart(t *testing.T) {
	cleanup := func() {
		mu.Lock()
		enabled = false
		mu.Unlock()
	}

	t.Run("start returns an error when called successively", func(t *testing.T) {
		t.Cleanup(cleanup)
		err := Start(&statsdClientMock{}, slog.Default())
		assert.NoError(t, err)

		err = Start(&statsdClientMock{}, slog.Default())
		assert.Error(t, err)
	})

	t.Run("should not race with other start calls", func(t *testing.T) {
		t.Cleanup(cleanup)
		wg := sync.WaitGroup{}
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				Start(&statsdClientMock{}, slog.Default())
				wg.Done()
			}()
		}
		wg.Wait()
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
			require.Equal(t, 1, len(mock.gaugeCall))
			require.Equal(t, 123.0, mock.gaugeCall[0].value)
			require.True(t, strings.HasSuffix(mock.gaugeCall[0].name, ".gc_gogc.percent"))
		})

		t.Run("Cumulative", func(t *testing.T) {
			// Note: This test could fail if an unexpected GC occurs. This
			// should be extremely unlikely.
			mock, rms := reportMetric("/gc/cycles/total:gc-cycles", metrics.KindUint64)
			require.Equal(t, 1, len(mock.gaugeCall))
			require.GreaterOrEqual(t, mock.gaugeCall[0].value, 1.0)
			require.True(t, strings.HasSuffix(mock.gaugeCall[0].name, ".gc_cycles_total.gc_cycles"), mock.gaugeCall[0].name)
			// Note: Only these two GC cycles are expected to occur here
			runtime.GC()
			runtime.GC()
			rms.report()
			require.Equal(t, 2, len(mock.gaugeCall))
			require.Greater(t, mock.gaugeCall[1].value, mock.gaugeCall[0].value)
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
			beforeCallCount := len(mock.gaugeCall)
			require.LessOrEqual(t, beforeCallCount, 1)
			createLockContention(100 * time.Millisecond)
			rms.report()
			require.Equal(t, beforeCallCount+1, len(mock.gaugeCall))
			require.Greater(t, mock.gaugeCall[beforeCallCount].value, 0.0)
			require.True(t, strings.HasSuffix(mock.gaugeCall[0].name, ".sync_mutex_wait_total.seconds"), mock.gaugeCall[0].name)
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
			require.Equal(t, len(summaries), len(mock.gaugeCall))
			for _, summary := range summaries {
				want := ".gc_pauses.seconds." + summary
				found := false
				for _, call := range mock.gaugeCall {
					if strings.HasSuffix(call.name, want) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("missing %s metric", want)
				}
			}
			rms.report()
			// Note: No GC cycle is expected to occur here
			require.Equal(t, len(summaries), len(mock.gaugeCall))
			// Note: Only this GC cycle is expected to occur here
			runtime.GC()
			rms.report()
			require.Equal(t, len(summaries)*2, len(mock.gaugeCall))
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
	rms := newRuntimeMetricStore(descs, mock, slog.Default())

	// This poulates most runtime/metrics.
	runtime.GC()

	// But nothing should be sent to statsd yet.
	assert.Equal(t, 0, len(mock.gaugeCall))

	// Flush the current metrics to our statsd mock.
	rms.report()

	// The exact number of statsd calls depends on the metric values and may
	// also change as new version of Go are being released. So we assert that we
	// get roughly the expected number of statsd calls (+/- 50%). This is meant
	// to catch severe regression. Might need to be updated in the future if
	// lots of new metrics are added.
	assert.InDelta(t, 87, len(mock.gaugeCall), 87/2) // typically 87
}

// BenchmarkReport is used to determine the overhead of collecting all metrics
// and discarding them in a statsd mock. This can be used as a stress test,
// identify regressions and to inform decisions about pollFrequency.
func BenchmarkReport(b *testing.B) {
	// Initialize store for all metrics with a mocked statsd client.
	descs := metrics.All()
	mock := &statsdClientMock{Discard: true}
	rms := newRuntimeMetricStore(descs, mock, slog.Default())

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
	rms := newRuntimeMetricStore([]metrics.Description{desc}, mock, slog.Default())
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
