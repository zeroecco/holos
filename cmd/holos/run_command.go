package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/zeroecco/holos/internal/compose"
	"github.com/zeroecco/holos/internal/images"
	"github.com/zeroecco/holos/internal/runtime"
	"gopkg.in/yaml.v3"
)

func runRun(args []string) error {
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	stateDir := flags.String("state-dir", runtime.DefaultStateDir(), "state directory")
	name := flags.String("name", "", "project name (default: derived from image with random suffix)")
	vcpu := flags.Int("vcpu", 0, "vCPU count (default 1)")
	memory := flags.String("memory", "", "memory size, e.g. \"512M\", \"2G\" (default 512M)")
	user := flags.String("user", "", "cloud-init user (default: ubuntu)")
	dockerfile := flags.String("dockerfile", "", "use a Dockerfile to provision the VM (image arg becomes optional)")
	uefi := flags.Bool("uefi", false, "boot via OVMF (auto-enabled when --device is set)")
	detach := flags.Bool("detach", true, "start in background (kept for symmetry; foreground is not supported)")
	var ports, volumes, devices, packages, runcmd stringList
	flags.Var(&ports, "p", "publish a port HOST:GUEST (repeatable)")
	flags.Var(&ports, "port", "publish a port HOST:GUEST (repeatable)")
	flags.Var(&volumes, "v", "bind mount HOSTPATH:GUESTPATH[:ro] (repeatable)")
	flags.Var(&volumes, "volume", "bind mount HOSTPATH:GUESTPATH[:ro] (repeatable)")
	flags.Var(&devices, "device", "PCI address to pass through, e.g. 0000:01:00.0 (repeatable)")
	flags.Var(&packages, "pkg", "cloud-init package to install (repeatable)")
	flags.Var(&runcmd, "runcmd", "shell command to run on first boot (repeatable)")
	flags.SetOutput(os.Stderr)
	if err := flags.Parse(args); err != nil {
		return err
	}
	_ = detach

	positional := flags.Args()
	for i, a := range positional {
		if a == "--" {
			positional = append(positional[:i], positional[i+1:]...)
			break
		}
	}

	var image string
	var trailing []string
	if *dockerfile != "" {
		if len(positional) > 0 {
			image = positional[0]
			trailing = positional[1:]
		}
	} else {
		if len(positional) == 0 {
			return errors.New("run requires an image (e.g. `holos run ubuntu:noble`)")
		}
		image = positional[0]
		trailing = positional[1:]
	}
	if len(trailing) > 0 {
		runcmd = append(runcmd, strings.Join(trailing, " "))
	}

	memMB := 0
	if *memory != "" {
		mb, err := parseMemoryMB(*memory)
		if err != nil {
			return err
		}
		memMB = mb
	}

	devList := make([]compose.ComposeDevice, len(devices))
	for i, d := range devices {
		devList[i] = compose.ComposeDevice{PCI: d}
	}

	projectName := *name
	if projectName == "" {
		projectName = generateRunName(image, *dockerfile)
	}
	if !runNamePattern.MatchString(projectName) {
		return fmt.Errorf("project name %q must be a DNS label (lowercase letters, digits, hyphens)", projectName)
	}

	const serviceName = "vm"
	resolvedUser := *user
	if resolvedUser == "" {
		resolvedUser = images.DefaultUser(image)
	}
	resolvedUEFI := *uefi || len(devList) > 0

	file := compose.File{
		Name: projectName,
		Services: map[string]compose.Service{
			serviceName: {
				Image:      image,
				Dockerfile: *dockerfile,
				VM: compose.VM{
					VCPU:     *vcpu,
					MemoryMB: memMB,
					UEFI:     resolvedUEFI,
				},
				Ports:   ports,
				Volumes: volumes,
				Devices: devList,
				CloudInit: compose.CloudInit{
					User:     resolvedUser,
					Packages: packages,
					RunCmd:   runcmd,
				},
			},
		},
	}

	runDir := filepath.Join(*stateDir, "runs", projectName)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return fmt.Errorf("create run dir: %w", err)
	}
	composePath := filepath.Join(runDir, "holos.yaml")
	yamlBytes, err := yaml.Marshal(file)
	if err != nil {
		return fmt.Errorf("marshal compose: %w", err)
	}
	if err := os.WriteFile(composePath, yamlBytes, 0o644); err != nil {
		return fmt.Errorf("write compose: %w", err)
	}

	project, err := loadProject(composePath, *stateDir)
	if err != nil {
		return fmt.Errorf("synthesise project (see %s):\n%w", composePath, err)
	}

	manager := runtime.NewManager(*stateDir)
	record, err := manager.Up(project)
	if err != nil {
		return err
	}

	printProjectStatus(record)
	loginUser := project.Services[serviceName].CloudInit.User
	fmt.Printf("compose file: %s\n", composePath)
	fmt.Printf("login user:   %s (cloud-init may take ~30s on first boot)\n", loginUser)
	fmt.Println()
	fmt.Println("next steps:")
	fmt.Printf("  holos exec    %s     # interactive shell over ssh (recommended)\n", projectName)
	fmt.Printf("  holos console %s     # serial console for boot/kernel logs\n", projectName)
	fmt.Printf("  holos logs    %s     # console.log tail\n", projectName)
	fmt.Printf("  holos down    %s\n", projectName)
	return nil
}

type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error {
	*s = append(*s, v)
	return nil
}

var runNamePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

var runNameSanitiser = regexp.MustCompile(`[^a-z0-9-]+`)

func generateRunName(image, dockerfilePath string) string {
	base := image
	if base == "" {
		base = "dockerfile"
	}
	base = filepath.Base(base)
	if dot := strings.LastIndexByte(base, '.'); dot > 0 {
		base = base[:dot]
	}
	base = strings.ToLower(base)
	base = runNameSanitiser.ReplaceAllString(base, "-")
	base = strings.Trim(base, "-")
	if base == "" {
		base = "vm"
	}
	const suffixLen = 7
	if len(base) > 63-suffixLen {
		base = base[:63-suffixLen]
		base = strings.TrimRight(base, "-")
	}
	suffix := randHex(3)
	_ = dockerfilePath
	return base + "-" + suffix
}

func randHex(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err == nil {
		return hex.EncodeToString(buf)
	}
	return randHexFallback(n)
}

func randHexFallback(n int) string {
	if n > sha256.Size {
		n = sha256.Size
	}
	seed := time.Now().UnixNano() ^ int64(os.Getpid())
	h := sha256.Sum256(fmt.Appendf(nil, "%d", seed))
	return hex.EncodeToString(h[:n])
}

func parseMemoryMB(raw string) (int, error) {
	s := strings.TrimSpace(strings.ToUpper(raw))
	if s == "" {
		return 0, fmt.Errorf("empty memory value")
	}

	multiplierMB := 1.0
	last := s[len(s)-1]
	switch last {
	case 'B':
		if len(s) < 2 {
			return 0, fmt.Errorf("invalid memory %q", raw)
		}
		s = s[:len(s)-1]
		last = s[len(s)-1]
		fallthrough
	case 'K', 'M', 'G', 'T':
		switch last {
		case 'K':
			multiplierMB = 1.0 / 1024.0
		case 'M':
			multiplierMB = 1
		case 'G':
			multiplierMB = 1024
		case 'T':
			multiplierMB = 1024 * 1024
		}
		s = s[:len(s)-1]
	}

	value, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0, fmt.Errorf("invalid memory %q: %w", raw, err)
	}
	if value <= 0 {
		return 0, fmt.Errorf("memory %q must be positive", raw)
	}
	mb := int(value * multiplierMB)
	if mb < 1 {
		return 0, fmt.Errorf("memory %q rounds to less than 1 MB", raw)
	}
	return mb, nil
}
