package compose

import (
	"fmt"
	"net/netip"
	"strconv"
	"strings"
)

const (
	portRangeSeparator = "-"
	portAddressOpen    = "["
	portAddressClose   = "]"
)

func parsePortRange(raw string) ([]int, error) {
	startRaw, endRaw, hasRange := strings.Cut(raw, portRangeSeparator)
	start, err := strconv.Atoi(startRaw)
	if err != nil {
		return nil, err
	}
	if !hasRange {
		return []int{start}, nil
	}
	end, err := strconv.Atoi(endRaw)
	if err != nil {
		return nil, err
	}
	if err := validatePortRangeBounds(start, end); err != nil {
		return nil, err
	}
	return portRangeValues(start, end), nil
}

func validatePortRangeBounds(start, end int) error {
	if end < start {
		return fmt.Errorf("range end must be >= start")
	}
	return nil
}

func portRangeValues(start, end int) []int {
	out := make([]int, 0, end-start+1)
	for port := start; port <= end; port++ {
		out = append(out, port)
	}
	return out
}

func parsePortAddress(kind, raw string) (string, error) {
	raw = trimPortAddressBrackets(raw)
	addr, err := netip.ParseAddr(raw)
	if err != nil {
		return "", fmt.Errorf("invalid %s address: %w", kind, err)
	}
	if !addr.Is4() {
		return "", fmt.Errorf("invalid %s address %q: only IPv4 addresses are supported", kind, raw)
	}
	return addr.String(), nil
}

func trimPortAddressBrackets(raw string) string {
	return strings.TrimPrefix(strings.TrimSuffix(raw, portAddressClose), portAddressOpen)
}
