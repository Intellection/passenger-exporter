package main

import (
	"bytes"
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var golden bool

func init() {
	flag.BoolVar(&golden, "golden", false, "Write test results to fixture files.")
}

func TestParsing(t *testing.T) {
	tests := map[string]func(t *testing.T) *Info{
		"newExporter": func(t *testing.T) *Info {
			e := newTestExporter()
			info, err := e.status()
			if err != nil {
				t.Fatalf("failed to get status: %v", err)
			}
			return info
		},
		"parseOutput": func(t *testing.T) *Info {
			f, err := os.Open("./test/passenger_xml_output.xml")
			if err != nil {
				t.Fatalf("open xml file failed: %v", err)
			}

			info, err := parseOutput(f)
			if err != nil {
				t.Fatalf("parse xml file failed: %v", err)
			}
			return info
		},
	}

	for name, test := range tests {
		info := test(t)

		if len(info.SuperGroups) == 0 {
			t.Fatalf("%v: no supergroups in output", name)
		}

		topLevelQueue := float64(info.TopLevelRequestQueueSize)
		if topLevelQueue == 0 {
			t.Fatalf("%v: no queuing requests parsed from output", name)
		}

		for _, sg := range info.SuperGroups {
			if want, got := "/srv/app/demo", sg.Group.Options.AppRoot; want != got {
				t.Fatalf("%s: incorrect app_root: wanted %s, got %s", name, want, got)
			}

			if len(sg.Group.Processes) == 0 {
				t.Fatalf("%v: no processes in output", name)
			}
			for _, proc := range sg.Group.Processes {
				if want, got := "38709", proc.ProcessGroupID; want != got {
					t.Fatalf("%s: incorrect process_group_id: wanted %s, got %s", name, want, got)
				}
			}
		}
	}
}

func TestScrape(t *testing.T) {
	flag.Parse()

	reg := prometheus.NewRegistry()
	reg.MustRegister(newTestExporter())
	server := httptest.NewServer(promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg}))
	defer server.Close()

	res, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("failed to GET test server: %v", err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	scrapeFixturePath := "./test/scrape_output.txt"
	if golden {
		idx := bytes.Index(body, []byte("# HELP passenger_app_group_count Number of app groups."))
		if err := os.WriteFile(scrapeFixturePath, body[idx:], 0666); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}
		t.Skipf("--golden passed: re-writing %s", scrapeFixturePath)
	}

	fixture, err := os.ReadFile(scrapeFixturePath)
	if err != nil {
		t.Fatalf("failed to read scrape fixture: %v", err)
	}

	if !bytes.Contains(body, fixture) {
		t.Fatalf("fixture data not contained within response body")
	}
}

func TestStatusTimeout(t *testing.T) {
	e := NewExporter("sleep 1", float64(time.Millisecond.Seconds()))
	_, err := e.status()
	if err == nil {
		t.Fatalf("failed to timeout")
	}

	if !strings.Contains(err.Error(), "status command timed out after 0.001000 seconds") {
		t.Fatalf("incorrect err: %v", err)
	}
}

type updateProcessSpec struct {
	name         string
	input        map[string]int
	processes    []Process
	maxProcesses int
	output       map[string]int
}

func newUpdateProcessSpec(
	name string,
	input map[string]int,
	processes []Process,
	maxProcesses int,
) updateProcessSpec {
	s := updateProcessSpec{
		name:         name,
		input:        input,
		processes:    processes,
		maxProcesses: maxProcesses,
	}
	s.output = updateProcesses(s.input, s.processes, s.maxProcesses)
	return s
}

func TestUpdateProcessIdentifiers(t *testing.T) {
	for _, spec := range []updateProcessSpec{
		newUpdateProcessSpec(
			"empty input",
			map[string]int{},
			[]Process{
				{PID: "abc"},
				{PID: "cdf"},
				{PID: "dfe"},
			},
			3,
		),
		newUpdateProcessSpec(
			"1:1",
			map[string]int{
				"abc": 0,
				"cdf": 1,
				"dfe": 2,
			},
			[]Process{
				{PID: "abc"},
				{PID: "cdf"},
				{PID: "dfe"},
			},
			6,
		),
		newUpdateProcessSpec(
			"increase processes",
			map[string]int{
				"abc": 0,
				"cdf": 1,
				"dfe": 2,
			},
			[]Process{
				{PID: "abc"},
				{PID: "cdf"},
				{PID: "dfe"},
				{PID: "ghi"},
				{PID: "jkl"},
				{PID: "lmn"},
			},
			9,
		),
		newUpdateProcessSpec(
			"reduce processes",
			map[string]int{
				"abc": 0,
				"cdf": 1,
				"dfe": 2,
				"ghi": 3,
				"jkl": 4,
				"lmn": 5,
			},
			[]Process{
				{PID: "abc"},
				{PID: "cdf"},
				{PID: "dfe"},
			},
			6,
		),
		newUpdateProcessSpec(
			"first process killed",
			map[string]int{
				"abc": 0,
				"cdf": 1,
				"dfe": 2,
			},
			[]Process{
				{PID: "cdf"},
				{PID: "dfe"},
			},
			3,
		),
		newUpdateProcessSpec(
			"second process killed",
			map[string]int{
				"abc": 0,
				"cdf": 1,
				"dfe": 2,
			},
			[]Process{
				{PID: "abc"},
				{PID: "dfe"},
			},
			3,
		),
	} {
		if len(spec.output) != len(spec.processes) {
			t.Fatalf("case %s: proceses improperly copied to output: len(output) (%d) does not match len(processes) (%d)", spec.name, len(spec.output), len(spec.processes))
		}

		for _, p := range spec.processes {
			if _, ok := spec.output[p.PID]; !ok {
				t.Fatalf("case %s: pid not copied into map", spec.name)
			}
		}

		newOutput := updateProcesses(spec.output, spec.processes, spec.maxProcesses)
		if !reflect.DeepEqual(newOutput, spec.output) {
			t.Fatalf("case %s: updateProcesses is not idempotent", spec.name)
		}
	}
}

func TestInsertingNewProcesses(t *testing.T) {
	spec := newUpdateProcessSpec(
		"inserting processes",
		map[string]int{
			"abc": 0,
			"cdf": 1,
			"dfe": 2,
			"efg": 3,
		},
		[]Process{
			{PID: "abc"},
			{PID: "dfe"},
			{PID: "newPID"},
			{PID: "newPID2"},
		},
		6,
	)

	if len(spec.output) != len(spec.processes) {
		t.Fatalf("case %s: proceses improperly copied to output: len(output) (%d) does not match len(processes) (%d)", spec.name, len(spec.output), len(spec.processes))
	}

	if want, got := 1, spec.output["newPID"]; want != got {
		t.Fatalf("updateProcesses did not correctly map the new PID: wanted %d, got %d", want, got)
	}
	if want, got := 3, spec.output["newPID2"]; want != got {
		t.Fatalf("updateProcesses did not correctly map the new PID: wanted %d, got %d", want, got)
	}
}

func TestProcessSurgeOverMaxProcesses(t *testing.T) {
	spec := newUpdateProcessSpec(
		"surging processes",
		map[string]int{
			"abc": 0,
			"def": 1,
			"hij": 2,
		},
		[]Process{
			{PID: "abc"},
			{PID: "def"},
			{PID: "hij"},
			{PID: "klm"},
		},
		3,
	)

	spec = newUpdateProcessSpec(
		"after process surge",
		spec.output,
		[]Process{
			{PID: "abc"},
			{PID: "def"},
			{PID: "klm"},
		},
		3,
	)

	if len(spec.output) != len(spec.processes) {
		t.Fatalf("case %s: proceses improperly copied to output: len(output) (%d) does not match len(processes) (%d)", spec.name, len(spec.output), len(spec.processes))
	}

	if want, got := 2, spec.output["klm"]; want != got {
		t.Fatalf("updateProcesses did not correctly map the new PID: wanted %d, got %d", want, got)
	}
}

func newTestExporter() *Exporter {
	return NewExporter("cat ./test/passenger_xml_output.xml", float64(time.Second.Seconds()))
}
