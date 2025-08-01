# metricmetadata

A tool that generates a CSV file listing all the metrics submitted by the go-runtime-metrics-internal library

### Usage

```bash
cd tools/metricmetadata
go run main.go
```

This will generate a `metadata.csv` file in Datadog's standard metadata format.

Once the file is generated, it should be manually uploaded to the Go Runtime Metrics v2 integration page.

For a detailed description of the CSV columns and their meanings, see the [Datadog metric metadata documentation](https://docs.datadoghq.com/developers/integrations/check_references/#metrics-metadata-file).

### Output

The tool generates a CSV file with all metrics including:
- Runtime Go metrics transformed from the Go runtime/metrics format
- Special metrics like `runtime.go.metrics.enabled` and `runtime.go.metrics.skipped_values`
- For histogram-type metrics, it includes the distribution metric plus statistical aggregates (.avg, .min, .max, .median, .p95, .p99)
