package compose

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zeroecco/holos/internal/config"
)

func TestLoadAndResolve(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeTestImage(t, dir)
	if err := os.MkdirAll(filepath.Join(dir, "www"), 0o755); err != nil {
		t.Fatal(err)
	}

	file := loadTestCompose(t, dir, testCompose)

	if file.Name != testComposeAppName {
		t.Fatalf("expected name %s, got %s", testComposeAppName, file.Name)
	}
	if len(file.Services) != 3 {
		t.Fatalf("expected 3 services, got %d", len(file.Services))
	}

	stateDir := testStateDir(dir)
	project, err := file.Resolve(dir, stateDir)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	if project.Name != testComposeAppName {
		t.Fatalf("expected project name %s, got %s", testComposeAppName, project.Name)
	}
	if project.SpecHash == "" {
		t.Fatal("expected non-empty spec hash")
	}

	// db has no dependencies, should come first.
	if project.ServiceOrder[0] != testComposeDBService {
		t.Fatalf("expected %s first in order, got %v", testComposeDBService, project.ServiceOrder)
	}
	if got := project.Services[testComposeDBService].VM.DiskSizeBytes; got != testComposeDBDiskBytes {
		t.Fatalf("expected db disk size 2GiB, got %d", got)
	}

	// web depends on api which depends on db, so web must be last.
	if project.ServiceOrder[len(project.ServiceOrder)-1] != testComposeWebService {
		t.Fatalf("expected %s last in order, got %v", testComposeWebService, project.ServiceOrder)
	}

	web := project.Services[testComposeWebService]
	if web.Replicas != testComposeWebReplicas {
		t.Fatalf("expected web replicas %d, got %d", testComposeWebReplicas, web.Replicas)
	}
	if len(web.Ports) != 1 {
		t.Fatalf("web ports len = %d, want 1: %+v", len(web.Ports), web.Ports)
	}
	assertPortForward(t, "web", web.Ports[0], testPortForwardWant{
		hostPort:  testComposeWebHostPort,
		guestPort: testComposeWebGuestPort,
		protocol:  config.DefaultProtocol,
	})
	if len(web.Mounts) != 1 {
		t.Fatalf("web mounts len = %d, want 1: %+v", len(web.Mounts), web.Mounts)
	}
	assertMount(t, "web", web.Mounts[0], testMountWant{
		kind:     config.MountKindBind,
		source:   filepath.Join(dir, "www"),
		target:   testComposeWebMount,
		readOnly: true,
	})
	if web.InternalNetwork == nil {
		t.Fatal("expected internal network config on web service")
	}
	if len(web.InternalNetwork.InstanceIPs) != testComposeWebReplicas {
		t.Fatalf("expected %d instance IPs for web, got %d", testComposeWebReplicas, len(web.InternalNetwork.InstanceIPs))
	}

	if len(project.Network.Hosts) == 0 {
		t.Fatal("expected hosts map to be populated")
	}
	if _, ok := project.Network.Hosts[testComposeDBService]; !ok {
		t.Fatal("expected db in hosts")
	}
	if _, ok := project.Network.Hosts[testComposeWebService]; !ok {
		t.Fatal("expected web in hosts")
	}
}

func TestServiceBaseDir(t *testing.T) {
	t.Parallel()

	defaultDir := filepath.Join("project", "root")
	includedDir := filepath.Join("project", "included")

	tests := []struct {
		name string
		file *File
		want string
	}{
		{name: "nil map", file: &File{}, want: defaultDir},
		{name: "missing service", file: &File{serviceBaseDirs: map[string]string{testComposeAPIService: includedDir}}, want: defaultDir},
		{name: "empty override", file: &File{serviceBaseDirs: map[string]string{testComposeWebService: ""}}, want: defaultDir},
		{name: "override", file: &File{serviceBaseDirs: map[string]string{testComposeWebService: includedDir}}, want: includedDir},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.file.serviceBaseDir(testComposeWebService, defaultDir); got != tt.want {
				t.Fatalf("serviceBaseDir() = %q, want %q", got, tt.want)
			}
		})
	}
}
