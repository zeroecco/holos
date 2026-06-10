package cloudinit

import (
	"testing"

	"github.com/zeroecco/holos/internal/config"
)

func assertCCFileMetadata(t *testing.T, name string, got ccFile, path, permissions, owner string) {
	t.Helper()

	if got.Path != path ||
		got.Permissions != permissions ||
		got.Owner != owner {
		t.Fatalf("%s metadata = %+v, want path %q perms %q owner %q", name, got, path, permissions, owner)
	}
}

func TestHostname(t *testing.T) {
	t.Parallel()

	if got := hostname(config.Manifest{}, "api-0"); got != "api-0" {
		t.Fatalf("hostname fallback = %q", got)
	}
	manifest := config.Manifest{CloudInit: config.CloudInit{Hostname: "api.internal"}}
	if got := hostname(manifest, "api-0"); got != "api.internal" {
		t.Fatalf("hostname override = %q", got)
	}
}

func TestHostsFileContent(t *testing.T) {
	t.Parallel()

	manifest := config.Manifest{
		ExtraHosts: map[string]string{
			"web":   "10.10.0.2",
			"web-0": "10.10.0.2",
			"db":    "10.10.0.3",
			"db-0":  "10.10.0.3",
		},
	}
	want := "127.0.0.1 localhost\n" +
		"127.0.1.1 web-0\n" +
		"::1 localhost ip6-localhost ip6-loopback\n" +
		"ff02::1 ip6-allnodes\n" +
		"ff02::2 ip6-allrouters\n\n" +
		"10.10.0.2 web web-0\n" +
		"10.10.0.3 db db-0\n"

	if got := hostsFileContent(manifest, "web-0"); got != want {
		t.Fatalf("hostsFileContent =\n%q\nwant\n%q", got, want)
	}
}

func TestHostsWriteFile(t *testing.T) {
	t.Parallel()

	manifest := config.Manifest{
		ExtraHosts: map[string]string{
			"web": "10.10.0.2",
		},
	}
	wf, ok := hostsWriteFile(manifest, "web-0")
	if !ok {
		t.Fatal("hostsWriteFile ok = false, want true")
	}
	assertCCFileMetadata(t, "hosts", wf, hostsFilePath, config.DefaultFilePermissions, config.DefaultFileOwner)
	assertContains(t, wf.Content, "10.10.0.2 web\n")
}

func TestHostsWriteFileSkipsEmptyExtraHosts(t *testing.T) {
	t.Parallel()

	if wf, ok := hostsWriteFile(config.Manifest{}, "web-0"); ok || wf != (ccFile{}) {
		t.Fatalf("hostsWriteFile empty = %+v, %v; want zero false", wf, ok)
	}
}
