package cloudinit

import (
	"fmt"

	"github.com/zeroecco/holos/internal/config"
)

// serialGettySystemdCmd enables auto-login on ttyS0 via systemd. The whole
// chain is guarded by `command -v systemctl` so it is a no-op on non-systemd
// distros (e.g. Alpine/OpenRC).
const serialGettySystemdCmd = "command -v systemctl >/dev/null 2>&1 && { systemctl daemon-reload; systemctl enable serial-getty@ttyS0.service; systemctl restart serial-getty@ttyS0.service; } ; command -v update-grub >/dev/null 2>&1 && update-grub || true"

const (
	serialConsoleGrubPath      = "/etc/default/grub.d/99-serial-console.cfg"
	serialConsoleAutologinPath = "/etc/systemd/system/serial-getty@ttyS0.service.d/autologin.conf"
	serialConsoleGrubContent   = "GRUB_CMDLINE_LINUX_DEFAULT=\"${GRUB_CMDLINE_LINUX_DEFAULT} console=ttyS0,115200\"\nGRUB_TERMINAL=\"serial console\"\nGRUB_SERIAL_COMMAND=\"serial --speed=115200\"\n"
	serialConsoleAgettyFormat  = "[Service]\nExecStart=\nExecStart=-/sbin/agetty --autologin %s --noclear %%I $TERM\n"
)

// serialConsoleFiles returns distro-specific write_files needed to land on a
// usable serial console. On systemd we add a GRUB drop-in and a serial-getty
// autologin override. On OpenRC (Alpine) the cloud image already exposes
// ttyS0, so there is nothing to write.
func serialConsoleFiles(manifest config.Manifest, family osFamily) []ccFile {
	if family != familySystemd {
		return nil
	}
	return []ccFile{
		{
			Path:        serialConsoleGrubPath,
			Content:     serialConsoleGrubContent,
			Permissions: config.DefaultFilePermissions,
			Owner:       config.DefaultFileOwner,
		},
		{
			Path:        serialConsoleAutologinPath,
			Content:     serialConsoleAgettyContent(manifest.CloudInit.User),
			Permissions: config.DefaultFilePermissions,
			Owner:       config.DefaultFileOwner,
		},
	}
}

func serialConsoleAgettyContent(user string) string {
	return fmt.Sprintf(serialConsoleAgettyFormat, user)
}

// serialConsoleRunCmd returns runcmd entries needed to activate the serial
// console on first boot. On Alpine the cloud image already spawns a getty on
// ttyS0, so no command is required.
func serialConsoleRunCmd(family osFamily) []string {
	if family != familySystemd {
		return nil
	}
	return []string{serialGettySystemdCmd}
}
