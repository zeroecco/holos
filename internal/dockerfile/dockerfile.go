package dockerfile

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/zeroecco/holos/internal/config"
)

const (
	buildScriptPath        = "/var/lib/holos/build.sh"
	buildScriptPrelude     = "#!/bin/bash\nset -e\n"
	buildScriptPermissions = "0755"
	buildScriptOwner       = "root:root"
)

// Result holds the cloud-init artifacts extracted from a Dockerfile.
type Result struct {
	FromImage   string                    // base image from FROM, empty if not present
	Script      string                    // shell script generated from RUN/ENV/WORKDIR
	WriteFiles  []config.WriteFile        // files from COPY instructions + the build script itself
	Ports       []config.PortForward      // guest ports declared by EXPOSE
	Healthcheck *config.HealthcheckConfig // optional Dockerfile HEALTHCHECK
}

// Parse reads a Dockerfile and converts it into cloud-init artifacts.
// COPY sources are resolved relative to contextDir.
func Parse(path string, contextDir string) (*Result, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open dockerfile: %w", err)
	}
	defer f.Close()

	result, err := parse(f, contextDir)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// ParseContent converts an inline Dockerfile body into cloud-init artifacts.
// COPY sources are resolved relative to contextDir.
func ParseContent(content string, contextDir string) (*Result, error) {
	return parse(strings.NewReader(content), contextDir)
}

func parse(r io.Reader, contextDir string) (*Result, error) {
	lines, err := joinContinuations(bufio.NewScanner(r))
	if err != nil {
		return nil, fmt.Errorf("read dockerfile: %w", err)
	}
	result := &Result{}
	var script strings.Builder
	script.WriteString(buildScriptPrelude)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		cmd, args := splitInstruction(line)

		switch cmd {
		case instructionFrom:
			result.FromImage = parseFrom(args)

		case instructionRun:
			script.WriteString("\n")
			script.WriteString(parseRun(args))
			script.WriteString("\n")

		case instructionAdd:
			wf, err := parseAdd(args, contextDir)
			if err != nil {
				return nil, fmt.Errorf("%s %s: %w", cmd, args, err)
			}
			result.WriteFiles = append(result.WriteFiles, wf)

		case instructionCopy:
			wf, err := parseCopy(args, contextDir)
			if err != nil {
				return nil, fmt.Errorf("%s %s: %w", cmd, args, err)
			}
			result.WriteFiles = append(result.WriteFiles, wf)

		case instructionEnv:
			for _, pair := range parseEnv(args) {
				fmt.Fprintf(&script, "export %s=%s\n", pair[0], shellQuote(pair[1]))
			}

		case instructionExpose:
			ports, err := parseExpose(args)
			if err != nil {
				return nil, fmt.Errorf("%s %s: %w", cmd, args, err)
			}
			for i := range ports {
				ports[i].Name = exposeName(len(result.Ports) + i)
			}
			result.Ports = append(result.Ports, ports...)

		case instructionWorkdir:
			dir := strings.TrimSpace(args)
			quoted := shellQuote(dir)
			fmt.Fprintf(&script, "mkdir -p %s && cd %s\n", quoted, quoted)

		case instructionHealth:
			healthcheck, err := parseHealthcheck(args)
			if err != nil {
				return nil, err
			}
			result.Healthcheck = healthcheck

		default:
			return nil, unsupportedInstructionError(cmd)
		}
	}

	result.Script = script.String()

	result.WriteFiles = append(result.WriteFiles, buildScriptWriteFile(result.Script))

	return result, nil
}

func buildScriptWriteFile(script string) config.WriteFile {
	return config.WriteFile{
		Path:        buildScriptPath,
		Content:     script,
		Permissions: buildScriptPermissions,
		Owner:       buildScriptOwner,
	}
}
