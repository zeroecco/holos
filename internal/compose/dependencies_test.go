package compose

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestTopoSortDetectsCycle(t *testing.T) {
	t.Parallel()

	file := &File{
		Name: "cycle",
		Services: map[string]Service{
			"a": {Image: "x.qcow2", DependsOn: DependsOn{"b"}},
			"b": {Image: "x.qcow2", DependsOn: DependsOn{"a"}},
		},
	}

	_, err := file.topoSort()
	if err == nil {
		t.Fatal("expected cycle detection error")
	}
}

func TestResolveAcceptsLongDependsOnSyntax(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeTestImage(t, dir)
	yamlDoc := `
name: longdeps
services:
  web:
    image: ./base.qcow2
    depends_on:
      db:
        condition: service_healthy
        restart: true
      redis:
        condition: service_started
        required: true
  db:
    image: ./base.qcow2
  redis:
    image: ./base.qcow2
`
	project := resolveTestCompose(t, dir, yamlDoc)
	dependencies := []string{"db", "redis"}
	assertStringSliceEqual(t, "service order", project.ServiceOrder, []string{"db", "redis", "web"})
	assertStringSliceEqual(t, "web depends_on", project.Services["web"].DependsOn, dependencies)
}

func TestDependsOnUnmarshalAcceptsLongFormFields(t *testing.T) {
	t.Parallel()

	var deps DependsOn
	err := yaml.Unmarshal([]byte(`
db:
  condition: service_healthy
  restart: true
redis:
  condition: service_started
  required: true
job:
  condition: service_completed_successfully
`), &deps)
	if err != nil {
		t.Fatalf("unmarshal depends_on: %v", err)
	}
	want := []string{"db", "job", "redis"}
	assertStringSliceEqual(t, "depends_on", []string(deps), want)
}

func TestDependsOnUnmarshalAcceptsNullMappingValue(t *testing.T) {
	t.Parallel()

	var deps DependsOn
	if err := yaml.Unmarshal([]byte("db:\nredis:\n"), &deps); err != nil {
		t.Fatalf("unmarshal depends_on: %v", err)
	}
	want := []string{"db", "redis"}
	assertStringSliceEqual(t, "depends_on", []string(deps), want)
}

func TestDependsOnRejectsInvalidMappingValue(t *testing.T) {
	t.Parallel()

	var deps DependsOn
	err := yaml.Unmarshal([]byte("db: service_started\n"), &deps)
	assertErrorContains(t, err, `depends_on service "db" must be a mapping`)
}

func TestDependsOnRejectsUnsupportedLongFormField(t *testing.T) {
	t.Parallel()

	var deps DependsOn
	err := yaml.Unmarshal([]byte(`
db:
  condition: service_started
  delay: 3s
`), &deps)
	assertErrorContains(t, err, "field delay not found")
}

func TestDependsOnRejectsUnsupportedCondition(t *testing.T) {
	t.Parallel()

	var deps DependsOn
	err := yaml.Unmarshal([]byte(`
db:
  condition: service_ready
`), &deps)
	assertErrorContains(t, err, `unsupported depends_on condition "service_ready"`)
}
