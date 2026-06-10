package images

import (
	"crypto/sha256"
	"crypto/sha512"
	"fmt"
	"hash"
	"path/filepath"
	"strings"
)

type imageHash struct {
	Algorithm string
	Value     string
}

const (
	hashAlgorithmSHA256 = "sha256"
	hashAlgorithmSHA512 = "sha512"

	defaultHashAlgorithm = hashAlgorithmSHA256

	sha256HexLength = sha256.Size * 2
	sha512HexLength = sha512.Size * 2
)

func (img Image) ChecksumAlgorithm() string {
	switch {
	case img.SHA256 != "" || img.SHA256URL != "":
		return hashAlgorithmSHA256
	case img.SHA512 != "" || img.SHA512URL != "":
		return hashAlgorithmSHA512
	default:
		return ""
	}
}

func expectedHash(img *Image) (imageHash, error) {
	switch {
	case img.SHA256 != "":
		return inlineImageHash(hashAlgorithmSHA256, img.SHA256), nil
	case img.SHA512 != "":
		return inlineImageHash(hashAlgorithmSHA512, img.SHA512), nil
	case img.SHA256URL != "":
		value, err := fetchChecksum(img.SHA256URL, filepath.Base(img.URL), sha256HexLength)
		return imageHash{Algorithm: hashAlgorithmSHA256, Value: value}, err
	case img.SHA512URL != "":
		value, err := fetchChecksum(img.SHA512URL, filepath.Base(img.URL), sha512HexLength)
		return imageHash{Algorithm: hashAlgorithmSHA512, Value: value}, err
	default:
		return imageHash{}, nil
	}
}

func inlineImageHash(algorithm, value string) imageHash {
	return imageHash{Algorithm: algorithm, Value: strings.ToLower(value)}
}

func isHex(s string) bool {
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}

func newHasher(algorithm string) (hash.Hash, error) {
	switch algorithm {
	case "", hashAlgorithmSHA256:
		return sha256.New(), nil
	case hashAlgorithmSHA512:
		return sha512.New(), nil
	default:
		return nil, fmt.Errorf("unsupported hash algorithm %q", algorithm)
	}
}
