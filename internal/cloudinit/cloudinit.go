package cloudinit

import (
	"fmt"

	"github.com/zeroecco/holos/internal/config"
	"gopkg.in/yaml.v3"
)

// Render produces cloud-init user-data, meta-data, and network-config.
// networkConfig is empty when there is no internal network.
func Render(manifest config.Manifest, instanceName string, instanceIndex int) (userData, metaData, networkConfig string) {
	family := osFamilyFromMetadata(manifest.ImageOS)

	cc := cloudConfig{
		Hostname:       hostname(manifest, instanceName),
		ManageEtcHosts: len(manifest.ExtraHosts) == 0,
		Users:          []ccUser{renderUser(manifest, family)},
	}

	if len(manifest.CloudInit.Packages) > 0 {
		cc.PackageUpdate = true
		cc.Packages = manifest.CloudInit.Packages
	}

	cc.WriteFiles = renderWriteFiles(manifest, instanceName, family)

	cc.BootCmd = manifest.CloudInit.BootCmd

	cc.RunCmd = renderRunCmd(manifest, family)

	data, _ := yaml.Marshal(cc)
	ud := "#cloud-config\n" + string(data)

	md := renderMetaData(manifest, instanceName)

	var nc string
	if manifest.InternalNetwork != nil {
		nc = renderNetworkConfig(manifest, instanceIndex)
	}

	return ud, md, nc
}

func renderWriteFiles(manifest config.Manifest, instanceName string, family osFamily) []ccFile {
	files := make([]ccFile, 0, len(manifest.CloudInit.WriteFiles)+2)
	if hosts, ok := hostsWriteFile(manifest, instanceName); ok {
		files = append(files, hosts)
	}
	files = append(files, serialConsoleFiles(manifest, family)...)
	files = append(files, cloudInitWriteFiles(manifest.CloudInit.WriteFiles)...)
	return files
}

func cloudInitWriteFiles(files []config.WriteFile) []ccFile {
	out := make([]ccFile, 0, len(files))
	for _, f := range files {
		out = append(out, ccFile{
			Path:        f.Path,
			Content:     f.Content,
			Permissions: f.Permissions,
			Owner:       f.Owner,
		})
	}
	return out
}

func renderRunCmd(manifest config.Manifest, family osFamily) []string {
	cmds := append([]string(nil), manifest.CloudInit.RunCmd...)
	cmds = append(cmds, serialConsoleRunCmd(family)...)
	cmds = append(cmds, volumeMountRunCmd(manifest)...)
	return cmds
}

func renderMetaData(manifest config.Manifest, instanceName string) string {
	return fmt.Sprintf("instance-id: %s\nlocal-hostname: %s\n", instanceName, hostname(manifest, instanceName))
}
