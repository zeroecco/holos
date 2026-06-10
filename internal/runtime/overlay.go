package runtime

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/zeroecco/holos/internal/config"
)

func (m *Manager) createOverlay(manifest config.Manifest, overlayPath string) error {
	qemuImg, err := m.qemuImgBinary()
	if err != nil {
		return err
	}

	if output, err := exec.Command(qemuImg, overlayCreateArgs(manifest, overlayPath)...).CombinedOutput(); err != nil {
		return fmt.Errorf("create overlay: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func overlayCreateArgs(manifest config.Manifest, overlayPath string) []string {
	args := []string{
		qemuImgCreateSubcommand,
		qemuImgFormatFlag, config.ImageFormatQCOW2,
		qemuImgBackingFormatFlag, manifest.ImageFormat,
		qemuImgBackingFileFlag, manifest.Image,
		overlayPath,
	}
	if manifest.VM.DiskSizeBytes > 0 {
		args = append(args, byteSizeArg(manifest.VM.DiskSizeBytes))
	}
	return args
}
