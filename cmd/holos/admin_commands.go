package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"

	"github.com/zeroecco/holos/internal/compose"
)

const (
	buildVCSRevisionKey    = "vcs.revision"
	buildVCSTimeKey        = "vcs.time"
	buildVCSModifiedKey    = "vcs.modified"
	buildCommitPlaceholder = "none"
	buildDatePlaceholder   = "unknown"
	buildModifiedTrue      = "true"
	buildDirtySuffix       = "-dirty"
)

func runValidate(args []string) error {
	flags := newFlagSet("validate")
	projectFlags := addProjectFlags(flags, "")
	capacity := flags.Bool("capacity", false, "fail if the project requests more host CPU or memory than available")
	network := flags.Bool("network", false, "check host bridges required by bridge and tap networks")
	if err := flags.Parse(args); err != nil {
		return err
	}

	project, err := loadProject(*projectFlags.filePath, *projectFlags.stateDir)
	if err != nil {
		return err
	}
	if *capacity {
		if err := validateProjectCapacity(project); err != nil {
			return err
		}
	}
	if *network {
		if err := validateProjectNetwork(project); err != nil {
			return err
		}
	}

	return writeValidateReport(os.Stdout, project)
}

func validateProjectNetwork(project *compose.Project) error {
	if err := checkMulticastPort(project.Network.MulticastPort); err != nil {
		return fmt.Errorf("internal network: %w", err)
	}
	for name, segment := range project.Network.Segments {
		if err := checkMulticastPort(segment.MulticastPort); err != nil {
			return fmt.Errorf("network %q: %w", name, err)
		}
		if (segment.Backend != "bridge" && segment.Backend != "tap") || segment.BridgeName == "" {
			continue
		}
		path := filepath.Join("/sys/class/net", segment.BridgeName)
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("network %q requires host bridge %q: %w", name, segment.BridgeName, err)
		}
		if segment.Backend == "bridge" {
			if err := checkBridgeHelper(segment.BridgeName); err != nil {
				return fmt.Errorf("network %q: %w", name, err)
			}
		}
		if segment.Backend == "tap" {
			if _, err := exec.LookPath("ip"); err != nil {
				return fmt.Errorf("network %q uses tap backend but ip is not installed", name)
			}
			tun, err := os.OpenFile("/dev/net/tun", os.O_RDWR, 0)
			if err != nil {
				return fmt.Errorf("network %q uses tap backend but /dev/net/tun is unavailable or inaccessible", name)
			}
			_ = tun.Close()
		}
	}
	return nil
}

func checkMulticastPort(port int) error {
	if port <= 0 {
		return nil
	}
	listener, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: port})
	if err != nil {
		return fmt.Errorf("multicast port %d is unavailable: %w", port, err)
	}
	return listener.Close()
}

func checkBridgeHelper(bridge string) error {
	paths := []string{"/usr/lib/qemu/qemu-bridge-helper", "/usr/libexec/qemu-bridge-helper", "/usr/lib/x86_64-linux-gnu/qemu/qemu-bridge-helper"}
	var helper string
	for _, path := range paths {
		if info, err := os.Stat(path); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			helper = path
			break
		}
	}
	if helper == "" {
		return fmt.Errorf("qemu-bridge-helper is not installed or executable")
	}
	config, err := os.ReadFile("/etc/qemu/bridge.conf")
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("/etc/qemu/bridge.conf is missing; allow bridge %q for unprivileged QEMU", bridge)
		}
		return fmt.Errorf("read /etc/qemu/bridge.conf: %w", err)
	}
	for _, line := range strings.Split(string(config), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "allow" && (fields[1] == bridge || fields[1] == "all") {
			return nil
		}
	}
	return fmt.Errorf("/etc/qemu/bridge.conf does not allow bridge %q", bridge)
}

func validateProjectCapacity(project *compose.Project) error {
	var vcpu, memoryMB int
	for _, manifest := range project.Services {
		vcpu += manifest.VM.VCPU * manifest.Replicas
		memoryMB += manifest.VM.MemoryMB * manifest.Replicas
	}
	hostCPU, hostMemoryMB, memoryOK := hostCapacity()
	if float64(vcpu) > hostCPU {
		return fmt.Errorf("project requests %d vCPUs across replicas, host capacity is %.2f CPUs", vcpu, hostCPU)
	}
	if memoryOK && memoryMB > hostMemoryMB {
		return fmt.Errorf("project requests %dMB memory across replicas, host reports %dMB", memoryMB, hostMemoryMB)
	}
	return nil
}

var capacityCgroupRoot = "/sys/fs/cgroup"
var capacityMeminfoPath = "/proc/meminfo"
var capacityCgroupPath = "/proc/self/cgroup"

func hostCapacity() (cpu float64, memoryMB int, memoryOK bool) {
	cpu = float64(goruntime.NumCPU())
	cgroupRoot := effectiveCgroupRoot(capacityCgroupRoot)
	if quota, ok := cgroupCPUQuota(cgroupRoot); ok && quota < cpu {
		cpu = quota
	}
	memoryMB, memoryOK = hostMemoryMB()
	if limit, ok := cgroupMemoryLimit(cgroupRoot); ok && (!memoryOK || limit < memoryMB) {
		memoryMB, memoryOK = limit, true
	}
	return cpu, memoryMB, memoryOK
}

func effectiveCgroupRoot(root string) string {
	if root != "/sys/fs/cgroup" {
		return root
	}
	data, err := os.ReadFile(capacityCgroupPath)
	if err != nil {
		return root
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.SplitN(line, ":", 3)
		if len(fields) == 3 && fields[0] == "0" && fields[1] == "" {
			return filepath.Join(root, fields[2])
		}
	}
	return root
}

func cgroupCPUQuota(root string) (float64, bool) {
	data, err := os.ReadFile(filepath.Join(root, "cpu.max"))
	if err == nil {
		fields := strings.Fields(string(data))
		if len(fields) >= 2 && fields[0] != "max" {
			quota, qErr := strconv.ParseFloat(fields[0], 64)
			period, pErr := strconv.ParseFloat(fields[1], 64)
			if qErr == nil && pErr == nil && quota > 0 && period > 0 {
				return quota / period, true
			}
		}
	}
	quotaData, qErr := os.ReadFile(filepath.Join(root, "cpu", "cpu.cfs_quota_us"))
	periodData, pErr := os.ReadFile(filepath.Join(root, "cpu", "cpu.cfs_period_us"))
	if qErr != nil || pErr != nil {
		return 0, false
	}
	quota, qErr := strconv.ParseFloat(strings.TrimSpace(string(quotaData)), 64)
	period, pErr := strconv.ParseFloat(strings.TrimSpace(string(periodData)), 64)
	if qErr != nil || pErr != nil || quota <= 0 || period <= 0 {
		return 0, false
	}
	return quota / period, true
}

func cgroupMemoryLimit(root string) (int, bool) {
	paths := []string{filepath.Join(root, "memory.max"), filepath.Join(root, "memory", "memory.limit_in_bytes")}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil || strings.TrimSpace(string(data)) == "max" {
			continue
		}
		bytes, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
		if err == nil && bytes > 0 {
			return int(bytes / (1024 * 1024)), true
		}
	}
	return 0, false
}

func hostMemoryMB() (int, bool) {
	data, err := os.ReadFile(capacityMeminfoPath)
	if err != nil {
		return 0, false
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "MemTotal:" {
			continue
		}
		kb, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return 0, false
		}
		return int(kb / 1024), true
	}
	return 0, false
}

func writeValidateReport(output io.Writer, project *compose.Project) error {
	fmt.Fprintf(output, "project: %s\n", project.Name)
	fmt.Fprintf(output, "spec_hash: %s\n", project.SpecHash)
	fmt.Fprintf(output, "services: %d\n", len(project.Services))
	fmt.Fprintf(output, "order: %v\n\n", project.ServiceOrder)

	writer := newTableWriter(output)
	fmt.Fprintln(writer, "SERVICE\tIMAGE\tREPLICAS\tVCPU\tMEMORY")
	for _, name := range project.ServiceOrder {
		m := project.Services[name]
		fmt.Fprintf(writer, "%s\t%s\t%d\t%d\t%dMB\n",
			name,
			filepath.Base(m.Image),
			m.Replicas,
			m.VM.VCPU,
			m.VM.MemoryMB,
		)
	}
	if err := writer.Flush(); err != nil {
		return err
	}

	fmt.Fprintf(output, "\nnetwork: %s (mcast %s:%d)\n",
		project.Network.Subnet,
		project.Network.MulticastGroup,
		project.Network.MulticastPort,
	)
	if len(project.Network.Segments) > 1 {
		fmt.Fprintln(output, "segments:")
		for _, name := range sortedNetworkSegmentNames(project.Network.Segments) {
			segment := project.Network.Segments[name]
			fmt.Fprintf(output, "  %s: %s (mcast %s:%d%s)\n",
				name,
				segment.Subnet,
				segment.MulticastGroup,
				segment.MulticastPort,
				networkSegmentBackendSuffix(segment),
			)
		}
	}
	fmt.Fprintln(output, "hosts:")
	for host, ip := range project.Network.Hosts {
		fmt.Fprintf(output, "  %s -> %s\n", host, ip)
	}
	return nil
}

func networkSegmentBackendSuffix(segment compose.NetworkSegmentPlan) string {
	if segment.Backend == "bridge" && segment.BridgeName != "" {
		return ", bridge " + segment.BridgeName
	}
	if segment.Backend == "tap" && segment.BridgeName != "" {
		return ", managed tap -> " + segment.BridgeName
	}
	return ""
}

func sortedNetworkSegmentNames(segments map[string]compose.NetworkSegmentPlan) []string {
	names := make([]string, 0, len(segments))
	for name := range segments {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func runVersion(args []string) error {
	flags := newFlagSet("version")
	short := flags.Bool("short", false, "print only the version string")
	if err := flags.Parse(args); err != nil {
		return err
	}

	build := newBuildVersion(version, commit, date)
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			build.applySetting(s.Key, s.Value)
		}
	}

	if *short {
		writeVersion(os.Stdout, build, true, goruntime.Version(), goruntime.GOOS, goruntime.GOARCH)
		return nil
	}
	writeVersion(os.Stdout, build, false, goruntime.Version(), goruntime.GOOS, goruntime.GOARCH)
	return nil
}

func writeVersion(output io.Writer, build buildVersion, short bool, goVersion, goos, goarch string) {
	if short {
		fmt.Fprintln(output, build.version)
		return
	}
	fmt.Fprintf(output, "holos %s\n", build.version)
	fmt.Fprintf(output, "  commit: %s\n", build.commit)
	fmt.Fprintf(output, "  built:  %s\n", build.date)
	fmt.Fprintf(output, "  go:     %s\n", goVersion)
	fmt.Fprintf(output, "  os/arch: %s/%s\n", goos, goarch)
}

type buildVersion struct {
	version string
	commit  string
	date    string
}

func newBuildVersion(version, commit, date string) buildVersion {
	return buildVersion{
		version: version,
		commit:  commit,
		date:    date,
	}
}

func (v *buildVersion) applySetting(key, value string) {
	switch key {
	case buildVCSRevisionKey:
		if buildMetadataCanAdopt(v.commit, buildCommitPlaceholder, value) {
			v.commit = value
		}
	case buildVCSTimeKey:
		if buildMetadataCanAdopt(v.date, buildDatePlaceholder, value) {
			v.date = value
		}
	case buildVCSModifiedKey:
		if buildVersionShouldAppendDirty(v.commit, value) {
			v.commit += buildDirtySuffix
		}
	}
}

func buildMetadataCanAdopt(current, placeholder, candidate string) bool {
	return current == placeholder && candidate != ""
}

func buildVersionShouldAppendDirty(commit, modified string) bool {
	return modified == buildModifiedTrue && commit != buildCommitPlaceholder && !strings.HasSuffix(commit, buildDirtySuffix)
}
