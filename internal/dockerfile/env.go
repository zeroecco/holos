package dockerfile

import "strings"

const (
	envLegacySeparator = " "
	envAssignmentByte  = '='
	envQuoteTrimCutset = "\"'"
)

// parseEnv returns key-value pairs from an ENV instruction.
// Supports both modern (ENV KEY=val KEY2=val2) and legacy (ENV KEY val) forms.
func parseEnv(args string) [][2]string {
	args = strings.TrimSpace(args)
	if !strings.ContainsRune(args, envAssignmentByte) {
		parts := strings.SplitN(args, envLegacySeparator, 2)
		if len(parts) == 2 {
			return [][2]string{{parts[0], strings.TrimSpace(parts[1])}}
		}
		return nil
	}

	var pairs [][2]string
	for _, tok := range splitQuoted(args) {
		if idx := strings.IndexByte(tok, envAssignmentByte); idx > 0 {
			key := tok[:idx]
			val := strings.Trim(tok[idx+1:], envQuoteTrimCutset)
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
