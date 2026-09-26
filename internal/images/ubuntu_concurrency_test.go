package images

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestUbuntuConcurrentPullReusesWinningSelection(t *testing.T) {
	for _, separateProcess := range []bool{false, true} {
		t.Run(fmt.Sprintf("separate process=%t", separateProcess), func(t *testing.T) {
			older := ubuntuCacheBuild("20260911", "older Ubuntu image")
			newer := ubuntuCacheBuild("20260912", "newer Ubuntu image")
			mirror := &testUbuntuCacheMirror{
				builds: map[string]testUbuntuCacheBuild{older.version: older},
				calls:  make(map[string]int),
			}
			started := make(chan struct{})
			release := make(chan struct{})
			var releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/"+older.path {
					close(started)
					<-release
				}
				mirror.serveHTTP(w, r)
			}))
			t.Cleanup(server.Close)
			t.Cleanup(unblock)
			img := registerUbuntuCacheImage(t, server.URL)
			cacheDir := t.TempDir()
			delayed := startUbuntuConcurrentPull(t, server.URL, cacheDir, separateProcess)
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("older pull did not start downloading")
			}

			// Publish and pin a newer build while the older download is pending.
			mirror.publish(newer)
			winner, format, err := Pull("ubuntu", cacheDir)
			if err != nil {
				t.Fatalf("newer Pull: %v", err)
			}
			if want := ubuntuResolvedCachePath(cacheDir, img, newer); winner != want {
				t.Fatalf("newer Pull = %q, want %q", winner, want)
			}
			assertUbuntuVerification(t, "ubuntu", cacheDir, winner, newer)
			unblock()
			select {
			case result := <-delayed:
				if result.err != nil {
					t.Fatalf("older Pull: %v", result.err)
				}
				if result.path != winner || result.format != format {
					t.Errorf("older Pull = (%q, %q), want winning selection (%q, %q)", result.path, result.format, winner, format)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("older pull did not finish")
			}
			assertUbuntuVerification(t, "ubuntu", cacheDir, winner, newer)
			oldPath := ubuntuResolvedCachePath(cacheDir, img, older)
			assertUbuntuCachedPayload(t, oldPath, older)
			assertUbuntuCachedPayload(t, winner, newer)
			assertNoPartialFiles(t, oldPath)
			assertNoPartialFiles(t, winner)
		})
	}
}

type ubuntuConcurrentPullResult struct {
	path   string
	format string
	err    error
}

func startUbuntuConcurrentPull(t *testing.T, baseURL, cacheDir string, separateProcess bool) <-chan ubuntuConcurrentPullResult {
	t.Helper()
	done := make(chan ubuntuConcurrentPullResult, 1)
	if !separateProcess {
		go func() {
			path, format, err := Pull("ubuntu", cacheDir)
			done <- ubuntuConcurrentPullResult{path: path, format: format, err: err}
		}()
		return done
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	resultPath := filepath.Join(t.TempDir(), "pull-result.json")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, executable, "-test.run=^TestUbuntuConcurrentPullProcess$")
	cmd.Env = append(os.Environ(), "HOLOS_TEST_UBUNTU_PULL_URL="+baseURL,
		"HOLOS_TEST_UBUNTU_PULL_CACHE="+cacheDir, "HOLOS_TEST_UBUNTU_PULL_RESULT="+resultPath)
	go func() {
		output, err := cmd.CombinedOutput()
		if err != nil {
			done <- ubuntuConcurrentPullResult{err: fmt.Errorf("subprocess: %w\n%s", err, output)}
			return
		}
		data, err := os.ReadFile(resultPath)
		var result struct{ Path, Format string }
		if err == nil {
			err = json.Unmarshal(data, &result)
		}
		done <- ubuntuConcurrentPullResult{path: result.Path, format: result.Format, err: err}
	}()
	return done
}

func TestUbuntuConcurrentPullProcess(t *testing.T) {
	baseURL := os.Getenv("HOLOS_TEST_UBUNTU_PULL_URL")
	if baseURL == "" {
		return
	}
	registerUbuntuCacheImage(t, baseURL)
	path, format, err := Pull("ubuntu", os.Getenv("HOLOS_TEST_UBUNTU_PULL_CACHE"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(struct{ Path, Format string }{Path: path, Format: format})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(os.Getenv("HOLOS_TEST_UBUNTU_PULL_RESULT"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestUbuntuConcurrentFailedPullPreservesVerifiedArtifact(t *testing.T) {
	for _, failure := range []string{"HTTP error", "checksum mismatch"} {
		t.Run(failure, func(t *testing.T) {
			build := ubuntuCacheBuild(testUbuntuSerial, "verified Ubuntu image")
			stream := ubuntuTestStream()
			item := stream.Products[testUbuntuProductID].Versions[testUbuntuSerial].Items["disk1.img"]
			item.SHA256 = build.hash
			stream.Products[testUbuntuProductID].Versions[testUbuntuSerial].Items["disk1.img"] = item

			started := make(chan struct{})
			release := make(chan struct{})
			var releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			var artifactRequests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/" + ubuntuStreamPath:
					_ = json.NewEncoder(w).Encode(stream)
				case "/" + build.path:
					if artifactRequests.Add(1) == 1 {
						close(started)
						<-release
						if failure == "HTTP error" {
							http.Error(w, "mirror unavailable", http.StatusServiceUnavailable)
						} else {
							_, _ = w.Write([]byte("corrupt image"))
						}
						return
					}
					_, _ = w.Write([]byte(build.payload))
				default:
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(server.Close)
			t.Cleanup(unblock)
			registerUbuntuCacheImage(t, server.URL)
			cacheDir := t.TempDir()
			failed := make(chan error, 1)
			go func() {
				_, _, err := Pull("ubuntu", cacheDir)
				failed <- err
			}()

			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("first pull did not start downloading")
			}
			// Finish another pull before allowing the first request to fail.
			path, _, err := Pull("ubuntu", cacheDir)
			if err != nil {
				t.Fatalf("concurrent successful Pull: %v", err)
			}
			assertUbuntuVerification(t, "ubuntu", cacheDir, path, build)
			unblock()
			select {
			case err := <-failed:
				if err == nil {
					t.Fatal("first pull unexpectedly succeeded")
				}
			case <-time.After(5 * time.Second):
				t.Fatal("failed pull did not finish")
			}
			assertUbuntuCachedPayload(t, path, build)
			assertUbuntuVerification(t, "ubuntu", cacheDir, path, build)
			assertNoPartialFiles(t, path)
		})
	}
}
