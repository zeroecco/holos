package virtimport

import "strings"

const virshDomainLineSeparator = "\n"

func parseVirshDomainNames(out []byte) []string {
	var names []string
	for _, line := range strings.Split(string(out), virshDomainLineSeparator) {
		if name := strings.TrimSpace(line); name != "" {
			names = append(names, name)
		}
	}
	return names
}
