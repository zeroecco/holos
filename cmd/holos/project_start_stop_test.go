package main

import (
	"testing"

	"github.com/zeroecco/holos/internal/compose"
	"github.com/zeroecco/holos/internal/config"
)

const (
	testStartStopProjectName = "demo"
	testStartStopWebService  = "web"
	testStartStopDBService   = "db"
	testStartStopAPIService  = "api"
)

func TestStartServiceName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		resolvedService    string
		resolvedProjectArg bool
		args               []string
		want               string
	}{
		{name: "resolved service wins", resolvedService: testStartStopWebService, resolvedProjectArg: true, args: []string{testStartStopProjectName, testStartStopDBService}, want: testStartStopWebService},
		{name: "project arg has no implicit service", resolvedProjectArg: true, args: []string{testStartStopProjectName}, want: ""},
		{name: "first arg is service when project not resolved", args: []string{testStartStopWebService}, want: testStartStopWebService},
		{name: "empty args", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := startServiceName(tt.resolvedService, tt.resolvedProjectArg, tt.args)
			if got != tt.want {
				t.Fatalf("startServiceName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCanResolveProjectArg(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		filePath string
		args     []string
		want     bool
	}{
		{name: "arg without file", args: []string{testStartStopProjectName}, want: true},
		{name: "explicit file wins", filePath: "compose.yml", args: []string{testStartStopProjectName}},
		{name: "no args"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := canResolveProjectArg(tt.filePath, tt.args); got != tt.want {
				t.Fatalf("canResolveProjectArg(%q, %v) = %v, want %v", tt.filePath, tt.args, got, tt.want)
			}
		})
	}
}

func TestLimitProjectToService(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		service   string
		wantOrder []string
		wantErr   string
	}{
		{
			name:      "limits to requested service",
			service:   testStartStopDBService,
			wantOrder: []string{testStartStopDBService},
		},
		{
			name:      "empty service leaves project unchanged",
			wantOrder: []string{testStartStopWebService, testStartStopDBService},
		},
		{
			name:      "missing service reports project",
			service:   testStartStopAPIService,
			wantErr:   `service "api" not found in project "demo"`,
			wantOrder: []string{testStartStopWebService, testStartStopDBService},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			project := &compose.Project{
				Name:         testStartStopProjectName,
				ServiceOrder: []string{testStartStopWebService, testStartStopDBService},
				Services: map[string]config.Manifest{
					testStartStopWebService: {},
					testStartStopDBService:  {},
				},
			}
			err := limitProjectToService(project, tt.service)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("limitProjectToService() error = %v, want %q", err, tt.wantErr)
				}
			} else if err != nil {
				t.Fatalf("limitProjectToService() error = %v, want nil", err)
			}
			assertStringSliceEqual(t, "ServiceOrder", project.ServiceOrder, tt.wantOrder)
			if tt.service == testStartStopDBService {
				if _, ok := project.Services[testStartStopWebService]; ok {
					t.Fatalf("limitProjectToService kept %s service, want pruned", testStartStopWebService)
				}
				if _, ok := project.Services[testStartStopDBService]; !ok {
					t.Fatalf("limitProjectToService removed %s service, want kept", testStartStopDBService)
				}
			}
		})
	}
}

func TestServiceArgAfterProject(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "project only", args: []string{testStartStopProjectName}},
		{name: "project and service", args: []string{testStartStopProjectName, testStartStopWebService}, want: testStartStopWebService},
		{name: "empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := serviceArgAfterProject(tt.args); got != tt.want {
				t.Fatalf("serviceArgAfterProject(%v) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}
