# Changelog

## 1.0.0

### Breaking Changes
* Rename `app_count` metric to `app_group_count`.
* Update `proc_start_time_seconds` metric to correctly be in unix seconds.
* Update default value for `-passenger.command.timeout-seconds` flag to `5s`.
* Remove process metrics collector.
* Remomve `/` endpoint showing link to metrics path.

### Bug Fixes
* Prevent index out of range panics when the number of passenger processes surges past the max pool size temporarily when replacing an existing process.

### Improvements
* Upgrade to Go `v1.20.1`.
* Switch Go modules.
* Upgrade Go dependencies.
* Upgrade bundled Passenger to `v6.0.17`.
* Switch container image from Alpine Linux to Debian Bullseye.
* Use builder pattern to build binary and copy it into runner image.
* Run container as `exporter` user instead of `nobody`.
* Add new fields parsed from passenger status command.
* Use expected types in structs instead of parsing afterwards.
* Configure `promu` to use `netgo` instead of `installsuffix`.
* Switch to `sirupsen/logrus` logger as `prometheus/common` no longer includes it.
* Add launch configuration for debugging in Visual Studio Code.
* Add GitHub Actions workflow for testing and linting.
* Simplify Makefile for single command builds.

## 0.7.1

### Bug Fixes
* Prevent index out of range panics when passenger processes are killed.

## 0.7.0

* Change group to `nobody` instead of `nogroup`.

## 0.6.0

* Run as user `nobody` and group `nogroup` instead of `root`.
* Update Go to `v1.9.4`.

## 0.5.1

### Bug Fixes
* Include correct files in the docker build.

## 0.5.0

### Improvements
* Added home page with link to metrics.
* Added new fields to output parsed from passenger status command.
* Removed mentions of nginx as this exporter can support other integration modes.

### Breaking Changes
* Changed metrics prefix from `passenger_nginx` to `passenger`. This affects _all_ passenger metrics.
* Renamed metrics:
  * Changed `passenger_top_level_queue` to `passenger_top_level_request_queue`.
  * Changed `passenger_app_queue` to `passenger_app_request_queue`.
* Changed unit of passenger command timeout duration to seconds.
* Removed deprecated `code_revision` field from output parsed from passenger status command.
