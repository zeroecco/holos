package runtime

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/zeroecco/holos/internal/compose"
	"github.com/zeroecco/holos/internal/config"
	"github.com/zeroecco/holos/internal/qemu"
)

// Manager coordinates project lifecycle and state persistence.
type Manager struct {
	stateDir string
}

// Record types persisted to disk.

// ProjectRecord is the on-disk JSON state for a running or stopped project.
type ProjectRecord struct {
	Name      string          `json:"name"`
	SpecHash  string          `json:"spec_hash"`
	Services  []ServiceRecord `json:"services"`
	Network   NetworkState    `json:"network"`
	UpdatedAt time.Time       `json:"updated_at"`
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
	LastStarted        time.Time          `json:"last_started"`
	LastExitTime       time.Time          `json:"last_exit_time,omitempty"`
}

// NewManager creates a Manager that stores state under the given directory.
func NewManager(stateDir string) *Manager {
	return &Manager{stateDir: stateDir}
}

// DefaultStateDir returns the state directory: HOLOS_STATE_DIR if set,
// /var/lib/holos for root, or ~/.local/state/holos for regular users.
func DefaultStateDir() string {
	if value := os.Getenv("HOLOS_STATE_DIR"); value != "" {
		return value
	}

	if os.Geteuid() == 0 {
		return "/var/lib/holos"
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ".holos"
	}
	return filepath.Join(home, ".local", "state", "holos")
}

// Up brings a compose project to the desired state, starting services
// in topological order.
func (m *Manager) Up(project *compose.Project) (*ProjectRecord, error) {
	if err := m.ensureLayout(); err != nil {
		return nil, err
	}

	record, err := m.loadProject(project.Name)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	if record != nil && record.SpecHash != "" && record.SpecHash != project.SpecHash {
		if err := m.tearDownProject(record); err != nil {
			return nil, err
		}
		record = nil
	}

	if record == nil {
		record = &ProjectRecord{Name: project.Name}
	}

	record.SpecHash = project.SpecHash
	record.Network = NetworkState{
		MulticastGroup: project.Network.MulticastGroup,
		MulticastPort:  project.Network.MulticastPort,
		Subnet:         project.Network.Subnet,
		Hosts:          project.Network.Hosts,
	}

	existingByService := make(map[string]*ServiceRecord)
	for i := range record.Services {
		existingByService[record.Services[i].Name] = &record.Services[i]
	}

	var services []ServiceRecord
	for _, svcName := range project.ServiceOrder {
		manifest := project.Services[svcName]
		existing := existingByService[svcName]

		svcRecord, err := m.reconcileService(project.Name, manifest, existing)
		if err != nil {
			return nil, fmt.Errorf("service %q: %w", svcName, err)
		}
		services = append(services, *svcRecord)
	}

	for name, existing := range existingByService {
		if _, ok := project.Services[name]; !ok {
			m.stopAllInstances(existing.Instances)
			m.removeInstanceDirs(existing.Instances)
		}
	}

	record.Services = services
	record.UpdatedAt = time.Now().UTC()

	if err := m.saveProject(record); err != nil {
		return nil, err
	}
	return record, nil
}

// Down stops and removes all resources for a project.
func (m *Manager) Down(projectName string) error {
	record, err := m.loadProject(projectName)
	if err != nil {
		return err
	}

	if err := m.tearDownProject(record); err != nil {
		return err
	}

	return os.Remove(projectFile(m.stateDir, projectName))
}

// StopProject stops all services without removing state.
func (m *Manager) StopProject(projectName string) (*ProjectRecord, error) {
	record, err := m.loadProject(projectName)
	if err != nil {
		return nil, err
	}

	for i := range record.Services {
		m.stopAllInstances(record.Services[i].Instances)
		for j := range record.Services[i].Instances {
			record.Services[i].Instances[j].Status = "stopped"
			record.Services[i].Instances[j].PID = 0
		}
	}

	record.UpdatedAt = time.Now().UTC()
	if err := m.saveProject(record); err != nil {
		return nil, err
	}
	return record, nil
}

// StopService stops a single service within a project.
func (m *Manager) StopService(projectName, serviceName string) (*ProjectRecord, error) {
	record, err := m.loadProject(projectName)
	if err != nil {
		return nil, err
	}

	found := false
	for i := range record.Services {
		if record.Services[i].Name == serviceName {
			found = true
			m.stopAllInstances(record.Services[i].Instances)
			for j := range record.Services[i].Instances {
				record.Services[i].Instances[j].Status = "stopped"
				record.Services[i].Instances[j].PID = 0
			}
		}
	}

	if !found {
		return nil, fmt.Errorf("service %q not found in project %q", serviceName, projectName)
	}

	record.UpdatedAt = time.Now().UTC()
	if err := m.saveProject(record); err != nil {
		return nil, err
	}
	return record, nil
}

// ProjectStatus returns the current state of a project, refreshing PID liveness.
func (m *Manager) ProjectStatus(projectName string) (*ProjectRecord, error) {
	record, err := m.loadProject(projectName)
	if err != nil {
		return nil, err
	}

	m.refreshProject(record)
	record.UpdatedAt = time.Now().UTC()
	if err := m.saveProject(record); err != nil {
		return nil, err
	}
	return record, nil
}

// ListProjects returns all known projects.
func (m *Manager) ListProjects() ([]*ProjectRecord, error) {
	if err := m.ensureLayout(); err != nil {
		return nil, err
	}

	matches, err := filepath.Glob(filepath.Join(projectsDir(m.stateDir), "*.json"))
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}

	projects := make([]*ProjectRecord, 0, len(matches))
	for _, match := range matches {
		payload, err := os.ReadFile(match)
		if err != nil {
			return nil, fmt.Errorf("read project %s: %w", match, err)
		}
		var record ProjectRecord
		if err := json.Unmarshal(payload, &record); err != nil {
			return nil, fmt.Errorf("decode project %s: %w", match, err)
		}
		m.refreshProject(&record)
		projects = append(projects, &record)
	}

	sort.Slice(projects, func(i, j int) bool {
		return projects[i].Name < projects[j].Name
	})
	return projects, nil
}

// RunningCount returns the number of instances with status "running".
func (s *ServiceRecord) RunningCount() int {
	count := 0
	for _, instance := range s.Instances {
		if instance.Status == "running" {
			count++
		}
	}
	return count
}

// PortSummary returns a human-readable string like "8080->80/tcp" for display
// in status tables, or "-" if no ports are mapped.
func (i InstanceRecord) PortSummary() string {
	if len(i.Ports) == 0 {
		return "-"
	}

	parts := make([]string, 0, len(i.Ports))
	for _, port := range i.Ports {
		parts = append(parts, fmt.Sprintf("%d->%d/%s", port.HostPort, port.GuestPort, port.Protocol))
	}
	return strings.Join(parts, ",")
}

// reconcileService ensures a service has the desired number of running instances.
func (m *Manager) reconcileService(project string, manifest config.Manifest, existing *ServiceRecord) (*ServiceRecord, error) {
	svc := &ServiceRecord{
		Name:            manifest.Name,
		DesiredReplicas: manifest.Replicas,
	}

	existingInstances := make(map[int]*InstanceRecord)
	if existing != nil {
		for i := range existing.Instances {
			inst := &existing.Instances[i]
			if inst.PID != 0 && processAlive(inst.PID) {
				inst.Status = "running"
			} else {
				inst.Status = "stopped"
				inst.PID = 0
			}
			existingInstances[inst.Index] = inst
		}
	}

	instances := make([]InstanceRecord, 0, manifest.Replicas)
	for index := 0; index < manifest.Replicas; index++ {
		if inst, ok := existingInstances[index]; ok && inst.Status == "running" {
			instances = append(instances, *inst)
			continue
		}

		if prev, ok := existingInstances[index]; ok && prev.WorkDir != "" && dirExists(prev.WorkDir) {
			inst, err := m.restartInstance(manifest, *prev)
			if err != nil {
				return nil, err
			}
			instances = append(instances, inst)
			continue
		}

		inst, err := m.startInstance(project, manifest, index)
		if err != nil {
			return nil, err
		}
		instances = append(instances, inst)
	}

	if existing != nil {
		for _, inst := range existing.Instances {
			if inst.Index >= manifest.Replicas {
				_ = m.stopInstance(inst)
				_ = os.RemoveAll(inst.WorkDir)
			}
		}
	}

	sort.Slice(instances, func(i, j int) bool {
		return instances[i].Index < instances[j].Index
	})
	svc.Instances = instances
	return svc, nil
}

func (m *Manager) tearDownProject(record *ProjectRecord) error {
	for i := len(record.Services) - 1; i >= 0; i-- {
		m.stopAllInstances(record.Services[i].Instances)
		m.removeInstanceDirs(record.Services[i].Instances)
	}
	return nil
}

func (m *Manager) refreshProject(record *ProjectRecord) {
	for i := range record.Services {
		for j := range record.Services[i].Instances {
			inst := &record.Services[i].Instances[j]
			if inst.PID != 0 && processAlive(inst.PID) {
				inst.Status = "running"
			} else {
				inst.Status = "stopped"
				inst.PID = 0
			}
		}
	}
}

// State directory layout and persistence.

func (m *Manager) ensureLayout() error {
	for _, dir := range []string{m.stateDir, projectsDir(m.stateDir), instancesRoot(m.stateDir)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("ensure state dir %s: %w", dir, err)
		}
	}
	return nil
}

func (m *Manager) loadProject(name string) (*ProjectRecord, error) {
	if err := m.ensureLayout(); err != nil {
		return nil, err
	}
	path := projectFile(m.stateDir, name)
	payload, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var record ProjectRecord
	if err := json.Unmarshal(payload, &record); err != nil {
		return nil, fmt.Errorf("decode project record: %w", err)
	}
	return &record, nil
}

func (m *Manager) saveProject(record *ProjectRecord) error {
	if err := m.ensureLayout(); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("encode project record: %w", err)
	}
	return os.WriteFile(projectFile(m.stateDir, record.Name), payload, 0o644)
}

func projectsDir(root string) string {
	return filepath.Join(root, "projects")
}

func instancesRoot(root string) string {
	return filepath.Join(root, "instances")
}

func projectFile(root, name string) string {
	return filepath.Join(projectsDir(root), name+".json")
}

func projectInstanceDir(root, project, service string, index int) string {
	return filepath.Join(instancesRoot(root), project, fmt.Sprintf("%s-%d", service, index))
}

// QEMU binary lookup.

func (m *Manager) qemuSystemCommand(args ...string) (*exec.Cmd, error) {
	binary, err := m.qemuSystemBinary()
	if err != nil {
		return nil, err
	}
	return exec.Command(binary, args...), nil
}

func (m *Manager) qemuSystemBinary() (string, error) {
	if value := os.Getenv("HOLOS_QEMU_SYSTEM"); value != "" {
		return value, nil
	}
	binary, err := exec.LookPath("qemu-system-x86_64")
	if err != nil {
		return "", errors.New("qemu-system-x86_64 not found; install QEMU/KVM or set HOLOS_QEMU_SYSTEM")
	}
	return binary, nil
}

func (m *Manager) qemuImgBinary() (string, error) {
	if value := os.Getenv("HOLOS_QEMU_IMG"); value != "" {
		return value, nil
	}
	binary, err := exec.LookPath("qemu-img")
	if err != nil {
		return "", errors.New("qemu-img not found; install QEMU tools or set HOLOS_QEMU_IMG")
	}
	return binary, nil
}
