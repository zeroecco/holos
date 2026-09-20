package images

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/zeroecco/holos/internal/config"
)

const testUbuntuCacheFeedPath = "/releases/streams/v1/com.ubuntu.cloud:released:download.json"

type testUbuntuCacheBuild struct {
	version string
	path    string
	payload string
	hash    string
}

func ubuntuCacheBuild(version, payload string) testUbuntuCacheBuild {
	return testUbuntuCacheBuild{
		version: version,
		path:    "server/releases/noble/release-" + version + "/ubuntu-24.04-server-cloudimg-amd64.img",
		payload: payload,
		hash:    fmt.Sprintf("%x", sha256.Sum256([]byte(payload))),
	}
}

type testUbuntuCacheMirror struct {
	server  *httptest.Server
	mu      sync.Mutex
	builds  map[string]testUbuntuCacheBuild
	calls   map[string]int
	offline bool
}

func newUbuntuCacheMirror(t *testing.T, builds ...testUbuntuCacheBuild) *testUbuntuCacheMirror {
	t.Helper()
	mirror := &testUbuntuCacheMirror{
		builds: make(map[string]testUbuntuCacheBuild),
		calls:  make(map[string]int),
	}
	for _, build := range builds {
		mirror.builds[build.version] = build
	}
	mirror.server = httptest.NewServer(http.HandlerFunc(mirror.serveHTTP))
	t.Cleanup(mirror.server.Close)
	return mirror
}

func (m *testUbuntuCacheMirror) serveHTTP(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls[r.URL.Path]++
	if r.URL.Path == testUbuntuCacheFeedPath {
		if m.offline {
			http.NotFound(w, r)
			return
		}
		versions := make(map[string]any, len(m.builds))
		for version, build := range m.builds {
			versions[version] = map[string]any{
				"label": "release",
				"items": map[string]any{
					"disk1.img": map[string]any{
						"ftype": "disk1.img", "path": build.path,
						"sha256": build.hash, "size": len(build.payload),
					},
				},
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"format": "products:1.0", "datatype": "image-downloads",
			"content_id": "com.ubuntu.cloud:released:download",
			"products": map[string]any{
				"com.ubuntu.cloud:server:24.04:amd64": map[string]any{
					"arch": "amd64", "os": "ubuntu", "release": "noble",
					"version": "24.04", "release_title": "24.04 LTS",
					"aliases": "noble,24.04", "versions": versions,
				},
			},
		})
		return
	}
	for _, build := range m.builds {
		if r.URL.Path == "/"+build.path {
			_, _ = w.Write([]byte(build.payload))
			return
		}
	}
	http.NotFound(w, r)
}

func (m *testUbuntuCacheMirror) publish(build testUbuntuCacheBuild) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.builds[build.version] = build
}

func (m *testUbuntuCacheMirror) removeFeed() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.offline = true
}

func (m *testUbuntuCacheMirror) requestCount(path string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls[path]
}

func (m *testUbuntuCacheMirror) totalRequests() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	var total int
	for _, count := range m.calls {
		total += count
	}
	return total
}

func registerUbuntuCacheImage(t *testing.T, baseURL string) Image {
	t.Helper()
	img := Image{
		Name: "ubuntu", Tag: "noble", URL: baseURL, UbuntuRelease: "noble",
		Format: config.ImageFormatQCOW2, Default: true, User: "ubuntu",
		OSFamily: config.ImageOSSystemd,
	}
	original := Registry
	Registry = []Image{img}
	t.Cleanup(func() { Registry = original })
	return img
}

func ubuntuResolvedCachePath(cacheDir string, img Image, build testUbuntuCacheBuild) string {
	img.URL = strings.TrimRight(img.URL, "/") + "/" + build.path
	img.SHA256 = build.hash
	return cachePath(cacheDir, &img)
}

func assertUbuntuCachedPayload(t *testing.T, path string, build testUbuntuCacheBuild) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read cached image: %v", err)
	}
	if string(got) != build.payload {
		t.Fatalf("cached image = %q, want %q", got, build.payload)
	}
}

func assertUbuntuVerification(t *testing.T, ref, cacheDir, path string, build testUbuntuCacheBuild) {
	t.Helper()
	got, err := Verify(ref, cacheDir)
	if err != nil {
		t.Fatalf("Verify(%q): %v", ref, err)
	}
	assertVerificationIdentity(t, got, testVerificationIdentityWant{
		ref: ref, path: path, format: config.ImageFormatQCOW2,
	})
	if !got.Verified || got.Skipped || got.Algorithm != hashAlgorithmSHA256 || got.Hash != build.hash {
		t.Fatalf("verification = %+v, want verified sha256 %s", got, build.hash)
	}
}

func TestUbuntuPullPinsVerifiedBuildOffline(t *testing.T) {
	for _, suffix := range []string{"", "/"} {
		t.Run("base URL suffix "+suffix, func(t *testing.T) {
			older := ubuntuCacheBuild("20260901", "earlier Ubuntu image")
			current := ubuntuCacheBuild("20260911", "current Ubuntu image")
			mirror := newUbuntuCacheMirror(t, older, current)
			img := registerUbuntuCacheImage(t, mirror.server.URL+suffix)
			cacheDir := t.TempDir()

			path, format, err := Pull("ubuntu", cacheDir)
			if err != nil {
				t.Fatalf("Pull: %v", err)
			}
			if want := ubuntuResolvedCachePath(cacheDir, img, current); path != want || format != config.ImageFormatQCOW2 {
				t.Fatalf("Pull = (%q, %q), want (%q, qcow2)", path, format, want)
			}
			assertUbuntuCachedPayload(t, path, current)
			assertUbuntuVerification(t, "ubuntu:noble", cacheDir, path, current)
			if Registry[0] != img {
				t.Fatalf("Pull mutated registry metadata: got %+v, want %+v", Registry[0], img)
			}
			if got := mirror.totalRequests(); got != 2 {
				t.Fatalf("initial requests = %d, want one feed and one image request", got)
			}

			mirror.publish(ubuntuCacheBuild("20260912", "newer Ubuntu image"))
			for _, feedMissing := range []bool{false, true} {
				if feedMissing {
					mirror.removeFeed()
				}
				got, _, err := Pull("ubuntu:noble", cacheDir)
				if err != nil || got != path {
					t.Fatalf("cached Pull with missing feed=%t = (%q, %v), want (%q, nil)", feedMissing, got, err, path)
				}
				assertUbuntuVerification(t, "ubuntu", cacheDir, path, current)
				if got := mirror.totalRequests(); got != 2 {
					t.Fatalf("cached Pull/Verify made additional network requests: %d", got)
				}
			}

			mirror.server.Close()
			if got, _, err := Pull("ubuntu", cacheDir); err != nil || got != path {
				t.Fatalf("offline Pull = (%q, %v), want (%q, nil)", got, err, path)
			}
			assertUbuntuVerification(t, "ubuntu", cacheDir, path, current)
		})
	}
}

func TestUbuntuPullChecksumMismatchDoesNotPin(t *testing.T) {
	bad := ubuntuCacheBuild("20260911", "authentic Ubuntu image")
	bad.payload = "corrupt download"
	mirror := newUbuntuCacheMirror(t, bad)
	img := registerUbuntuCacheImage(t, mirror.server.URL)
	cacheDir := t.TempDir()

	_, _, err := Pull("ubuntu", cacheDir)
	assertErrorContains(t, err, "sha256 mismatch")
	badPath := ubuntuResolvedCachePath(cacheDir, img, bad)
	if _, err := os.Stat(badPath); !os.IsNotExist(err) {
		t.Fatalf("failed download left a cached image: %v", err)
	}
	if err := filepath.WalkDir(cacheDir, func(path string, entry fs.DirEntry, err error) error {
		if err == nil && !entry.IsDir() {
			t.Errorf("failed download left cache state: %s", path)
		}
		return err
	}); err != nil {
		t.Fatalf("inspect cache after failed download: %v", err)
	}
	requests := mirror.totalRequests()
	if _, err := Verify("ubuntu", cacheDir); !os.IsNotExist(err) {
		t.Fatalf("Verify after failed download = %v, want not-exist error", err)
	}
	if got := mirror.totalRequests(); got != requests {
		t.Fatalf("Verify without a pin made network requests: %d -> %d", requests, got)
	}

	good := ubuntuCacheBuild("20260912", "good replacement image")
	mirror.publish(good)
	path, _, err := Pull("ubuntu", cacheDir)
	if err != nil {
		t.Fatalf("Pull after failed download: %v", err)
	}
	if want := ubuntuResolvedCachePath(cacheDir, img, good); path != want {
		t.Fatalf("Pull after failed download = %q, want newer build %q", path, want)
	}
	assertUbuntuCachedPayload(t, path, good)
	assertUbuntuVerification(t, "ubuntu", cacheDir, path, good)
	if got := mirror.requestCount(testUbuntuCacheFeedPath); got != 2 {
		t.Fatalf("feed requests = %d, want fresh resolution after failed download", got)
	}
}

func TestUbuntuVerifyDetectsTamperAndPullRepairsPinnedBuild(t *testing.T) {
	build := ubuntuCacheBuild("20260911", "verified Ubuntu image")
	mirror := newUbuntuCacheMirror(t, build)
	registerUbuntuCacheImage(t, mirror.server.URL)
	cacheDir := t.TempDir()
	path, _, err := Pull("ubuntu", cacheDir)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if err := os.WriteFile(path, []byte("tampered cached bytes"), 0o600); err != nil {
		t.Fatalf("tamper cached image: %v", err)
	}
	mirror.removeFeed()
	requests := mirror.totalRequests()
	_, err = Verify("ubuntu", cacheDir)
	assertErrorContains(t, err, "sha256 mismatch")
	if got := mirror.totalRequests(); got != requests {
		t.Fatalf("Verify made network requests: %d -> %d", requests, got)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "tampered cached bytes" {
		t.Fatalf("Verify changed corrupt image: got %q, error %v", got, err)
	}

	got, _, err := Pull("ubuntu", cacheDir)
	if err != nil || got != path {
		t.Fatalf("repair Pull = (%q, %v), want (%q, nil)", got, err, path)
	}
	assertUbuntuCachedPayload(t, path, build)
	assertUbuntuVerification(t, "ubuntu", cacheDir, path, build)
	if got := mirror.requestCount(testUbuntuCacheFeedPath); got != 1 {
		t.Fatalf("repair fetched the missing feed: %d requests", got)
	}
	if got := mirror.requestCount("/" + build.path); got != 2 {
		t.Fatalf("artifact requests = %d, want initial download and repair", got)
	}
}

func TestUbuntuMissingCacheResolvesNewBuildOnlyOnPull(t *testing.T) {
	build := ubuntuCacheBuild("20260911", "original image")
	mirror := newUbuntuCacheMirror(t, build)
	img := registerUbuntuCacheImage(t, mirror.server.URL)
	cacheDir := t.TempDir()
	if _, err := Verify("ubuntu", cacheDir); !os.IsNotExist(err) {
		t.Fatalf("Verify without cached metadata = %v, want not-exist error", err)
	}
	if got := mirror.totalRequests(); got != 0 {
		t.Fatalf("Verify without cached metadata made %d requests", got)
	}
	path, _, err := Pull("ubuntu", cacheDir)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove cached image: %v", err)
	}
	newer := ubuntuCacheBuild("20260912", "new build after cache removal")
	mirror.publish(newer)
	requests := mirror.totalRequests()
	if _, err := Verify("ubuntu", cacheDir); !os.IsNotExist(err) {
		t.Fatalf("Verify with missing image = %v, want not-exist error", err)
	}
	if got := mirror.totalRequests(); got != requests {
		t.Fatalf("Verify with missing image made network requests: %d -> %d", requests, got)
	}
	newPath, _, err := Pull("ubuntu", cacheDir)
	if err != nil {
		t.Fatalf("Pull after removing cached image: %v", err)
	}
	if want := ubuntuResolvedCachePath(cacheDir, img, newer); newPath != want || newPath == path {
		t.Fatalf("new build path = %q, want %q distinct from %q", newPath, want, path)
	}
	assertUbuntuCachedPayload(t, newPath, newer)
	assertUbuntuVerification(t, "ubuntu", cacheDir, newPath, newer)
	if got := mirror.requestCount(testUbuntuCacheFeedPath); got != 2 {
		t.Fatalf("feed requests = %d, want resolution after cache removal", got)
	}
}

func TestUbuntuAliasesRetainIndependentPinnedBuilds(t *testing.T) {
	older := ubuntuCacheBuild("20260911", "noble alias image")
	mirror := newUbuntuCacheMirror(t, older)
	img := registerUbuntuCacheImage(t, mirror.server.URL)
	alias := img
	alias.Tag = "24.04"
	alias.Default = false
	Registry = append(Registry, alias)
	cacheDir := t.TempDir()
	oldPath, _, err := Pull("ubuntu:noble", cacheDir)
	if err != nil {
		t.Fatalf("Pull(noble): %v", err)
	}
	newer := ubuntuCacheBuild("20260912", "numeric alias image")
	mirror.publish(newer)
	newPath, _, err := Pull("ubuntu:24.04", cacheDir)
	if err != nil {
		t.Fatalf("Pull(24.04): %v", err)
	}
	if oldPath == newPath {
		t.Fatalf("different pinned builds share cache path %q", oldPath)
	}
	mirror.removeFeed()
	assertUbuntuCachedPayload(t, oldPath, older)
	assertUbuntuCachedPayload(t, newPath, newer)
	assertUbuntuVerification(t, "ubuntu:noble", cacheDir, oldPath, older)
	assertUbuntuVerification(t, "ubuntu:24.04", cacheDir, newPath, newer)
	if Registry[0] != img || Registry[1] != alias {
		t.Fatalf("resolving aliases mutated registry metadata: %+v", Registry)
	}
	if got := mirror.totalRequests(); got != 4 {
		t.Fatalf("requests = %d, want feed and image per uncached alias", got)
	}
}
