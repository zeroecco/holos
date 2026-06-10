package main

import (
	"bytes"
	"testing"
)

func TestWriteProjectRemoved(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	writeProjectRemoved(&out, "demo")
	if got, want := out.String(), "project \"demo\" removed\n"; got != want {
		t.Fatalf("project removed output = %q, want %q", got, want)
	}
}
