package dockerfile

import "strings"

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	for _, ch := range s {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') || ch == '_' || ch == '-' ||
			ch == '.' || ch == '/' || ch == ':') {
			return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
		}
	}
	return s
}
