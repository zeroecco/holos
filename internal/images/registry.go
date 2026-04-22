package images

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

// bodyIdleTimeout bounds how long a download may go without receiving
// any bytes. The Transport only covers connect/TLS/header phases, so
// once headers arrive a silent mirror can keep the TCP stream open
// indefinitely while `holos pull` hangs. 60 seconds is roomy enough
// for a legitimate slow peer but short enough that an operator on a
// broken mirror notices and Ctrl-Cs within a minute.
var bodyIdleTimeout = 60 * time.Second

// tempFileFactory is the indirection that lets tests replace the
// partial-file sink so they can exercise failure modes (e.g. Close
// returning ENOSPC) without needing a real quota-capped filesystem.
// Production always uses os.Create.
var tempFileFactory = func(name string) (io.WriteCloser, error) {
	return os.Create(name)
}

// httpClient is the package-wide client used for image downloads.
// We avoid a total Client.Timeout because cloud images can legitimately
// take a long time to transfer over slow home links (the Debian
// generic qcow2 is ~400 MB). Instead we set per-phase timeouts on the
// Transport so a stalled DNS lookup, TCP connect, TLS handshake, or
// response-header wait cannot hang `holos pull` or `holos up`
// indefinitely. An idle-connection read stall after headers is still
// possible; for that, responsive cancellation comes from the caller's
// context when we expose one. This at minimum fixes the common
// "offline mirror, no DNS" failure mode the default client silently
// absorbs forever.
var httpClient = &http.Client{
	Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   30 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		IdleConnTimeout:       90 * time.Second,
		ForceAttemptHTTP2:     true,
	},
}

// Image describes a downloadable cloud image.
type Image struct {
	Name    string // short name (e.g. "alpine")
	URL     string
	Format  string // qcow2 or raw
	Default bool   // true = default tag for this distro
	Tag     string // version tag (e.g. "3.21", "noble")
	// SHA256 is the expected hex-encoded sha256 of the artifact at URL.
	// When set, Pull verifies the downloaded bytes and aborts on
	// mismatch. Empty means verification is skipped (registry entries
	// that track a mutable "latest" URL can't pin a hash).
	SHA256 string
	// User is the conventional cloud-init user for this distro
	// (alpine, debian, fedora, …). cloud-init will *create* whatever
	// user we ask for, but matching the convention means tools that
	// expect "$distro@vm" find the account, console autologin works
	// without surprises, and operators don't get a Password: prompt
	// because a user named "ubuntu" failed to materialise on Debian.
	// Empty falls back to compose's global default.
	User string
}

// Registry maps short names like "alpine" or "ubuntu:noble" to download URLs.
//
// SHA256 values are populated for images served from pinned, immutable URLs
// (e.g. Alpine's versioned artifact path). Entries whose URL tracks a
// mutable "latest" / "current" alias deliberately leave SHA256 empty;
// their upstream hash rotates on every publisher rebuild, so pinning it
// here would guarantee spurious verification failures. Callers that want
// strict verification against such distros should populate a local entry
// with the exact versioned URL plus its SHA256.
var Registry = []Image{
	// Alpine Linux (tiny-cloud, NoCloud datasource, BIOS). Pinned artifact,
	// but we ship no SHA256 by default (upstream publishes .sha256 alongside
	// the image; see docs for how to pin locally).
	{Name: "alpine", Tag: "3.21", URL: "https://dl-cdn.alpinelinux.org/alpine/v3.21/releases/cloud/nocloud_alpine-3.21.6-x86_64-bios-tiny-r0.qcow2", Format: "qcow2", Default: true, User: "alpine"},

	// Arch Linux (cloud-init, official arch-boxes). Rolling release, URL tracks "latest".
	{Name: "arch", Tag: "latest", URL: "https://geo.mirror.pkgbuild.com/images/latest/Arch-Linux-x86_64-cloudimg.qcow2", Format: "qcow2", Default: true, User: "arch"},

	// Debian (generic variant, cloud-init). URL uses "latest" symlink.
	//
	// Why "generic" and not "nocloud":
	// Debian publishes two flavours of the bookworm cloud image. The
	// "nocloud" variant is intentionally minimal: it strips out
	// openssh-server because it's meant as a base for custom images,
	// not for direct use. holos assumes sshd exists in the guest for
	// `holos exec` and healthchecks, so we'd silently produce VMs
	// where exec fails with "Connection reset by peer" forever
	// (sshd never bound port 22 because the package isn't installed).
	// The "generic" variant ships sshd enabled, still supports the
	// NoCloud datasource we feed via the cloud-localds ISO, and is
	// only ~25 MB larger.
	{Name: "debian", Tag: "12", URL: "https://cloud.debian.org/images/cloud/bookworm/latest/debian-12-generic-amd64.qcow2", Format: "qcow2", Default: true, User: "debian"},
	{Name: "debian", Tag: "bookworm", URL: "https://cloud.debian.org/images/cloud/bookworm/latest/debian-12-generic-amd64.qcow2", Format: "qcow2", User: "debian"},

	// Ubuntu (cloud images, NoCloud compatible). The "current" alias rotates on rebuild.
	{Name: "ubuntu", Tag: "noble", URL: "https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img", Format: "qcow2", Default: true, User: "ubuntu"},
	{Name: "ubuntu", Tag: "24.04", URL: "https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img", Format: "qcow2", User: "ubuntu"},
	{Name: "ubuntu", Tag: "jammy", URL: "https://cloud-images.ubuntu.com/jammy/current/jammy-server-cloudimg-amd64.img", Format: "qcow2", User: "ubuntu"},
	{Name: "ubuntu", Tag: "22.04", URL: "https://cloud-images.ubuntu.com/jammy/current/jammy-server-cloudimg-amd64.img", Format: "qcow2", User: "ubuntu"},

	// Fedora Cloud Base. Point release URL but still versioned.
	{Name: "fedora", Tag: "43", URL: "https://download.fedoraproject.org/pub/fedora/linux/releases/43/Cloud/x86_64/images/Fedora-Cloud-Base-Generic-43-1.6.x86_64.qcow2", Format: "qcow2", Default: true, User: "fedora"},
}

// Resolve looks up an image reference. Accepts:
//   - "alpine"         -> default alpine image
//   - "alpine:3.21"    -> specific tag
//   - "ubuntu:noble"   -> specific tag
//   - "./path/to.qcow2" or "/abs/path" -> returned as-is (local file)
func Resolve(ref string) (*Image, error) {
	if isLocalPath(ref) {
		return nil, nil
	}

	name, tag := parseRef(ref)

	for i := range Registry {
		img := &Registry[i]
		if img.Name != name {
			continue
		}
		if tag == "" && img.Default {
			return img, nil
		}
		if tag != "" && img.Tag == tag {
			return img, nil
		}
	}

	if tag != "" {
		return nil, fmt.Errorf("unknown image %q (tag %q); run 'holos images' to list available images", name, tag)
	}
	return nil, fmt.Errorf("unknown image %q; run 'holos images' to list available images", name)
}

// Pull downloads an image to the cache directory, returning the local path.
// If already cached, returns immediately.
//
// When the resolved registry entry carries a non-empty SHA256, the newly
// downloaded bytes are verified against it; a mismatch deletes the partial
// file and returns an error. Cached files are trusted: the file is only
// in the cache if a prior successful pull placed it there.
func Pull(ref string, cacheDir string) (localPath string, format string, err error) {
	img, err := Resolve(ref)
	if err != nil {
		return "", "", err
	}

	if img == nil {
		return ref, inferFormat(ref), nil
	}

	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", "", fmt.Errorf("create image cache: %w", err)
	}

	filename := cacheFilename(img)
	cached := filepath.Join(cacheDir, filename)

	if _, err := os.Stat(cached); err == nil {
		return cached, img.Format, nil
	}

	fmt.Printf("pulling %s:%s ...\n", img.Name, img.Tag)

	if err := download(img.URL, cached, img.SHA256); err != nil {
		_ = os.Remove(cached)
		return "", "", fmt.Errorf("pull %s: %w", ref, err)
	}

	fmt.Printf("cached  %s\n", cached)
	return cached, img.Format, nil
}

// DefaultCacheDir returns the image cache directory.
func DefaultCacheDir(stateDir string) string {
	return filepath.Join(stateDir, "images")
}

// DefaultUser returns the conventional cloud-init user for an image
// reference, or "" when the ref points at a local file or an unknown
// distro. This lets compose pick the right account for cloud-init to
// create *before* falling back to the global default. The difference
// between `holos exec` working and a console autologin attempt that
// can't find a user named "ubuntu" on a Debian image.
func DefaultUser(ref string) string {
	img, err := Resolve(ref)
	if err != nil || img == nil {
		return ""
	}
	return img.User
}

// ListAvailable returns all registered images.
func ListAvailable() []Image {
	return Registry
}

// download streams url into dest while hashing. When expectSHA256 is
// non-empty, the final hash must match (case-insensitive). On mismatch
// the partial file is deleted and an explanatory error is returned so
// callers can surface tampered or truncated downloads to the user.
//
// A per-request context is bound to an idle-timeout watchdog so that a
// mirror which sends headers and then stalls does not leave the
// caller stuck inside io.Copy. The watchdog cancels the request the
// moment bodyIdleTimeout elapses without a successful Read; the
// Transport propagates the cancellation into the outstanding Read as
// an error, so io.Copy unblocks promptly.
func download(url, dest, expectSHA256 string) error {
	// Concurrent `holos pull` or `holos up` invocations racing on
	// the same uncached image must not share a partial-file path.
	// Before this change both processes opened `dest + ".part"`,
	// interleaved their writes, and produced a corrupt blob that
	// either failed a supplied sha256 check (flaky) or, for images
	// without a pinned hash, got renamed into the cache and poisoned
	// every later boot. A per-call random suffix keeps each
	// downloader isolated; rename is atomic on POSIX within the
	// same filesystem, so one winner claims the cache slot and the
	// losers just discard wasted bandwidth without corrupting state.
	suffix, err := randomHexSuffix()
	if err != nil {
		return fmt.Errorf("generate tmp suffix: %w", err)
	}
	tmp := dest + ".part." + suffix

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", "holos/image-pull")

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}

	file, err := tempFileFactory(tmp)
	if err != nil {
		return err
	}

	hasher := sha256.New()
	writer := io.MultiWriter(file, hasher)

	body := newIdleTimeoutReader(resp.Body, bodyIdleTimeout, cancel)
	size, err := io.Copy(writer, body)
	body.Stop()
	if err != nil {
		file.Close()
		_ = os.Remove(tmp)
		if body.TimedOut() {
			return fmt.Errorf("download stalled (no bytes for %s): %w", bodyIdleTimeout, err)
		}
		return err
	}

	// Close *before* we promote the partial file. On NFS,
	// aggressive write-back caching, or a full disk, the last
	// delayed writes can surface at Close rather than Write, so
	// ignoring the return value lets a truncated file slip through
	// with a "valid" hash over the bytes we managed to hand off
	// before the failure. Any Close error voids the download: blow
	// away the .part and return, so `holos pull` retries next time
	// rather than caching a bad artifact forever.
	if err := file.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("finalize %s: %w", tmp, err)
	}

	gotHex := hex.EncodeToString(hasher.Sum(nil))

	if expectSHA256 != "" && !strings.EqualFold(gotHex, expectSHA256) {
		_ = os.Remove(tmp)
		return fmt.Errorf(
			"sha256 mismatch for %s:\n  expected %s\n  got      %s",
			url, strings.ToLower(expectSHA256), gotHex,
		)
	}

	verified := "unverified"
	if expectSHA256 != "" {
		verified = "verified"
	}
	fmt.Printf("  %s  %d MB  sha256:%s (%s)\n",
		filepath.Base(dest),
		size/(1024*1024),
		gotHex[:16],
		verified,
	)

	return os.Rename(tmp, dest)
}

// randomHexSuffix returns a short hex string suitable for
// disambiguating per-call temp files inside the image cache. 16 hex
// chars (8 bytes) is overkill for collision avoidance but trivial
// to read in a directory listing if a crash leaves debris behind.
func randomHexSuffix() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// idleTimeoutReader wraps an HTTP response body with a watchdog that
// fires if no bytes arrive within `timeout`. When it fires it calls
// the request's cancel func, which aborts the outstanding Transport
// Read and makes io.Copy return quickly. Every successful Read with
// n > 0 resets the watchdog, so long-but-healthy transfers pass
// through untouched.
type idleTimeoutReader struct {
	r       io.ReadCloser
	timeout time.Duration
	timer   *time.Timer
	fired   atomicBool
}

// newIdleTimeoutReader starts the watchdog immediately so that a
// mirror which never sends the first byte is still caught.
func newIdleTimeoutReader(r io.ReadCloser, timeout time.Duration, cancel context.CancelFunc) *idleTimeoutReader {
	itr := &idleTimeoutReader{r: r, timeout: timeout}
	itr.timer = time.AfterFunc(timeout, func() {
		itr.fired.Store(true)
		cancel()
	})
	return itr
}

func (i *idleTimeoutReader) Read(p []byte) (int, error) {
	n, err := i.r.Read(p)
	if n > 0 {
		// Reset keeps the watchdog honest on fast connections; we
		// ignore its return value because races with an in-flight
		// expiry are fine (the next Read sees ctx.Err()).
		i.timer.Reset(i.timeout)
	}
	return n, err
}

// Stop prevents the watchdog from firing after a normal end-of-body.
// Callers must invoke Stop before checking TimedOut, otherwise a
// late expiry could race with the success path.
func (i *idleTimeoutReader) Stop() {
	i.timer.Stop()
}

func (i *idleTimeoutReader) TimedOut() bool {
	return i.fired.Load()
}

// atomicBool is a tiny wrapper so the watchdog's "did I fire?" flag
// is safe to read from the Read() goroutine while the timer's
// goroutine may be writing it.
type atomicBool struct{ v atomic.Bool }

func (b *atomicBool) Store(x bool) { b.v.Store(x) }
func (b *atomicBool) Load() bool   { return b.v.Load() }

func cacheFilename(img *Image) string {
	h := sha256.Sum256([]byte(img.URL))
	short := hex.EncodeToString(h[:4])
	ext := ".qcow2"
	if img.Format == "raw" {
		ext = ".raw"
	}
	return fmt.Sprintf("%s-%s-%s%s", img.Name, img.Tag, short, ext)
}

func parseRef(ref string) (name, tag string) {
	if idx := strings.LastIndex(ref, ":"); idx != -1 {
		return ref[:idx], ref[idx+1:]
	}
	return ref, ""
}

// isLocalPath decides whether a reference should be treated as a filesystem
// path rather than a registry name. We accept:
//
//   - Absolute paths ("/...")
//   - Relative paths explicitly rooted at "./" or "../"
//   - Bare filenames ending in .qcow2/.raw/.img, but only if they contain
//     no colon (so registry references like "ubuntu:noble" are never
//     misinterpreted even if a future tag happened to end in ".img")
func isLocalPath(ref string) bool {
	if strings.HasPrefix(ref, "/") || strings.HasPrefix(ref, "./") || strings.HasPrefix(ref, "../") {
		return true
	}
	if strings.ContainsRune(ref, ':') {
		return false
	}
	if strings.HasSuffix(ref, ".qcow2") || strings.HasSuffix(ref, ".raw") || strings.HasSuffix(ref, ".img") {
		return true
	}
	return false
}

func inferFormat(path string) string {
	switch filepath.Ext(path) {
	case ".raw":
		return "raw"
	default:
		return "qcow2"
	}
}
