package compose

import "github.com/zeroecco/holos/internal/config"

// User selection is a fallback chain:
//  1. explicit cloud_init.user from the compose file
//  2. docker-compose-compatible service.user
//  3. the image's conventional cloud user (debian -> "debian",
//     alpine -> "alpine", etc.) so cloud-init creates an account
//     that matches what the rest of the ecosystem expects
//  4. the global default ("ubuntu")
//
// Using the image default in the middle slot is what keeps
// `holos run debian` from producing a VM whose console autologin
// fails because no `ubuntu` user materialised.
func resolveServiceUser(svc Service, resolver composeImageResolver) string {
	if svc.CloudInit.User != "" {
		return svc.CloudInit.User
	}
	if svc.User != "" {
		return svc.User
	}
	if user := resolver.DefaultUser(svc.Image); user != "" {
		return user
	}
	return config.DefaultUser
}

func resolveServiceWriteFiles(baseDir string, svc Service, dockerfileWriteFiles []config.WriteFile, configs map[string]Config, secrets map[string]Secret) ([]config.WriteFile, error) {
	writeFiles := make([]config.WriteFile, 0, len(dockerfileWriteFiles)+len(svc.CloudInit.WriteFiles))
	writeFiles = append(writeFiles, dockerfileWriteFiles...)

	env, err := resolveEnvironment(baseDir, svc.EnvFile, svc.Environment)
	if err != nil {
		return nil, err
	}
	if envFile, ok := environmentFile(env); ok {
		writeFiles = append(writeFiles, envFile)
	}

	resourceFiles, err := resolveResourceWriteFiles(baseDir, svc, configs, secrets)
	if err != nil {
		return nil, err
	}
	writeFiles = append(writeFiles, resourceFiles...)

	writeFiles = append(writeFiles, normalizeComposeWriteFiles(svc.CloudInit.WriteFiles)...)
	return writeFiles, nil
}

func normalizeComposeWriteFiles(files []WriteFile) []config.WriteFile {
	out := make([]config.WriteFile, 0, len(files))
	for _, file := range files {
		out = append(out, normalizeComposeWriteFile(file))
	}
	return out
}

func normalizeComposeWriteFile(wf WriteFile) config.WriteFile {
	perms := wf.Permissions
	if perms == "" {
		perms = config.DefaultFilePermissions
	}
	owner := wf.Owner
	if owner == "" {
		owner = config.DefaultFileOwner
	}
	return config.WriteFile{
		Path:        wf.Path,
		Content:     wf.Content,
		Permissions: perms,
		Owner:       owner,
	}
}
