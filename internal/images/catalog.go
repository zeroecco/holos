package images

import "github.com/zeroecco/holos/internal/config"

// Image describes a downloadable cloud image.
type Image struct {
	Name    string // short name (e.g. "alpine")
	URL     string
	Format  string // qcow2 or raw
	Default bool   // true = default tag for this distro
	Tag     string // version tag (e.g. "3.21", "noble")
	// SHA256 is the expected hex-encoded sha256 of the artifact at URL.
	// When set, Pull verifies the downloaded bytes and aborts on
	// mismatch. Empty means verification is skipped (registry entries
	// that track a mutable "latest" URL can't pin a hash).
	SHA256 string
	// SHA512 is the expected hex-encoded sha512 of the artifact at URL.
	SHA512 string
	// SHA256URL/SHA512URL point at upstream checksum manifests for the
	// artifact named by URL.
	SHA256URL string
	SHA512URL string
	// User is the conventional cloud-init user for this distro
	// (alpine, debian, fedora, rocky, etc.). cloud-init will create whatever
	// user we ask for, but matching the convention means tools that
	// expect "$distro@vm" find the account, console autologin works
	// without surprises, and operators don't get a Password: prompt
	// because a user named "ubuntu" failed to materialise on Debian.
	// Empty falls back to compose's global default.
	User string
	// OSFamily is explicit guest metadata consumed by cloud-init rendering.
	// Supported values are config.ImageOSSystemd and config.ImageOSOpenRC.
	OSFamily string
	// MinMemoryMB raises the implicit VM memory for images that cannot boot
	// reliably with Holos' global default. User-specified VM memory still wins.
	MinMemoryMB int
	// RequiresVGA asks compose to add an explicit VGA device for BIOS boots.
	// Some images' GRUB configs assume a graphics terminal exists even when
	// holos runs headless with -display none.
	RequiresVGA bool
}

// Registry maps short names like "alpine" or "ubuntu:noble" to download URLs.
//
// Registry entries prefer dated or versioned artifacts over mutable "latest" /
// "current" aliases. Checksums still come from upstream checksum URLs so the
// registry avoids embedding static hashes while also avoiding symlink/checksum
// races during publisher rotations.
var Registry = []Image{
	// Alpine Linux (tiny-cloud, NoCloud datasource, BIOS).
	{Name: "alpine", Tag: "3.21", URL: "https://dl-cdn.alpinelinux.org/alpine/v3.21/releases/cloud/nocloud_alpine-3.21.6-x86_64-bios-tiny-r0.qcow2", Format: config.ImageFormatQCOW2, Default: true, User: "alpine", OSFamily: config.ImageOSOpenRC, SHA512URL: "https://dl-cdn.alpinelinux.org/alpine/v3.21/releases/cloud/nocloud_alpine-3.21.6-x86_64-bios-tiny-r0.qcow2.sha512"},

	// Arch Linux (cloud-init, official arch-boxes). Tag stays "latest" because
	// Arch is rolling, but the artifact URL is pinned to a dated image build.
	{Name: "arch", Tag: "latest", URL: "https://geo.mirror.pkgbuild.com/images/v20260515.530093/Arch-Linux-x86_64-cloudimg-20260515.530093.qcow2", Format: config.ImageFormatQCOW2, Default: true, User: "arch", OSFamily: config.ImageOSSystemd, SHA256URL: "https://geo.mirror.pkgbuild.com/images/v20260515.530093/Arch-Linux-x86_64-cloudimg-20260515.530093.qcow2.SHA256"},

	// Debian (generic variant, cloud-init). Use dated build directories rather
	// than "latest" because the symlink and SHA512SUMS can update at different
	// times during publisher rotations.
	//
	// Why "generic" and not "nocloud":
	// Debian publishes two flavours of the bookworm cloud image. The
	// "nocloud" variant is intentionally minimal: it strips out
	// openssh-server because it's meant as a base for custom images,
	// not for direct use. holos assumes sshd exists in the guest for
	// `holos exec` and healthchecks, so we'd silently produce VMs
	// where exec fails with "Connection reset by peer" forever
	// (sshd never bound port 22 because the package isn't installed).
	// The "generic" variant ships sshd enabled, still supports the
	// NoCloud datasource we feed via the cloud-localds ISO, and is
	// only ~25 MB larger.
	{Name: "debian", Tag: "12", URL: "https://cloud.debian.org/images/cloud/bookworm/20260518-2482/debian-12-generic-amd64-20260518-2482.qcow2", Format: config.ImageFormatQCOW2, Default: true, User: "debian", OSFamily: config.ImageOSSystemd, SHA512URL: "https://cloud.debian.org/images/cloud/bookworm/20260518-2482/SHA512SUMS"},
	{Name: "debian", Tag: "bookworm", URL: "https://cloud.debian.org/images/cloud/bookworm/20260518-2482/debian-12-generic-amd64-20260518-2482.qcow2", Format: config.ImageFormatQCOW2, User: "debian", OSFamily: config.ImageOSSystemd, SHA512URL: "https://cloud.debian.org/images/cloud/bookworm/20260518-2482/SHA512SUMS"},
	{Name: "debian", Tag: "13", URL: "https://cloud.debian.org/images/cloud/trixie/20260518-2482/debian-13-generic-amd64-20260518-2482.qcow2", Format: config.ImageFormatQCOW2, User: "debian", OSFamily: config.ImageOSSystemd, SHA512URL: "https://cloud.debian.org/images/cloud/trixie/20260518-2482/SHA512SUMS", RequiresVGA: true},
	{Name: "debian", Tag: "trixie", URL: "https://cloud.debian.org/images/cloud/trixie/20260518-2482/debian-13-generic-amd64-20260518-2482.qcow2", Format: config.ImageFormatQCOW2, User: "debian", OSFamily: config.ImageOSSystemd, SHA512URL: "https://cloud.debian.org/images/cloud/trixie/20260518-2482/SHA512SUMS", RequiresVGA: true},

	// Ubuntu (cloud images, NoCloud compatible). Use dated directories for the
	// same reason: "current" can rotate independently from SHA256SUMS fetches.
	{Name: "ubuntu", Tag: "noble", URL: "https://cloud-images.ubuntu.com/noble/20260323/noble-server-cloudimg-amd64.img", Format: config.ImageFormatQCOW2, Default: true, User: "ubuntu", OSFamily: config.ImageOSSystemd, SHA256URL: "https://cloud-images.ubuntu.com/noble/20260323/SHA256SUMS"},
	{Name: "ubuntu", Tag: "24.04", URL: "https://cloud-images.ubuntu.com/noble/20260323/noble-server-cloudimg-amd64.img", Format: config.ImageFormatQCOW2, User: "ubuntu", OSFamily: config.ImageOSSystemd, SHA256URL: "https://cloud-images.ubuntu.com/noble/20260323/SHA256SUMS"},
	{Name: "ubuntu", Tag: "resolute", URL: "https://cloud-images.ubuntu.com/resolute/20260421/resolute-server-cloudimg-amd64.img", Format: config.ImageFormatQCOW2, User: "ubuntu", OSFamily: config.ImageOSSystemd, SHA256URL: "https://cloud-images.ubuntu.com/resolute/20260421/SHA256SUMS"},
	{Name: "ubuntu", Tag: "26", URL: "https://cloud-images.ubuntu.com/resolute/20260421/resolute-server-cloudimg-amd64.img", Format: config.ImageFormatQCOW2, User: "ubuntu", OSFamily: config.ImageOSSystemd, SHA256URL: "https://cloud-images.ubuntu.com/resolute/20260421/SHA256SUMS"},
	{Name: "ubuntu", Tag: "26.04", URL: "https://cloud-images.ubuntu.com/resolute/20260421/resolute-server-cloudimg-amd64.img", Format: config.ImageFormatQCOW2, User: "ubuntu", OSFamily: config.ImageOSSystemd, SHA256URL: "https://cloud-images.ubuntu.com/resolute/20260421/SHA256SUMS"},
	{Name: "ubuntu", Tag: "jammy", URL: "https://cloud-images.ubuntu.com/jammy/20260320/jammy-server-cloudimg-amd64.img", Format: config.ImageFormatQCOW2, User: "ubuntu", OSFamily: config.ImageOSSystemd, SHA256URL: "https://cloud-images.ubuntu.com/jammy/20260320/SHA256SUMS"},
	{Name: "ubuntu", Tag: "22.04", URL: "https://cloud-images.ubuntu.com/jammy/20260320/jammy-server-cloudimg-amd64.img", Format: config.ImageFormatQCOW2, User: "ubuntu", OSFamily: config.ImageOSSystemd, SHA256URL: "https://cloud-images.ubuntu.com/jammy/20260320/SHA256SUMS"},

	// Fedora Cloud Base. Point release URL but still versioned.
	{Name: "fedora", Tag: "44", URL: "https://download.fedoraproject.org/pub/fedora/linux/releases/44/Cloud/x86_64/images/Fedora-Cloud-Base-Generic-44-1.7.x86_64.qcow2", Format: config.ImageFormatQCOW2, Default: true, User: "fedora", OSFamily: config.ImageOSSystemd, SHA256URL: "https://download.fedoraproject.org/pub/fedora/linux/releases/44/Cloud/x86_64/images/Fedora-Cloud-44-1.7-x86_64-CHECKSUM"},
	{Name: "fedora", Tag: "43", URL: "https://download.fedoraproject.org/pub/fedora/linux/releases/43/Cloud/x86_64/images/Fedora-Cloud-Base-Generic-43-1.6.x86_64.qcow2", Format: config.ImageFormatQCOW2, User: "fedora", OSFamily: config.ImageOSSystemd, SHA256URL: "https://dl.fedoraproject.org/pub/fedora/linux/releases/43/Cloud/x86_64/images/Fedora-Cloud-43-1.6-x86_64-CHECKSUM"},

	// Red Hat-family cloud images. AlmaLinux entries use dated artifacts rather
	// than "latest" because the symlink and CHECKSUM can update at different
	// times during publisher rotations.
	{Name: "almalinux", Tag: "10", URL: "https://repo.almalinux.org/almalinux/10/cloud/x86_64/images/AlmaLinux-10-GenericCloud-10.1-20260518.0.x86_64.qcow2", Format: config.ImageFormatQCOW2, Default: true, User: "almalinux", OSFamily: config.ImageOSSystemd, SHA256URL: "https://repo.almalinux.org/almalinux/10/cloud/x86_64/images/CHECKSUM"},
	{Name: "almalinux", Tag: "9", URL: "https://repo.almalinux.org/almalinux/9/cloud/x86_64/images/AlmaLinux-9-GenericCloud-9.7-20260518.x86_64.qcow2", Format: config.ImageFormatQCOW2, User: "almalinux", OSFamily: config.ImageOSSystemd, SHA256URL: "https://repo.almalinux.org/almalinux/9/cloud/x86_64/images/CHECKSUM"},
	{Name: "rocky", Tag: "10", URL: "https://dl.rockylinux.org/pub/rocky/10/images/x86_64/Rocky-10-GenericCloud-Base-10.1-20251116.0.x86_64.qcow2", Format: config.ImageFormatQCOW2, Default: true, User: "rocky", OSFamily: config.ImageOSSystemd, SHA256URL: "https://dl.rockylinux.org/pub/rocky/10/images/x86_64/Rocky-10-GenericCloud-Base-10.1-20251116.0.x86_64.qcow2.CHECKSUM", RequiresVGA: true},
	{Name: "rocky", Tag: "9", URL: "https://dl.rockylinux.org/pub/rocky/9/images/x86_64/Rocky-9-GenericCloud-Base-9.7-20251123.2.x86_64.qcow2", Format: config.ImageFormatQCOW2, User: "rocky", OSFamily: config.ImageOSSystemd, SHA256URL: "https://dl.rockylinux.org/pub/rocky/9/images/x86_64/Rocky-9-GenericCloud-Base-9.7-20251123.2.x86_64.qcow2.CHECKSUM"},
	{Name: "centos-stream", Tag: "10", URL: "https://cloud.centos.org/centos/10-stream/x86_64/images/CentOS-Stream-GenericCloud-10-20260513.0.x86_64.qcow2", Format: config.ImageFormatQCOW2, Default: true, User: "cloud-user", OSFamily: config.ImageOSSystemd, MinMemoryMB: 1024, SHA256URL: "https://cloud.centos.org/centos/10-stream/x86_64/images/CHECKSUM"},
}
