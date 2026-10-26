package runtime

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/zeroecco/holos/internal/compose"
	"github.com/zeroecco/holos/internal/config"
)

func TestUpPreviewAndRefusalPreserveState(t *testing.T) {
	m := NewManager(t.TempDir())
	manifest := config.Manifest{Name: "web", Replicas: 1, VM: config.VMConfig{MemoryMB: 256}}
	work := t.TempDir()
	marker := filepath.Join(work, "root.qcow2")
	if err := os.WriteFile(marker, []byte("valuable disk"), 0600); err != nil {
		t.Fatal(err)
	}
	record := &ProjectRecord{Name: "demo", SpecHash: "old", Services: []ServiceRecord{{Name: "web", SpecHash: serviceFingerprint(manifest), Instances: []InstanceRecord{{Name: "web-0", WorkDir: work}}}}}
	if err := m.saveProject(record); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(projectFile(m.stateDir, "demo"))
	if err != nil {
		t.Fatal(err)
	}
	manifest.VM.MemoryMB = 512
	project := &compose.Project{Name: "demo", SpecHash: "new", ServiceOrder: []string{"web"}, Services: map[string]config.Manifest{"web": manifest}}
	var plan []ServiceChange
	if _, err := m.UpWithOptions(project, UpOptions{DryRun: true, Report: func(changes []ServiceChange) { plan = changes }}); err != nil {
		t.Fatal(err)
	}
	if len(plan) != 1 || plan[0].Action != "replace" {
		t.Fatalf("plan: %+v", plan)
	}
	if _, err := m.Up(project); err == nil || !strings.Contains(err.Error(), "--recreate") {
		t.Fatalf("expected explicit recreation error, got %v", err)
	}
	after, _ := os.ReadFile(projectFile(m.stateDir, "demo"))
	disk, _ := os.ReadFile(marker)
	if string(disk) != "valuable disk" || string(before) != string(after) {
		t.Fatal("preview or refused update changed disk/state")
	}
}

func TestUpdatePlanServiceIsolationAndLegacy(t *testing.T) {
	web := config.Manifest{Name: "web", Replicas: 1}
	db := config.Manifest{Name: "db", Replicas: 1}
	record := &ProjectRecord{Name: "demo", Services: []ServiceRecord{
		{Name: "web", SpecHash: serviceFingerprint(web), Instances: []InstanceRecord{{Name: "web-0"}}},
		{Name: "db", SpecHash: serviceFingerprint(db), Instances: []InstanceRecord{{Name: "db-0"}}},
	}}
	web.CloudInit.Packages = []string{"nginx"}
	project := &compose.Project{ServiceOrder: []string{"db", "web"}, Services: map[string]config.Manifest{"web": web, "db": db}}
	plan := projectChanges(project, record)
	if plan[0].Action != "keep" || plan[1].Action != "replace" {
		t.Fatalf("plan: %+v", plan)
	}
	record.Services[1].SpecHash = ""
	plan = projectChanges(project, record)
	if plan[0].Action != "replace" || !strings.Contains(plan[0].Reason, "legacy") {
		t.Fatalf("legacy plan: %+v", plan)
	}
	delete(project.Services, "web")
	project.ServiceOrder = []string{"db"}
	plan = projectChanges(project, record)
	if plan[1].Action != "remove" {
		t.Fatalf("removal plan: %+v", plan)
	}
}

func TestPrepareUpdatesCheckpointsOnlyAffectedServices(t *testing.T) {
	m := NewManager(t.TempDir())
	keep, replace := t.TempDir(), t.TempDir()
	record := &ProjectRecord{Name: "demo", Services: []ServiceRecord{
		{Name: "db", SpecHash: "keep", Instances: []InstanceRecord{{WorkDir: keep}}},
		{Name: "web", SpecHash: "old", Instances: []InstanceRecord{{WorkDir: replace}}},
	}}
	if err := m.saveProject(record); err != nil {
		t.Fatal(err)
	}
	if err := m.prepareUpdates(record, []ServiceChange{{Service: "web", Action: "replace"}}, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatal("unaffected directory removed", err)
	}
	if _, err := os.Stat(replace); !os.IsNotExist(err) {
		t.Fatal("replacement directory remains", err)
	}
	persisted, err := m.loadProject("demo")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(record.Services, persisted.Services) || len(persisted.Services) != 1 || persisted.Services[0].Name != "db" {
		t.Fatalf("bad checkpoint: %+v", persisted)
	}
}
