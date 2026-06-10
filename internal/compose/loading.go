package compose

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	defaultComposeFilePrimary   = "holos.yaml"
	defaultComposeFileSecondary = "holos.yml"
)

// DefaultFiles returns filenames to search for in priority order.
func DefaultFiles() []string {
	return []string{defaultComposeFilePrimary, defaultComposeFileSecondary}
}

// FindFile locates a compose file in the given directory.
func FindFile(dir string) (string, error) {
	for _, name := range DefaultFiles() {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("no %s found in %s", defaultComposeFilePrimary, dir)
}

// Load reads and parses a compose file.
//
// Decoding is strict (KnownFields(true)) so typos like `portz:` or
// `volume:` (singular) fail loudly instead of being silently dropped.
// docker-compose users hit this regularly with `enviroment:` and
// `volums:`; the YAML round-trips fine and the misspelled key just
// vanishes, leaving them debugging missing port mappings or volume
// mounts. We'd rather refuse to load.
func Load(path string) (*File, error) {
	return load(path, map[string]bool{})
}

func load(path string, seen map[string]bool) (*File, error) {
	abs, err := filepath.Abs(path)
	if err == nil {
		path = abs
	}
	if seen[path] {
		return nil, fmt.Errorf("include cycle involving %s", path)
	}
	seen[path] = true
	defer delete(seen, path)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read compose file: %w", err)
	}

	data, err = stripExtensionFields(data)
	if err != nil {
		return nil, fmt.Errorf("parse compose file: %w", err)
	}

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)

	var file File
	if err := decoder.Decode(&file); err != nil {
		return nil, fmt.Errorf("parse compose file: %w", err)
	}

	if file.Name == "" {
		file.Name = defaultProjectName(path)
	}
	file.baseDir = filepath.Dir(path)

	if err := mergeIncludes(&file, filepath.Dir(path), seen); err != nil {
		return nil, err
	}
	return &file, nil
}

func defaultProjectName(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return ""
	}
	return filepath.Base(filepath.Dir(abs))
}

func stripExtensionFields(data []byte) ([]byte, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	normalizeComposeYAMLNode(&doc)

	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(&doc); err != nil {
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
