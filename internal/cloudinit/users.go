package cloudinit

import "github.com/zeroecco/holos/internal/config"

const (
	systemdUserShell = "/bin/bash"
	openRCUserShell  = "/bin/sh"
	systemdUserSudo  = "ALL=(ALL) NOPASSWD:ALL"
)

var systemdUserGroups = []string{"adm", "sudo"}

// renderUser builds the cloud-config users[0] entry. On systemd distros we
// set shell, groups, and sudo explicitly (matching existing behavior); on
// Alpine we omit those because /bin/bash, the "adm"/"sudo" groups, and the
// sudo binary are not present in the default cloud image.
func renderUser(manifest config.Manifest, family osFamily) ccUser {
	user := ccUser{
		Name:              manifest.CloudInit.User,
		SSHAuthorizedKeys: manifest.CloudInit.SSHAuthorizedKeys,
	}
	return applyUserFamilyDefaults(user, family)
}

func applyUserFamilyDefaults(user ccUser, family osFamily) ccUser {
	switch family {
	case familySystemd:
		user.Groups = systemdUserGroups
		user.Shell = systemdUserShell
		user.Sudo = systemdUserSudo
	case familyOpenRC:
		// Leave defaults to cloud-init / tiny-cloud; /bin/sh is guaranteed.
		user.Shell = openRCUserShell
	}
	return user
}
