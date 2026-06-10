package compose

import (
	"fmt"
	"sort"
	"strings"

	"github.com/zeroecco/holos/internal/config"
)

const (
	environmentFilePath       = "/etc/environment"
	environmentAssignment     = "="
	environmentPrefixJoin     = " "
	environmentFileLineFormat = "%s=%q\n"
)

func environmentFile(env Environment) (config.WriteFile, bool) {
	if len(env) == 0 {
		return config.WriteFile{}, false
	}
	keys := assignedEnvironmentKeys(env)
	if len(keys) == 0 {
		return config.WriteFile{}, false
	}
	return environmentWriteFile(environmentFileContent(env, keys)), true
}

func environmentFileContent(env Environment, keys []string) string {
	var b strings.Builder
	for _, key := range keys {
		b.WriteString(environmentFileAssignment(key, *env[key]))
	}
	return b.String()
}

func environmentWriteFile(content string) config.WriteFile {
	return config.WriteFile{
		Path:        environmentFilePath,
		Content:     content,
		Permissions: config.DefaultFilePermissions,
		Owner:       config.DefaultFileOwner,
	}
}

func environmentFileAssignment(key, value string) string {
	return fmt.Sprintf(environmentFileLineFormat, key, value)
}

func environmentPrefix(env Environment) string {
	if len(env) == 0 {
		return ""
	}
	keys := assignedEnvironmentKeys(env)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, environmentShellAssignment(key, *env[key]))
	}
	return strings.Join(parts, environmentPrefixJoin)
}

func environmentShellAssignment(key, value string) string {
	return key + environmentAssignment + shellQuote(value)
}

func assignedEnvironmentKeys(env Environment) []string {
	keys := make([]string, 0, len(env))
	for key, value := range env {
		if value != nil {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}
