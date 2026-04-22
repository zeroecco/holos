package dockerfile

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zeroecco/holos/internal/config"
)

const buildScriptPath = "/var/lib/holos/build.sh"

// Result holds the cloud-init artifacts extracted from a Dockerfile.
type Result struct {
	FromImage  string             // base image from FROM, empty if not present
	Script     string             // shell script generated from RUN/ENV/WORKDIR
	WriteFiles []config.WriteFile // files from COPY instructions + the build script itself
}

// Parse reads a Dockerfile and converts it into cloud-init artifacts.
// COPY sources are resolved relative to contextDir.
func Parse(path string, contextDir string) (*Result, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open dockerfile: %w", err)
	}
	defer f.Close()

	lines, err := joinContinuations(bufio.NewScanner(f))
	if err != nil {
		return nil, fmt.Errorf("read dockerfile: %w", err)
	}

	result := &Result{}
	var script strings.Builder
	script.WriteString("#!/bin/bash\nset -e\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		cmd, args := splitInstruction(line)

		switch cmd {
		case "FROM":
			result.FromImage = parseFrom(args)

		case "RUN":
			script.WriteString("\n")
			script.WriteString(parseRun(args))
			script.WriteString("\n")

		case "COPY", "ADD":
			wf, err := parseCopy(args, contextDir)
			if err != nil {
				return nil, fmt.Errorf("%s %s: %w", cmd, args, err)
			}
			result.WriteFiles = append(result.WriteFiles, wf)

		case "ENV":
			for _, pair := range parseEnv(args) {
				fmt.Fprintf(&script, "export %s=%s\n", pair[0], shellQuote(pair[1]))
			}

		case "WORKDIR":
			dir := strings.TrimSpace(args)
			fmt.Fprintf(&script, "mkdir -p %s && cd %s\n", shellQuote(dir), shellQuote(dir))

		case "USER":
			// USER changes the effective UID for subsequent RUN
			// instructions in a container build. In our shell-script
			// provisioning model there is no equivalent: everything
			// runs as root. Silently dropping this would surprise
			// users whose Dockerfile expects later RUN steps to
			// execute as a non-root user, so emit a warning.
			fmt.Fprintf(os.Stderr, "holos: warning: Dockerfile USER %s ignored; RUN steps execute as root\n", strings.TrimSpace(args))
			continue
		case "EXPOSE", "CMD", "ENTRYPOINT", "LABEL", "VOLUME",
			"HEALTHCHECK", "STOPSIGNAL", "SHELL", "ONBUILD", "ARG":
			continue
		}
	}

	result.Script = script.String()

	result.WriteFiles = append(result.WriteFiles, config.WriteFile{
		Path:        buildScriptPath,
		Content:     result.Script,
		Permissions: "0755",
		Owner:       "root:root",
	})

	return result, nil
}

// BuildCommand returns the runcmd entry that executes the generated build script.
func BuildCommand() string {
	return "bash " + buildScriptPath
}

// joinContinuations merges backslash-continued lines.
func joinContinuations(scanner *bufio.Scanner) ([]string, error) {
	var lines []string
	var buf strings.Builder
	for scanner.Scan() {
		text := scanner.Text()
		trimmed := strings.TrimRight(text, " \t")
		if strings.HasSuffix(trimmed, "\\") {
			buf.WriteString(strings.TrimSuffix(trimmed, "\\"))
			buf.WriteByte(' ')
			continue
		}
		buf.WriteString(text)
		lines = append(lines, buf.String())
		buf.Reset()
	}
	if buf.Len() > 0 {
		lines = append(lines, buf.String())
	}
	return lines, scanner.Err()
}

func splitInstruction(line string) (cmd, args string) {
	parts := strings.SplitN(line, " ", 2)
	cmd = strings.ToUpper(parts[0])
	if len(parts) > 1 {
		args = strings.TrimSpace(parts[1])
	}
	return
}

func parseFrom(args string) string {
	// FROM [--platform=...] image[:tag] [AS name]
	fields := strings.Fields(args)
	for _, f := range fields {
		if strings.HasPrefix(f, "--") {
			continue
		}
		return f
	}
	return args
}

func parseRun(args string) string {
	args = strings.TrimSpace(args)
	// Exec form: ["cmd", "arg1", ...]
	if strings.HasPrefix(args, "[") {
		var parts []string
		if err := json.Unmarshal([]byte(args), &parts); err == nil {
			return strings.Join(parts, " ")
		}
	}
	return args
}

func parseCopy(args string, contextDir string) (config.WriteFile, error) {
	var owner, perms string
	fields := strings.Fields(args)
	var paths []string
	for _, f := range fields {
		switch {
		case strings.HasPrefix(f, "--chown="):
			owner = strings.TrimPrefix(f, "--chown=")
		case strings.HasPrefix(f, "--chmod="):
			perms = strings.TrimPrefix(f, "--chmod=")
		case strings.HasPrefix(f, "--from="):
			return config.WriteFile{}, fmt.Errorf("multi-stage --from is not supported")
		case strings.HasPrefix(f, "--"):
			continue
		default:
			paths = append(paths, f)
		}
	}

	if len(paths) < 2 {
		return config.WriteFile{}, fmt.Errorf("requires source and destination")
	}

	// Dockerfile's real COPY accepts `COPY src1 src2 dst/` and
	// copies every source into dst. holos emits a single cloud-init
	// write_files entry per COPY, so the historical code just took
	// paths[0] and silently dropped the rest. That turned
	// `COPY package.json package-lock.json /app/` into "only
	// package.json arrived", which is a nasty, silent misbehavior.
	// Refuse the multi-source form outright so the operator either
	// splits it into one COPY per file (explicit, supported) or
	// learns we do not implement the fan-out.
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
		owner = "root:root"
	}
	if perms == "" {
		perms = "0644"
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

// resolveCopySource turns a COPY source (relative to contextDir) into
// an absolute path on disk while enforcing that the final path stays
// under the build context. This mirrors docker's well-known rule that
// "the <src> path must be inside of the context of the build"; without
// it, `COPY ../../etc/shadow /tmp/x` would happily be read off the
// host and then embedded into the VM's cloud-init write_files,
// leaking arbitrary host files into the guest's filesystem.
//
// The check runs in four steps:
//
//  1. Refuse absolute source paths up front. docker itself errors on
//     these ("Forbidden path outside the build context"); our
//     filepath.Join contract would paste them unchanged onto the
//     context root, producing a path that does not exist.
//  2. Canonicalize the context root AND the joined source via
//     EvalSymlinks so macOS's /var -> /private/var, NixOS's /bin
//     symlinks, and similar host-level links do not produce false
//     "escapes" from filepath.Rel. EvalSymlinks requires the target
//     to exist, which is fine because the caller immediately stats
//     the result and missing files should surface as errors anyway.
//  3. filepath.Rel against the canonical context; a result starting
//     with ".." means the source escapes the context.
//  4. Catch symlinks INSIDE the context that point to /etc/shadow
//     and friends, caught implicitly by (2) since EvalSymlinks
//     follows the chain before we do the Rel check.
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
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("source %q escapes build context %q", src, canonContext)
	}

	return resolved, nil
}

// parseEnv returns key-value pairs from an ENV instruction.
// Supports both modern (ENV KEY=val KEY2=val2) and legacy (ENV KEY val) forms.
func parseEnv(args string) [][2]string {
	args = strings.TrimSpace(args)
	if !strings.Contains(args, "=") {
		parts := strings.SplitN(args, " ", 2)
		if len(parts) == 2 {
			return [][2]string{{parts[0], strings.TrimSpace(parts[1])}}
		}
		return nil
	}

	var pairs [][2]string
	for _, tok := range splitQuoted(args) {
		if idx := strings.IndexByte(tok, '='); idx > 0 {
			key := tok[:idx]
			val := strings.Trim(tok[idx+1:], "\"'")
			pairs = append(pairs, [2]string{key, val})
		}
	}
	return pairs
}

// splitQuoted splits on spaces but respects double-quoted values.
func splitQuoted(s string) []string {
	var tokens []string
	var cur strings.Builder
	inQuote := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch {
		case ch == '"':
			inQuote = !inQuote
			cur.WriteByte(ch)
		case ch == ' ' && !inQuote && cur.Len() > 0:
			tokens = append(tokens, cur.String())
			cur.Reset()
		default:
			cur.WriteByte(ch)
		}
	}
	if cur.Len() > 0 {
		tokens = append(tokens, cur.String())
	}
	return tokens
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	safe := true
	for _, ch := range s {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') || ch == '_' || ch == '-' ||
			ch == '.' || ch == '/' || ch == ':') {
			safe = false
			break
		}
	}
	if safe {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
