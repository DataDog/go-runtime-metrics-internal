package runtimemetrics

import (
	"fmt"
	"math"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func assertTagValue(t *testing.T, tagName, expectedTagValue string, actualTags []string) {
	t.Helper()

	expectedTag := fmt.Sprintf("%s:%s", tagName, expectedTagValue)

	for _, tag := range actualTags {
		if strings.HasPrefix(tag, fmt.Sprintf("%s:", tagName)) {
			if tag != expectedTag {
				t.Errorf("expected tag %s, got %s", expectedTag, tag)
			}
			return
		}
	}
	t.Errorf("tag %s not found", tagName)
}

func TestGetBaseTags(t *testing.T) {
	gogcTests := []struct {
		name     string
		gogc     int
		expected string
	}{
		{
			"should return the correct value for a specific gogc value",
			154,
			"154",
		},
		{
			// GOGC=0 means that the GC is running continuously
			// https://github.com/golang/go/issues/39419#issuecomment-640066815
			"should return zero when gogc is zero",
			0,
			"0",
		},
		{
			"should return off when gogc if off",
			-1,
			"off",
		},
	}

	for _, tt := range gogcTests {
		t.Run(tt.name, func(t *testing.T) {
			old := debug.SetGCPercent(tt.gogc)
			defer debug.SetGCPercent(old)

			assertTagValue(t, "gogc", tt.expected, getBaseTags())
		})
	}

	gomemlimitTests := []struct {
		name       string
		gomemlimit int64
		expected   string
	}{
		{
			"should return the correct value for a specific gomemlimit value",
			123456789,
			formatByteSize(123456789),
		},
		{
			"should return zero when gomemlimit is zero",
			0,
			formatByteSize(0),
		},
		{
			"should return unlimited when gomemlimit if off",
			math.MaxInt64,
			"unlimited",
		},
	}

	for _, tt := range gomemlimitTests {
		t.Run(tt.name, func(t *testing.T) {
			old := debug.SetMemoryLimit(tt.gomemlimit)
			defer debug.SetMemoryLimit(old)

			assertTagValue(t, "gomemlimit", tt.expected, getBaseTags())
		})
	}

	t.Run("should return the correct value for a specific gomaxprocs value", func(t *testing.T) {
		old := runtime.GOMAXPROCS(42)
		defer runtime.GOMAXPROCS(old)

		assertTagValue(t, "gomaxprocs", "42", getBaseTags())
	})

	t.Run("should return the correct goversion", func(t *testing.T) {
		assertTagValue(t, "goversion", runtime.Version(), getBaseTags())
	})
}

func TestFormatByteSize(t *testing.T) {
	t.Run("should format byte size correctly", func(t *testing.T) {
		tests := []struct {
			bytes    uint64
			expected string
		}{
			{0, "0 B"},
			{1023, "1023 B"},
			{1024, "1 KiB"},
			{1025, "1 KiB"},
			{1024 * 1024, "1 MiB"},
			{1024 * 1024 * 1024, "1 GiB"},
			{1024 * 1024 * 1024 * 1024, "1 TiB"},
			{1024 * 1024 * 1024 * 1024 * 1024, "1 PiB"},
			{1024 * 1024 * 1024 * 1024 * 1024 * 1024, "1 EiB"},
		}

		for _, test := range tests {
			result := formatByteSize(test.bytes)
			assert.Equal(t, test.expected, result)
		}
	})
}

func TestTagRefresher(t *testing.T) {
	newCountSource := func() func() []string {
		count := 0
		return func() []string {
			count++
			return []string{fmt.Sprintf("count:%d", count)}
		}
	}
	newClock := func() (func() time.Time, func(time.Duration)) {
		now := time.Now()
		get := func() time.Time { return now }
		add := func(d time.Duration) { now = now.Add(d) }
		return get, add
	}

	t.Run("returns the correct tag on first call", func(t *testing.T) {
		getTime, _ := newClock()
		refresher := newTagCacher(1*time.Second, getTime, newCountSource())
		tags := refresher()
		assert.Equal(t, []string{"count:1"}, tags)
	})

	t.Run("caches the tag for the interval", func(t *testing.T) {
		getTime, addTime := newClock()
		refresher := newTagCacher(5*time.Second, getTime, newCountSource())

		tags := refresher()
		assert.Equal(t, []string{"count:1"}, tags)

		addTime(time.Second)
		tags = refresher()
		assert.Equal(t, []string{"count:1"}, tags)

		addTime(3 * time.Second)
		tags = refresher()
		assert.Equal(t, []string{"count:1"}, tags)
	})

	t.Run("updates the tag when the interval elapses", func(t *testing.T) {
		getTime, addTime := newClock()
		refresher := newTagCacher(5*time.Second, getTime, newCountSource())

		tags := refresher()
		assert.Equal(t, []string{"count:1"}, tags)

		addTime(5 * time.Second)
		tags = refresher()
		assert.Equal(t, []string{"count:2"}, tags)

		addTime(5 * time.Second)
		tags = refresher()
		assert.Equal(t, []string{"count:3"}, tags)
	})
}
