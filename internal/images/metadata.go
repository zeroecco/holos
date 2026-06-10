package images

// DefaultUser returns the conventional cloud-init user for an image
// reference, or "" when the ref points at a local file or an unknown
// distro. This lets compose pick the right account for cloud-init to
// create *before* falling back to the global default. The difference
// between `holos exec` working and a console autologin attempt that
// can't find a user named "ubuntu" on a Debian image.
func DefaultUser(ref string) string {
	img, ok := resolveMetadataImage(ref)
	if !ok {
		return ""
	}
	return img.User
}

// RequiresVGA reports whether an image should get an explicit VGA device when
// booted through BIOS. Some images' GRUB configs can stall with holos'
// otherwise headless -nodefaults serial layout unless a graphics terminal
// device exists.
func RequiresVGA(ref string) bool {
	img, ok := resolveMetadataImage(ref)
	if !ok {
		return false
	}
	return img.RequiresVGA
}

// MinMemoryMB returns the image-specific memory floor, or 0 when the global
// default is sufficient.
func MinMemoryMB(ref string) int {
	img, ok := resolveMetadataImage(ref)
	if !ok {
		return 0
	}
	return img.MinMemoryMB
}

// ListAvailable returns all registered images.
func ListAvailable() []Image {
	return Registry
}

// OSFamily returns the registry-provided guest OS family metadata for ref.
func OSFamily(ref string) string {
	img, ok := resolveMetadataImage(ref)
	if !ok {
		return ""
	}
	return img.OSFamily
}

func resolveMetadataImage(ref string) (*Image, bool) {
	img, err := Resolve(ref)
	return img, metadataImageResolved(img, err)
}

func metadataImageResolved(img *Image, err error) bool {
	return err == nil && img != nil
}
