package config

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	macOctetCount     = 6
	macOctetSeparator = ":"
	macOctetHexFormat = "%02x"
)

func offsetMAC(base string, index int) string {
	parts := strings.Split(base, macOctetSeparator)
	if len(parts) != macOctetCount {
		return base
	}
	lastOctet := len(parts) - 1
	last, err := strconv.ParseUint(parts[lastOctet], 16, 8)
	if err != nil {
		return base
	}
	parts[lastOctet] = fmt.Sprintf(macOctetHexFormat, byte(last)+byte(index))
	return strings.Join(parts, macOctetSeparator)
}
