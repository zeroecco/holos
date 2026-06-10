package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"time"
)

func randHex(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err == nil {
		return hex.EncodeToString(buf)
	}
	return randHexFallback(n)
}

func randHexFallback(n int) string {
	if n > sha256.Size {
		n = sha256.Size
	}
	seed := time.Now().UnixNano() ^ int64(os.Getpid())
	h := sha256.Sum256(fmt.Appendf(nil, "%d", seed))
	return hex.EncodeToString(h[:n])
}
