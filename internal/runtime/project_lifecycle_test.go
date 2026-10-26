package runtime

import (
	"os"
	"testing"

	"github.com/zeroecco/holos/internal/compose"
)

func TestLoadProjectForUpPreservesSavedState(t *testing.T) {
	t.Parallel()
	manager := NewManager(t.TempDir())
	project := &compose.Project{Name: "demo"}
	record, err := manager.loadProjectForUp(project)
	if err != nil {
		t.Fatal(err)
	}
	if record.Name != project.Name || len(record.Services) != 0 {
		t.Fatalf("new project record: %+v", record)
	}

	record.Services = []ServiceRecord{{Name: "web", Instances: []InstanceRecord{{Name: "web-0", WorkDir: "/existing/disk"}}}}
	if err := manager.saveProject(record); err != nil {
		t.Fatal(err)
	}
	loaded, err := manager.loadProjectForUp(project)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Services) != 1 || len(loaded.Services[0].Instances) != 1 || loaded.Services[0].Instances[0].WorkDir != "/existing/disk" {
		t.Fatalf("saved instances were lost: %+v", loaded)
	}

	// A corrupt record must not be treated as a new, empty project.
	path := projectFile(manager.stateDir, project.Name)
	if err := os.WriteFile(path, []byte("{incomplete"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.loadProjectForUp(project); err == nil {
		t.Fatal("accepted a corrupt project record")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "{incomplete" {
		t.Fatal("loading rewrote the corrupt record")
	}
}
