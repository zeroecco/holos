package dockerfile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zeroecco/holos/internal/config"
)

const (
	copyChownFlag          = "--chown="
	copyChmodFlag          = "--chmod="
	copyFromFlag           = "--from="
	copyOptionPrefix       = "--"
	defaultCopyOwner       = "root:root"
	defaultCopyPermissions = "0644"
)

func parseCopy(args string, contextDir string) (config.WriteFile, error) {
	var owner, perms string
	fields := strings.Fields(args)
	var paths []string
	for _, f := range fields {
		switch {
		case strings.HasPrefix(f, copyChownFlag):
			owner = strings.TrimPrefix(f, copyChownFlag)
		case strings.HasPrefix(f, copyChmodFlag):
			perms = strings.TrimPrefix(f, copyChmodFlag)
		case strings.HasPrefix(f, copyFromFlag):
			return config.WriteFile{}, fmt.Errorf("multi-stage --from is not supported")
		case strings.HasPrefix(f, copyOptionPrefix):
			continue
		default:
			paths = append(paths, f)
		}
	}

	if len(paths) < 2 {
		return config.WriteFile{}, fmt.Errorf("requires source and destination")
	}

	// holos emits one cloud-init write_files entry per COPY, so reject
	// Docker's multi-source fan-out instead of silently dropping sources.
	if len(paths) > 2 {
		return config.WriteFile{}, fmt.Errorf(
			"multi-source COPY with %d sources is not supported; split into one COPY per source",
			len(paths)-1)
	}

	src := paths[0]
	dst := paths[len(paths)-1]

	srcPath, err := resolveCopySource(contextDir, src)
	if err != nil {
		return config.WriteFile{}, err
	}

	info, err := os.Stat(srcPath)
	if err != nil {
		return config.WriteFile{}, fmt.Errorf("source %q: %w", src, err)
	}
	if info.IsDir() {
		return config.WriteFile{}, fmt.Errorf("source %q is a directory; use a volume mount instead", src)
	}

	content, err := os.ReadFile(srcPath)
	if err != nil {
		return config.WriteFile{}, fmt.Errorf("read %q: %w", src, err)
	}

	if owner == "" {
		owner = defaultCopyOwner
	}
	if perms == "" {
		perms = defaultCopyPermissions
	}
	if strings.HasSuffix(dst, "/") {
		dst = filepath.Join(dst, filepath.Base(src))
	}

	return config.WriteFile{
		Path:        dst,
		Content:     string(content),
		Permissions: perms,
		Owner:       owner,
	}, nil
}

// resolveCopySource turns a COPY source into an absolute path while
// enforcing Docker's rule that sources stay under the build context.
// EvalSymlinks is applied to both sides so symlinks inside the context
// cannot point at arbitrary host files.
func resolveCopySource(contextDir, src string) (string, error) {
	if filepath.IsAbs(src) {
		return "", fmt.Errorf("source %q escapes build context: absolute paths are not allowed in COPY", src)
	}

	absContext, err := filepath.Abs(contextDir)
	if err != nil {
		return "", fmt.Errorf("resolve context dir: %w", err)
	}
	canonContext, err := filepath.EvalSymlinks(absContext)
	if err != nil {
		return "", fmt.Errorf("resolve context dir %q: %w", absContext, err)
	}

	joined := filepath.Clean(filepath.Join(canonContext, src))
	resolved, err := filepath.EvalSymlinks(joined)
	if err != nil {
		return "", fmt.Errorf("source %q: %w", src, err)
	}

	rel, err := filepath.Rel(canonContext, resolved)
	if err != nil {
		return "", fmt.Errorf("source %q is not reachable from build context: %w", src, err)
	}
	if copySourceEscapesContext(rel) {
		return "", fmt.Errorf("source %q escapes build context %q", src, canonContext)
	}

	return resolved, nil
}

func copySourceEscapesContext(rel string) bool {
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
