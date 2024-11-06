package runtimemetrics

import (
	"fmt"
	"math"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"

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

			tags := getBaseTags()
			assertTagValue(t, "gogc", tt.expected, tags)
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

			tags := getBaseTags()
			assertTagValue(t, "gomemlimit", tt.expected, tags)
		})
	}

	t.Run("should return the correct value for a specific gomaxprocs value", func(t *testing.T) {
		old := runtime.GOMAXPROCS(42)
		defer runtime.GOMAXPROCS(old)

		tags := getBaseTags()
		assertTagValue(t, "gomaxprocs", "42", tags)
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
