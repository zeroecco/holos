package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/zeroecco/holos/internal/compose"
	"github.com/zeroecco/holos/internal/config"
)

const testProbeEnv = "HOLOS_TEST_PROBE"

func writeTestFile(t *testing.T, dir, name, content string, perm os.FileMode) string {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), perm); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeTestOVMFFirmware(t *testing.T, dir string) (string, string) {
	t.Helper()

	code := writeTestFile(t, dir, "OVMF_CODE.fd", "code", 0o600)
	vars := writeTestFile(t, dir, "OVMF_VARS.fd", "vars", 0o600)
	return code, vars
}

func TestParseMemoryMB(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in      string
		want    int
		wantErr bool
	}{
		{"512", 512, false},
		{"512M", 512, false},
		{"512m", 512, false},
		{"512MB", 512, false},
		{"2G", 2048, false},
		{"2GB", 2048, false},
		{"2g", 2048, false},
		{"1T", 1024 * 1024, false},
		{"4096K", 4, false},
		{"  1G  ", 1024, false},
		{"", 0, true},
		{"-1", 0, true},
		{"abc", 0, true},
		{"512K", 0, true}, // 512 KiB rounds to 0 MB; rejected
		{"0", 0, true},
	}
	for _, c := range cases {
		got, err := parseMemoryMB(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseMemoryMB(%q) = %d, want error", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseMemoryMB(%q) error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("parseMemoryMB(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestNormalizeMemoryMBInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		raw            string
		wantValue      string
		wantMultiplier float64
		wantErr        bool
	}{
		{name: "blank", raw: " \t ", wantValue: "", wantMultiplier: 1},
		{name: "plain megabytes", raw: "512", wantValue: "512", wantMultiplier: 1},
		{name: "kilobytes", raw: "4096K", wantValue: "4096", wantMultiplier: 1.0 / 1024.0},
		{name: "megabytes with b", raw: "512mb", wantValue: "512", wantMultiplier: 1},
		{name: "gigabytes", raw: "2G", wantValue: "2", wantMultiplier: 1024},
		{name: "terabytes", raw: "1TB", wantValue: "1", wantMultiplier: 1024 * 1024},
		{name: "bare bytes suffix", raw: "B", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			value, multiplier, err := normalizeMemoryMBInput(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Fatalf("normalizeMemoryMBInput(%q) err = %v, wantErr %v", tt.raw, err, tt.wantErr)
			}
			if value != tt.wantValue || multiplier != tt.wantMultiplier {
				t.Fatalf("normalizeMemoryMBInput(%q) = (%q, %v), want (%q, %v)",
					tt.raw, value, multiplier, tt.wantValue, tt.wantMultiplier)
			}
		})
	}
}

func TestCommandRegistryIsConsistent(t *testing.T) {
	t.Parallel()

	if len(commandOrder) != len(commands) {
		t.Fatalf("commandOrder has %d entries, commands has %d", len(commandOrder), len(commands))
	}
	seen := make(map[string]bool, len(commandOrder))
	for _, name := range commandOrder {
		if seen[name] {
			t.Fatalf("commandOrder contains duplicate command %q", name)
		}
		seen[name] = true

		command, ok := commands[name]
		if !ok {
			t.Fatalf("commandOrder references missing command %q", name)
		}
		if command.run == nil {
			t.Fatalf("command %q has no run function", name)
		}
		if command.usage == "" {
			t.Fatalf("command %q has empty usage", name)
		}
		if command.description == "" {
			t.Fatalf("command %q has empty description", name)
		}
	}
	for name := range commands {
		if !seen[name] {
			t.Fatalf("commands contains %q, but commandOrder does not", name)
		}
	}
	for alias, target := range commandAliases {
		aliased, ok := resolveCommand(alias)
		if !ok {
			t.Fatalf("alias %q did not resolve", alias)
		}
		targetCommand, ok := commands[target]
		if !ok {
			t.Fatalf("alias %q targets missing command %q", alias, target)
		}
		if aliased.usage != targetCommand.usage {
			t.Fatalf("alias %q resolved to usage %q, want %q", alias, aliased.usage, targetCommand.usage)
		}
	}
}

func TestFormatUsageLine(t *testing.T) {
	t.Parallel()

	command := command{
		usage:       "holos test",
		description: "run test command",
	}
	if got, want := formatUsageLine(command), "  holos test                                               run test command"; got != want {
		t.Fatalf("formatUsageLine = %q, want %q", got, want)
	}
}

func TestBuildVersionAppliesVCSDefaults(t *testing.T) {
	t.Parallel()

	build := newBuildVersion("dev", buildCommitPlaceholder, buildDatePlaceholder)
	build.applySetting(buildVCSRevisionKey, "abc123")
	build.applySetting(buildVCSTimeKey, "2026-06-10T12:00:00Z")

	if build.commit != "abc123" {
		t.Fatalf("commit = %q, want vcs revision", build.commit)
	}
	if build.date != "2026-06-10T12:00:00Z" {
		t.Fatalf("date = %q, want vcs time", build.date)
	}
}

func TestBuildVersionPreservesLinkedMetadata(t *testing.T) {
	t.Parallel()

	build := newBuildVersion("1.2.3", "linked-sha", "linked-date")
	build.applySetting(buildVCSRevisionKey, "ignored-sha")
	build.applySetting(buildVCSTimeKey, "ignored-date")

	if build.commit != "linked-sha" || build.date != "linked-date" {
		t.Fatalf("build metadata = (%q, %q), want linked values", build.commit, build.date)
	}
}

func TestBuildVersionMarksDirtyCommit(t *testing.T) {
	t.Parallel()

	build := newBuildVersion("dev", "abc123", buildDatePlaceholder)
	build.applySetting(buildVCSModifiedKey, buildModifiedTrue)
	build.applySetting(buildVCSModifiedKey, buildModifiedTrue)

	if build.commit != "abc123"+buildDirtySuffix {
		t.Fatalf("commit = %q, want single dirty suffix", build.commit)
	}
}

func TestBuildMetadataCanAdopt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		current     string
		placeholder string
		candidate   string
		want        bool
	}{
		{name: "placeholder with candidate", current: buildCommitPlaceholder, placeholder: buildCommitPlaceholder, candidate: "abc123", want: true},
		{name: "placeholder with empty candidate", current: buildCommitPlaceholder, placeholder: buildCommitPlaceholder},
		{name: "linked value", current: "linked-sha", placeholder: buildCommitPlaceholder, candidate: "abc123"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := buildMetadataCanAdopt(tt.current, tt.placeholder, tt.candidate); got != tt.want {
				t.Fatalf("buildMetadataCanAdopt(%q, %q, %q) = %v, want %v",
					tt.current, tt.placeholder, tt.candidate, got, tt.want)
			}
		})
	}
}

func TestBuildVersionShouldAppendDirty(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		commit   string
		modified string
		want     bool
	}{
		{name: "modified commit", commit: "abc123", modified: buildModifiedTrue, want: true},
		{name: "not modified", commit: "abc123", modified: "false"},
		{name: "unknown commit", commit: buildCommitPlaceholder, modified: buildModifiedTrue},
		{name: "already dirty", commit: "abc123" + buildDirtySuffix, modified: buildModifiedTrue},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := buildVersionShouldAppendDirty(tt.commit, tt.modified); got != tt.want {
				t.Fatalf("buildVersionShouldAppendDirty(%q, %q) = %v, want %v",
					tt.commit, tt.modified, got, tt.want)
			}
		})
	}
}

func TestWriteVersion(t *testing.T) {
	t.Parallel()

	build := newBuildVersion("1.2.3", "abc123", "2026-06-10T12:00:00Z")

	t.Run("short", func(t *testing.T) {
		t.Parallel()

		var out bytes.Buffer
		writeVersion(&out, build, true, "go1.25.0", "linux", "amd64")
		if got, want := out.String(), "1.2.3\n"; got != want {
			t.Fatalf("short version output = %q, want %q", got, want)
		}
	})

	t.Run("full", func(t *testing.T) {
		t.Parallel()

		var out bytes.Buffer
		writeVersion(&out, build, false, "go1.25.0", "linux", "amd64")
		want := "holos 1.2.3\n" +
			"  commit: abc123\n" +
			"  built:  2026-06-10T12:00:00Z\n" +
			"  go:     go1.25.0\n" +
			"  os/arch: linux/amd64\n"
		if got := out.String(); got != want {
			t.Fatalf("full version output = %q, want %q", got, want)
		}
	})
}

func TestWriteValidateReport(t *testing.T) {
	t.Parallel()

	project := &compose.Project{
		Name:         "demo",
		SpecHash:     "abc123",
		ServiceOrder: []string{"web", "db"},
		Services: map[string]config.Manifest{
			"web": {
				Image:    "/var/lib/holos/images/web.qcow2",
				Replicas: 2,
				VM:       config.VMConfig{VCPU: 4, MemoryMB: 2048},
			},
			"db": {
				Image:    "postgres.raw",
				Replicas: 1,
				VM:       config.VMConfig{VCPU: 2, MemoryMB: 1024},
			},
		},
		Network: compose.NetworkPlan{
			Subnet:         "10.10.0.0/24",
			MulticastGroup: "239.1.2.3",
			MulticastPort:  12345,
			Hosts: map[string]string{
				"web": "10.10.0.10",
			},
		},
	}

	var out bytes.Buffer
	if err := writeValidateReport(&out, project); err != nil {
		t.Fatalf("writeValidateReport: %v", err)
	}
	want := "project: demo\n" +
		"spec_hash: abc123\n" +
		"services: 2\n" +
		"order: [web db]\n\n" +
		"SERVICE  IMAGE         REPLICAS  VCPU  MEMORY\n" +
		"web      web.qcow2     2         4     2048MB\n" +
		"db       postgres.raw  1         2     1024MB\n\n" +
		"network: 10.10.0.0/24 (mcast 239.1.2.3:12345)\n" +
		"hosts:\n" +
		"  web -> 10.10.0.10\n"
	if got := out.String(); got != want {
		t.Fatalf("validate report = %q, want %q", got, want)
	}
}

func TestDoctorCommandRequiresExecutableProbe(t *testing.T) {
	dir := t.TempDir()
	probe := writeTestFile(t, dir, "probe", "#!/bin/sh\necho probe-ok\n", 0o755)

	t.Setenv(testProbeEnv, probe)
	check := checkCommand("probe", testProbeEnv, []string{doctorVersionFlag}, "test probe")
	if check.Status != doctorStatusOK {
		t.Fatalf("checkCommand status = %s (%s), want %s", check.Status, check.Message, doctorStatusOK)
	}
	assertContains(t, check.Message, "probe-ok")

	notExecutable := writeTestFile(t, dir, "not-executable", "#!/bin/sh\n", 0o644)
	t.Setenv(testProbeEnv, notExecutable)
	check = checkCommand("probe", testProbeEnv, []string{doctorVersionFlag}, "test probe")
	if check.Status != doctorStatusFail {
		t.Fatalf("non-executable status = %s, want %s", check.Status, doctorStatusFail)
	}
}

func TestCheckOVMFRequiresCodeAndVarsPair(t *testing.T) {
	dir := t.TempDir()
	code, vars := writeTestOVMFFirmware(t, dir)

	t.Setenv(doctorOVMFCodeEnv, code)
	t.Setenv(doctorOVMFVarsEnv, "")
	if check := checkOVMF(); check.Status != doctorStatusFail {
		t.Fatalf("single env OVMF status = %s, want %s", check.Status, doctorStatusFail)
	}

	t.Setenv(doctorOVMFCodeEnv, code)
	t.Setenv(doctorOVMFVarsEnv, vars)
	if check := checkOVMF(); check.Status != doctorStatusOK {
		t.Fatalf("paired env OVMF status = %s (%s), want %s", check.Status, check.Message, doctorStatusOK)
	}
}

func TestStringListAppends(t *testing.T) {
	t.Parallel()

	var list stringList
	if got := list.String(); got != "" {
		t.Fatalf("empty String() = %q, want empty", got)
	}
	for _, v := range []string{"8080:80", "9090:90", "5432:5432"} {
		if err := list.Set(v); err != nil {
			t.Fatalf("Set(%q): %v", v, err)
		}
	}
	want := stringList{"8080:80", "9090:90", "5432:5432"}
	assertStringSliceEqual(t, "stringList", []string(list), []string(want))
	if got := list.String(); got != "8080:80,9090:90,5432:5432" {
		t.Errorf("String() = %q", got)
	}
}
