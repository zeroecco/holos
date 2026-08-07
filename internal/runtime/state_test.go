package runtime

import (
	"os"
	"path/filepath"
	"testing"
)

func projectRecordNames(records []*ProjectRecord) []string {
	names := make([]string, len(records))
	for i, record := range records {
		names[i] = record.Name
	}
	return names
}

func assertSavedProjectRecord(t *testing.T, got, want *ProjectRecord) {
	t.Helper()

	if got.Name != want.Name || got.SpecHash != want.SpecHash {
		t.Fatalf("loaded project identity = %#v, want %#v", got, want)
	}
	if len(got.Services) != len(want.Services) {
		t.Fatalf("loaded services len = %d, want %d: %#v", len(got.Services), len(want.Services), got.Services)
	}
	for i, wantService := range want.Services {
		if got.Services[i].Name != wantService.Name {
			t.Fatalf("loaded service[%d].Name = %q, want %q: %#v", i, got.Services[i].Name, wantService.Name, got.Services)
		}
	}
}

func TestSaveProjectWritesPrivateRecord(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	manager := NewManager(stateDir)
	record := &ProjectRecord{
		Name:     "demo",
		SpecHash: "abc123",
		Services: []ServiceRecord{
			{Name: "web", DesiredReplicas: 1},
		},
	}

	if err := manager.saveProject(record); err != nil {
		t.Fatalf("saveProject: %v", err)
	}
	assertMode(t, projectFile(stateDir, record.Name), projectRecordPerm)

	loaded, err := manager.loadProject(record.Name)
	if err != nil {
		t.Fatalf("loadProject: %v", err)
	}
	assertSavedProjectRecord(t, loaded, record)
}

func TestSaveProjectAtomicallyReplacesRecord(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	manager := NewManager(stateDir)
	if err := manager.saveProject(&ProjectRecord{Name: "demo", SpecHash: "old"}); err != nil {
		t.Fatalf("save initial project: %v", err)
	}
	if err := manager.saveProject(&ProjectRecord{Name: "demo", SpecHash: "new"}); err != nil {
		t.Fatalf("replace project: %v", err)
	}

	loaded, err := manager.loadProject("demo")
	if err != nil {
		t.Fatalf("load replaced project: %v", err)
	}
	if loaded.SpecHash != "new" {
		t.Fatalf("loaded SpecHash = %q, want new", loaded.SpecHash)
	}
	leftovers, err := filepath.Glob(filepath.Join(projectsDir(stateDir), ".project-*.tmp"))
	if err != nil {
		t.Fatalf("glob temporary records: %v", err)
	}
	if len(leftovers) != 0 {
		t.Fatalf("temporary project records remain after save: %v", leftovers)
	}
	assertMode(t, projectFile(stateDir, "demo"), projectRecordPerm)
}

func TestSaveProjectRejectsUnsafeOrMissingIdentity(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	manager := NewManager(stateDir)
	for _, record := range []*ProjectRecord{nil, {Name: "../escaped"}, {}} {
		if err := manager.saveProject(record); err == nil {
			t.Fatalf("saveProject(%#v) succeeded, want error", record)
		}
	}
	if _, err := os.Stat(filepath.Join(stateDir, "escaped.json")); !os.IsNotExist(err) {
		t.Fatalf("unsafe project record escaped state directory: %v", err)
	}
}

func TestSaveUpdatedProjectStampsAndPersistsRecord(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	manager := NewManager(stateDir)
	record := &ProjectRecord{Name: "demo"}

	if err := manager.saveUpdatedProject(record); err != nil {
		t.Fatalf("saveUpdatedProject: %v", err)
	}
	if record.UpdatedAt.IsZero() {
		t.Fatal("UpdatedAt = zero, want timestamp")
	}

	loaded, err := manager.loadProject(record.Name)
	if err != nil {
		t.Fatalf("loadProject: %v", err)
	}
	if !loaded.UpdatedAt.Equal(record.UpdatedAt) {
		t.Fatalf("loaded UpdatedAt = %s, want %s", loaded.UpdatedAt, record.UpdatedAt)
	}
}

func TestSortProjectRecords(t *testing.T) {
	t.Parallel()

	projects := []*ProjectRecord{
		{Name: "web"},
		{Name: "api"},
		{Name: "db"},
	}

	sortProjectRecords(projects)

	want := []string{"api", "db", "web"}
	assertStringSliceEqual(t, "sorted project names", projectRecordNames(projects), want)
}
