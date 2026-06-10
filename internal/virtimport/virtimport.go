// Package virtimport converts libvirt domain XML into holos compose
// services so existing virsh-defined VMs can be brought under holos
// without hand-translating every field.
//
// The mapping is intentionally lossy: libvirt expresses things holos
// has no concept of (multiple disks, bridged networks, custom emulator
// binaries, NUMA topology, etc.). Anything we can't translate cleanly
// becomes a warning rather than a silent omission, so the operator
// knows what to review before `holos up`.
//
// The resulting compose.Service is meant to be a starting point. The
// caller is expected to round-trip it through yaml.Marshal and let the
// user review it before committing.
package virtimport

import (
	"encoding/xml"
	"fmt"

	"github.com/zeroecco/holos/internal/compose"
)

// ImportedVolume describes an extra libvirt file disk converted into a holos
// named volume.
type ImportedVolume struct {
	Name       string
	SourcePath string
}

// ImportedNetwork describes a libvirt NIC preserved as Compose network
// metadata that can resolve to a bridge or tap-backed runtime network when the
// host bridge is configured.
type ImportedNetwork struct {
	Name   string
	Type   string
	Source string
	Model  string
}

// Convert turns one libvirt domain XML blob into a compose.Service plus
// a sanitised service name and a list of human-readable warnings about
// anything we couldn't translate. The error return is reserved for
// XML parse failures; lossy conversions are reported via warnings so
// the operator can review them and decide whether they matter.
func Convert(xmlBytes []byte) (name string, svc compose.Service, warnings []string, err error) {
	name, svc, _, warnings, err = ConvertWithVolumes(xmlBytes)
	return name, svc, warnings, err
}

// ConvertWithVolumes is like Convert but also returns top-level named-volume
// declarations needed for extra imported file disks.
func ConvertWithVolumes(xmlBytes []byte) (name string, svc compose.Service, volumes []ImportedVolume, warnings []string, err error) {
	name, svc, volumes, _, warnings, err = ConvertWithResources(xmlBytes)
	return name, svc, volumes, warnings, err
}

// ConvertWithResources returns every top-level Compose resource needed by the
// imported service.
func ConvertWithResources(xmlBytes []byte) (name string, svc compose.Service, volumes []ImportedVolume, networks []ImportedNetwork, warnings []string, err error) {
	var d Domain
	if err := xml.Unmarshal(xmlBytes, &d); err != nil {
		return "", compose.Service{}, nil, nil, nil, fmt.Errorf("parse libvirt xml: %w", err)
	}

	name = sanitizeName(d.Name)
	if name == "" {
		return "", compose.Service{}, nil, nil, nil, fmt.Errorf("domain has no usable name")
	}
	if name != d.Name {
		warnings = append(warnings, renamedDomainWarning(d.Name, name))
	}

	applyDomainVMConfig(&svc, d)
	importedVolumes, diskWarnings := applyDomainDisks(name, &svc, d)
	volumes = append(volumes, importedVolumes...)
	warnings = append(warnings, diskWarnings...)
	warnings = append(warnings, applyDomainHostDevices(&svc, d)...)
	importedNetworks, networkWarnings := applyDomainInterfaces(name, &svc, d)
	networks = append(networks, importedNetworks...)
	warnings = append(warnings, networkWarnings...)

	return name, svc, volumes, networks, warnings, nil
}

func renamedDomainWarning(original, sanitized string) string {
	return fmt.Sprintf("renamed domain %q to %q to satisfy compose naming rules", original, sanitized)
}
