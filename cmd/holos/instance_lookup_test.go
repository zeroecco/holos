package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zeroecco/holos/internal/compose"
	"github.com/zeroecco/holos/internal/config"
	"github.com/zeroecco/holos/internal/runtime"
)

const (
	testProjectsDirSuffix  = "projects"
	testProjectRecordExt   = ".json"
	testLookupProjectName  = "demo"
	testLookupVMService    = "vm"
	testLookupDBService    = "db"
	testLookupVM0          = "vm-0"
	testLookupVM1          = "vm-1"
	testLookupDB0          = "db-0"
	testLookupVM0LogPath   = "/tmp/vm-0.log"
	testLookupVM1LogPath   = "/tmp/vm-1.log"
	testLookupDB0LogPath   = "/tmp/db-0.log"
	testLookupVM0SSHPort   = 2222
	testLookupVM1SSHPort   = 2223
	testLookupVM0Serial    = "/tmp/s0"
	testLookupVM1Serial    = "/tmp/s1"
	testLookupBadInstance  = "vm-99"
	testLookupRunProject   = "ubuntu-noble-34cf77"
	testLookupAdHocProject = "ad-hoc"
	testLookupLoginUser    = "debian"
	testLookupEchoCmd      = "echo"
	testLookupEchoArg      = "hello"
	testLookupCmdName      = "ls"
	testLookupCmdArg       = "-la"
)

func assertErrorMentionsAll(t *testing.T, err error, want ...string) {
	t.Helper()

	if err == nil {
		t.Fatalf("expected error mentioning %v, got nil", want)
	}
	for _, substring := range want {
		if !strings.Contains(err.Error(), substring) {
			t.Fatalf("error = %v, want substring %q", err, substring)
		}
	}
}

func testLogPaths(instances []runtime.InstanceRecord) []string {
	paths := make([]string, 0, len(instances))
	for _, inst := range instances {
		paths = append(paths, inst.LogPath)
	}
	return paths
}

func testInstanceNames(instances []runtime.InstanceRecord) []string {
	names := make([]string, 0, len(instances))
	for _, inst := range instances {
		names = append(names, inst.Name)
	}
	return names
}

func testLogLookupProjectRecord() *runtime.ProjectRecord {
	return testProjectRecord(testLookupProjectName,
		testServiceRecord(testLookupVMService,
			testInstanceRecord(testLookupVM0, testLookupVM0LogPath),
			testInstanceRecord(testLookupVM1, testLookupVM1LogPath),
		),
		testServiceRecord(testLookupDBService,
			testInstanceRecord(testLookupDB0, testLookupDB0LogPath),
		),
	)
}

// TestResolveLogTargets pins the dual-mode lookup that fixed the
// confusing "service \"vm-0\" not found" error: the same identifier
// `ps` shows in its INSTANCE column should be acceptable to `logs`,
// not just the SERVICE name.
func TestResolveLogTargets(t *testing.T) {
	t.Parallel()

	record := testLogLookupProjectRecord()

	cases := []struct {
		name     string
		target   string
		wantLogs []string
	}{
		{"service-name-fans-out", testLookupVMService, []string{testLookupVM0LogPath, testLookupVM1LogPath}},
		{"single-instance", testLookupVM0, []string{testLookupVM0LogPath}},
		{"different-service-instance", testLookupDB0, []string{testLookupDB0LogPath}},
		{"unknown", "nope", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := resolveLogTargets(record, c.target)
			assertStringSliceEqual(t, c.target+" log targets", testLogPaths(got), c.wantLogs)
		})
	}
}

func TestLogTargets(t *testing.T) {
	t.Parallel()

	record := testLogLookupProjectRecord()

	tests := []struct {
		name     string
		filter   string
		wantLogs []string
	}{
		{name: "all", wantLogs: []string{testLookupVM0LogPath, testLookupVM1LogPath, testLookupDB0LogPath}},
		{name: "filtered", filter: testLookupDBService, wantLogs: []string{testLookupDB0LogPath}},
		{name: "missing", filter: "missing", wantLogs: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := logTargets(record, tt.filter)
			assertStringSliceEqual(t, "logTargets()", testLogPaths(got), tt.wantLogs)
		})
	}
}

func TestInstancesForServiceName(t *testing.T) {
	t.Parallel()

	record := testProjectRecord(testLookupProjectName,
		testServiceRecord(testLookupVMService,
			testInstanceRecord(testLookupVM0, testLookupVM0LogPath),
			testInstanceRecord(testLookupVM1, testLookupVM1LogPath),
		),
	)

	got, ok := instancesForServiceName(record, testLookupVMService)
	if !ok {
		t.Fatal("instancesForServiceName ok = false, want true")
	}
	assertStringSliceEqual(t, "instancesForServiceName names", testInstanceNames(got), []string{testLookupVM0, testLookupVM1})
	if got, ok := instancesForServiceName(record, "missing"); ok || got != nil {
		t.Fatalf("instancesForServiceName missing = (%+v, %v), want nil false", got, ok)
	}
}

func TestInstanceForName(t *testing.T) {
	t.Parallel()

	record := testLogLookupProjectRecord()

	got, ok := instanceForName(record, testLookupDB0)
	if !ok {
		t.Fatal("instanceForName ok = false, want true")
	}
	if got.Name != testLookupDB0 || got.LogPath != testLookupDB0LogPath {
		t.Fatalf("instanceForName = %+v, want %s", got, testLookupDB0)
	}
	if got, ok := instanceForName(record, "missing-0"); ok || got.Name != "" {
		t.Fatalf("instanceForName missing = (%+v, %v), want zero false", got, ok)
	}
}

// TestResolveLogTargetsServiceWinsOnCollision documents the
// tie-break: when a service and an instance share a name (someone
// names a service "vm-0"), the service interpretation wins and we
// fan out to all of its replicas. Asserted explicitly so a future
// refactor can't quietly flip the order without a failing test.
func TestResolveLogTargetsServiceWinsOnCollision(t *testing.T) {
	t.Parallel()

	record := testProjectRecord("",
		testServiceRecord("vm-0", // weird but legal
			testInstanceRecord("vm-0-0", "/tmp/vm-0-0.log"),
		),
		testServiceRecord("other",
			testInstanceRecord("vm-0", "/tmp/other.log"),
		),
	)
	got := resolveLogTargets(record, "vm-0")
	assertStringSliceEqual(t, "collision log targets", testLogPaths(got), []string{"/tmp/vm-0-0.log"})
}

// TestLookupProjectRecord_HitAndMiss exercises the on-disk lookup
// that lets `holos logs|console|exec <project>` work without an
// `-f` detour. We seed a state directory with a single project
// record, then confirm the helper recovers it by name and returns
// (nil, false) for an unknown name.
//
// This is the primary regression guard for the bug report:
//
//	"holos logs ubuntu-noble-34cf77" failing with
//	"open .../projects/my-stack.json: no such file or directory"
//
// because logs was loading a stale cwd holos.yaml instead of
// resolving the positional as a project name.
func TestLookupProjectRecord_HitAndMiss(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	manager := runtime.NewManager(stateDir)

	if err := writeFakeProjectRecord(stateDir, "ubuntu-noble-34cf77"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if rec, ok := lookupProjectRecord(manager, "ubuntu-noble-34cf77"); !ok || rec.Name != "ubuntu-noble-34cf77" {
		t.Errorf("lookupProjectRecord(known) = (%v, %v), want hit", rec, ok)
	}
	if rec, ok := lookupProjectRecord(manager, "no-such-project"); ok || rec != nil {
		t.Errorf("lookupProjectRecord(unknown) = (%v, %v), want miss", rec, ok)
	}
	if rec, ok := lookupProjectRecord(manager, ""); ok || rec != nil {
		t.Errorf("lookupProjectRecord(\"\") = (%v, %v), want miss", rec, ok)
	}
}

// TestSoleInstanceAndFindInstanceInRecord covers the two helpers
// that drive `holos exec/console <project>` with and without an
// explicit instance argument.
func TestSoleInstanceAndFindInstanceInRecord(t *testing.T) {
	t.Parallel()

	single := testProjectRecord("demo",
		testServiceRecord("vm", testInstanceRecord("vm-0", "")),
	)
	if inst, svc, ok := soleInstance(single); !ok || inst.Name != "vm-0" || svc.Name != "vm" {
		t.Errorf("soleInstance(single) = (%v, %v, %v), want vm-0 under vm", inst, svc, ok)
	}

	multi := testProjectRecord("",
		testServiceRecord("web",
			testInstanceRecord("web-0", ""),
			testInstanceRecord("web-1", ""),
		),
	)
	if _, _, ok := soleInstance(multi); ok {
		t.Errorf("soleInstance(multi) returned true; want false (cannot disambiguate)")
	}

	if inst, svc, ok := findInstanceInRecord(multi, "web-1"); !ok || inst.Name != "web-1" || svc.Name != "web" {
		t.Errorf("findInstanceInRecord(web-1) = (%v, %v, %v)", inst, svc, ok)
	}
	if _, _, ok := findInstanceInRecord(multi, "nope-0"); ok {
		t.Errorf("findInstanceInRecord(unknown) returned true; want false")
	}
}

func TestInstanceList(t *testing.T) {
	t.Parallel()

	var empty runtime.ProjectRecord
	if got := instanceList(&empty); got != "" {
		t.Fatalf("instanceList(empty) = %q, want empty", got)
	}

	record := testProjectRecord("demo",
		testServiceRecord("web",
			testInstanceRecord("web-0", ""),
			testInstanceRecord("web-1", ""),
		),
		testServiceRecord("db",
			testInstanceRecord("db-0", ""),
		),
	)
	if got, want := instanceList(record), "web-0, web-1, db-0"; got != want {
		t.Fatalf("instanceList = %q, want %q", got, want)
	}
}

func TestInstancesInRecordPreservesServiceOrder(t *testing.T) {
	t.Parallel()

	record := testProjectRecord("demo",
		testServiceRecord("web",
			testInstanceRecord("web-0", ""),
			testInstanceRecord("web-1", ""),
		),
		testServiceRecord("db",
			testInstanceRecord("db-0", ""),
		),
	)

	got := instancesInRecord(record)
	assertStringSliceEqual(t, "instancesInRecord names", testInstanceNames(got), []string{"web-0", "web-1", "db-0"})
}

func TestMissingInstanceDetail(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		positional []string
		want       string
	}{
		{name: "project only", positional: []string{"demo"}, want: ""},
		{name: "project and missing instance", positional: []string{"demo", "vm-9"}, want: ` (no instance "vm-9")`},
		{name: "empty", positional: nil, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := missingInstanceDetail(tt.positional); got != tt.want {
				t.Fatalf("missingInstanceDetail(%v) = %q, want %q", tt.positional, got, tt.want)
			}
		})
	}
}

func TestServiceHasLoginUser(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		svc   config.Manifest
		found bool
		want  bool
	}{
		{name: "found with user", svc: config.Manifest{CloudInit: config.CloudInit{User: "debian"}}, found: true, want: true},
		{name: "found without user", found: true},
		{name: "missing with user", svc: config.Manifest{CloudInit: config.CloudInit{User: "debian"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := serviceHasLoginUser(tt.svc, tt.found); got != tt.want {
				t.Fatalf("serviceHasLoginUser = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProjectRecordLoginUser(t *testing.T) {
	t.Parallel()

	record := testProjectRecord(testLookupProjectName,
		testServiceRecord(testLookupVMService),
		testServiceRecordWithLoginUser(testLookupDBService, testLookupLoginUser),
	)
	if got := projectRecordLoginUser(record); got != testLookupLoginUser {
		t.Fatalf("projectRecordLoginUser = %q, want %q", got, testLookupLoginUser)
	}
	if got := projectRecordLoginUser(testProjectRecord(testLookupProjectName, testServiceRecord(testLookupVMService))); got != "" {
		t.Fatalf("projectRecordLoginUser without login user = %q, want empty", got)
	}
}

func TestComposeProjectLoginUser(t *testing.T) {
	t.Parallel()

	project := &compose.Project{
		Services: map[string]config.Manifest{
			testLookupVMService: {Name: testLookupVMService},
			testLookupDBService: {
				Name:      testLookupDBService,
				CloudInit: config.CloudInit{User: testLookupLoginUser},
			},
		},
	}
	if got := composeProjectLoginUser(project); got != testLookupLoginUser {
		t.Fatalf("composeProjectLoginUser = %q, want %q", got, testLookupLoginUser)
	}
	if got := composeProjectLoginUser(&compose.Project{Services: map[string]config.Manifest{testLookupVMService: {Name: testLookupVMService}}}); got != "" {
		t.Fatalf("composeProjectLoginUser without login user = %q, want empty", got)
	}
}

// TestResolveInstanceTarget_ProjectMode walks the project-name
// branch of the unified lookup used by exec and console:
//
//	[<project>]              -> sole instance
//	[<project> <inst>]       -> explicit instance
//	[<project> <inst> args]  -> explicit instance + cmd tail
//	[<project> <bad-inst>]   -> error mentions known instances
//
// The compose-file branch is exercised by integration tests; this
// covers everything that doesn't need a real holos.yaml on disk.
func TestResolveInstanceTarget_ProjectMode(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	manager := runtime.NewManager(stateDir)
	if err := writeProjectRecord(stateDir, testProjectRecord(testLookupProjectName,
		testServiceRecord(testLookupVMService,
			testRunningInstance(testLookupVM0, testLookupVM0SSHPort, testLookupVM0Serial),
			testRunningInstance(testLookupVM1, testLookupVM1SSHPort, testLookupVM1Serial),
		),
	)); err != nil {
		t.Fatalf("seed: %v", err)
	}

	t.Run("project-only-with-multi-instance-errors", func(t *testing.T) {
		_, err := resolveInstanceTarget(manager, "", stateDir, []string{testLookupProjectName})
		assertErrorMentionsAll(t, err, testLookupVM0, testLookupVM1)
	})

	t.Run("project-and-instance", func(t *testing.T) {
		tgt, err := resolveInstanceTarget(manager, "", stateDir, []string{testLookupProjectName, testLookupVM1})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if tgt.Inst.Name != testLookupVM1 || tgt.ProjectName != testLookupProjectName {
			t.Errorf("got inst=%q project=%q", tgt.Inst.Name, tgt.ProjectName)
		}
		assertStringSliceEqual(t, "CmdArgs", tgt.CmdArgs, nil)
	})

	t.Run("project-instance-and-cmd-tail", func(t *testing.T) {
		tgt, err := resolveInstanceTarget(manager, "", stateDir, []string{testLookupProjectName, testLookupVM0, testLookupEchoCmd, testLookupEchoArg})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if tgt.Inst.Name != testLookupVM0 {
			t.Errorf("got inst=%q, want %s", tgt.Inst.Name, testLookupVM0)
		}
		assertStringSliceEqual(t, "CmdArgs", tgt.CmdArgs, []string{testLookupEchoCmd, testLookupEchoArg})
	})

	t.Run("project-and-unknown-instance", func(t *testing.T) {
		_, err := resolveInstanceTarget(manager, "", stateDir, []string{testLookupProjectName, testLookupBadInstance})
		assertErrorMentionsAll(t, err, testLookupBadInstance, testLookupVM0)
	})
}

// TestResolveInstanceTarget_SingleInstanceProject covers the
// happy path: `holos run` always produces one service with one
// replica, so `holos exec <project>` should work with no further
// arguments.
func TestResolveInstanceTarget_SingleInstanceProject(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	manager := runtime.NewManager(stateDir)
	if err := writeProjectRecord(stateDir, testProjectRecord(testLookupRunProject,
		testServiceRecord(testLookupVMService,
			testRunningInstance(testLookupVM0, testLookupVM0SSHPort, testLookupVM0Serial),
		),
	)); err != nil {
		t.Fatalf("seed: %v", err)
	}

	tgt, err := resolveInstanceTarget(manager, "", stateDir, []string{testLookupRunProject})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tgt.Inst.Name != testLookupVM0 {
		t.Errorf("got %q, want %s", tgt.Inst.Name, testLookupVM0)
	}
}

// TestResolveInstanceTarget_SingleInstanceCmdTail pins the
// `holos exec <project> <cmd...>` shorthand that was broken before:
// the old path forwarded any second positional into
// findInstanceInRecord and errored with "no instance \"ls\"", making
// the operator either type vm-0 or switch to -f. For single-instance
// projects the sole-instance resolver must kick in and treat
// positional[1:] as the remote command verbatim.
func TestResolveInstanceTarget_SingleInstanceCmdTail(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	manager := runtime.NewManager(stateDir)
	if err := writeProjectRecord(stateDir, testProjectRecord(testLookupAdHocProject,
		testServiceRecordWithLoginUser(testLookupVMService, testLookupLoginUser,
			testRunningInstance(testLookupVM0, testLookupVM0SSHPort, testLookupVM0Serial),
		),
	)); err != nil {
		t.Fatalf("seed: %v", err)
	}

	tgt, err := resolveInstanceTarget(manager, "", stateDir, []string{testLookupAdHocProject, testLookupCmdName, testLookupCmdArg})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tgt.Inst.Name != testLookupVM0 {
		t.Errorf("got inst=%q, want %s", tgt.Inst.Name, testLookupVM0)
	}
	assertStringSliceEqual(t, "CmdArgs", tgt.CmdArgs, []string{"ls", "-la"})
	// LoginUser is sourced from ServiceRecord.LoginUser now, so a
	// user-authored compose project launched from some arbitrary
	// directory answers the right user (debian/alpine/...) without
	// needing state_dir/runs/<project>/holos.yaml to exist.
	if tgt.LoginUser != testLookupLoginUser {
		t.Errorf("LoginUser = %q, want %s (from ServiceRecord)", tgt.LoginUser, testLookupLoginUser)
	}
}

// TestResolveInstanceTarget_RejectsTraversalNames proves the project
// name supplied as a bare CLI argument cannot escape the state
// directory. Without the validator at the CLI boundary a call like
// `holos exec ../../../etc/passwd` would be fed directly into
// loadProject -> filepath.Join -> os.ReadFile.
func TestResolveInstanceTarget_RejectsTraversalNames(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	manager := runtime.NewManager(stateDir)

	// A sibling project is seeded so the traversal's only route out
	// is the name itself, not a stale record on disk.
	if err := writeFakeProjectRecord(stateDir, "legit"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	badNames := []string{
		"../etc/passwd",
		"legit/../legit",
		"legit\x00injected",
		"UPPER",
		"",
	}
	for _, name := range badNames {
		if _, err := resolveInstanceTarget(manager, "", stateDir, []string{name}); err == nil {
			t.Errorf("expected error for bad name %q, got nil", name)
		}
	}
}

func testProjectRecord(name string, services ...runtime.ServiceRecord) *runtime.ProjectRecord {
	return &runtime.ProjectRecord{Name: name, Services: services}
}

func testServiceRecord(name string, instances ...runtime.InstanceRecord) runtime.ServiceRecord {
	return runtime.ServiceRecord{
		Name:            name,
		DesiredReplicas: len(instances),
		Instances:       instances,
	}
}

func testServiceRecordWithLoginUser(name, loginUser string, instances ...runtime.InstanceRecord) runtime.ServiceRecord {
	service := testServiceRecord(name, instances...)
	service.LoginUser = loginUser
	return service
}

func testInstanceRecord(name, logPath string) runtime.InstanceRecord {
	return runtime.InstanceRecord{Name: name, LogPath: logPath}
}

func testRunningInstance(name string, sshPort int, serialPath string) runtime.InstanceRecord {
	return runtime.InstanceRecord{
		Name:       name,
		Status:     runtime.InstanceStatusRunning,
		SSHPort:    sshPort,
		SerialPath: serialPath,
	}
}

// writeFakeProjectRecord is the minimum payload that ProjectStatus
// will accept. We avoid using runtime internals (saveProject etc.)
// so the test stays decoupled from runtime implementation details.
func writeFakeProjectRecord(stateDir, name string) error {
	return writeProjectRecord(stateDir, &runtime.ProjectRecord{Name: name})
}

func writeProjectRecord(stateDir string, record *runtime.ProjectRecord) error {
	dir := filepath.Join(stateDir, testProjectsDirSuffix)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return os.WriteFile(projectRecordPath(stateDir, record.Name), payload, 0o644)
}

func projectRecordPath(stateDir, name string) string {
	return filepath.Join(stateDir, testProjectsDirSuffix, name+testProjectRecordExt)
}
