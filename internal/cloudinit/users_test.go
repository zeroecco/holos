package cloudinit

import (
	"testing"

	"github.com/zeroecco/holos/internal/config"
)

func TestRenderUserSystemd(t *testing.T) {
	t.Parallel()

	user := renderUser(config.Manifest{
		CloudInit: config.CloudInit{
			User:              config.DefaultUser,
			SSHAuthorizedKeys: []string{"ssh-ed25519 AAAATEST"},
		},
	}, familySystemd)

	assertUser(t, user, ccUser{
		Name:              config.DefaultUser,
		Groups:            systemdUserGroups,
		Shell:             systemdUserShell,
		Sudo:              systemdUserSudo,
		SSHAuthorizedKeys: []string{"ssh-ed25519 AAAATEST"},
	})
}

func TestRenderUserOpenRC(t *testing.T) {
	t.Parallel()

	user := renderUser(config.Manifest{
		CloudInit: config.CloudInit{
			User:              "alpine",
			SSHAuthorizedKeys: []string{"ssh-ed25519 AAAATEST"},
		},
	}, familyOpenRC)

	assertUser(t, user, ccUser{
		Name:              "alpine",
		Shell:             openRCUserShell,
		SSHAuthorizedKeys: []string{"ssh-ed25519 AAAATEST"},
	})
}

func assertUser(t *testing.T, got, want ccUser) {
	t.Helper()

	if got.Name != want.Name || got.Shell != want.Shell || got.Sudo != want.Sudo {
		t.Fatalf("user scalar fields = %#v, want %#v", got, want)
	}
	assertStringSliceEqual(t, "user groups", got.Groups, want.Groups)
	assertStringSliceEqual(t, "user ssh authorized keys", got.SSHAuthorizedKeys, want.SSHAuthorizedKeys)
}
