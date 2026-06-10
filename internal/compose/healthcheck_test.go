package compose

import (
	"testing"

	"github.com/zeroecco/holos/internal/config"
	"gopkg.in/yaml.v3"
)

func testCMDTrueHealthcheck() []string {
	return []string{"CMD", "true"}
}

func testCurlHealthcheck() []string {
	return []string{"curl", "-f", "http://localhost:8080/health"}
}

type testHealthcheckTimingWant struct {
	intervalSec    int
	retries        int
	startPeriodSec int
	timeoutSec     int
}

func assertHealthcheckTiming(t *testing.T, got *config.HealthcheckConfig, want testHealthcheckTimingWant) {
	t.Helper()

	if got.IntervalSec != want.intervalSec ||
		got.Retries != want.retries ||
		got.StartPeriodSec != want.startPeriodSec ||
		got.TimeoutSec != want.timeoutSec {
		t.Fatalf("healthcheck timing = %+v, want %+v", got, want)
	}
}

type testComposeHealthcheckFieldsWant struct {
	interval      string
	retries       int
	startPeriod   string
	startInterval string
	timeout       string
	disable       bool
}

func assertComposeHealthcheckFields(t *testing.T, got Healthcheck, want testComposeHealthcheckFieldsWant) {
	t.Helper()

	if got.Interval != want.interval ||
		got.Retries != want.retries ||
		got.StartPeriod != want.startPeriod ||
		got.StartInterval != want.startInterval ||
		got.Timeout != want.timeout ||
		got.Disable != want.disable {
		t.Fatalf("healthcheck fields = %+v, want %+v", got, want)
	}
}

// TestResolveHealthcheck_ListForm confirms the YAML `test:` list form
// flows through to the resolved config unchanged.
func TestResolveHealthcheck_ListForm(t *testing.T) {
	t.Parallel()

	yamlDoc := `
name: hc
services:
  api:
    image: ./base.qcow2
    healthcheck:
      test: ["curl", "-f", "http://localhost:8080/health"]
      interval: 5s
      retries: 4
      start_period: 10s
      timeout: 2s
`
	dir := t.TempDir()
	writeTestImage(t, dir)
	proj := resolveTestCompose(t, dir, yamlDoc)
	hc := proj.Services["api"].Healthcheck
	if hc == nil {
		t.Fatal("missing healthcheck")
	}
	assertStringSliceEqual(t, "test", hc.Test, testCurlHealthcheck())
	assertHealthcheckTiming(t, hc, testHealthcheckTimingWant{
		intervalSec:    5,
		retries:        4,
		startPeriodSec: 10,
		timeoutSec:     2,
	})
}

// TestResolveHealthcheck_StringForm verifies the shorthand string form
// is wrapped in `sh -c` so shell features (pipes, env expansion) work.
func TestResolveHealthcheck_StringForm(t *testing.T) {
	t.Parallel()

	yamlDoc := `
name: hc2
services:
  api:
    image: ./base.qcow2
    healthcheck:
      test: "pg_isready | grep -q accepting"
`
	dir := t.TempDir()
	writeTestImage(t, dir)
	proj := resolveTestCompose(t, dir, yamlDoc)
	hc := proj.Services["api"].Healthcheck
	if hc == nil {
		t.Fatal("missing healthcheck")
	}
	assertStringSliceEqual(t, "test", hc.Test, []string{"sh", "-c", "pg_isready | grep -q accepting"})
	// Defaults apply when the compose omits the fields.
	if hc.IntervalSec != config.DefaultHealthIntervalSec {
		t.Fatalf("interval = %d, want default %d", hc.IntervalSec, config.DefaultHealthIntervalSec)
	}
	if hc.Retries != config.DefaultHealthRetries {
		t.Fatalf("retries = %d, want default %d", hc.Retries, config.DefaultHealthRetries)
	}
	if hc.TimeoutSec != config.DefaultHealthTimeoutSec {
		t.Fatalf("timeout = %d, want default %d", hc.TimeoutSec, config.DefaultHealthTimeoutSec)
	}
}

func TestHealthcheckUnmarshalTestForms(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		want []string
	}{
		{name: "string", body: `test: "pg_isready | grep -q accepting"`, want: []string{healthcheckShellCommand, healthcheckShellFlag, "pg_isready | grep -q accepting"}},
		{name: "list", body: `test: ["curl", "-f", "http://localhost:8080/health"]`, want: testCurlHealthcheck()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var hc Healthcheck
			if err := yaml.Unmarshal([]byte(tt.body), &hc); err != nil {
				t.Fatalf("unmarshal healthcheck: %v", err)
			}
			assertStringSliceEqual(t, "test", hc.Test, tt.want)
		})
	}
}

func TestHealthcheckConfig(t *testing.T) {
	t.Parallel()

	source := &Healthcheck{Test: testCMDTrueHealthcheck()}
	got := healthcheckConfig(source, 5, 4, 10, 2)
	assertStringSliceEqual(t, "Test", got.Test, testCMDTrueHealthcheck())
	assertHealthcheckTiming(t, got, testHealthcheckTimingWant{
		intervalSec:    5,
		retries:        4,
		startPeriodSec: 10,
		timeoutSec:     2,
	})

	source.Test[0] = "MUTATED"
	if got.Test[0] != "CMD" {
		t.Fatalf("Test shares backing array with source: %v", got.Test)
	}
}

func TestHealthcheckUnmarshalAcceptsKnownFields(t *testing.T) {
	t.Parallel()

	var hc Healthcheck
	err := yaml.Unmarshal([]byte(`
test: ["CMD", "true"]
interval: 2s
retries: 4
start_period: 10s
start_interval: 1s
timeout: 5s
disable: true
`), &hc)
	if err != nil {
		t.Fatalf("unmarshal healthcheck: %v", err)
	}
	assertStringSliceEqual(t, "Test", hc.Test, testCMDTrueHealthcheck())
	assertComposeHealthcheckFields(t, hc, testComposeHealthcheckFieldsWant{
		interval:      "2s",
		retries:       4,
		startPeriod:   "10s",
		startInterval: "1s",
		timeout:       "5s",
		disable:       true,
	})
}

func TestHealthcheckUnmarshalRejectsUnknownField(t *testing.T) {
	t.Parallel()

	var hc Healthcheck
	err := yaml.Unmarshal([]byte("test: ['true']\nretriez: 3\n"), &hc)
	assertErrorContains(t, err, "field retriez not found in type compose.Healthcheck")
}

func TestResolveHealthcheckDisabledForms(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		yamlDoc string
	}{
		{
			name: "disable true",
			yamlDoc: `
name: hcoff
services:
  api:
    image: ./base.qcow2
    healthcheck:
      disable: true
`,
		},
		{
			name: "none test",
			yamlDoc: `
name: hcoff
services:
  api:
    image: ./base.qcow2
    healthcheck:
      test: ["NONE"]
`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeTestImage(t, dir)
			proj := resolveTestCompose(t, dir, tc.yamlDoc)
			if hc := proj.Services["api"].Healthcheck; hc != nil {
				t.Fatalf("healthcheck = %+v, want nil", hc)
			}
		})
	}
}

func TestResolveHealthcheckNoneWithExtraArgIsCommand(t *testing.T) {
	t.Parallel()

	yamlDoc := `
name: hcnonecommand
services:
  api:
    image: ./base.qcow2
    healthcheck:
      test: ["NONE", "true"]
`
	dir := t.TempDir()
	writeTestImage(t, dir)
	proj := resolveTestCompose(t, dir, yamlDoc)
	hc := proj.Services["api"].Healthcheck
	if hc == nil {
		t.Fatal("missing healthcheck")
	}
	assertStringSliceEqual(t, "test", hc.Test, []string{"NONE", "true"})
}

func TestHealthcheckRetries(t *testing.T) {
	t.Parallel()

	if got := healthcheckRetries(0); got != config.DefaultHealthRetries {
		t.Fatalf("healthcheckRetries(0) = %d, want default %d", got, config.DefaultHealthRetries)
	}
	if got := healthcheckRetries(4); got != 4 {
		t.Fatalf("healthcheckRetries(4) = %d, want 4", got)
	}
}

func TestResolveAcceptsHealthcheckStartInterval(t *testing.T) {
	t.Parallel()

	yamlDoc := `
name: hcinterval
services:
  api:
    image: ./base.qcow2
    healthcheck:
      test: ["CMD", "true"]
      interval: 2s
      start_interval: 1s
`
	dir := t.TempDir()
	writeTestImage(t, dir)
	resolveTestCompose(t, dir, yamlDoc)
}

// TestHealthcheckRejectsUnknownFields pins that typos inside the
// healthcheck block surface as an error rather than being silently
// dropped. The outer Load() uses KnownFields(true), but the custom
// Healthcheck.UnmarshalYAML has to re-enforce it because
// yaml.Node.Decode has no strict-fields toggle.
func TestHealthcheckRejectsUnknownFields(t *testing.T) {
	t.Parallel()

	yamlDoc := `
name: hctypo
services:
 api:
    image: ./img.qcow2
    healthcheck:
      test: ["true"]
      retriez: 3
`
	dir := t.TempDir()
	path := writeTestFile(t, dir, "holos.yaml", yamlDoc)
	_, err := Load(path)
	assertErrorContains(t, err, "retriez")
}
