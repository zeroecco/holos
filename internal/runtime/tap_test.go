package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zeroecco/holos/internal/config"
	"github.com/zeroecco/holos/internal/qemu"
)

func TestManagedTapAttachments(t *testing.T) {
	t.Parallel()

	manifest := config.Manifest{InternalNetwork: &config.InternalNetworkConfig{
		Backend:    "tap",
		BridgeName: "br0",
		Segments: []config.InternalNetworkSegment{
			{Name: "socket"},
			{Name: "lan", Backend: "tap", BridgeName: "br1"},
		},
	}}

	taps := managedTapAttachments("demo", manifest, "web-0")
	if len(taps) != 2 {
		t.Fatalf("taps len = %d, want 2: %#v", len(taps), taps)
	}
	if taps[0].NetdevID != "net1" || taps[0].Bridge != "br0" {
		t.Fatalf("primary tap = %#v, want net1 br0", taps[0])
	}
	if taps[1].NetdevID != "net3" || taps[1].Bridge != "br1" {
		t.Fatalf("segment tap = %#v, want net3 br1", taps[1])
	}
	for _, tap := range taps {
		if !strings.HasPrefix(tap.IfName, tapNamePrefix) || len(tap.IfName) > 15 {
			t.Fatalf("tap ifname = %q, want %s* <= 15 chars", tap.IfName, tapNamePrefix)
		}
	}
}

func TestPrepareManagedTapsRunsIPCommands(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "ip.log")
	ip := filepath.Join(dir, "ip")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + shellQuote(logPath) + "\n"
	if err := os.WriteFile(ip, []byte(script), 0o755); err != nil {
		t.Fatalf("write ip mock: %v", err)
	}
	t.Setenv(ipEnv, ip)

	manager := NewManager(dir)
	manifest := config.Manifest{InternalNetwork: &config.InternalNetworkConfig{
		Backend:    "tap",
		BridgeName: "br0",
	}}
	spec := qemu.LaunchSpec{Name: "web-0", Index: 0}

	taps, err := manager.prepareManagedTaps("demo", manifest, &spec)
	if err != nil {
		t.Fatalf("prepareManagedTaps: %v", err)
	}
	if len(taps) != 1 {
		t.Fatalf("taps len = %d, want 1", len(taps))
	}
	if got := spec.TapIfNames["net1"]; got != taps[0].IfName {
		t.Fatalf("spec tap net1 = %q, want %q", got, taps[0].IfName)
	}
	log := readTestFile(t, logPath)
	assertTapLogContains(t, log, "tuntap add dev "+taps[0].IfName+" mode tap")
	assertTapLogContains(t, log, "link set "+taps[0].IfName+" master br0")
	assertTapLogContains(t, log, "link set "+taps[0].IfName+" up")

	manager.cleanupInstanceTaps(InstanceRecord{TapIfNames: map[string]string{"net1": taps[0].IfName}})
	log = readTestFile(t, logPath)
	assertTapLogContains(t, log, "link del "+taps[0].IfName)
}

func shellQuote(path string) string {
	return "'" + strings.ReplaceAll(path, "'", "'\\''") + "'"
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func assertTapLogContains(t *testing.T, got string, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Fatalf("tap log missing %q in:\n%s", want, got)
	}
}
