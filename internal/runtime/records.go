package runtime

import (
	"fmt"
	"strings"
	"time"

	"github.com/zeroecco/holos/internal/qemu"
)

const (
	InstanceStatusRunning = "running"
	InstanceStatusStopped = "stopped"
	defaultHostAddr       = "127.0.0.1"
	noPortsSummary        = "-"
	portSummaryFormat     = "%s:%d->%s%d/%s"
	portSummarySeparator  = ","
	portAddressSeparator  = ":"
)

// ProjectRecord is the on-disk JSON state for a running or stopped project.
type ProjectRecord struct {
	Name      string          `json:"name"`
	SpecHash  string          `json:"spec_hash"`
	Services  []ServiceRecord `json:"services"`
	Volumes   []VolumeRecord  `json:"volumes,omitempty"`
	Network   NetworkState    `json:"network"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// VolumeRecord tracks the declared size of a persistent named volume.
type VolumeRecord struct {
	Name      string `json:"name"`
	SizeBytes int64  `json:"size_bytes"`
}

// NetworkState records the internal network configuration for a project.
type NetworkState struct {
	MulticastGroup string            `json:"multicast_group"`
	MulticastPort  int               `json:"multicast_port"`
	Subnet         string            `json:"subnet"`
	Hosts          map[string]string `json:"hosts"`
}

// ServiceRecord tracks the desired and actual replica count for one service.
type ServiceRecord struct {
	Name            string           `json:"name"`
	DesiredReplicas int              `json:"desired_replicas"`
	Instances       []InstanceRecord `json:"instances"`
	// LoginUser is the cloud-init user the service was resolved
	// with. We persist it here so `holos exec <project>` can pick
	// the right account (debian, alpine, fedora, custom, ...)
	// without needing to locate or re-parse the original compose
	// file. Pre-exec records written by older holos versions leave
	// this empty; callers fall back to lookupLoginUser in that
	// case for graceful upgrade.
	LoginUser       string   `json:"login_user,omitempty"`
	PreStopCommands []string `json:"pre_stop_commands,omitempty"`
}

// InstanceRecord is the persisted state of a single QEMU VM instance,
// including its PID, work directory paths, and port mappings.
type InstanceRecord struct {
	Name               string             `json:"name"`
	Index              int                `json:"index"`
	PID                int                `json:"pid"`
	Status             string             `json:"status"`
	WorkDir            string             `json:"work_dir"`
	OverlayPath        string             `json:"overlay_path"`
	SeedPath           string             `json:"seed_path"`
	LogPath            string             `json:"log_path"`
	SerialPath         string             `json:"serial_path"`
	QMPPath            string             `json:"qmp_path"`
	Ports              []qemu.PortMapping `json:"ports"`
	StopGracePeriodSec int                `json:"stop_grace_period_sec,omitempty"`
	// SSHPort is the host-side forward to the guest's sshd, used by
	// `holos exec`. Zero means no ssh forward was provisioned (e.g.
	// instance records from a pre-exec version of holos).
	SSHPort      int       `json:"ssh_port,omitempty"`
	LastStarted  time.Time `json:"last_started"`
	LastExitTime time.Time `json:"last_exit_time,omitempty"`
}

// RunningCount returns the number of running instances.
func (s *ServiceRecord) RunningCount() int {
	count := 0
	for _, instance := range s.Instances {
		if instance.Status == InstanceStatusRunning {
			count++
		}
	}
	return count
}

// PortSummary returns the first instance's port summary for service-level
// status tables, or "-" when the service has no instances yet.
func (s *ServiceRecord) PortSummary() string {
	instance, ok := servicePortSummaryInstance(s)
	if !ok {
		return noPortsSummary
	}
	return instance.PortSummary()
}

func servicePortSummaryInstance(service *ServiceRecord) (InstanceRecord, bool) {
	if len(service.Instances) == 0 {
		return InstanceRecord{}, false
	}
	return service.Instances[0], true
}

// PortSummary returns a human-readable string like "127.0.0.1:8080->80/tcp"
// for display in status tables, or "-" if no ports are mapped.
func (i InstanceRecord) PortSummary() string {
	if len(i.Ports) == 0 {
		return noPortsSummary
	}
	return portSummaries(i.Ports)
}

func portSummaries(ports []qemu.PortMapping) string {
	parts := make([]string, 0, len(ports))
	for _, port := range ports {
		parts = append(parts, portSummary(port))
	}
	return strings.Join(parts, portSummarySeparator)
}

func portSummary(port qemu.PortMapping) string {
	return fmt.Sprintf(portSummaryFormat, portSummaryHostAddr(port), port.HostPort, portSummaryGuestTarget(port), port.GuestPort, port.Protocol)
}

func portSummaryHostAddr(port qemu.PortMapping) string {
	return effectiveHostAddr(port.HostAddr)
}

func portSummaryGuestTarget(port qemu.PortMapping) string {
	if port.GuestAddr == "" {
		return ""
	}
	return port.GuestAddr + portAddressSeparator
}
