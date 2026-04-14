package runtime

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/zeroecco/holos/internal/cloudinit"
	"github.com/zeroecco/holos/internal/config"
)

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
