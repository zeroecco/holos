package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/zeroecco/holos/internal/compose"
)

func loadProjectWithPath(filePath, stateDir string) (*compose.Project, string, error) {
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
	project, err := file.Resolve(filepath.Dir(abs), stateDir)
	if err != nil {
		return nil, "", err
	}
	return project, abs, nil
}

func loadProject(filePath string, stateDir string) (*compose.Project, error) {
	if filePath == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("get working directory: %w", err)
		}
		found, err := compose.FindFile(cwd)
		if err != nil {
			return nil, err
		}
		filePath = found
	}
	file, err := compose.Load(filePath)
	if err != nil {
		return nil, err
	}
	baseDir := filepath.Dir(filePath)
	abs, err := filepath.Abs(baseDir)
	if err == nil {
		baseDir = abs
	}
	return file.Resolve(baseDir, stateDir)
}
