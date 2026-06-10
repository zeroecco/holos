package runtime

import (
	"errors"
	"maps"
	"os"
	"path/filepath"
	"testing"

	"github.com/zeroecco/holos/internal/config"
)

const (
	projectUpWebServiceName   = "web"
	projectUpDBServiceName    = "db"
	projectUpAPIServiceName   = "api"
	projectUpQueueServiceName = "queue"
	projectUpOldServiceName   = "old"
)

func assertServiceRecordDesiredReplicas(t *testing.T, records map[string]*ServiceRecord, name string, want int) {
	t.Helper()

	record := records[name]
	if record == nil {
		t.Fatalf("%s service lookup = nil, want DesiredReplicas=%d", name, want)
	}
	if record.DesiredReplicas != want {
		t.Fatalf("%s DesiredReplicas = %d, want %d", name, record.DesiredReplicas, want)
	}
}

func TestExistingServicesByName(t *testing.T) {
	t.Parallel()

	services := []ServiceRecord{
		{Name: projectUpWebServiceName, DesiredReplicas: 2},
		{Name: projectUpDBServiceName, DesiredReplicas: 1},
	}

	byName := existingServicesByName(services)
	if len(byName) != 2 {
		t.Fatalf("existingServicesByName returned %d entries, want 2", len(byName))
	}
	assertServiceRecordDesiredReplicas(t, byName, projectUpWebServiceName, 2)
	assertServiceRecordDesiredReplicas(t, byName, projectUpDBServiceName, 1)
}

func TestServicesWithDependents(t *testing.T) {
	t.Parallel()

	services := map[string]config.Manifest{
		projectUpDBServiceName: {Name: projectUpDBServiceName},
		projectUpAPIServiceName: {
			Name:      projectUpAPIServiceName,
			DependsOn: []string{projectUpDBServiceName},
		},
		projectUpWebServiceName: {
			Name:      projectUpWebServiceName,
			DependsOn: []string{projectUpAPIServiceName},
		},
		projectUpQueueServiceName: {Name: projectUpQueueServiceName},
	}

	names := []string{
		projectUpDBServiceName,
		projectUpAPIServiceName,
		projectUpWebServiceName,
		projectUpQueueServiceName,
	}
	got := servicesWithDependents(names, services)
	want := map[string]bool{
		projectUpDBServiceName:  true,
		projectUpAPIServiceName: true,
	}
	if !maps.Equal(got, want) {
		t.Fatalf("servicesWithDependents = %v, want %v", got, want)
	}
	for _, name := range []string{projectUpWebServiceName, projectUpQueueServiceName} {
		if got[name] {
			t.Fatalf("servicesWithDependents[%q] = true, want false", name)
		}
	}
}

func TestShouldWaitForServiceHealth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		manifest      config.Manifest
		hasDependents bool
		want          bool
	}{
		{name: "no healthcheck", hasDependents: true, want: false},
		{name: "no dependents", manifest: config.Manifest{Healthcheck: &config.HealthcheckConfig{}}, want: false},
		{name: "healthcheck gates dependents", manifest: config.Manifest{Healthcheck: &config.HealthcheckConfig{}}, hasDependents: true, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := shouldWaitForServiceHealth(tt.manifest, tt.hasDependents); got != tt.want {
				t.Fatalf("shouldWaitForServiceHealth = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestServiceRecordHasInstances(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		record *ServiceRecord
		want   bool
	}{
		{name: "nil"},
		{name: "empty", record: &ServiceRecord{}},
		{name: "has instance", record: &ServiceRecord{Instances: []InstanceRecord{{Name: "web-0"}}}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := serviceRecordHasInstances(tt.record); got != tt.want {
				t.Fatalf("serviceRecordHasInstances = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCleanupRemovedServicesRemovesOnlyUndesiredWorkdirs(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	keptDir := filepath.Join(root, "kept")
	removedDir := filepath.Join(root, "removed")
	for _, dir := range []string{keptDir, removedDir} {
		if err := os.MkdirAll(dir, stateDirPerm); err != nil {
			t.Fatalf("create workdir %s: %v", dir, err)
		}
	}

	existing := map[string]*ServiceRecord{
		projectUpWebServiceName: {
			Name: projectUpWebServiceName,
			Instances: []InstanceRecord{
				{Name: instanceDirName(projectUpWebServiceName, 0), WorkDir: keptDir},
			},
		},
		projectUpOldServiceName: {
			Name: projectUpOldServiceName,
			Instances: []InstanceRecord{
				{Name: instanceDirName(projectUpOldServiceName, 0), WorkDir: removedDir},
			},
		},
	}
	desired := map[string]config.Manifest{
		projectUpWebServiceName: {Name: projectUpWebServiceName},
	}

	manager := &Manager{}
	if err := manager.cleanupRemovedServices("", existing, desired); err != nil {
		t.Fatalf("cleanupRemovedServices: %v", err)
	}

	if _, err := os.Stat(keptDir); err != nil {
		t.Fatalf("kept workdir stat: %v", err)
	}
	if _, err := os.Stat(removedDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("removed workdir stat err = %v, want os.ErrNotExist", err)
	}
	if got := existing[projectUpOldServiceName].Instances[0].Status; got != InstanceStatusStopped {
		t.Fatalf("removed service instance status = %q, want %q", got, InstanceStatusStopped)
	}
}
