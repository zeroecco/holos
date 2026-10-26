package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zeroecco/holos/internal/runtime"
)

func TestServiceShorthandUsesSavedStateWithoutImageResolution(t *testing.T) {
	stateDir := t.TempDir()
	composePath := filepath.Join(t.TempDir(), "holos.yaml")
	// The missing base image must not prevent access to an existing VM.
	if err := os.WriteFile(composePath, []byte("name: demo\nservices:\n  web:\n    image: ./missing.qcow2\n"), 0600); err != nil {
		t.Fatal(err)
	}
	record := testProjectRecord("demo", testServiceRecordWithLoginUser("web", "alpine", testInstanceRecord("web-0", "")))
	if err := writeProjectRecord(stateDir, record); err != nil {
		t.Fatal(err)
	}
	target, err := resolveInstanceTarget(runtime.NewManager(stateDir), composePath, stateDir, []string{"web", "uname", "-a"})
	if err != nil {
		t.Fatal(err)
	}
	if target.Inst.Name != "web-0" || target.LoginUser != "alpine" || len(target.CmdArgs) != 2 {
		t.Fatalf("target: %+v", target)
	}
	target, err = resolveInstanceTarget(runtime.NewManager(stateDir), "", stateDir, []string{"demo", "web", "uname"})
	if err != nil || target.Inst.Name != "web-0" {
		t.Fatalf("project shorthand: %+v, %v", target, err)
	}
	record.Services[0].Instances = append(record.Services[0].Instances, testInstanceRecord("web-1", ""))
	if err := writeProjectRecord(stateDir, record); err != nil {
		t.Fatal(err)
	}
	_, err = resolveInstanceTarget(runtime.NewManager(stateDir), composePath, stateDir, []string{"web"})
	assertErrorMentionsAll(t, err, "specify an instance", "web-0", "web-1")
}

func TestCommandHelpSucceeds(t *testing.T) {
	for _, name := range commandOrder {
		args := []string{"help", name}
		if err := run(args); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	if err := run([]string{"help", "not-a-command"}); err == nil {
		t.Fatal("unknown command accepted")
	}
}
