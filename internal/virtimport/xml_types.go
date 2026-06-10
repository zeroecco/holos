package virtimport

import "encoding/xml"

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
