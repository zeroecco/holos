package main

import "github.com/zeroecco/holos/internal/compose"

func runComposeFile(projectName, image, user string, runCmd []string, devices []compose.ComposeDevice, memoryMB int, opts runOptions) compose.File {
	return compose.File{
		Name: projectName,
		Services: map[string]compose.Service{
			runServiceName: {
				Image:      image,
				ImageOS:    opts.imageOS,
				Dockerfile: opts.dockerfile,
				VM: compose.VM{
					VCPU:     opts.vcpu,
					MemoryMB: memoryMB,
					UEFI:     opts.uefi || len(devices) > 0,
				},
				Ports:   composePorts(opts.ports),
				Volumes: composeVolumes(opts.volumes),
				Devices: devices,
				CloudInit: compose.CloudInit{
					User:     user,
					Packages: opts.packages,
					RunCmd:   runCmd,
				},
			},
		},
	}
}
