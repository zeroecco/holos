package compose

import (
	"fmt"
	"path/filepath"

	"github.com/zeroecco/holos/internal/config"
	"github.com/zeroecco/holos/internal/dockerfile"
)

const (
	defaultBuildContextDir = "."
	defaultDockerfileName  = "Dockerfile"
)

func (b ComposeBuild) dockerfilePath(baseDir string) (path string, contextDir string, ok bool, err error) {
	if !b.isSet() {
		return "", "", false, nil
	}
	contextDir = resolveBuildContextDir(baseDir, b.Context)
	if hasInlineDockerfile(b) {
		return "", contextDir, true, nil
	}
	return resolveBuildDockerfilePath(contextDir, b.Dockerfile), contextDir, true, nil
}

func resolveBuildContextDir(baseDir string, contextDir string) string {
	if contextDir == "" {
		contextDir = defaultBuildContextDir
	}
	if filepath.IsAbs(contextDir) {
		return contextDir
	}
	return filepath.Join(baseDir, contextDir)
}

func resolveBuildDockerfilePath(contextDir string, dockerfilePath string) string {
	if dockerfilePath == "" {
		dockerfilePath = defaultDockerfileName
	}
	if filepath.IsAbs(dockerfilePath) {
		return dockerfilePath
	}
	return filepath.Join(contextDir, dockerfilePath)
}

func hasInlineDockerfile(build ComposeBuild) bool {
	return !isBlankScalarString(build.DockerfileInline)
}

func resolveDockerfileBuild(svc *Service, baseDir string) ([]config.WriteFile, []string, []config.PortForward, *config.HealthcheckConfig, error) {
	if svc.Dockerfile == "" && !svc.Build.isSet() {
		return nil, nil, nil, nil, nil
	}

	dfPath, dfContext := resolveStandaloneDockerfilePath(baseDir, svc.Dockerfile)
	if svc.Dockerfile == "" {
		var ok bool
		var err error
		dfPath, dfContext, ok, err = svc.Build.dockerfilePath(baseDir)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		if !ok {
			return nil, nil, nil, nil, fmt.Errorf("build: dockerfile path is required")
		}
	}
	if dfContext == "" {
		dfContext = filepath.Dir(dfPath)
	}

	var result *dockerfile.Result
	var err error
	if svc.Dockerfile == "" && hasInlineDockerfile(svc.Build) {
		result, err = dockerfile.ParseContent(svc.Build.DockerfileInline, dfContext)
	} else {
		result, err = dockerfile.Parse(dfPath, dfContext)
	}
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("dockerfile: %w", err)
	}
	if svc.Image == "" && result.FromImage != "" {
		svc.Image = result.FromImage
	}
	return result.WriteFiles, []string{dockerfile.BuildCommand()}, result.Ports, result.Healthcheck, nil
}

func resolveStandaloneDockerfilePath(baseDir string, dockerfilePath string) (path string, contextDir string) {
	if dockerfilePath == "" {
		return "", ""
	}
	path = dockerfilePath
	if !filepath.IsAbs(path) {
		path = filepath.Join(baseDir, path)
	}
	return path, filepath.Dir(path)
}
