package compose

import (
	"strings"
)

const (
	hostnameDomainSeparator = "."
	pciDefaultDomainPrefix  = "0000:"
	pciShortAddressColons   = 1
)

func composeHostname(hostname, domainName string) string {
	if hostname == "" {
		return ""
	}
	if domainName == "" || strings.Contains(hostname, hostnameDomainSeparator) {
		return hostname
	}
	return hostname + hostnameDomainSeparator + domainName
}

func resolveServiceHostname(svc Service) string {
	if svc.CloudInit.Hostname != "" {
		return svc.CloudInit.Hostname
	}
	return composeHostname(svc.Hostname, svc.DomainName)
}

func normalizePCIAddress(addr string) string {
	if strings.Count(addr, ":") == pciShortAddressColons {
		return pciDefaultDomainPrefix + addr
	}
	return addr
}
