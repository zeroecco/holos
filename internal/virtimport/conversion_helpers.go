package virtimport

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

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
	multiplier, ok := memoryUnitMultiplier(m.Unit)
	if !ok {
		return 0, fmt.Errorf("unknown memory unit %q", m.Unit)
	}
	return v * multiplier, nil
}

func memoryUnitMultiplier(unit string) (int64, bool) {
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "", "kib", "k", "kb":
		return 1 << 10, true
	case "b", "bytes":
		return 1, true
	case "mib", "m", "mb":
		return 1 << 20, true
	case "gib", "g", "gb":
		return 1 << 30, true
	case "tib", "t", "tb":
		return 1 << 40, true
	default:
		return 0, false
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

func cpuModelName(cpu *CPUConfig) string {
	if cpu == nil {
		return ""
	}
	switch cpu.Mode {
	case "host-passthrough", "host-model":
		return "host"
	default:
		if cpu.Model == nil {
			return ""
		}
		return strings.TrimSpace(cpu.Model.Value)
	}
}

func domainDiskImagePath(disk Disk) (path string, warning string, ok bool) {
	if disk.Device != "" && disk.Device != "disk" {
		// CDROMs, floppies. Silently ignored, they're rarely what someone
		// wants to import.
		return "", "", false
	}
	if disk.Type != "" && disk.Type != "file" {
		return "", fmt.Sprintf(
			"disk %q has type %q (only file-backed disks are imported)",
			disk.Target.Dev, disk.Type), false
	}
	path = strings.TrimSpace(disk.Source.File)
	return path, "", path != ""
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
