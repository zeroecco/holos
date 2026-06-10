package images

import (
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
)

func verifyFile(path string, expect imageHash) error {
	hasher, err := newHasher(expect.Algorithm)
	if err != nil {
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := io.Copy(hasher, file); err != nil {
		return err
	}
	got := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(got, expect.Value) {
		return checksumMismatchError(expect.Algorithm, path, expect.Value, got)
	}
	return nil
}

func verifyDownloadedHash(url, tmp string, expect imageHash, gotHex string) error {
	if expect.Value == "" {
		return nil
	}
	if !strings.EqualFold(gotHex, expect.Value) {
		_ = os.Remove(tmp)
		return checksumMismatchError(expect.Algorithm, url, expect.Value, gotHex)
	}
	return nil
}

func checksumMismatchError(algorithm, target, expected, got string) error {
	return fmt.Errorf("%s mismatch for %s:\n  expected %s\n  got      %s",
		algorithm, target, normalizedExpectedHash(expected), got)
}

func normalizedExpectedHash(expected string) string {
	return strings.ToLower(expected)
}
