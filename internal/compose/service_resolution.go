package compose

import (
	"fmt"

	"github.com/zeroecco/holos/internal/config"
)

func (f *File) resolveService(name string, svc Service, baseDir string, cacheDir string, network NetworkPlan, hosts map[string]string, instanceIPs []string, resolver composeImageResolver) (config.Manifest, error) {
	replicas, err := serviceReplicas(svc)
	if err != nil {
		return config.Manifest{}, err
	}

	ports, err := parsePorts(svc.Ports)
	if err != nil {
		return config.Manifest{}, err
	}

	mounts, err := parseVolumes(svc.Volumes, baseDir, f.Volumes)
	if err != nil {
		return config.Manifest{}, err
	}

	var dfWriteFiles []config.WriteFile
	var dfRunCmd []string
	var dfHealthcheck *config.HealthcheckConfig
	dfWriteFiles, dfRunCmd, dfHealthcheck, err = resolveDockerfileBuild(&svc, baseDir)
	if err != nil {
		return config.Manifest{}, err
	}

	image, imageFormat, err := resolveImage(svc.Image, svc.ImageFormat, baseDir, cacheDir, resolver)
	if err != nil {
		return config.Manifest{}, err
	}
	imageOS := resolveImageOS(svc.Image, svc.ImageOS, resolver)

	user := resolveServiceUser(svc, resolver)
	if err := config.ValidateUserName(user); err != nil {
		return config.Manifest{}, fmt.Errorf("cloud_init.user: %w", err)
	}
	hostname := resolveServiceHostname(svc)

	baseMAC := generateMAC(0x00, f.Name, name)

	writeFiles, err := resolveServiceWriteFiles(baseDir, svc, dfWriteFiles, f.Configs, f.Secrets)
	if err != nil {
		return config.Manifest{}, err
	}
	devices, err := resolveServiceDevices(svc)
	if err != nil {
		return config.Manifest{}, err
	}
	vmConfig, err := resolveServiceVMConfig(svc, resolver, devices)
	if err != nil {
		return config.Manifest{}, err
	}

	gracePeriodSec, err := parseStopGracePeriod(svc.StopGracePeriod)
	if err != nil {
		return config.Manifest{}, err
	}

	healthcheck, err := resolveServiceHealthcheck(svc.Healthcheck, dfHealthcheck)
	if err != nil {
		return config.Manifest{}, err
	}

	extraHosts := resolveServiceExtraHosts(hosts, svc.ExtraHosts)

	labels, err := resolveLabels(baseDir, svc.LabelFile, svc.Labels)
	if err != nil {
		return config.Manifest{}, err
	}

	return config.Manifest{
		APIVersion:  config.DefaultAPIVersion,
		Kind:        config.DefaultKind,
		Name:        name,
		Replicas:    replicas,
		Image:       image,
		ImageFormat: imageFormat,
		ImageOS:     imageOS,
		VM:          vmConfig,
		Devices:     devices,
		Labels:      labels,
		Network:     config.NetworkConfig{Mode: config.DefaultNetworkMode},
		Ports:       ports,
		Mounts:      mounts,
		CloudInit: config.CloudInit{
			Hostname:          hostname,
			User:              user,
			SSHAuthorizedKeys: svc.CloudInit.SSHAuthorizedKeys,
			Packages:          svc.CloudInit.Packages,
			WriteFiles:        writeFiles,
			RunCmd:            serviceRunCmd(svc, dfRunCmd),
			BootCmd:           svc.CloudInit.BootCmd,
		},
		InternalNetwork: &config.InternalNetworkConfig{
			MulticastGroup: network.MulticastGroup,
			MulticastPort:  network.MulticastPort,
			Subnet:         network.Subnet,
			InstanceIPs:    instanceIPs,
			BaseMAC:        baseMAC,
			UserBaseMAC:    generateMAC(0x01, f.Name, name),
		},
		ExtraHosts:         extraHosts,
		StopGracePeriodSec: gracePeriodSec,
		Healthcheck:        healthcheck,
		PreStopCommands:    servicePreStopCmd(svc),
		DependsOn:          append([]string(nil), svc.DependsOn...),
	}, nil
}

func resolveServiceExtraHosts(projectHosts map[string]string, serviceHosts ExtraHosts) map[string]string {
	extraHosts := make(map[string]string, len(projectHosts)+len(serviceHosts))
	copyExtraHosts(extraHosts, projectHosts)
	copyExtraHosts(extraHosts, serviceHosts)
	return extraHosts
}

func copyExtraHosts(dst map[string]string, src map[string]string) {
	for host, addr := range src {
		dst[host] = addr
	}
}
