package images

import (
	"crypto/rand"
	"encoding/hex"
)

const (
	downloadTempInfix       = ".part."
	downloadTempSuffixBytes = 8
)

// randomHexSuffix returns a short hex string suitable for
// disambiguating per-call temp files inside the image cache. 16 hex
// chars (8 bytes) is overkill for collision avoidance but trivial
// to read in a directory listing if a crash leaves debris behind.
func randomHexSuffix() (string, error) {
	var b [downloadTempSuffixBytes]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
