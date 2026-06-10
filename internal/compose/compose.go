package compose

import (
	"fmt"

	"github.com/zeroecco/holos/internal/config"
	"github.com/zeroecco/holos/internal/images"
)

// maxReplicas is a soft cap on `replicas:` for a single service to
// catch typos at parse time. It is intentionally larger than the
// per-project total so a single-service stack can use the full
// subnet and the error messages stay specific ("replicas 1000000
// exceeds maximum of 256" vs a surprise project-wide reject).
const maxReplicas = 256

// maxProjectInstances is the number of usable host addresses in
// subnetCIDR. The allocator starts at .2 and must stop at .254 to
// avoid producing nonsense octets like 10.10.0.270; reserving .1 for
// the gateway placeholder and .255 as the broadcast address leaves
// 253 addresses for VMs. This is the SUM of replicas across every
// service in a project, not a per-service limit.
const (
	maxProjectInstances = 253
	subnetCIDR          = "10.10.0.0/24"
)

// Resolve validates the compose file and produces a Project.
// stateDir is used for the image cache when pulling remote images.
func (f *File) Resolve(baseDir string, stateDir string) (*Project, error) {
	if err := f.validate(); err != nil {
		return nil, err
	}

	order, err := f.topoSort()
	if err != nil {
		return nil, err
	}

	network := f.planNetwork()

	if _, err := projectInstanceCount(f.Services, order); err != nil {
		return nil, err
	}
	for name := range network.Segments {
		if err := validateProjectInstanceCapacity(networkInstanceCount(f.Services, order, name)); err != nil {
			return nil, fmt.Errorf("network %q: %w", name, err)
		}
	}

	hosts, serviceNetworks, err := allocateServiceNetworks(f.Services, order, network, f.Name)
	if err != nil {
		return nil, err
	}
	network.Hosts = hosts

	cacheDir := images.DefaultCacheDir(stateDir)

	services := make(map[string]config.Manifest, len(f.Services))
	for _, name := range order {
		svc := f.Services[name]
		serviceBaseDir := f.serviceBaseDir(name, baseDir)
		manifest, err := f.resolveService(name, svc, serviceBaseDir, cacheDir, serviceNetworks[name], f.resolver())
		if err != nil {
			return nil, fmt.Errorf("service %q: %w", name, err)
		}
		// Run the canonical Manifest validator on every resolved
		// service so out-of-range fields (memory_mb, port numbers,
		// healthcheck timing, ...) are caught at compose load time
		// instead of surfacing as a runtime panic or, worse, a
		// silently misconfigured VM. Without this, `holos validate`
		// can return success on YAML the runtime will reject.
		if err := manifest.Validate(); err != nil {
			return nil, fmt.Errorf("service %q: %w", name, err)
		}
		services[name] = manifest
	}

	specHash, err := f.specHash()
	if err != nil {
		return nil, err
	}

	volumes, err := f.resolveVolumes(services)
	if err != nil {
		return nil, err
	}

	return &Project{
		Name:         f.Name,
		SpecHash:     specHash,
		ServiceOrder: order,
		Services:     services,
		Network:      network,
		Volumes:      volumes,
	}, nil
}

func (f *File) serviceBaseDir(name string, defaultBaseDir string) string {
	if f.serviceBaseDirs != nil && f.serviceBaseDirs[name] != "" {
		return f.serviceBaseDirs[name]
	}
	return defaultBaseDir
}
