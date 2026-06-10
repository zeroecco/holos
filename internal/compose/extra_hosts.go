package compose

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// ExtraHosts accepts Docker Compose extra_hosts map syntax and list syntax.
// The list form accepts HOST=IP and HOST:IP, matching Compose.
type ExtraHosts map[string]string

const (
	extraHostEqualsSeparator = "="
	extraHostColonSeparator  = ":"
	ipv6OpenBracket          = "["
	ipv6CloseBracket         = "]"
)

func (e *ExtraHosts) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.MappingNode:
		out := make(map[string]string, len(node.Content)/2)
		for i := 0; i < len(node.Content); i += 2 {
			var value string
			if err := node.Content[i+1].Decode(&value); err != nil {
				return err
			}
			out[node.Content[i].Value] = trimBracketedIPv6(value)
		}
		*e = ExtraHosts(out)
		return nil
	case yaml.SequenceNode:
		out := make(map[string]string, len(node.Content))
		for _, item := range node.Content {
			var raw string
			if err := item.Decode(&raw); err != nil {
				return err
			}
			host, addr, err := parseExtraHost(raw)
			if err != nil {
				return err
			}
			out[host] = addr
		}
		*e = ExtraHosts(out)
		return nil
	default:
		return fmt.Errorf("line %d: extra_hosts must be a mapping or list", node.Line)
	}
}

func parseExtraHost(raw string) (string, string, error) {
	host, addr, ok := splitExtraHost(raw)
	if !ok || host == "" || addr == "" {
		return "", "", fmt.Errorf("invalid extra_hosts entry %q: expected host=ip or host:ip", raw)
	}
	return host, trimBracketedIPv6(addr), nil
}

func splitExtraHost(raw string) (host, addr string, ok bool) {
	host, addr, ok = strings.Cut(raw, extraHostEqualsSeparator)
	if ok {
		return host, addr, true
	}
	host, addr, ok = strings.Cut(raw, extraHostColonSeparator)
	if !ok {
		return "", "", false
	}
	return host, addr, true
}

func trimBracketedIPv6(addr string) string {
	if isBracketedIPv6(addr) {
		return strings.TrimSuffix(strings.TrimPrefix(addr, ipv6OpenBracket), ipv6CloseBracket)
	}
	return addr
}

func isBracketedIPv6(addr string) bool {
	return strings.HasPrefix(addr, ipv6OpenBracket) && strings.HasSuffix(addr, ipv6CloseBracket)
}
