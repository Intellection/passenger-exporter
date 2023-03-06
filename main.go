package main

import (
	"bytes"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html/charset"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/version"
	"github.com/sirupsen/logrus"
)

// Info represents the info section of passenger's status.
type Info struct {
	PassengerVersion         string       `xml:"passenger_version"`
	AppGroupCount            int64        `xml:"group_count"`
	CurrentProcessCount      int64        `xml:"process_count"`
	MaxProcessCount          int64        `xml:"max"`
	CapacityUsed             int64        `xml:"capacity_used"`
	TopLevelRequestQueueSize int64        `xml:"get_wait_list_size"`
	SuperGroups              []SuperGroup `xml:"supergroups>supergroup"`
}

// SuperGroup represents the super group section of passenger's status.
type SuperGroup struct {
	Name             string `xml:"name"`
	State            string `xml:"state"`
	RequestQueueSize int64  `xml:"get_wait_list_size"`
	CapacityUsed     int64  `xml:"capacity_used"`
	Group            Group  `xml:"group"`
}

// Group represents the group section of passenger's status.
type Group struct {
	Name                  string    `xml:"name"`
	ComponentName         string    `xml:"component_name"`
	AppRoot               string    `xml:"app_root"`
	AppType               string    `xml:"app_type"`
	Environment           string    `xml:"environment"`
	UUID                  string    `xml:"uuid"`
	EnabledProcessCount   int64     `xml:"enabled_process_count"`
	DisablingProcessCount int64     `xml:"disabling_process_count"`
	DisabledProcessCount  int64     `xml:"disabled_process_count"`
	CapacityUsed          int64     `xml:"capacity_used"`
	RequestQueueSize      int64     `xml:"get_wait_list_size"`
	DisableWaitListSize   int64     `xml:"disable_wait_list_size"`
	ProcessesSpawning     int64     `xml:"processes_being_spawned"`
	LifeStatus            string    `xml:"life_status"`
	User                  string    `xml:"user"`
	UID                   int64     `xml:"uid"`
	Group                 string    `xml:"group"`
	GID                   int64     `xml:"gid"`
	Default               bool      `xml:"default,attr"`
	Options               Options   `xml:"options"`
	Processes             []Process `xml:"processes>process"`
}

// Process represents the process section of passenger's status.
type Process struct {
	PID                 string `xml:"pid"`
	StickySessionID     string `xml:"sticky_session_id"`
	GUPID               string `xml:"gupid"`
	Concurrency         int64  `xml:"concurrency"`
	Sessions            int64  `xml:"sessions"`
	Busyness            int64  `xml:"busyness"`
	RequestsProcessed   int64  `xml:"processed"`
	SpawnerCreationTime int64  `xml:"spawner_creation_time"`
	SpawnStartTime      int64  `xml:"spawn_start_time"`
	SpawnEndTime        int64  `xml:"spawn_end_time"`
	LastUsed            int64  `xml:"last_used"`
	LastUsedDesc        string `xml:"last_used_desc"`
	Uptime              string `xml:"uptime"`
	LifeStatus          string `xml:"life_status"`
	Enabled             string `xml:"enabled"`
	HasMetrics          bool   `xml:"has_metrics"`
	CPU                 int64  `xml:"cpu"`
	RSS                 int64  `xml:"rss"`
	PSS                 int64  `xml:"pss"`
	PrivateDirty        int64  `xml:"private_dirty"`
	Swap                int64  `xml:"swap"`
	RealMemory          int64  `xml:"real_memory"`
	VMSize              int64  `xml:"vmsize"`
	ProcessGroupID      string `xml:"process_group_id"`
	Command             string `xml:"command"`
}

// Options represents the options section of passenger's status.
type Options struct {
	AppRoot                       string `xml:"app_root"`
	AppGroupName                  string `xml:"app_group_name"`
	AppType                       string `xml:"app_type"`
	StartCommand                  string `xml:"start_command"`
	StartupFile                   string `xml:"startup_file"`
	ProcessTitle                  string `xml:"process_title"`
	LogLevel                      int64  `xml:"log_level"`
	StartTimeout                  int64  `xml:"start_timeout"`
	Environment                   string `xml:"environment"`
	BaseURI                       string `xml:"base_uri"`
	SpawnMethod                   string `xml:"spawn_method"`
	BindAddress                   string `xml:"bind_address"`
	DefaultUser                   string `xml:"default_user"`
	DefaultGroup                  string `xml:"default_group"`
	RestartDirectory              string `xml:"restart_dir"`
	IntegrationMode               string `xml:"integration_mode"`
	RubyBinPath                   string `xml:"ruby"`
	PythonBinPath                 string `xml:"python"`
	NodeJSBinPath                 string `xml:"nodejs"`
	Debugger                      bool   `xml:"debugger"`
	APIKey                        string `xml:"api_key"`
	MinProcesses                  int64  `xml:"min_processes"`
	MaxProcesses                  int64  `xml:"max_processes"`
	MaxPreloaderIdleTime          int64  `xml:"max_preloader_idle_time"`
	MaxOutOfBandWorkInstances     int64  `xml:"max_out_of_band_work_instances"`
	StickySessionCookieAttributes string `xml:"sticky_sessions_cookie_attributes"`
}

const (
	namespace             = "passenger"
	microsecondsPerSecond = 1000000
)

var (
	processIdentifiers = make(map[string]int)
	log                = logrus.New()
)

// Exporter collects metrics from passenger.
type Exporter struct {
	// binary file path for querying passenger state.
	cmd  string
	args []string

	// Passenger command timeout.
	timeout time.Duration

	// Passenger metrics.
	up                   *prometheus.Desc
	version              *prometheus.Desc
	topLevelRequestQueue *prometheus.Desc
	maxProcessCount      *prometheus.Desc
	currentProcessCount  *prometheus.Desc
	appGroupCount        *prometheus.Desc

	// App metrics.
	appRequestQueue  *prometheus.Desc
	appProcsSpawning *prometheus.Desc

	// Process metrics.
	requestsProcessed *prometheus.Desc
	procStartTime     *prometheus.Desc
	procMemory        *prometheus.Desc
}

// NewExporter returns an initialized exporter.
func NewExporter(cmd string, timeout float64) *Exporter {
	cmdComponents := strings.Split(cmd, " ")
	timeoutDuration := time.Duration(timeout * float64(time.Second))
	return &Exporter{
		cmd:     cmdComponents[0],
		args:    cmdComponents[1:],
		timeout: timeoutDuration,
		up: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "up"),
			"Current health of passenger.",
			nil,
			nil,
		),
		version: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "version"),
			"Version of passenger.",
			[]string{"version"},
			nil,
		),
		topLevelRequestQueue: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "top_level_request_queue"),
			"Number of requests in the top-level queue.",
			nil,
			nil,
		),
		maxProcessCount: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "max_processes"),
			"Configured maximum number of processes.",
			nil,
			nil,
		),
		currentProcessCount: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "current_processes"),
			"Current number of processes.",
			nil,
			nil,
		),
		appGroupCount: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "app_group_count"),
			"Number of app groups.",
			nil,
			nil,
		),
		appRequestQueue: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "app_request_queue"),
			"Number of requests in the app queue.",
			[]string{"name"},
			nil,
		),
		appProcsSpawning: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "app_procs_spawning"),
			"Number of processes spawning.",
			[]string{"name"},
			nil,
		),
		requestsProcessed: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "requests_processed_total"),
			"Number of requests served by a process.",
			[]string{"name", "id"},
			nil,
		),
		procStartTime: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "proc_start_time_seconds"),
			"Number of seconds since process started.",
			[]string{"name", "id"},
			nil,
		),
		procMemory: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "proc_memory"),
			"Memory consumed by a process",
			[]string{"name", "id"},
			nil,
		),
	}
}

// Describe describes all the metrics exported by the passenger exporter.
func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- e.up
	ch <- e.version
	ch <- e.topLevelRequestQueue
	ch <- e.maxProcessCount
	ch <- e.currentProcessCount
	ch <- e.appGroupCount
	ch <- e.appRequestQueue
	ch <- e.appProcsSpawning
	ch <- e.requestsProcessed
	ch <- e.procStartTime
	ch <- e.procMemory
}

// Collect fetches the statistics from passenger, and delivers them as
// Prometheus metrics.
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	info, err := e.status()
	if err != nil {
		ch <- prometheus.MustNewConstMetric(e.up, prometheus.GaugeValue, 0)
		log.Errorf("failed to collect status from passenger: %s", err)
		return
	}
	ch <- prometheus.MustNewConstMetric(e.up, prometheus.GaugeValue, 1)
	ch <- prometheus.MustNewConstMetric(e.version, prometheus.GaugeValue, 1, info.PassengerVersion)

	ch <- prometheus.MustNewConstMetric(e.topLevelRequestQueue, prometheus.GaugeValue, float64(info.TopLevelRequestQueueSize))
	ch <- prometheus.MustNewConstMetric(e.maxProcessCount, prometheus.GaugeValue, float64(info.MaxProcessCount))
	ch <- prometheus.MustNewConstMetric(e.currentProcessCount, prometheus.GaugeValue, float64(info.CurrentProcessCount))
	ch <- prometheus.MustNewConstMetric(e.appGroupCount, prometheus.GaugeValue, float64(info.AppGroupCount))

	for _, sg := range info.SuperGroups {
		ch <- prometheus.MustNewConstMetric(e.appRequestQueue, prometheus.GaugeValue, float64(sg.Group.RequestQueueSize), sg.Name)
		ch <- prometheus.MustNewConstMetric(e.appProcsSpawning, prometheus.GaugeValue, float64(sg.Group.ProcessesSpawning), sg.Name)

		// Update process identifiers map.
		processIdentifiers = updateProcesses(processIdentifiers, sg.Group.Processes, int(info.MaxProcessCount))
		for _, proc := range sg.Group.Processes {
			if bucketID, ok := processIdentifiers[proc.PID]; ok {
				ch <- prometheus.MustNewConstMetric(e.procMemory, prometheus.GaugeValue, float64(proc.RealMemory), sg.Name, strconv.Itoa(bucketID))
				ch <- prometheus.MustNewConstMetric(e.requestsProcessed, prometheus.CounterValue, float64(proc.RequestsProcessed), sg.Name, strconv.Itoa(bucketID))
				ch <- prometheus.MustNewConstMetric(e.procStartTime, prometheus.GaugeValue, float64(proc.SpawnStartTime/microsecondsPerSecond),
					sg.Name, strconv.Itoa(bucketID),
				)
			}
		}
	}
}

func (e *Exporter) status() (*Info, error) {
	var (
		out bytes.Buffer
		cmd = exec.Command(e.cmd, e.args...)
	)
	cmd.Stdout = &out

	err := cmd.Start()
	if err != nil {
		return nil, err
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-time.After(e.timeout):
		if err := cmd.Process.Kill(); err != nil {
			log.Errorf("failed to kill process: %s", err)
		}
		err = fmt.Errorf("status command timed out after %f seconds", e.timeout.Seconds())
		return nil, err
	case err := <-done:
		if err != nil {
			return nil, err
		}
	}

	return parseOutput(&out)
}

func parseOutput(r io.Reader) (*Info, error) {
	var info Info
	decoder := xml.NewDecoder(r)
	decoder.CharsetReader = charset.NewReaderLabel
	err := decoder.Decode(&info)
	if err != nil {
		return nil, err
	}
	return &info, nil
}

// updateProcesses updates the global map from process id:exporter id. Process
// TTLs cause new processes to be created on a user-defined cycle. When a new
// process replaces an old process, the new process's statistics will be
// bucketed with those of the process it replaced.
// Processes are restarted at an offset, user-defined interval. The
// restarted process is appended to the end of the status output.  For
// maintaining consistent process identifiers between process starts,
// pids are mapped to an identifier based on process count. When a new
// process/pid appears, it is mapped to either the first empty place
// within the global map storing process identifiers, or mapped to
// pid:id pair in the map.
func updateProcesses(old map[string]int, processes []Process, maxProcesses int) map[string]int {
	var (
		updated = make(map[string]int, maxProcesses)
		found   = make([]string, maxProcesses)
		missing []string
	)

	if len(processes) > maxProcesses {
		processes = processes[:maxProcesses]
	}

	for _, p := range processes {
		if id, ok := old[p.PID]; ok {
			found[id] = p.PID
			// id also serves as an index.
			// By putting the pid at a certain index, we can loop
			// through the array to find the values that are the 0
			// value (empty string).
			// If index i has the empty value, then it was never
			// updated, so we slot the first of the missingPIDs
			// into that position. Passenger-status orders output
			// by pid, increasing. We can then assume that
			// unclaimed pid positions map in order to the missing
			// pids.
		} else {
			missing = append(missing, p.PID)
		}
	}

	j := 0
	for i, pid := range found {
		if pid == "" {
			if j >= len(missing) {
				continue
			}
			pid = missing[j]
			j++
		}
		updated[pid] = i
	}

	// If the number of elements in missing iterated through is less
	// than len(missing), there are new elements to be added to the map.
	// Unused pids from the last collection are not copied from old to
	// updated, thereby cleaning the return value of unused PIDs.
	if j < len(missing) {
		count := len(found)
		for i, pid := range missing[j:] {
			updated[pid] = count + i
		}
	}

	return updated
}

func main() {
	var (
		cmd           = flag.String("passenger.command", "passenger-status --show=xml", "Passenger command for querying passenger status.")
		timeout       = flag.Float64("passenger.command.timeout-seconds", 5, "Timeout in seconds for passenger.command.")
		pidFile       = flag.String("passenger.pid-file", "", "Optional path to a file containing the passenger PID for additional metrics.")
		metricsPath   = flag.String("web.telemetry-path", "/metrics", "Path under which to expose metrics.")
		listenAddress = flag.String("web.listen-address", ":9149", "Address to listen on for web interface and telemetry.")
	)
	flag.Parse()

	// Create a new registry.
	reg := prometheus.NewRegistry()

	// Add Go module build info.
	reg.MustRegister(
		collectors.NewBuildInfoCollector(),
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{
			PidFn: func() (int, error) {
				content, err := os.ReadFile(*pidFile)
				if err != nil {
					return 0, fmt.Errorf("error reading pidfile %q: %s", *pidFile, err)
				}
				value, err := strconv.Atoi(strings.TrimSpace(string(content)))
				if err != nil {
					return 0, fmt.Errorf("error parsing pidfile %q: %s", *pidFile, err)
				}
				return value, nil
			},
			Namespace:    namespace,
			ReportErrors: false,
		}),
		NewExporter(*cmd, *timeout),
	)

	// Expose /metrics HTTP endpoint using the created custom registry.
	http.Handle(*metricsPath, promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg}))

	log.Infoln("Starting passenger-exporter", version.Info())
	log.Infoln("Build context", version.BuildContext())
	log.Infoln("Listening on", *listenAddress)

	log.Fatal(http.ListenAndServe(*listenAddress, nil))
}
