package compose

import "strings"

func (c ComposeCommand) shellFragment() string {
	if c.Scalar {
		return c.Args[0]
	}
	return shellJoin(c.Args)
}

func shellJoin(args []string) string {
	return strings.Join(shellQuoteArgs(args), " ")
}

func shellQuoteArgs(args []string) []string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = shellQuote(arg)
	}
	return quoted
}

func shellQuote(arg string) string {
	if arg == "" {
		return "''"
	}
	for _, r := range arg {
		if !((r >= 'A' && r <= 'Z') ||
			(r >= 'a' && r <= 'z') ||
			(r >= '0' && r <= '9') ||
			strings.ContainsRune("_+-=/:.,@", r)) {
			return "'" + strings.ReplaceAll(arg, "'", "'\"'\"'") + "'"
		}
	}
	return arg
}
