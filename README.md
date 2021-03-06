# Passenger Exporter

Prometheus exporter for [Phusion Passenger](https://www.phusionpassenger.com) metrics.

## Flags

```
  -log.format value
      If set use a syslog logger or JSON logging.
      Example: logger:syslog?appname=bob&local=7 or logger:stdout?json=true.
      Defaults to stderr.
  -log.level value
      Only log messages with the given severity or above.
      Valid levels: [debug, info, warn, error, fatal]. (default info)
  -passenger.command string
      Passenger command for querying passenger status.
      (default "passenger-status --show=xml")
  -passenger.pid-file string
    	Optional path to a file containing the passenger PID for additional metrics.
  -passenger.command.timeout-seconds float
      Timeout for passenger.command. (default 0.5 seconds)
  -web.listen-address string
      Address to listen on for web interface and telemetry. (default ":9149")
  -web.telemetry-path string
      Path under which to expose metrics. (default "/metrics")
```


## Running Tests

Tests can be run with:
```
go test .
```

Additionally, the test/scrape_output.txt can be regenerated by passing the
`--golden` flag:
```
go test -v . --golden
```
