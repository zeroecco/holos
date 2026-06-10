package compose

import "testing"

func TestValidateRejectsEmptyServices(t *testing.T) {
	t.Parallel()

	file := &File{Name: "test", Services: map[string]Service{}}
	if err := file.validate(); err == nil {
		t.Fatal("expected validation error for empty services")
	}
}

func TestValidateRejectsMissingDependency(t *testing.T) {
	t.Parallel()

	file := &File{
		Name: "test",
		Services: map[string]Service{
			"a": {Image: "x.qcow2", DependsOn: DependsOn{"nonexistent"}},
		},
	}
	if err := file.validate(); err == nil {
		t.Fatal("expected validation error for missing dependency")
	}
}

func TestServiceHasRunnableSource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		svc  Service
		want bool
	}{
		{name: "image", svc: Service{Image: "base.qcow2"}, want: true},
		{name: "standalone dockerfile", svc: Service{Dockerfile: "Dockerfile"}, want: true},
		{name: "compose build", svc: Service{Build: ComposeBuild{Context: "."}}, want: true},
		{name: "missing source"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := serviceHasRunnableSource(tt.svc); got != tt.want {
				t.Fatalf("serviceHasRunnableSource = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestResolveValidatesManifest pins the contract that compose
// resolution runs every resolved service through Manifest.Validate
// before returning. Without this, holos validate would happily accept
// memory_mb: -1 (later panicking deep in the runtime) and out-of-range
// host ports (later silently misconfiguring qemu user-net).
func TestResolveValidatesManifest(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeTestImage(t, dir)

	cases := []struct {
		name string
		body string
	}{
		{
			name: "negative memory",
			body: `
name: bad
services:
  vm:
    image: ./base.qcow2
    vm:
      memory_mb: -1
`,
		},
		{
			name: "tiny disk",
			body: `
name: bad
services:
  vm:
    image: ./base.qcow2
    vm:
      disk_size: 100
`,
		},
		{
			name: "host port out of range",
			body: `
name: bad
services:
  vm:
    image: ./base.qcow2
    ports:
      - "99999:80"
`,
		},
		{
			name: "negative replicas",
			body: `
name: bad
services:
  vm:
    image: ./base.qcow2
    replicas: -1
`,
		},
		{
			name: "replicas above cap",
			body: `
name: bad
services:
  vm:
    image: ./base.qcow2
    replicas: 100000
`,
		},
		{
			name: "project replicas exceed subnet",
			body: `
name: bad
services:
  a:
    image: ./base.qcow2
    replicas: 200
  b:
    image: ./base.qcow2
    replicas: 100
`,
		},
		{
			name: "static host port overflows across replicas",
			body: `
name: bad
services:
  vm:
    image: ./base.qcow2
    replicas: 2
    ports:
      - "65535:80"
`,
		},
		// 8080:80 and 8081:81 look disjoint on paper, but the
		// runtime shifts both by the replica index, so replica 1
		// tries to bind 8081 for *both* mappings. Pre-fix this
		// slipped through validation and blew up mid-`holos up`
		// with an opaque bind error.
		{
			name: "static host ports collide after replica offset",
			body: `
name: bad
services:
  vm:
    image: ./base.qcow2
    replicas: 2
    ports:
      - "8080:80"
      - "8081:81"
`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeTestFile(t, dir, tc.name+".yaml", tc.body)
			file, err := Load(path)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if _, err := file.Resolve(dir, dir); err == nil {
				t.Fatalf("expected resolve error for %q, got nil", tc.name)
			}
		})
	}
}

// TestResolveRejectsMissingLocalImage pins the contract that a
// compose file pointing at a local qcow2/raw that is not on disk is
// rejected at resolution time, which is what `holos validate` runs.
// Without this the failure surfaces much later inside qemu-img in
// `holos up`, and users reasonably assume `validate` caught anything
// it would.
func TestResolveRejectsMissingLocalImage(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	body := `
name: missing
services:
  vm:
    image: ./missing.qcow2
`
	path := writeTestFile(t, dir, "missing.yaml", body)
	file, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	_, err = file.Resolve(dir, dir)
	assertErrorContains(t, err, "missing.qcow2")
}

// TestLoadRejectsUnknownFields ensures the strict YAML decoder catches
// typos that previously slipped through silently. Each case is the
// minimum YAML needed to elicit the misspelled key, asserting against
// the Go field that should have caught it.
func TestLoadRejectsUnknownFields(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		body string
	}{
		{
			name: "top level typo",
			body: `
name: typo
servicez:
  vm:
    image: ./base.qcow2
`,
		},
		{
			name: "service-level typo",
			body: `
name: typo
services:
  vm:
    image: ./base.qcow2
    portz:
      - "8080:80"
`,
		},
		{
			name: "nested vm typo",
			body: `
name: typo
services:
  vm:
    image: ./base.qcow2
    vm:
      memry_mb: 512
`,
		},
	}

	dir := t.TempDir()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeTestFile(t, dir, tc.name+".yaml", tc.body)
			if _, err := Load(path); err == nil {
				t.Fatalf("expected unknown-field error for %q, got nil", tc.name)
			}
		})
	}
}
