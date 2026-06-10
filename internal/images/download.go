package images

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

// tempFileFactory is the indirection that lets tests replace the
// partial-file sink so they can exercise failure modes (e.g. Close
// returning ENOSPC) without needing a real quota-capped filesystem.
// Production always uses os.Create.
var tempFileFactory = func(name string) (io.WriteCloser, error) {
	return os.Create(name)
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
func download(url, dest string, expect imageHash) error {
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
	tmp := dest + downloadTempInfix + suffix

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resp, err := openDownloadResponse(ctx, url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	file, err := tempFileFactory(tmp)
	if err != nil {
		return err
	}

	hasher, err := newHasher(expect.Algorithm)
	if err != nil {
		return err
	}
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

	if err := verifyDownloadedHash(url, tmp, expect, gotHex); err != nil {
		return err
	}

	printDownloadSummary(dest, size, expect, gotHex)

	return os.Rename(tmp, dest)
}
