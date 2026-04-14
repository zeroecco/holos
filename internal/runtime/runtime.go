package runtime

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/rich/holosteric/internal/cloudinit"
	"github.com/rich/holosteric/internal/compose"
	"github.com/rich/holosteric/internal/config"
	"github.com/rich/holosteric/internal/qemu"
)

type Manager struct {
	stateDir string
}

type ProjectRecord struct {
	Name     string          `json:"name"`
	SpecHash string          `json:"spec_hash"`
	Services []ServiceRecord `json:"services"`
	Network  NetworkState    `json:"network"`
	UpdatedAt time.Time      `json:"updated_at"`
}

type NetworkState struct {
	MulticastGroup string            `json:"multicast_group"`
	MulticastPort  int               `json:"multicast_port"`
	Subnet         string            `json:"subnet"`
	Hosts          map[string]string `json:"hosts"`
}

type ServiceRecord struct {
	Name            string           `json:"name"`
	DesiredReplicas int              `json:"desired_replicas"`
	Instances       []InstanceRecord `json:"instances"`
}

type InstanceRecord struct {
	Name         string             `json:"name"`
	Index        int                `json:"index"`
	PID          int                `json:"pid"`
	Status       string             `json:"status"`
	WorkDir      string             `json:"work_dir"`
	OverlayPath  string             `json:"overlay_path"`
	SeedPath     string             `json:"seed_path"`
	LogPath      string             `json:"log_path"`
	QMPPath      string             `json:"qmp_path"`
	Ports        []qemu.PortMapping `json:"ports"`
	LastStarted  time.Time          `json:"last_started"`
	LastExitTime time.Time          `json:"last_exit_time,omitempty"`
}

func NewManager(stateDir string) *Manager {
	return &Manager{stateDir: stateDir}
}

func DefaultStateDir() string {
	if value := os.Getenv("HOLOSTERIC_STATE_DIR"); value != "" {
		return value
	}

	if os.Geteuid() == 0 {
		return "/var/lib/holosteric"
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ".holosteric"
	}
	return filepath.Join(home, ".local", "state", "holosteric")
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

	// Remove services that no longer exist in the compose file.
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

func (s *ServiceRecord) RunningCount() int {
	count := 0
	for _, instance := range s.Instances {
		if instance.Status == "running" {
			count++
		}
	}
	return count
}

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

		if inst, ok := existingInstances[index]; ok {
			_ = os.RemoveAll(inst.WorkDir)
		}

		inst, err := m.startInstance(project, manifest, index)
		if err != nil {
			return nil, err
		}
		instances = append(instances, inst)
	}

	// Scale down excess instances.
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

func (m *Manager) startInstance(project string, manifest config.Manifest, index int) (InstanceRecord, error) {
	workDir := projectInstanceDir(m.stateDir, project, manifest.Name, index)
	if err := os.RemoveAll(workDir); err != nil {
		return InstanceRecord{}, fmt.Errorf("remove instance workdir: %w", err)
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return InstanceRecord{}, fmt.Errorf("create instance workdir: %w", err)
	}

	overlayPath := filepath.Join(workDir, "root.qcow2")
	if err := m.createOverlay(manifest, overlayPath); err != nil {
		return InstanceRecord{}, err
	}

	instanceName := manifest.InstanceName(index)

	seedPath, err := m.createSeedImage(manifest, instanceName, index, workDir)
	if err != nil {
		return InstanceRecord{}, err
	}

	ports, err := allocatePorts(manifest, index)
	if err != nil {
		return InstanceRecord{}, err
	}

	logPath := filepath.Join(workDir, "console.log")
	qmpPath := filepath.Join(workDir, "qmp.sock")
	qemuLogPath := filepath.Join(workDir, "qemu.log")

	spec := qemu.LaunchSpec{
		Name:        instanceName,
		Index:       index,
		OverlayPath: overlayPath,
		SeedPath:    seedPath,
		LogPath:     logPath,
		QMPPath:     qmpPath,
		Ports:       ports,
	}

	if manifest.VM.UEFI {
		ovmfCode, ovmfVars, err := m.prepareUEFI(workDir)
		if err != nil {
			return InstanceRecord{}, err
		}
		spec.OVMFCode = ovmfCode
		spec.OVMFVars = ovmfVars
	}

	args, err := qemu.BuildArgs(manifest, spec)
	if err != nil {
		return InstanceRecord{}, err
	}

	qemuLog, err := os.OpenFile(qemuLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return InstanceRecord{}, fmt.Errorf("open qemu log: %w", err)
	}
	defer qemuLog.Close()

	command, err := m.qemuSystemCommand(args...)
	if err != nil {
		return InstanceRecord{}, err
	}
	command.Stdout = qemuLog
	command.Stderr = qemuLog
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := command.Start(); err != nil {
		return InstanceRecord{}, fmt.Errorf("start qemu: %w", err)
	}

	pid := command.Process.Pid
	_ = command.Process.Release()

	time.Sleep(300 * time.Millisecond)
	if !processAlive(pid) {
		content, _ := os.ReadFile(qemuLogPath)
		return InstanceRecord{}, fmt.Errorf("qemu exited early for %s: %s", instanceName, strings.TrimSpace(string(content)))
	}

	return InstanceRecord{
		Name:        instanceName,
		Index:       index,
		PID:         pid,
		Status:      "running",
		WorkDir:     workDir,
		OverlayPath: overlayPath,
		SeedPath:    seedPath,
		LogPath:     logPath,
		QMPPath:     qmpPath,
		Ports:       ports,
		LastStarted: time.Now().UTC(),
	}, nil
}

func (m *Manager) createOverlay(manifest config.Manifest, overlayPath string) error {
	qemuImg, err := m.qemuImgBinary()
	if err != nil {
		return err
	}

	args := []string{
		"create",
		"-f", "qcow2",
		"-F", manifest.ImageFormat,
		"-b", manifest.Image,
		overlayPath,
	}
	if output, err := exec.Command(qemuImg, args...).CombinedOutput(); err != nil {
		return fmt.Errorf("create overlay: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func (m *Manager) createSeedImage(manifest config.Manifest, instanceName string, index int, workDir string) (string, error) {
	userData, metaData, networkConfig := cloudinit.Render(manifest, instanceName, index)
	seedDir := filepath.Join(workDir, "seed")
	if err := os.MkdirAll(seedDir, 0o755); err != nil {
		return "", fmt.Errorf("create seed dir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(seedDir, "user-data"), []byte(userData), 0o644); err != nil {
		return "", fmt.Errorf("write user-data: %w", err)
	}
	if err := os.WriteFile(filepath.Join(seedDir, "meta-data"), []byte(metaData), 0o644); err != nil {
		return "", fmt.Errorf("write meta-data: %w", err)
	}

	hasNetwork := networkConfig != ""
	if hasNetwork {
		if err := os.WriteFile(filepath.Join(seedDir, "network-config"), []byte(networkConfig), 0o644); err != nil {
			return "", fmt.Errorf("write network-config: %w", err)
		}
	}

	if cloudLocalDS, err := exec.LookPath("cloud-localds"); err == nil {
		outputPath := filepath.Join(workDir, "seed.img")
		args := []string{}
		if hasNetwork {
			args = append(args, "--network-config", filepath.Join(seedDir, "network-config"))
		}
		args = append(args, outputPath, filepath.Join(seedDir, "user-data"), filepath.Join(seedDir, "meta-data"))
		command := exec.Command(cloudLocalDS, args...)
		if output, err := command.CombinedOutput(); err != nil {
			return "", fmt.Errorf("create cloud-init seed: %w: %s", err, strings.TrimSpace(string(output)))
		}
		return outputPath, nil
	}

	outputPath := filepath.Join(workDir, "seed.iso")
	isoBuilder, args, err := isoCommand(outputPath, seedDir, hasNetwork)
	if err != nil {
		return "", err
	}

	command := exec.Command(isoBuilder, args...)
	if output, err := command.CombinedOutput(); err != nil {
		return "", fmt.Errorf("create seed iso: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return outputPath, nil
}

func (m *Manager) tearDownProject(record *ProjectRecord) error {
	for i := len(record.Services) - 1; i >= 0; i-- {
		m.stopAllInstances(record.Services[i].Instances)
		m.removeInstanceDirs(record.Services[i].Instances)
	}
	return nil
}

func (m *Manager) stopAllInstances(instances []InstanceRecord) {
	for idx := range instances {
		_ = m.stopInstance(instances[idx])
		instances[idx].Status = "stopped"
		instances[idx].PID = 0
		instances[idx].LastExitTime = time.Now().UTC()
	}
}

func (m *Manager) removeInstanceDirs(instances []InstanceRecord) {
	for _, inst := range instances {
		_ = os.RemoveAll(inst.WorkDir)
	}
}

func (m *Manager) stopInstance(instance InstanceRecord) error {
	if instance.PID == 0 || !processAlive(instance.PID) {
		return nil
	}

	process, err := os.FindProcess(instance.PID)
	if err != nil {
		return fmt.Errorf("find process %d: %w", instance.PID, err)
	}

	if err := process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf("signal pid %d: %w", instance.PID, err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if !processAlive(instance.PID) {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}

	if err := process.Signal(syscall.SIGKILL); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf("kill pid %d: %w", instance.PID, err)
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

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

// State directory layout.

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

func (m *Manager) qemuSystemCommand(args ...string) (*exec.Cmd, error) {
	binary, err := m.qemuSystemBinary()
	if err != nil {
		return nil, err
	}
	return exec.Command(binary, args...), nil
}

func (m *Manager) qemuSystemBinary() (string, error) {
	if value := os.Getenv("HOLOSTERIC_QEMU_SYSTEM"); value != "" {
		return value, nil
	}
	binary, err := exec.LookPath("qemu-system-x86_64")
	if err != nil {
		return "", errors.New("qemu-system-x86_64 not found; install QEMU/KVM or set HOLOSTERIC_QEMU_SYSTEM")
	}
	return binary, nil
}

func (m *Manager) qemuImgBinary() (string, error) {
	if value := os.Getenv("HOLOSTERIC_QEMU_IMG"); value != "" {
		return value, nil
	}
	binary, err := exec.LookPath("qemu-img")
	if err != nil {
		return "", errors.New("qemu-img not found; install QEMU tools or set HOLOSTERIC_QEMU_IMG")
	}
	return binary, nil
}

func allocatePorts(manifest config.Manifest, index int) ([]qemu.PortMapping, error) {
	mappings := make([]qemu.PortMapping, 0, len(manifest.Ports))
	for _, port := range manifest.Ports {
		hostPort := port.HostPort
		if hostPort > 0 {
			hostPort += index
			if err := ensureTCPPortAvailable(hostPort); err != nil {
				return nil, err
			}
		} else {
			allocated, err := allocateEphemeralTCPPort()
			if err != nil {
				return nil, err
			}
			hostPort = allocated
		}

		mappings = append(mappings, qemu.PortMapping{
			Name:      port.Name,
			HostPort:  hostPort,
			GuestPort: port.GuestPort,
			Protocol:  port.Protocol,
		})
	}
	return mappings, nil
}

func ensureTCPPortAvailable(port int) error {
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return fmt.Errorf("host port %d is unavailable: %w", port, err)
	}
	return listener.Close()
}

func allocateEphemeralTCPPort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("allocate ephemeral port: %w", err)
	}
	defer listener.Close()

	address, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, errors.New("unexpected tcp listener address type")
	}
	return address.Port, nil
}

func isoCommand(outputPath, seedDir string, hasNetwork bool) (string, []string, error) {
	files := []string{
		filepath.Join(seedDir, "user-data"),
		filepath.Join(seedDir, "meta-data"),
	}
	if hasNetwork {
		files = append(files, filepath.Join(seedDir, "network-config"))
	}

	for _, candidate := range []string{"genisoimage", "mkisofs"} {
		if binary, err := exec.LookPath(candidate); err == nil {
			args := []string{"-output", outputPath, "-volid", "cidata", "-joliet", "-rock"}
			args = append(args, files...)
			return binary, args, nil
		}
	}

	if binary, err := exec.LookPath("xorriso"); err == nil {
		args := []string{"-as", "mkisofs", "-output", outputPath, "-volid", "cidata", "-joliet", "-rock"}
		args = append(args, files...)
		return binary, args, nil
	}

	return "", nil, errors.New("no cloud-init media builder found; install cloud-localds, genisoimage, mkisofs, or xorriso")
}

// UEFI / OVMF support for GPU passthrough.

var ovmfCodePaths = []string{
	"/usr/share/OVMF/OVMF_CODE_4M.fd",
	"/usr/share/OVMF/OVMF_CODE.fd",
	"/usr/share/edk2/ovmf/OVMF_CODE.fd",
	"/usr/share/edk2-ovmf/x64/OVMF_CODE.fd",
	"/usr/share/qemu/OVMF_CODE.fd",
}

var ovmfVarsPaths = []string{
	"/usr/share/OVMF/OVMF_VARS_4M.fd",
	"/usr/share/OVMF/OVMF_VARS.fd",
	"/usr/share/edk2/ovmf/OVMF_VARS.fd",
	"/usr/share/edk2-ovmf/x64/OVMF_VARS.fd",
	"/usr/share/qemu/OVMF_VARS.fd",
}

func (m *Manager) prepareUEFI(workDir string) (codePath, varsPath string, err error) {
	codePath, err = findOVMF("HOLOSTERIC_OVMF_CODE", ovmfCodePaths)
	if err != nil {
		return "", "", err
	}

	templatePath, err := findOVMF("HOLOSTERIC_OVMF_VARS", ovmfVarsPaths)
	if err != nil {
		return "", "", err
	}

	varsPath = filepath.Join(workDir, "OVMF_VARS.fd")
	if err := copyFile(templatePath, varsPath); err != nil {
		return "", "", fmt.Errorf("copy OVMF_VARS: %w", err)
	}

	return codePath, varsPath, nil
}

func findOVMF(envVar string, searchPaths []string) (string, error) {
	if value := os.Getenv(envVar); value != "" {
		if _, err := os.Stat(value); err == nil {
			return value, nil
		}
		return "", fmt.Errorf("%s=%q not found", envVar, value)
	}

	for _, path := range searchPaths {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("OVMF firmware not found; install ovmf/edk2-ovmf or set %s", envVar)
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}
