package images

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

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
