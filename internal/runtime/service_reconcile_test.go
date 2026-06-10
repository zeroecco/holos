package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/zeroecco/holos/internal/config"
)

const (
	testWebServiceName = "web"
	testDBServiceName  = "db"
)

func testInstanceName(service string, index int) string {
	return fmt.Sprintf("%s-%d", service, index)
}

func TestServiceRecordForManifest(t *testing.T) {
	t.Parallel()

	manifest := config.Manifest{
		Name:     testWebServiceName,
		Replicas: 3,
		CloudInit: config.CloudInit{
			User: "debian",
		},
	}
	record := serviceRecordForManifest(manifest)
	if record.Name != testWebServiceName {
		t.Fatalf("Name = %q, want %q", record.Name, testWebServiceName)
	}
	if record.DesiredReplicas != 3 {
		t.Fatalf("DesiredReplicas = %d, want 3", record.DesiredReplicas)
	}
	if record.LoginUser != "debian" {
		t.Fatalf("LoginUser = %q, want debian", record.LoginUser)
	}
	if len(record.Instances) != 0 {
		t.Fatalf("Instances = %#v, want empty", record.Instances)
	}
}

func TestShouldReuseInstance(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		inst *InstanceRecord
		want bool
	}{
		{name: "nil", inst: nil, want: false},
		{name: "running", inst: &InstanceRecord{Status: InstanceStatusRunning}, want: true},
		{name: "stopped", inst: &InstanceRecord{Status: InstanceStatusStopped}, want: false},
		{name: "empty status", inst: &InstanceRecord{}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := shouldReuseInstance(tt.inst); got != tt.want {
				t.Fatalf("shouldReuseInstance(%+v) = %v, want %v", tt.inst, got, tt.want)
			}
		})
	}
}

func TestShouldRestartInstance(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	missingDir := filepath.Join(t.TempDir(), "missing")
	file := filepath.Join(t.TempDir(), "work")
	if err := os.WriteFile(file, []byte("not a dir"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	tests := []struct {
		name string
		inst *InstanceRecord
		want bool
	}{
		{name: "nil", inst: nil, want: false},
		{name: "empty workdir", inst: &InstanceRecord{}, want: false},
		{name: "missing workdir", inst: &InstanceRecord{WorkDir: missingDir}, want: false},
		{name: "file workdir", inst: &InstanceRecord{WorkDir: file}, want: false},
		{name: "existing workdir", inst: &InstanceRecord{WorkDir: workDir}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := shouldRestartInstance(tt.inst); got != tt.want {
				t.Fatalf("shouldRestartInstance(%+v) = %v, want %v", tt.inst, got, tt.want)
			}
		})
	}
}

func TestReconcileServiceInstanceReusesRunningRecord(t *testing.T) {
	t.Parallel()

	existing := InstanceRecord{
		Name:   testInstanceName(testWebServiceName, 0),
		Index:  0,
		PID:    1234,
		Status: InstanceStatusRunning,
	}
	manager := &Manager{}

	got, err := manager.reconcileServiceInstance("project", config.Manifest{Name: testWebServiceName}, existing.Index, &existing)
	if err != nil {
		t.Fatalf("reconcileServiceInstance: %v", err)
	}
	if got.Name != existing.Name || got.Index != existing.Index || got.PID != existing.PID || got.Status != existing.Status {
		t.Fatalf("reconcileServiceInstance = %+v, want %+v", got, existing)
	}
}

func TestExistingInstancesByIndexNormalizesStoppedRecords(t *testing.T) {
	t.Parallel()

	service := &ServiceRecord{
		Instances: []InstanceRecord{
			{Index: 2, PID: 0, Status: InstanceStatusRunning},
			{Index: 0, PID: 0, Status: InstanceStatusStopped},
		},
	}

	byIndex := existingInstancesByIndex(service)
	if len(byIndex) != 2 {
		t.Fatalf("existingInstancesByIndex returned %d entries, want 2", len(byIndex))
	}
	for _, index := range []int{0, 2} {
		inst, ok := byIndex[index]
		if !ok {
			t.Fatalf("missing index %d in existingInstancesByIndex result", index)
		}
		if inst.Status != InstanceStatusStopped {
			t.Fatalf("index %d status = %q, want %q", index, inst.Status, InstanceStatusStopped)
		}
		if inst.PID != 0 {
			t.Fatalf("index %d PID = %d, want 0", index, inst.PID)
		}
	}
	if service.Instances[0].Status != InstanceStatusStopped {
		t.Fatalf("source service instance was not normalized: %+v", service.Instances[0])
	}
}

func TestRefreshInstanceStatusMarksDeadProcessStopped(t *testing.T) {
	t.Parallel()

	inst := InstanceRecord{PID: 0, Status: InstanceStatusRunning}
	refreshInstanceStatus(&inst)
	if inst.Status != InstanceStatusStopped {
		t.Fatalf("status = %q, want %q", inst.Status, InstanceStatusStopped)
	}
	if inst.PID != 0 {
		t.Fatalf("PID = %d, want 0", inst.PID)
	}
}

func TestRefreshProjectNormalizesAllInstances(t *testing.T) {
	t.Parallel()

	manager := &Manager{}
	record := &ProjectRecord{
		Services: []ServiceRecord{
			{
				Name: testWebServiceName,
				Instances: []InstanceRecord{
					{
						Name:   testInstanceName(testWebServiceName, 0),
						PID:    0,
						Status: InstanceStatusRunning,
					},
				},
			},
			{
				Name: testDBServiceName,
				Instances: []InstanceRecord{
					{
						Name:   testInstanceName(testDBServiceName, 0),
						PID:    0,
						Status: InstanceStatusRunning,
					},
				},
			},
		},
	}

	manager.refreshProject(record)

	for _, svc := range record.Services {
		for _, inst := range svc.Instances {
			if inst.Status != InstanceStatusStopped {
				t.Fatalf("%s status = %q, want %q", inst.Name, inst.Status, InstanceStatusStopped)
			}
			if inst.PID != 0 {
				t.Fatalf("%s PID = %d, want 0", inst.Name, inst.PID)
			}
		}
	}
}

func TestExistingInstancesByIndexNilService(t *testing.T) {
	t.Parallel()

	if got := existingInstancesByIndex(nil); len(got) != 0 {
		t.Fatalf("existingInstancesByIndex(nil) = %#v, want empty map", got)
	}
}

func TestAttachSortedInstances(t *testing.T) {
	t.Parallel()

	service := &ServiceRecord{Name: testWebServiceName}
	attachSortedInstances(service, []InstanceRecord{
		{Name: testInstanceName(testWebServiceName, 2), Index: 2},
		{Name: testInstanceName(testWebServiceName, 0), Index: 0},
		{Name: testInstanceName(testWebServiceName, 1), Index: 1},
	})

	got := make([]int, 0, len(service.Instances))
	for _, inst := range service.Instances {
		got = append(got, inst.Index)
	}
	want := []int{0, 1, 2}
	assertIntSliceEqual(t, "sorted indexes", got, want)
}

func TestStopExcessReplicasRemovesInstancesAtOrAboveDesiredCount(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	instances := []InstanceRecord{
		testInstanceWithWorkDir(t, root, 0),
		testInstanceWithWorkDir(t, root, 1),
		testInstanceWithWorkDir(t, root, 2),
	}
	existing := &ServiceRecord{Instances: instances}
	manager := &Manager{}

	manager.stopExcessReplicas(existing, 2)

	assertDirExists(t, instances[0].WorkDir)
	assertDirExists(t, instances[1].WorkDir)
	assertPathRemoved(t, instances[2].WorkDir)
}

func testInstanceWithWorkDir(t *testing.T, root string, index int) InstanceRecord {
	t.Helper()

	workDir := filepath.Join(root, testInstanceName(testWebServiceName, index))
	if err := os.MkdirAll(workDir, stateDirPerm); err != nil {
		t.Fatalf("create workdir: %v", err)
	}
	return InstanceRecord{Name: testInstanceName(testWebServiceName, index), Index: index, WorkDir: workDir}
}

func assertDirExists(t *testing.T, path string) {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if !info.IsDir() {
		t.Fatalf("%s is not a directory", path)
	}
}

func assertPathRemoved(t *testing.T, path string) {
	t.Helper()

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("stat removed path %s = %v, want not exist", path, err)
	}
}
