package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zeroecco/holos/internal/compose"
)

func loadResolvedProject(filePath, stateDir string, strict bool) (*compose.Project, string, error) {
	file, composePath, err := loadComposeFile(filePath)
	if err != nil {
		return nil, "", err
	}

	warnings := file.CompatibilityWarnings()
	if strict && len(warnings) > 0 {
		return nil, "", fmt.Errorf("ignored Compose settings (strict mode):\n%s", strings.Join(warnings, "\n"))
	}
	for _, warning := range warnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", warning)
	}
	project, err := file.Resolve(filepath.Dir(composePath), stateDir)
	if err != nil {
		return nil, "", err
	}
	return project, composePath, nil
}

func loadProject(filePath string, stateDir string) (*compose.Project, error) {
	project, _, err := loadResolvedProject(filePath, stateDir, false)
	return project, err
}

// loadComposeFile reads project identity without resolving images or provisioning.
// State-only commands must remain usable when image sources are unavailable.
func loadComposeFile(filePath string) (*compose.File, string, error) {
	if filePath == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, "", fmt.Errorf("get working directory: %w", err)
		}
		found, err := compose.FindFile(cwd)
		if err != nil {
			return nil, "", err
		}
		filePath = found
	}
	abs, err := filepath.Abs(filePath)
	if err != nil {
		return nil, "", fmt.Errorf("resolve compose path: %w", err)
	}

	file, err := compose.Load(abs)
	if err != nil {
		return nil, "", err
	}
	return file, abs, nil
}
