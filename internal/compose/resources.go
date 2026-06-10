package compose

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/zeroecco/holos/internal/config"
	"gopkg.in/yaml.v3"
)

// Config and Secret accept Docker Compose top-level resource declarations.
type Config struct {
	Name           string        `yaml:"name,omitempty"`
	File           string        `yaml:"file,omitempty"`
	Environment    string        `yaml:"environment,omitempty"`
	Content        string        `yaml:"content,omitempty"`
	TemplateDriver string        `yaml:"template_driver,omitempty"`
	External       any           `yaml:"external,omitempty"`
	Labels         ComposeLabels `yaml:"labels,omitempty"`
}

type Secret struct {
	Name           string         `yaml:"name,omitempty"`
	File           string         `yaml:"file,omitempty"`
	Environment    string         `yaml:"environment,omitempty"`
	External       any            `yaml:"external,omitempty"`
	Labels         ComposeLabels  `yaml:"labels,omitempty"`
	Driver         string         `yaml:"driver,omitempty"`
	DriverOpts     map[string]any `yaml:"driver_opts,omitempty"`
	TemplateDriver string         `yaml:"template_driver,omitempty"`
}

type ServiceResources []ServiceResource

type ServiceResource struct {
	Source string `yaml:"source,omitempty"`
	Target string `yaml:"target,omitempty"`
	UID    string `yaml:"uid,omitempty"`
	GID    string `yaml:"gid,omitempty"`
	Mode   any    `yaml:"mode,omitempty"`
}

const (
	configDefaultPermissions = "0444"
	secretDefaultPermissions = "0400"
	secretDefaultTargetDir   = "/run/secrets"
)

func (r *ServiceResources) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.SequenceNode {
		return fmt.Errorf("line %d: resource references must be a list", node.Line)
	}
	out := make([]ServiceResource, 0, len(node.Content))
	for _, item := range node.Content {
		ref, err := decodeServiceResource(item)
		if err != nil {
			return err
		}
		out = append(out, ref)
	}
	*r = ServiceResources(out)
	return nil
}

func decodeServiceResource(node *yaml.Node) (ServiceResource, error) {
	switch node.Kind {
	case yaml.ScalarNode:
		return ServiceResource{Source: node.Value}, nil
	case yaml.MappingNode:
		var ref ServiceResource
		if err := node.Decode(&ref); err != nil {
			return ServiceResource{}, err
		}
		return ref, nil
	default:
		return ServiceResource{}, fmt.Errorf("line %d: resource references must be strings or mappings", node.Line)
	}
}

func resolveResourceWriteFiles(baseDir string, svc Service, configs map[string]Config, secrets map[string]Secret) ([]config.WriteFile, error) {
	writeFiles := make([]config.WriteFile, 0, len(svc.Configs)+len(svc.Secrets))
	configFiles, err := resolveConfigWriteFiles(baseDir, svc.Configs, configs)
	if err != nil {
		return nil, err
	}
	writeFiles = append(writeFiles, configFiles...)

	secretFiles, err := resolveSecretWriteFiles(baseDir, svc.Secrets, secrets)
	if err != nil {
		return nil, err
	}
	writeFiles = append(writeFiles, secretFiles...)
	return writeFiles, nil
}

func resolveConfigWriteFiles(baseDir string, refs ServiceResources, configs map[string]Config) ([]config.WriteFile, error) {
	writeFiles := make([]config.WriteFile, 0, len(refs))
	for _, ref := range refs {
		source, err := resourceSource(ref)
		if err != nil {
			return nil, err
		}
		def, ok := configs[source]
		if !ok {
			return nil, fmt.Errorf("config %q is not declared", source)
		}
		content, err := configContent(baseDir, source, def)
		if err != nil {
			return nil, err
		}
		file, err := resourceWriteFile(ref, configTarget(ref, source), content, configDefaultPermissions)
		if err != nil {
			return nil, fmt.Errorf("config %q: %w", source, err)
		}
		writeFiles = append(writeFiles, file)
	}
	return writeFiles, nil
}

func resolveSecretWriteFiles(baseDir string, refs ServiceResources, secrets map[string]Secret) ([]config.WriteFile, error) {
	writeFiles := make([]config.WriteFile, 0, len(refs))
	for _, ref := range refs {
		source, err := resourceSource(ref)
		if err != nil {
			return nil, err
		}
		def, ok := secrets[source]
		if !ok {
			return nil, fmt.Errorf("secret %q is not declared", source)
		}
		content, err := secretContent(baseDir, source, def)
		if err != nil {
			return nil, err
		}
		file, err := resourceWriteFile(ref, secretTarget(ref, source), content, secretDefaultPermissions)
		if err != nil {
			return nil, fmt.Errorf("secret %q: %w", source, err)
		}
		writeFiles = append(writeFiles, file)
	}
	return writeFiles, nil
}

func resourceSource(ref ServiceResource) (string, error) {
	if strings.TrimSpace(ref.Source) == "" {
		return "", fmt.Errorf("resource reference requires source")
	}
	return ref.Source, nil
}

func configContent(baseDir string, source string, def Config) (string, error) {
	if resourceExternal(def.External) {
		return "", fmt.Errorf("config %q is external and cannot be injected into guests", source)
	}
	switch {
	case def.Content != "":
		return def.Content, nil
	case def.File != "":
		return readResourceFile(baseDir, def.File)
	case def.Environment != "":
		return resourceEnvironmentValue("config", source, def.Environment)
	default:
		return "", fmt.Errorf("config %q needs file, content, or environment", source)
	}
}

func secretContent(baseDir string, source string, def Secret) (string, error) {
	if resourceExternal(def.External) {
		return "", fmt.Errorf("secret %q is external and cannot be injected into guests", source)
	}
	switch {
	case def.File != "":
		return readResourceFile(baseDir, def.File)
	case def.Environment != "":
		return resourceEnvironmentValue("secret", source, def.Environment)
	default:
		return "", fmt.Errorf("secret %q needs file or environment", source)
	}
}

func readResourceFile(baseDir string, path string) (string, error) {
	resolved := path
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(baseDir, resolved)
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return string(data), nil
}

func resourceEnvironmentValue(kind, source, name string) (string, error) {
	value, ok := os.LookupEnv(name)
	if !ok {
		return "", fmt.Errorf("%s %q environment variable %q is not set", kind, source, name)
	}
	return value, nil
}

func resourceExternal(value any) bool {
	switch v := value.(type) {
	case nil:
		return false
	case bool:
		return v
	default:
		return true
	}
}

func configTarget(ref ServiceResource, source string) string {
	return resourceTarget(ref.Target, source, "/")
}

func secretTarget(ref ServiceResource, source string) string {
	return resourceTarget(ref.Target, source, secretDefaultTargetDir)
}

func resourceTarget(target, source, defaultDir string) string {
	if strings.TrimSpace(target) == "" {
		target = source
	}
	if filepath.IsAbs(target) {
		return target
	}
	return filepath.Join(defaultDir, target)
}

func resourceWriteFile(ref ServiceResource, target, content, defaultPermissions string) (config.WriteFile, error) {
	permissions, err := resourcePermissions(ref.Mode, defaultPermissions)
	if err != nil {
		return config.WriteFile{}, err
	}
	return config.WriteFile{
		Path:        target,
		Content:     content,
		Permissions: permissions,
		Owner:       resourceOwner(ref),
	}, nil
}

func resourceOwner(ref ServiceResource) string {
	uid := ref.UID
	if uid == "" {
		uid = "root"
	}
	gid := ref.GID
	if gid == "" {
		gid = "root"
	}
	return uid + ":" + gid
}

func resourcePermissions(mode any, fallback string) (string, error) {
	switch v := mode.(type) {
	case nil:
		return fallback, nil
	case int:
		return formatResourceMode(int64(v))
	case int64:
		return formatResourceMode(v)
	case float64:
		if math.Trunc(v) != v {
			return "", fmt.Errorf("mode must be an integer")
		}
		return formatResourceMode(int64(v))
	case string:
		return normalizeResourceModeString(v, fallback)
	default:
		return "", fmt.Errorf("mode has unsupported type %T", mode)
	}
}

func formatResourceMode(mode int64) (string, error) {
	if mode < 0 {
		return "", fmt.Errorf("mode must be non-negative")
	}
	return fmt.Sprintf("%04o", mode), nil
}

func normalizeResourceModeString(mode string, fallback string) (string, error) {
	mode = strings.TrimSpace(mode)
	if mode == "" {
		return fallback, nil
	}
	if _, err := strconv.ParseUint(mode, 8, 32); err != nil {
		return "", fmt.Errorf("mode %q must be octal", mode)
	}
	if len(mode) >= 4 {
		return mode, nil
	}
	return strings.Repeat("0", 4-len(mode)) + mode, nil
}
