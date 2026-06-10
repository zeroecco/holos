package compose

import (
	"slices"
	"testing"

	"github.com/zeroecco/holos/internal/config"
)

const (
	testComposeAppConfigPath       = "/etc/app.conf"
	testComposeAppModeContent      = "MODE=prod\n"
	testComposeAppConfigContent    = "app\n"
	testComposeExecutablePath      = "/usr/local/bin/app"
	testComposeExecutableContent   = "#!/bin/sh\n"
	testComposeExecutablePerms     = "0755"
	testComposeExecutableOwner     = "app:app"
	testComposeDockerfileBuildPath = "/var/lib/holos/build.sh"
	testComposeDockerfileBuildBody = "build\n"
)

func TestResolveServiceUser(t *testing.T) {
	t.Parallel()

	resolver := testImageResolver{users: map[string]string{
		"debian:12": "debian",
	}}
	tests := []struct {
		name string
		svc  Service
		want string
	}{
		{
			name: "explicit cloud init user wins",
			svc: Service{
				Image: "debian:12",
				User:  "compose-user",
				CloudInit: CloudInit{
					User: "cloud-user",
				},
			},
			want: "cloud-user",
		},
		{
			name: "compose service user before image default",
			svc:  Service{Image: "debian:12", User: "compose-user"},
			want: "compose-user",
		},
		{
			name: "image default before global default",
			svc:  Service{Image: "debian:12"},
			want: "debian",
		},
		{
			name: "global default fallback",
			svc:  Service{Image: "unknown:image"},
			want: config.DefaultUser,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := resolveServiceUser(tt.svc, resolver); got != tt.want {
				t.Fatalf("resolveServiceUser() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeComposeWriteFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   WriteFile
		want config.WriteFile
	}{
		{
			name: "defaults",
			in:   WriteFile{Path: testComposeAppConfigPath, Content: testComposeAppModeContent},
			want: config.WriteFile{
				Path:        testComposeAppConfigPath,
				Content:     testComposeAppModeContent,
				Permissions: config.DefaultFilePermissions,
				Owner:       config.DefaultFileOwner,
			},
		},
		{
			name: "explicit",
			in: WriteFile{
				Path:        testComposeExecutablePath,
				Content:     testComposeExecutableContent,
				Permissions: testComposeExecutablePerms,
				Owner:       testComposeExecutableOwner,
			},
			want: config.WriteFile{
				Path:        testComposeExecutablePath,
				Content:     testComposeExecutableContent,
				Permissions: testComposeExecutablePerms,
				Owner:       testComposeExecutableOwner,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := normalizeComposeWriteFile(tt.in); got != tt.want {
				t.Fatalf("normalizeComposeWriteFile = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestNormalizeComposeWriteFiles(t *testing.T) {
	t.Parallel()

	got := normalizeComposeWriteFiles([]WriteFile{
		{Path: testComposeAppConfigPath, Content: testComposeAppConfigContent},
		{
			Path:        testComposeExecutablePath,
			Content:     testComposeExecutableContent,
			Permissions: testComposeExecutablePerms,
			Owner:       testComposeExecutableOwner,
		},
	})
	want := []config.WriteFile{
		{
			Path:        testComposeAppConfigPath,
			Content:     testComposeAppConfigContent,
			Permissions: config.DefaultFilePermissions,
			Owner:       config.DefaultFileOwner,
		},
		{
			Path:        testComposeExecutablePath,
			Content:     testComposeExecutableContent,
			Permissions: testComposeExecutablePerms,
			Owner:       testComposeExecutableOwner,
		},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("normalizeComposeWriteFiles = %+v, want %+v", got, want)
	}
}

func TestResolveServiceWriteFilesOrder(t *testing.T) {
	t.Parallel()

	dockerfileWriteFiles := []config.WriteFile{{Path: testComposeDockerfileBuildPath, Content: testComposeDockerfileBuildBody}}
	svc := Service{
		Environment: Environment{
			testEnvAppKey: stringPtr("prod"),
		},
		CloudInit: CloudInit{
			WriteFiles: []WriteFile{{Path: testComposeAppConfigPath, Content: testComposeAppConfigContent}},
		},
	}

	got, err := resolveServiceWriteFiles(t.TempDir(), svc, dockerfileWriteFiles)
	if err != nil {
		t.Fatalf("resolveServiceWriteFiles: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("write_files len = %d, want 3: %+v", len(got), got)
	}
	gotPaths := writeFilePaths(got)
	wantPaths := []string{testComposeDockerfileBuildPath, environmentFilePath, testComposeAppConfigPath}
	assertStringSliceEqual(t, "write_files order", gotPaths, wantPaths)
}

func writeFilePaths(files []config.WriteFile) []string {
	paths := make([]string, 0, len(files))
	for _, file := range files {
		paths = append(paths, file.Path)
	}
	return paths
}
