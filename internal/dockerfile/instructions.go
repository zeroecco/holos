package dockerfile

import (
	"bufio"
	"encoding/json"
	"strings"
)

const (
	instructionSeparator = " "
	lineContinuation     = "\\"
	lineTrimCutset       = " \t"
)

const (
	instructionFrom    = "FROM"
	instructionRun     = "RUN"
	instructionCopy    = "COPY"
	instructionEnv     = "ENV"
	instructionWorkdir = "WORKDIR"
)

var supportedInstructionNames = []string{
	instructionFrom,
	instructionRun,
	instructionCopy,
	instructionEnv,
	instructionWorkdir,
}

// joinContinuations merges backslash-continued lines.
func joinContinuations(scanner *bufio.Scanner) ([]string, error) {
	var lines []string
	var buf strings.Builder
	for scanner.Scan() {
		text := scanner.Text()
		trimmed := strings.TrimRight(text, lineTrimCutset)
		if strings.HasSuffix(trimmed, lineContinuation) {
			buf.WriteString(strings.TrimSuffix(trimmed, lineContinuation))
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
	parts := strings.SplitN(line, instructionSeparator, 2)
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
	// Exec form preserves argv boundaries. Quote each element before
	// dropping it into the generated bash script.
	if strings.HasPrefix(args, "[") {
		var parts []string
		if err := json.Unmarshal([]byte(args), &parts); err == nil {
			quoted := make([]string, len(parts))
			for i, p := range parts {
				quoted[i] = shellQuote(p)
			}
			return strings.Join(quoted, instructionSeparator)
		}
	}
	return args
}
