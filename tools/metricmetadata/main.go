// This tool generates a CSV file listing all metrics collected by the go-runtime-metrics library.
// It uses go:linkname to access unexported functions from the runtimemetrics package.
// This approach allows the tool to stay in sync with the library's internal implementation
// without polluting the public API with functions that are only needed for tooling.
// It panics on any unexpected conditions to ensure issues are caught early.

package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"regexp"
	"runtime/metrics"
	"sort"
	"strings"
	_ "unsafe" // Required for go:linkname

	// Need to import the package to establish the linkage
	_ "github.com/DataDog/go-runtime-metrics-internal/pkg/runtimemetrics"
)

// Link to unexported functions and variables in the runtimemetrics package
//
//go:linkname supportedMetricsTable github.com/DataDog/go-runtime-metrics-internal/pkg/runtimemetrics.supportedMetricsTable
var supportedMetricsTable map[string]struct{}

//go:linkname datadogMetricName github.com/DataDog/go-runtime-metrics-internal/pkg/runtimemetrics.datadogMetricName
func datadogMetricName(runtimeName string) (string, error)

// regex extracted from https://cs.opensource.google/go/go/+/refs/tags/go1.20.3:src/runtime/metrics/description.go;l=13
var runtimeMetricRegex = regexp.MustCompile(`^(?P<name>/[^:]+):(?P<unit>[^:*/]+(?:[*/][^:*/]+)*)$`)

// Metric represents a metric with its details in Datadog's metadata format.
// See: https://docs.datadoghq.com/developers/integrations/check_references/#metrics-metadata-file
type Metric struct {
	MetricName    string
	MetricType    string
	Interval      string
	UnitName      string
	PerUnitName   string
	Description   string
	Orientation   string
	Integration   string
	ShortName     string
	CuratedMetric string
	SampleTags    string
}

var (
	// runtimeUnitMapping maps runtime metric units to their Datadog units
	// Empty string means no standard Datadog unit exists
	runtimeUnitMapping = map[string]string{
		// Standard units
		"bytes":   "byte",
		"seconds": "second",
		"threads": "thread",
		"objects": "object",
		"percent": "percent",
		"events":  "event",

		// Units without standard Datadog equivalents
		"goroutines":  "",
		"cpu-seconds": "",
		"gc-cycles":   "",
		"gc-cycle":    "",
		"calls":       "",
	}

	// histogramStats defines all histogram statistics with their descriptions
	histogramStats = []struct {
		suffix     string
		descPrefix string
	}{
		{"avg", "Average"},
		{"min", "Minimum"},
		{"max", "Maximum"},
		{"median", "Median"},
		{"p95", "95th percentile"},
		{"p99", "99th percentile"},
	}

	// specialMetrics are handled separately with their units defined directly
	specialMetrics = []Metric{
		{
			MetricName:  "runtime.go.metrics.enabled",
			MetricType:  "gauge",
			Description: "Indicator that runtime metrics collection is enabled (always 1)",
			Orientation: "0",
			Integration: "go-runtime-metrics-v2",
			ShortName:   "enabled",
		},
		{
			MetricName:  "runtime.go.metrics.skipped_values",
			MetricType:  "count",
			Description: "Count of metric values skipped due to invalid data",
			Orientation: "-1",
			Integration: "go-runtime-metrics-v2",
			ShortName:   "skipped_values",
		},
	}
)

func isHistogram(runtimeName string) bool {
	for _, desc := range metrics.All() {
		if desc.Name == runtimeName {
			return desc.Kind == metrics.KindFloat64Histogram
		}
	}
	panic(fmt.Sprintf("metric %s not found in runtime/metrics", runtimeName))
}

func getDescription(runtimeName string) string {
	for _, desc := range metrics.All() {
		if desc.Name == runtimeName {
			description := desc.Description

			// Replace runtime metric references with their corresponding Datadog metric names
			words := strings.Fields(description)
			for i, word := range words {
				// Remove any trailing punctuation to get the clean word
				cleanWord := strings.TrimRight(word, ".,;:()")

				// Check if the clean word matches the runtime metric pattern
				if runtimeMetricRegex.MatchString(cleanWord) {
					if ddName, err := datadogMetricName(cleanWord); err == nil {
						// Replace with the Datadog metric name, preserving any trailing punctuation
						suffix := word[len(cleanWord):]
						words[i] = ddName + suffix
					}
				}
			}
			return strings.Join(words, " ")
		}
	}
	panic(fmt.Sprintf("metric %s not found in runtime/metrics", runtimeName))
}

// mapRuntimeUnit maps a runtime unit to its Datadog equivalent
func mapRuntimeUnit(runtimeUnit, runtimeName string) string {
	datadogUnit, exists := runtimeUnitMapping[runtimeUnit]
	if !exists {
		panic(fmt.Sprintf("unknown runtime unit '%s' in metric %s", runtimeUnit, runtimeName))
	}
	return datadogUnit
}

// processDescription ensures descriptions fit within the backend's 400 character limit
func processDescription(desc string, runtimeName string) string {
	const maxLength = 400
	const linkText = " For more information, see: https://pkg.go.dev/runtime/metrics."

	if len(desc) <= maxLength {
		return desc
	}

	// First sentence + link to see more information
	if idx := strings.Index(desc, ". "); idx > 0 && len(desc[:idx+1]+linkText) <= maxLength {
		return desc[:idx+1] + linkText
	}

	maxTextLength := maxLength - len(linkText) - 3
	truncated := desc[:maxTextLength]
	if lastSpace := strings.LastIndex(truncated, " "); lastSpace > 0 {
		return truncated[:lastSpace] + "..." + linkText
	}

	return truncated + "..." + linkText // If no word boundary found
}

// getOrientation returns the orientation for a metric (-1, 0, or 1)
func getOrientation(metricPath string) string {
	// Lower is better (-1) for pause times, latencies, errors, and GC overhead
	lowerIsBetter := []string{"pauses", "latencies", "/cpu/classes/gc/"}

	// Check patterns in runtime name
	for _, pattern := range lowerIsBetter {
		if strings.Contains(metricPath, pattern) {
			return "-1"
		}
	}
	return "0"
}

func getShortName(metricPath string) string {
	path := strings.TrimPrefix(metricPath, "/")

	replacer := strings.NewReplacer(
		"/", " ",
		"-", " ",
		"classes", "",
		"automatic", "auto",
	)
	shortName := replacer.Replace(path)

	return strings.Join(strings.Fields(shortName), " ")
}

func createMetric(name, metricType, unit, desc, orientation, shortName string) Metric {
	return Metric{
		MetricName:    name,
		MetricType:    metricType,
		UnitName:      unit,
		Description:   desc,
		Orientation:   orientation,
		Integration:   "go-runtime-metrics-v2",
		ShortName:     shortName,
		Interval:      "",
		PerUnitName:   "",
		CuratedMetric: "",
		SampleTags:    "",
	}
}

func writeCSV(metrics []Metric) {
	file, err := os.Create("metadata.csv")
	if err != nil {
		panic(fmt.Sprintf("failed to create CSV file: %v", err))
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	header := []string{
		"metric_name", "metric_type", "interval", "unit_name", "per_unit_name",
		"description", "orientation", "integration", "short_name",
		"curated_metric", "sample_tags",
	}
	if err := writer.Write(header); err != nil {
		panic(fmt.Sprintf("failed to write header: %v", err))
	}

	for _, m := range metrics {
		record := []string{
			m.MetricName, m.MetricType, m.Interval, m.UnitName, m.PerUnitName,
			m.Description, m.Orientation, m.Integration, m.ShortName,
			m.CuratedMetric, m.SampleTags,
		}
		if err := writer.Write(record); err != nil {
			panic(fmt.Sprintf("failed to write metric %s: %v", m.MetricName, err))
		}
	}
}

func main() {
	metrics := append([]Metric{}, specialMetrics...)

	for runtimeName := range supportedMetricsTable {
		description := getDescription(runtimeName)

		// Parse the runtime metric name using regex
		matches := runtimeMetricRegex.FindStringSubmatch(runtimeName)
		if matches == nil {
			panic(fmt.Sprintf("runtime metric name does not follow expected format: %s", runtimeName))
		}

		// Extract components using named capture groups
		nameIndex := runtimeMetricRegex.SubexpIndex("name")
		unitIndex := runtimeMetricRegex.SubexpIndex("unit")
		metricPath := matches[nameIndex]
		runtimeUnit := matches[unitIndex]

		ddName, err := datadogMetricName(runtimeName)
		if err != nil {
			panic(fmt.Sprintf("failed to transform metric %s: %v", runtimeName, err))
		}

		unit := mapRuntimeUnit(runtimeUnit, runtimeName)
		orientation := getOrientation(metricPath)
		shortName := getShortName(metricPath)

		if isHistogram(runtimeName) {
			metrics = append(metrics, createMetric(
				ddName, "distribution", unit, processDescription(description, runtimeName), orientation, shortName,
			))

			for _, stat := range histogramStats {
				statDescription := "(" + stat.descPrefix + ") " + description
				metrics = append(metrics, createMetric(
					ddName+"."+stat.suffix,
					"gauge",
					unit,
					processDescription(statDescription, runtimeName),
					orientation,
					stat.suffix+" "+shortName,
				))
			}
		} else {
			metrics = append(metrics, createMetric(
				ddName, "gauge", unit, processDescription(description, runtimeName), orientation, shortName,
			))
		}
	}

	sort.Slice(metrics, func(i, j int) bool {
		return metrics[i].MetricName < metrics[j].MetricName
	})

	writeCSV(metrics)

	fmt.Printf("Successfully generated metadata.csv with %d metrics\n", len(metrics))
}
