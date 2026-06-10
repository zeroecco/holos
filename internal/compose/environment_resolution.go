package compose

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	envFileRawFormat       = "raw"
	envFileCommentPrefix   = "#"
	envFileAssignmentToken = "="
)

func resolveEnvironment(baseDir string, files EnvFiles, inline Environment) (Environment, error) {
	out := Environment{}
	for _, file := range files {
		if err := validateEnvFile(file); err != nil {
			return nil, err
		}
		path := resolveEnvFilePath(baseDir, file.Path)
		env, err := readEnvFile(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) && !file.required() {
				continue
			}
			return nil, fmt.Errorf("env_file %q: %w", file.Path, err)
		}
		mergeEnvironment(out, env)
	}
	mergeEnvironment(out, inline)
	return out, nil
}

func mergeEnvironment(dst Environment, src Environment) {
	for key, value := range src {
		dst[key] = value
	}
}

func validateEnvFile(file EnvFile) error {
	if file.Path == "" {
		return fmt.Errorf("env_file path is required")
	}
	if file.Format != "" && file.Format != envFileRawFormat {
		return fmt.Errorf("env_file format %q is unsupported; only raw is implemented", file.Format)
	}
	return nil
}

func resolveEnvFilePath(baseDir string, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(baseDir, path)
}

func readEnvFile(path string) (Environment, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	out := Environment{}
	for lineNo, line := range strings.Split(string(data), "\n") {
		key, value, ok, err := parseEnvFileLine(lineNo+1, line)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		out[key] = value
	}
	return out, nil
}

func parseEnvFileLine(lineNo int, line string) (key string, value *string, ok bool, err error) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, envFileCommentPrefix) {
		return "", nil, false, nil
	}
	key, rawValue, hasAssignment := strings.Cut(line, envFileAssignmentToken)
	if !hasAssignment {
		return line, nil, true, nil
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return "", nil, false, fmt.Errorf("line %d: empty variable name", lineNo)
	}
	return key, stringPtr(strings.TrimSpace(rawValue)), true, nil
}
