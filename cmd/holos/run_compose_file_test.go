package main

import "testing"

func TestRunComposePaths(t *testing.T) {
	stateDir := "state/holos"
	projectName := "demo"

	if got, want := runComposeRootDir(stateDir), "state/holos/runs"; got != want {
		t.Fatalf("runComposeRootDir = %q, want %q", got, want)
	}
	if got, want := runComposeProjectDir(stateDir, projectName), "state/holos/runs/demo"; got != want {
		t.Fatalf("runComposeProjectDir = %q, want %q", got, want)
	}
	if got, want := runComposeFilePath(stateDir, projectName), "state/holos/runs/demo/holos.yaml"; got != want {
		t.Fatalf("runComposeFilePath = %q, want %q", got, want)
	}
	wantDirs := []string{
		stateDir,
		"state/holos/runs",
		"state/holos/runs/demo",
	}
	assertStringSliceEqual(t, "runComposeDirs", runComposeDirs(stateDir, projectName), wantDirs)
}
