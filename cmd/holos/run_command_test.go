package main

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/zeroecco/holos/internal/compose"
)

func assertContains(t *testing.T, got, want string) {
	t.Helper()

	if !strings.Contains(got, want) {
		t.Fatalf("expected content to contain %q, got:\n%s", want, got)
	}
}

func assertErrorContains(t *testing.T, err error, want string) {
	t.Helper()

	if err == nil {
		t.Fatalf("expected error containing %q, got nil", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %v, want substring %q", err, want)
	}
}

func assertErrorOmits(t *testing.T, err error, forbidden string) {
	t.Helper()

	if err == nil {
		t.Fatalf("expected error omitting %q, got nil", forbidden)
	}
	if strings.Contains(err.Error(), forbidden) {
		t.Fatalf("error = %v, want to omit substring %q", err, forbidden)
	}
}

func assertHexLength(t *testing.T, name, got string, wantBytes int) {
	t.Helper()

	wantLen := 2 * wantBytes
	if len(got) != wantLen {
		t.Errorf("%s(%d) = %q (len %d), want length %d", name, wantBytes, got, len(got), wantLen)
	}
	for _, c := range got {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("%s(%d) = %q contains non-hex rune %q", name, wantBytes, got, c)
		}
	}
}

func assertStringSliceEqual(t *testing.T, name string, got, want []string) {
	t.Helper()

	if !slices.Equal(got, want) {
		t.Fatalf("%s = %v, want %v", name, got, want)
	}
}

func composePortShorts(ports []compose.ComposePort) []string {
	out := make([]string, 0, len(ports))
	for _, port := range ports {
		out = append(out, port.Short)
	}
	return out
}

func composeVolumeShorts(volumes []compose.ComposeVolume) []string {
	out := make([]string, 0, len(volumes))
	for _, volume := range volumes {
		out = append(out, volume.Short)
	}
	return out
}

func composeDevicePCIs(devices []compose.ComposeDevice) []string {
	out := make([]string, 0, len(devices))
	for _, device := range devices {
		out = append(out, device.PCI)
	}
	return out
}

func TestGenerateRunName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		image      string
		wantPrefix string
	}{
		{"ubuntu:noble", "ubuntu-noble-"},
		{"alpine", "alpine-"},
		{"./images/my-image.qcow2", "my-image-"},
		{"/var/lib/libvirt/images/web.raw", "web-"},
		{"", "dockerfile-"},
		{"REGISTRY/Foo_Bar:1.0", "foo-bar-"},
		{"!!!", runNameFallbackBase + runNameSeparator},
	}
	for _, c := range cases {
		got := generateRunName(c.image)
		if !strings.HasPrefix(got, c.wantPrefix) {
			t.Errorf("generateRunName(%q) = %q, want prefix %q", c.image, got, c.wantPrefix)
		}
		if !runNamePattern.MatchString(got) {
			t.Errorf("generateRunName(%q) = %q, fails compose name validation", c.image, got)
		}
		if len(got) > maxRunNameLength {
			t.Errorf("generateRunName(%q) = %q (len %d), exceeds 63-char limit",
				c.image, got, len(got))
		}
	}
}

func TestRunNameBase(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want string
	}{
		{in: "ubuntu:noble", want: "ubuntu-noble"},
		{in: "./images/my-image.qcow2", want: "my-image"},
		{in: "REGISTRY/Foo_Bar:1.0", want: "foo-bar-1"},
		{in: ".profile", want: "profile"},
		{in: "image", want: "image"},
		{in: "!!!", want: runNameFallbackBase},
		{in: strings.Repeat("a", 200), want: strings.Repeat("a", maxRunNameLength-runNameSuffixLength)},
		{in: strings.Repeat("a", maxRunNameLength-runNameSuffixLength-1) + "-tail", want: strings.Repeat("a", maxRunNameLength-runNameSuffixLength-1)},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()

			if got := runNameBase(tt.in); got != tt.want {
				t.Fatalf("runNameBase(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestGenerateRunNameUnique(t *testing.T) {
	t.Parallel()

	// Repeated invocations on the same image should produce distinct
	// names (random suffix). We're not asserting a strong uniqueness
	// guarantee here, just that the suffix isn't a constant.
	seen := make(map[string]bool)
	for i := 0; i < 16; i++ {
		seen[generateRunName("alpine")] = true
	}
	if len(seen) < 8 {
		t.Errorf("expected diverse run names across 16 calls, got only %d unique", len(seen))
	}
}

func TestGenerateRunNameLongImage(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("a", 200)
	got := generateRunName(long)
	if len(got) > maxRunNameLength {
		t.Fatalf("generateRunName(<200 a's>) = %q (len %d), exceeds 63-char limit", got, len(got))
	}
	if !runNamePattern.MatchString(got) {
		t.Errorf("generateRunName(<200 a's>) = %q, fails compose name validation", got)
	}
}

// TestRandHexLengthContract pins randHex's documented "exactly 2*n
// chars" promise. generateRunName depends on it: the suffix must be
// exactly 6 chars to keep names within DNS's 63-char label limit.
// This covers the success path; the fallback path is verified by
// TestGenerateRunNameLongImageFallback below using a stub that
// exercises the post-failure branch directly.
func TestRandHexLengthContract(t *testing.T) {
	t.Parallel()

	for _, n := range []int{1, 3, 8, 16, 32} {
		got := randHex(n)
		assertHexLength(t, "randHex", got, n)
	}
}

// TestRandHexFallbackLengthContract directly exercises the branch
// that used to return strconv.FormatInt(pid, 16): variable-length
// and silently capable of blowing the 63-char DNS label limit when
// combined with a long image name in generateRunName. The current
// implementation must return exactly 2*n chars, all valid hex.
func TestRandHexFallbackLengthContract(t *testing.T) {
	t.Parallel()

	for _, n := range []int{1, 3, 8, 16, 32} {
		got := randHexFallback(n)
		assertHexLength(t, "randHexFallback", got, n)
	}

	// Asking for more bytes than sha256 gives us must be safely
	// capped, not panic. The result is shorter than 2*n in this
	// degenerate case, but the function shouldn't crash; callers
	// in this codebase only ever request n <= 8.
	if got := randHexFallback(64); len(got) != 2*sha256.Size {
		t.Errorf("randHexFallback(64) = %q (len %d), want %d (capped to sha256.Size)",
			got, len(got), 2*sha256.Size)
	}
}

func TestWriteRunComposeFilePermissions(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	projectName := "alpine-test"
	composePath, err := writeRunComposeFile(stateDir, projectName, compose.File{
		Name: projectName,
		Services: map[string]compose.Service{
			"vm": {Image: "alpine"},
		},
	})
	if err != nil {
		t.Fatalf("writeRunComposeFile failed: %v", err)
	}

	runDir := filepath.Dir(composePath)
	dirInfo, err := os.Stat(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if got := dirInfo.Mode().Perm(); got != runComposeDirPerm {
		t.Fatalf("run dir mode = %o, want %o", got, runComposeDirPerm)
	}
	runsInfo, err := os.Stat(filepath.Dir(runDir))
	if err != nil {
		t.Fatal(err)
	}
	if got := runsInfo.Mode().Perm(); got != runComposeDirPerm {
		t.Fatalf("runs root mode = %o, want %o", got, runComposeDirPerm)
	}
	stateInfo, err := os.Stat(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if got := stateInfo.Mode().Perm(); got != runComposeDirPerm {
		t.Fatalf("state dir mode = %o, want %o", got, runComposeDirPerm)
	}

	fileInfo, err := os.Stat(composePath)
	if err != nil {
		t.Fatal(err)
	}
	if got := fileInfo.Mode().Perm(); got != runComposeFilePerm {
		t.Fatalf("compose file mode = %o, want %o", got, runComposeFilePerm)
	}
}

func TestWriteRunComposeFilePreservesShortPorts(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	projectName := "port-test"
	composePath, err := writeRunComposeFile(stateDir, projectName, compose.File{
		Name: projectName,
		Services: map[string]compose.Service{
			"vm": {
				Image: "alpine",
				Ports: []compose.ComposePort{
					{Short: "127.0.0.1:8080:80"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("writeRunComposeFile failed: %v", err)
	}
	body, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("read compose: %v", err)
	}
	assertContains(t, string(body), "- 127.0.0.1:8080:80")
}

func TestNewRunRequestBuildsComposeService(t *testing.T) {
	t.Parallel()

	req, err := newRunRequest([]string{"alpine", "--", "echo", "hello"}, runOptions{
		name:     "test-run",
		memory:   "1G",
		ports:    []string{"127.0.0.1:8080:80"},
		volumes:  []string{"/tmp:/data:ro"},
		devices:  []string{"0000:01:00.0"},
		packages: []string{"curl"},
	})
	if err != nil {
		t.Fatalf("newRunRequest failed: %v", err)
	}

	if req.projectName != "test-run" {
		t.Fatalf("project name = %q, want test-run", req.projectName)
	}
	svc := req.file.Services[runServiceName]
	if svc.Image != "alpine" {
		t.Fatalf("image = %q, want alpine", svc.Image)
	}
	if svc.VM.MemoryMB != 1024 {
		t.Fatalf("memory = %d MB, want 1024", svc.VM.MemoryMB)
	}
	if !svc.VM.UEFI {
		t.Fatal("UEFI should be enabled when a device is attached")
	}
	assertStringSliceEqual(t, "run command", svc.CloudInit.RunCmd, []string{"echo hello"})
	assertStringSliceEqual(t, "ports", composePortShorts(svc.Ports), []string{"127.0.0.1:8080:80"})
	assertStringSliceEqual(t, "volumes", composeVolumeShorts(svc.Volumes), []string{"/tmp:/data:ro"})
	assertStringSliceEqual(t, "devices", composeDevicePCIs(svc.Devices), []string{"0000:01:00.0"})
	assertStringSliceEqual(t, "packages", svc.CloudInit.Packages, []string{"curl"})
}

func TestParseRunCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		args       []string
		dockerfile string
		runCmd     []string
		wantImage  string
		wantCmd    []string
		wantErr    string
	}{
		{name: "image only", args: []string{"alpine"}, wantImage: "alpine"},
		{name: "trailing command", args: []string{"alpine", "echo", "hello world"}, wantImage: "alpine", wantCmd: []string{"echo hello world"}},
		{name: "separator", args: []string{"alpine", runCommandSeparator, "echo", "hello"}, wantImage: "alpine", wantCmd: []string{"echo hello"}},
		{name: "leading separator", args: []string{runCommandSeparator, "echo", "hello"}, wantImage: "echo", wantCmd: []string{"hello"}},
		{name: "preserves configured run commands", args: []string{"alpine", "echo", "hello"}, runCmd: []string{"echo setup"}, wantImage: "alpine", wantCmd: []string{"echo setup", "echo hello"}},
		{name: "dockerfile without image", dockerfile: "./Dockerfile", runCmd: []string{"echo setup"}, wantCmd: []string{"echo setup"}},
		{name: "missing image", wantErr: runMissingImageErrorMsg},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotImage, gotCmd, err := parseRunCommand(tt.args, tt.dockerfile, tt.runCmd)
			if tt.wantErr != "" {
				assertErrorContains(t, err, tt.wantErr)
				return
			}
			if err != nil {
				t.Fatalf("parseRunCommand: %v", err)
			}
			if gotImage != tt.wantImage {
				t.Fatalf("image = %q, want %q", gotImage, tt.wantImage)
			}
			assertStringSliceEqual(t, "runCmd", gotCmd, tt.wantCmd)
		})
	}
}

func TestNewRunRequestRequiresImageWithoutDockerfile(t *testing.T) {
	t.Parallel()

	_, err := newRunRequest(nil, runOptions{name: "test-run"})
	assertErrorContains(t, err, "run requires an image")
}

func TestNewRunRequestAllowsMissingImageWithDockerfile(t *testing.T) {
	t.Parallel()

	request, err := newRunRequest(nil, runOptions{name: "test-run", dockerfile: "./Dockerfile", user: "ubuntu"})
	if err != nil {
		t.Fatalf("newRunRequest with Dockerfile and no image error = %v", err)
	}
	svc := request.file.Services[runServiceName]
	if svc.Image != "" {
		t.Fatalf("service image = %q, want empty", svc.Image)
	}
	if svc.Dockerfile != "./Dockerfile" {
		t.Fatalf("service Dockerfile = %q, want ./Dockerfile", svc.Dockerfile)
	}
}

func TestRunRejectsInvalidUserBeforeWritingCompose(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	err := runRun([]string{"--state-dir", stateDir, "--user", "bad user", "alpine"})
	assertErrorContains(t, err, "--user")
	if _, statErr := os.Stat(runComposeRootDir(stateDir)); !os.IsNotExist(statErr) {
		t.Fatalf("runs dir should not be written on invalid --user, stat err=%v", statErr)
	}
}
