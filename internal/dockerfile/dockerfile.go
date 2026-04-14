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

		case "EXPOSE", "CMD", "ENTRYPOINT", "LABEL", "VOLUME",
			"HEALTHCHECK", "STOPSIGNAL", "SHELL", "ONBUILD", "ARG", "USER":
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

	src := paths[0]
	dst := paths[len(paths)-1]

	srcPath := filepath.Join(contextDir, src)
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
