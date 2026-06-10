package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/zeroecco/holos/internal/compose"
	"gopkg.in/yaml.v3"
)

const (
	runComposeRootSubdir = "runs"
	runComposeFilename   = "holos.yaml"
	runComposeDirPerm    = os.FileMode(0o700)
	runComposeFilePerm   = os.FileMode(0o600)
)

func composePorts(ports []string) []compose.ComposePort {
	out := make([]compose.ComposePort, len(ports))
	for i, port := range ports {
		out[i] = compose.ComposePort{Short: port}
	}
	return out
}

func composeVolumes(volumes []string) []compose.ComposeVolume {
	out := make([]compose.ComposeVolume, len(volumes))
	for i, volume := range volumes {
		out[i] = compose.ComposeVolume{Short: volume}
	}
	return out
}

func writeRunComposeFile(stateDir, projectName string, file compose.File) (string, error) {
	for _, dir := range runComposeDirs(stateDir, projectName) {
		if err := os.MkdirAll(dir, runComposeDirPerm); err != nil {
			return "", fmt.Errorf("create run dir %s: %w", dir, err)
		}
		if err := os.Chmod(dir, runComposeDirPerm); err != nil {
			return "", fmt.Errorf("tighten run dir %s: %w", dir, err)
		}
	}
	composePath := runComposeFilePath(stateDir, projectName)
	yamlBytes, err := yaml.Marshal(file)
	if err != nil {
		return "", fmt.Errorf("marshal compose: %w", err)
	}
	if err := os.WriteFile(composePath, yamlBytes, runComposeFilePerm); err != nil {
		return "", fmt.Errorf("write compose: %w", err)
	}
	if err := os.Chmod(composePath, runComposeFilePerm); err != nil {
		return "", fmt.Errorf("tighten compose file: %w", err)
	}
	return composePath, nil
}

func runComposeDirs(stateDir, projectName string) []string {
	return []string{stateDir, runComposeRootDir(stateDir), runComposeProjectDir(stateDir, projectName)}
}

func runComposeRootDir(stateDir string) string {
	return filepath.Join(stateDir, runComposeRootSubdir)
}

func runComposeProjectDir(stateDir, projectName string) string {
	return filepath.Join(runComposeRootDir(stateDir), projectName)
}

func runComposeFilePath(stateDir, projectName string) string {
	return filepath.Join(runComposeProjectDir(stateDir, projectName), runComposeFilename)
}
