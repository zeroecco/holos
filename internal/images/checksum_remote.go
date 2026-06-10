package images

import (
	"fmt"
	"io"
	"net/http"
)

func fetchChecksum(url, filename string, hexLen int) (string, error) {
	resp, err := httpClient.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if hash := findChecksum(string(data), filename, hexLen); hash != "" {
		return hash, nil
	}
	return "", fmt.Errorf("checksum for %s not found in %s", filename, url)
}
