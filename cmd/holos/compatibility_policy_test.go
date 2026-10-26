package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStrictRejectsIgnoredFieldsBeforeImageResolution(t *testing.T) {
	path := filepath.Join(t.TempDir(), "holos.yaml")
	if err := os.WriteFile(path, []byte("name: demo\nservices:\n  web:\n    image: ./does-not-exist.qcow2\n    restart: always\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, run := range []func([]string) error{runValidate, runUp} {
		err := run([]string{"--strict", "-f", path, "--state-dir", t.TempDir()})
		if err == nil || !strings.Contains(err.Error(), "services.web.restart") || !strings.Contains(err.Error(), "strict mode") {
			t.Fatalf("expected compatibility error before image resolution, got %v", err)
		}
	}
}
