//go:build integration

package integration

import (
	"os"
	"path/filepath"
)

// writeFile writes content to dir/name and returns the absolute path.
func writeFile(dir, name, content string) (string, error) {
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func readFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
