package cloudinit

import "github.com/zeroecco/holos/internal/config"

// osFamily enumerates the init-system / userland conventions we need to
// branch on when rendering cloud-init user-data.
type osFamily int

const (
	familySystemd osFamily = iota
	familyOpenRC
)

// osFamilyFromMetadata maps the explicit image metadata resolved from the
// registry or compose file into cloud-init rendering behavior. Unknown or empty
// metadata stays systemd for compatibility with existing local images.
func osFamilyFromMetadata(imageOS string) osFamily {
	if imageOS == config.ImageOSOpenRC {
		return familyOpenRC
	}
	return familySystemd
}
