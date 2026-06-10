package runtime

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/zeroecco/holos/internal/cloudinit"
	"github.com/zeroecco/holos/internal/config"
)

const (
	cloudLocalDSBinary        = "cloud-localds"
	genisoimageBinary         = "genisoimage"
	mkisofsBinary             = "mkisofs"
	xorrisoBinary             = "xorriso"
	cloudLocalDSNetworkConfig = "--network-config"
	isoOutputFlag             = "-output"
	isoVolumeIDFlag           = "-volid"
	isoVolumeID               = "cidata"
	isoJolietFlag             = "-joliet"
	isoRockRidgeFlag          = "-rock"
	xorrisoAsFlag             = "-as"
	seedDirPerm               = os.FileMode(0o700)
	seedFilePerm              = os.FileMode(0o600)
)

func (m *Manager) createSeedImage(manifest config.Manifest, instanceName string, index int, workDir string) (string, error) {
	userData, metaData, networkConfig := cloudinit.Render(manifest, instanceName, index)
	paths := newSeedPaths(workDir)
	hasNetwork := networkConfig != ""
	if err := writeSeedContent(paths, userData, metaData, networkConfig, hasNetwork); err != nil {
		return "", err
	}

	if cloudLocalDS, err := exec.LookPath(cloudLocalDSBinary); err == nil {
		outputPath := paths.cloudLocalDSImage
		if err := runSeedBuilder("cloud-init seed", outputPath, cloudLocalDS, cloudLocalDSArgs(outputPath, paths, hasNetwork)); err != nil {
			return "", err
		}
		return outputPath, nil
	}

	outputPath := paths.isoImage
	isoBuilder, args, err := isoCommand(outputPath, paths, hasNetwork)
	if err != nil {
		return "", err
	}

	if err := runSeedBuilder("seed iso", outputPath, isoBuilder, args); err != nil {
		return "", err
	}
	return outputPath, nil
}

func runSeedBuilder(label, outputPath, binary string, args []string) error {
	command := exec.Command(binary, args...)
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("create %s: %w: %s", label, err, strings.TrimSpace(string(output)))
	}
	// External tools follow the process umask, which on most distros yields
	// 0644. The seed image embeds user-data verbatim, so tighten it after
	// the fact.
	if err := os.Chmod(outputPath, seedFilePerm); err != nil {
		return fmt.Errorf("tighten %s: %w", label, err)
	}
	return nil
}

func writeSeedContent(paths seedPaths, userData, metaData, networkConfig string, hasNetwork bool) error {
	// Seed material is sensitive: user-data carries cloud-init runcmd,
	// write_files (often app secrets), and SSH authorized keys. Keep the
	// directory and every file inside it owner-only.
	if err := os.MkdirAll(paths.dir, seedDirPerm); err != nil {
		return fmt.Errorf("create seed dir: %w", err)
	}
	if err := os.WriteFile(paths.userData, []byte(userData), seedFilePerm); err != nil {
		return fmt.Errorf("write user-data: %w", err)
	}
	if err := os.WriteFile(paths.metaData, []byte(metaData), seedFilePerm); err != nil {
		return fmt.Errorf("write meta-data: %w", err)
	}
	if hasNetwork {
		if err := os.WriteFile(paths.networkConfig, []byte(networkConfig), seedFilePerm); err != nil {
			return fmt.Errorf("write network-config: %w", err)
		}
	}
	return nil
}

func cloudLocalDSArgs(outputPath string, paths seedPaths, hasNetwork bool) []string {
	args := []string{}
	if hasNetwork {
		args = append(args, cloudLocalDSNetworkConfig, paths.networkConfig)
	}
	return append(args, outputPath, paths.userData, paths.metaData)
}

func isoCommand(outputPath string, paths seedPaths, hasNetwork bool) (string, []string, error) {
	files := seedContentFiles(paths, hasNetwork)

	for _, candidate := range []string{genisoimageBinary, mkisofsBinary} {
		if binary, err := exec.LookPath(candidate); err == nil {
			return binary, append(isoBuilderArgs(outputPath), files...), nil
		}
	}

	if binary, err := exec.LookPath(xorrisoBinary); err == nil {
		return binary, xorrisoISOArgs(outputPath, files), nil
	}

	return "", nil, fmt.Errorf("no cloud-init media builder found; install %s, %s, %s, or %s",
		cloudLocalDSBinary, genisoimageBinary, mkisofsBinary, xorrisoBinary)
}

func isoBuilderArgs(outputPath string) []string {
	return []string{isoOutputFlag, outputPath, isoVolumeIDFlag, isoVolumeID, isoJolietFlag, isoRockRidgeFlag}
}

func xorrisoISOArgs(outputPath string, files []string) []string {
	args := append([]string{xorrisoAsFlag, mkisofsBinary}, isoBuilderArgs(outputPath)...)
	return append(args, files...)
}

func seedContentFiles(paths seedPaths, hasNetwork bool) []string {
	files := []string{
		paths.userData,
		paths.metaData,
	}
	if hasNetwork {
		files = append(files, paths.networkConfig)
	}
	return files
}
