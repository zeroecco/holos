package main

import (
	"testing"
)

func TestAddProjectFlags(t *testing.T) {
	t.Setenv("HOLOS_STATE_DIR", "/tmp/holos-test-state")
	flags := newFlagSet("test")
	projectFlags := addProjectFlags(flags, "")

	fileFlag := flags.Lookup("f")
	if fileFlag == nil {
		t.Fatal("missing -f flag")
	}
	if fileFlag.Usage != "path to holos.yaml" {
		t.Fatalf("-f usage = %q, want %q", fileFlag.Usage, "path to holos.yaml")
	}
	if *projectFlags.filePath != "" {
		t.Fatalf("filePath default = %q, want empty", *projectFlags.filePath)
	}

	stateFlag := flags.Lookup("state-dir")
	if stateFlag == nil {
		t.Fatal("missing -state-dir flag")
	}
	if stateFlag.Usage != "state directory" {
		t.Fatalf("-state-dir usage = %q, want %q", stateFlag.Usage, "state directory")
	}
	if *projectFlags.stateDir != "/tmp/holos-test-state" {
		t.Fatalf("stateDir default = %q, want %q", *projectFlags.stateDir, "/tmp/holos-test-state")
	}
}

func TestAddProjectFlagsCustomFileUsage(t *testing.T) {
	t.Parallel()

	const customUsage = "custom compose path"
	flags := newFlagSet("test")
	addProjectFlags(flags, customUsage)

	if got := flags.Lookup("f").Usage; got != customUsage {
		t.Fatalf("-f usage = %q, want %q", got, customUsage)
	}
}
