package compose

import (
	"testing"

	"github.com/zeroecco/holos/internal/config"
)

func TestServiceReplicasPrecedence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		svc  Service
		want int
	}{
		{name: "default", want: config.DefaultReplicas},
		{name: "replicas", svc: Service{Replicas: 2}, want: 2},
		{name: "scale", svc: Service{Scale: "3"}, want: 3},
		{name: "deploy replicas", svc: Service{Deploy: Deploy{Replicas: 4}}, want: 4},
		{name: "matching values", svc: Service{Replicas: 5, Scale: int64(5), Deploy: Deploy{Replicas: 5}}, want: 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := serviceReplicas(tt.svc)
			if err != nil {
				t.Fatalf("serviceReplicas: %v", err)
			}
			if got != tt.want {
				t.Fatalf("serviceReplicas = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestServiceReplicasRejectsDisagreement(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		svc     Service
		wantErr string
	}{
		{name: "replicas scale", svc: Service{Replicas: 2, Scale: 3}, wantErr: "replicas and scale disagree"},
		{name: "replicas deploy", svc: Service{Replicas: 2, Deploy: Deploy{Replicas: 3}}, wantErr: "replicas and deploy.replicas disagree"},
		{name: "scale deploy", svc: Service{Scale: 2, Deploy: Deploy{Replicas: 3}}, wantErr: "scale and deploy.replicas disagree"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := serviceReplicas(tt.svc)
			assertErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestValidateReplicaCount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		replicas int
		wantErr  string
	}{
		{name: "valid", replicas: 1},
		{name: "too low", replicas: 0, wantErr: "replicas must be >= 1"},
		{name: "too high", replicas: maxReplicas + 1, wantErr: "exceeds maximum"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateReplicaCount(tt.replicas)
			if tt.wantErr != "" {
				assertErrorContains(t, err, tt.wantErr)
				return
			}
			if err != nil {
				t.Fatalf("validateReplicaCount: %v", err)
			}
		})
	}
}

func TestProjectInstanceCount(t *testing.T) {
	t.Parallel()

	services := map[string]Service{
		"api": {Replicas: 2},
		"web": {Scale: "3"},
	}
	got, err := projectInstanceCount(services, []string{"api", "web"})
	if err != nil {
		t.Fatalf("projectInstanceCount: %v", err)
	}
	if got != 5 {
		t.Fatalf("projectInstanceCount = %d, want 5", got)
	}

	_, err = projectInstanceCount(map[string]Service{
		"bad": {Replicas: -1},
	}, []string{"bad"})
	assertErrorContains(t, err, `service "bad":`)
}

func TestValidateProjectInstanceCapacity(t *testing.T) {
	t.Parallel()

	if err := validateProjectInstanceCapacity(maxProjectInstances); err != nil {
		t.Fatalf("validateProjectInstanceCapacity at limit: %v", err)
	}
	assertErrorContains(t, validateProjectInstanceCapacity(maxProjectInstances+1), "internal network")
}
