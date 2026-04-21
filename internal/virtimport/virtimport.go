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
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/zeroecco/holos/internal/compose"
)

// Domain is the subset of the libvirt domain XML schema we map onto
// holos. Fields outside this struct are silently ignored by encoding/xml,
// which is fine: Convert produces warnings for anything we drop on the
// floor that the operator might care about (disks, interfaces, hostdev
// types we don't recognise).
type Domain struct {
	XMLName       xml.Name   `xml:"domain"`
	Type          string     `xml:"type,attr"`
	Name          string     `xml:"name"`
	Memory        Memory     `xml:"memory"`
	CurrentMemory Memory     `xml:"currentMemory"`
	VCPU          VCPU       `xml:"vcpu"`
	OS            OSConfig   `xml:"os"`
	CPU           *CPUConfig `xml:"cpu,omitempty"`
	Devices       Devices    `xml:"devices"`
}

// Memory carries libvirt's <memory unit="KiB">N</memory> form.
type Memory struct {
	Unit  string `xml:"unit,attr"`
	Value string `xml:",chardata"`
}

// VCPU carries libvirt's <vcpu placement="static">N</vcpu> form.
type VCPU struct {
	Placement string `xml:"placement,attr,omitempty"`
	Value     int    `xml:",chardata"`
}

// OSConfig is the <os> block. Loader presence is how we detect UEFI.
type OSConfig struct {
	Type   OSType  `xml:"type"`
	Loader *Loader `xml:"loader,omitempty"`
}

// OSType is the <type arch=... machine=...>hvm</type> element.
type OSType struct {
	Arch    string `xml:"arch,attr,omitempty"`
	Machine string `xml:"machine,attr,omitempty"`
	Value   string `xml:",chardata"`
}

// Loader is OVMF or similar firmware path. Its presence implies UEFI.
type Loader struct {
	ReadOnly string `xml:"readonly,attr,omitempty"`
	Type     string `xml:"type,attr,omitempty"`
	Path     string `xml:",chardata"`
}

// CPUConfig is the <cpu> element. Mode == "host-passthrough" or
// "host-model" both map to holos's cpu_model: host shorthand.
type CPUConfig struct {
	Mode  string    `xml:"mode,attr,omitempty"`
	Match string    `xml:"match,attr,omitempty"`
	Model *CPUModel `xml:"model,omitempty"`
}

// CPUModel is the named CPU model, e.g. "Skylake-Client-IBRS".
type CPUModel struct {
	Fallback string `xml:"fallback,attr,omitempty"`
	Value    string `xml:",chardata"`
}

// Devices wraps the <devices> block's three children we look at.
type Devices struct {
	Disks      []Disk      `xml:"disk"`
	Interfaces []Interface `xml:"interface"`
	HostDevs   []HostDev   `xml:"hostdev"`
}

// Disk models <disk>. We only honour file-backed disks of device="disk".
type Disk struct {
	Type   string      `xml:"type,attr"`
	Device string      `xml:"device,attr"`
	Driver *DiskDriver `xml:"driver,omitempty"`
	Source DiskSource  `xml:"source"`
	Target DiskTarget  `xml:"target"`
}

// DiskDriver gives us the on-disk format (qcow2/raw).
type DiskDriver struct {
	Name string `xml:"name,attr,omitempty"`
	Type string `xml:"type,attr,omitempty"`
}

// DiskSource holds whichever location attribute matches Disk.Type.
type DiskSource struct {
	File   string `xml:"file,attr,omitempty"`
	Dev    string `xml:"dev,attr,omitempty"`
	Pool   string `xml:"pool,attr,omitempty"`
	Volume string `xml:"volume,attr,omitempty"`
}

// DiskTarget is mostly informational; we use the dev name in warnings.
type DiskTarget struct {
	Dev string `xml:"dev,attr,omitempty"`
	Bus string `xml:"bus,attr,omitempty"`
}

// Interface is <interface>. holos has its own internal multicast
// network so we never import these directly. We just describe them
// in a warning so the operator knows where to add `ports:`.
type Interface struct {
	Type   string          `xml:"type,attr,omitempty"`
	Source *InterfaceSrc   `xml:"source,omitempty"`
	Model  *InterfaceModel `xml:"model,omitempty"`
}

// InterfaceSrc captures whichever of network/bridge/dev is set.
type InterfaceSrc struct {
	Network string `xml:"network,attr,omitempty"`
	Bridge  string `xml:"bridge,attr,omitempty"`
	Dev     string `xml:"dev,attr,omitempty"`
}

// InterfaceModel is <model type="virtio"/>.
type InterfaceModel struct {
	Type string `xml:"type,attr,omitempty"`
}

// HostDev is a passthrough device. Only PCI maps cleanly to holos.
type HostDev struct {
	Mode   string        `xml:"mode,attr,omitempty"`
	Type   string        `xml:"type,attr,omitempty"`
	Source HostDevSource `xml:"source"`
}

// HostDevSource pulls the PCI address out of <source><address .../></source>.
type HostDevSource struct {
	Address *PCIAddress `xml:"address,omitempty"`
}

// PCIAddress holds a libvirt-formatted (hex, 0x-prefixed) PCI address.
type PCIAddress struct {
	Domain   string `xml:"domain,attr,omitempty"`
	Bus      string `xml:"bus,attr,omitempty"`
	Slot     string `xml:"slot,attr,omitempty"`
	Function string `xml:"function,attr,omitempty"`
}

// Convert turns one libvirt domain XML blob into a compose.Service plus
// a sanitised service name and a list of human-readable warnings about
// anything we couldn't translate. The error return is reserved for
// XML parse failures; lossy conversions are reported via warnings so
// the operator can review them and decide whether they matter.
func Convert(xmlBytes []byte) (name string, svc compose.Service, warnings []string, err error) {
	var d Domain
	if err := xml.Unmarshal(xmlBytes, &d); err != nil {
		return "", compose.Service{}, nil, fmt.Errorf("parse libvirt xml: %w", err)
	}

	name = sanitizeName(d.Name)
	if name == "" {
		return "", compose.Service{}, nil, fmt.Errorf("domain has no usable name")
	}
	if name != d.Name {
		warnings = append(warnings, fmt.Sprintf("renamed domain %q to %q to satisfy compose naming rules", d.Name, name))
	}

	// Memory: prefer <currentMemory> (the live ceiling) and fall back
	// to <memory> (the boot-time max) so a guest with ballooning
	// configured doesn't end up with the wrong value.
	memBytes, memErr := memoryToBytes(d.CurrentMemory)
	if memErr != nil || memBytes == 0 {
		memBytes, memErr = memoryToBytes(d.Memory)
	}
	if memErr == nil && memBytes > 0 {
		svc.VM.MemoryMB = int(memBytes / (1 << 20))
	}

	if d.VCPU.Value > 0 {
		svc.VM.VCPU = d.VCPU.Value
	}

	if m := d.OS.Type.Machine; m != "" {
		svc.VM.Machine = simplifyMachine(m)
	}

	if d.CPU != nil {
		switch d.CPU.Mode {
		case "host-passthrough", "host-model":
			svc.VM.CPUModel = "host"
		default:
			if d.CPU.Model != nil && strings.TrimSpace(d.CPU.Model.Value) != "" {
				svc.VM.CPUModel = strings.TrimSpace(d.CPU.Model.Value)
			}
		}
	}

	if d.OS.Loader != nil && strings.TrimSpace(d.OS.Loader.Path) != "" {
		svc.VM.UEFI = true
	}

	primaryFound := false
	for _, disk := range d.Devices.Disks {
		if disk.Device != "" && disk.Device != "disk" {
			// CDROMs, floppies. Silently ignored, they're
			// rarely what someone wants to import.
			continue
		}
		if disk.Type != "" && disk.Type != "file" {
			warnings = append(warnings, fmt.Sprintf(
				"disk %q has type %q (only file-backed disks are imported)",
				disk.Target.Dev, disk.Type))
			continue
		}
		path := strings.TrimSpace(disk.Source.File)
		if path == "" {
			continue
		}
		if !primaryFound {
			svc.Image = path
			if disk.Driver != nil && disk.Driver.Type != "" {
				svc.ImageFormat = disk.Driver.Type
			}
			primaryFound = true
			continue
		}
		warnings = append(warnings, fmt.Sprintf(
			"extra disk %q skipped; declare it under top-level volumes: and reference it from the service",
			path))
	}
	if !primaryFound {
		warnings = append(warnings, "no file-backed disk found; set image: by hand before running `holos up`")
	}

	for _, hd := range d.Devices.HostDevs {
		if hd.Type != "pci" {
			warnings = append(warnings, fmt.Sprintf("hostdev type %q is not supported (only pci passthrough imports)", hd.Type))
			continue
		}
		if hd.Source.Address == nil {
			continue
		}
		svc.Devices = append(svc.Devices, compose.ComposeDevice{
			PCI: formatPCI(*hd.Source.Address),
		})
	}

	for _, iface := range d.Devices.Interfaces {
		desc := describeInterface(iface)
		warnings = append(warnings, fmt.Sprintf(
			"interface %s not imported. holos services share an internal subnet; expose with ports: instead",
			desc))
	}

	return name, svc, warnings, nil
}

// describeInterface produces a short human-readable fragment for warnings.
func describeInterface(iface Interface) string {
	parts := []string{}
	if iface.Type != "" {
		parts = append(parts, "type="+iface.Type)
	}
	if iface.Source != nil {
		switch {
		case iface.Source.Network != "":
			parts = append(parts, "network="+iface.Source.Network)
		case iface.Source.Bridge != "":
			parts = append(parts, "bridge="+iface.Source.Bridge)
		case iface.Source.Dev != "":
			parts = append(parts, "dev="+iface.Source.Dev)
		}
	}
	if len(parts) == 0 {
		return "(unspecified)"
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

// memoryToBytes converts a libvirt Memory value into bytes. libvirt's
// default unit is KiB when the attribute is missing, which matches the
// libvirt manual.
func memoryToBytes(m Memory) (int64, error) {
	s := strings.TrimSpace(m.Value)
	if s == "" {
		return 0, fmt.Errorf("empty memory value")
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid memory value %q: %w", s, err)
	}
	switch strings.ToLower(strings.TrimSpace(m.Unit)) {
	case "", "kib", "k", "kb":
		return v * (1 << 10), nil
	case "b", "bytes":
		return v, nil
	case "mib", "m", "mb":
		return v * (1 << 20), nil
	case "gib", "g", "gb":
		return v * (1 << 30), nil
	case "tib", "t", "tb":
		return v * (1 << 40), nil
	default:
		return 0, fmt.Errorf("unknown memory unit %q", m.Unit)
	}
}

// simplifyMachine collapses libvirt's versioned machine names ("pc-q35-7.2")
// to the family name ("q35") that holos actually uses; everything else
// passes through unchanged so exotic boards still work.
func simplifyMachine(m string) string {
	switch {
	case m == "q35", strings.HasPrefix(m, "pc-q35"):
		return "q35"
	case m == "pc", strings.HasPrefix(m, "pc-i440fx"):
		return "pc"
	default:
		return m
	}
}

var nameSanitizer = regexp.MustCompile(`[^a-z0-9-]+`)

// sanitizeName lower-cases the libvirt domain name and replaces
// disallowed characters with '-' so the result satisfies compose's
// DNS-label constraint. Truncates to 63 chars to match the same limit.
func sanitizeName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = nameSanitizer.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 63 {
		s = s[:63]
		s = strings.TrimRight(s, "-")
	}
	return s
}

// formatPCI re-renders a libvirt PCI address into the canonical
// "DDDD:BB:SS.F" form holos expects (lower-case hex, no 0x prefixes).
func formatPCI(a PCIAddress) string {
	parse := func(s string) int64 {
		s = strings.TrimSpace(s)
		s = strings.TrimPrefix(strings.ToLower(s), "0x")
		v, _ := strconv.ParseInt(s, 16, 64)
		return v
	}
	return fmt.Sprintf("%04x:%02x:%02x.%x",
		parse(a.Domain), parse(a.Bus), parse(a.Slot), parse(a.Function))
}

// Virsh is a thin wrapper around the virsh CLI used to fetch domain
// XML. Tests substitute their own implementation by passing fixture
// XML directly to Convert; this struct only matters at the CLI layer.
type Virsh struct {
	// Binary is the path to the virsh executable. Empty means look
	// up "virsh" on PATH.
	Binary string
	// URI is passed as `-c <uri>` to virsh. Empty means use the
	// libvirt default (qemu:///system for root, qemu:///session for
	// regular users).
	URI string
}

// DumpXML returns the raw XML for a single domain.
func (v Virsh) DumpXML(domain string) ([]byte, error) {
	out, err := v.run("dumpxml", domain)
	if err != nil {
		return nil, fmt.Errorf("virsh dumpxml %s: %w", domain, err)
	}
	return out, nil
}

// ListDomains returns the names of every defined domain (running or
// shut off). It uses --name so the output is one bare name per line
// without status columns to parse around.
func (v Virsh) ListDomains() ([]string, error) {
	out, err := v.run("list", "--all", "--name")
	if err != nil {
		return nil, fmt.Errorf("virsh list: %w", err)
	}
	var names []string
	for _, line := range strings.Split(string(out), "\n") {
		if name := strings.TrimSpace(line); name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}

func (v Virsh) run(args ...string) ([]byte, error) {
	bin := v.Binary
	if bin == "" {
		bin = "virsh"
	}
	full := []string{}
	if v.URI != "" {
		full = append(full, "-c", v.URI)
	}
	full = append(full, args...)
	cmd := exec.Command(bin, full...)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && len(exitErr.Stderr) > 0 {
			return nil, fmt.Errorf("%s: %s", err, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, err
	}
	return out, nil
}
