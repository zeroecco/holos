package compose

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func mergeIncludes(file *File, baseDir string, seen map[string]bool) error {
	for _, include := range file.Include {
		projectDir := includeProjectDir(baseDir, include)
		for _, includePath := range include.Path {
			path := resolveIncludePath(projectDir, includePath)
			exists, err := includeFileExists(path)
			if err != nil {
				return fmt.Errorf("include %q: %w", includePath, err)
			}
			if !exists {
				continue
			}
			included, err := load(path, seen)
			if err != nil {
				return fmt.Errorf("include %q: %w", includePath, err)
			}
			attributeIncludedServiceBaseDirs(included, projectDir)
			mergeIncludedFile(file, included)
		}
	}
	return nil
}

func includeFileExists(path string) (bool, error) {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func includeProjectDir(baseDir string, include IncludeFile) string {
	if include.ProjectDirectory == "" {
		return baseDir
	}
	if filepath.IsAbs(include.ProjectDirectory) {
		return include.ProjectDirectory
	}
	return filepath.Join(baseDir, include.ProjectDirectory)
}

func resolveIncludePath(projectDir string, includePath string) string {
	if filepath.IsAbs(includePath) {
		return includePath
	}
	return filepath.Join(projectDir, includePath)
}

func attributeIncludedServiceBaseDirs(file *File, projectDir string) {
	if file.serviceBaseDirs == nil {
		file.serviceBaseDirs = map[string]string{}
	}
	for name := range file.Services {
		if _, exists := file.serviceBaseDirs[name]; !exists {
			file.serviceBaseDirs[name] = projectDir
		}
	}
}

func mergeIncludedFile(dst *File, src *File) {
	mergeMap(&dst.Services, src.Services)
	mergeMap(&dst.Volumes, src.Volumes)
	mergeMap(&dst.Networks, src.Networks)
	mergeMap(&dst.Configs, src.Configs)
	mergeMap(&dst.Secrets, src.Secrets)
	mergeMap(&dst.Models, src.Models)
	if dst.serviceBaseDirs == nil {
		dst.serviceBaseDirs = map[string]string{}
	}
	for name, dir := range src.serviceBaseDirs {
		if _, exists := dst.serviceBaseDirs[name]; !exists {
			dst.serviceBaseDirs[name] = dir
		}
	}
}

func mergeMap[M ~map[string]V, V any](dst *M, src M) {
	if len(src) == 0 {
		return
	}
	if *dst == nil {
		*dst = M{}
	}
	for key, value := range src {
		if _, exists := (*dst)[key]; !exists {
			(*dst)[key] = value
		}
	}
}
