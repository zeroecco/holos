package images

import (
	"fmt"
	"path/filepath"
)

func printDownloadSummary(dest string, size int64, expect imageHash, gotHex string) {
	fmt.Print(downloadSummaryLine(dest, size, expect, gotHex))
}

func downloadSummaryLine(dest string, size int64, expect imageHash, gotHex string) string {
	algorithm := expect.Algorithm
	if algorithm == "" {
		algorithm = defaultHashAlgorithm
	}
	verification := "unverified"
	if expect.Value != "" {
		verification = "verified"
	}
	return fmt.Sprintf("  %s  %d MB  %s:%s (%s)\n",
		filepath.Base(dest),
		size/(1024*1024),
		algorithm,
		hashDisplayPrefix(gotHex),
		verification,
	)
}
