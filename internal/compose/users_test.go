package compose

import (
	"testing"

	"github.com/zeroecco/holos/internal/config"
)

func TestResolveRejectsInvalidCloudInitUser(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	stateDir := testStateDir(dir)
	writeTestImage(t, dir)

	file := &File{
		Name: "baduser",
		Services: map[string]Service{
			"vm": {
				Image: "./base.qcow2",
				CloudInit: CloudInit{
					User: "bad user",
				},
			},
		},
	}
	_, err := file.Resolve(dir, stateDir)
	assertErrorContains(t, err, "cloud_init.user")
}

func TestResolveAcceptsComposeUserSyntax(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	stateDir := testStateDir(dir)
	writeTestImage(t, dir)

	file := &File{
		Name: "composeuser",
		Services: map[string]Service{
			"vm": {
				Image: "./base.qcow2",
				User:  "alpine",
			},
		},
	}
	project, err := file.Resolve(dir, stateDir)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got := project.Services["vm"].CloudInit.User; got != "alpine" {
		t.Fatalf("cloud-init user = %q, want alpine", got)
	}

	file.Services["vm"] = Service{
		Image: "./base.qcow2",
		User:  "alpine",
		CloudInit: CloudInit{
			User: "operator",
		},
	}
	project, err = file.Resolve(dir, stateDir)
	if err != nil {
		t.Fatalf("resolve override: %v", err)
	}
	if got := project.Services["vm"].CloudInit.User; got != "operator" {
		t.Fatalf("cloud-init override user = %q, want operator", got)
	}
}

func TestUserResolutionChain(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	stateDir := testStateDir(dir)
	writeTestImage(t, dir)

	cases := []struct {
		name        string
		image       string
		explicit    string
		wantUser    string
		description string
	}{
		{"explicit-wins", "debian:bookworm", "operator", "operator", "explicit cloud_init.user beats image default"},
		{"image-default-debian", "debian:bookworm", "", "debian", "debian image yields debian user"},
		{"image-default-alpine", "alpine", "", "alpine", "alpine image yields alpine user"},
		{"image-default-fedora", "fedora", "", "fedora", "fedora image yields fedora user"},
		{"local-falls-back", "./base.qcow2", "", config.DefaultUser, "local image falls back to default user"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			file := &File{
				Name:          "usertest",
				imageResolver: composeTestImages,
				Services: map[string]Service{
					"vm": {
						Image:     c.image,
						CloudInit: CloudInit{User: c.explicit},
					},
				},
			}
			project, err := file.Resolve(dir, stateDir)
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
			got := project.Services["vm"].CloudInit.User
			if got != c.wantUser {
				t.Errorf("%s: user = %q, want %q", c.description, got, c.wantUser)
			}
		})
	}
}
