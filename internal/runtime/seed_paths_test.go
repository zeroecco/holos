package runtime

import "testing"

func TestNewSeedPaths(t *testing.T) {
	workDir := testPathWorkDir(0)

	paths := newSeedPaths(workDir)

	assertPath(t, "dir", paths.dir, "state/holos/instances/demo/web-0/seed")
	assertPath(t, "userData", paths.userData, "state/holos/instances/demo/web-0/seed/user-data")
	assertPath(t, "metaData", paths.metaData, "state/holos/instances/demo/web-0/seed/meta-data")
	assertPath(t, "networkConfig", paths.networkConfig, "state/holos/instances/demo/web-0/seed/network-config")
	assertPath(t, "cloudLocalDSImage", paths.cloudLocalDSImage, "state/holos/instances/demo/web-0/seed.img")
	assertPath(t, "isoImage", paths.isoImage, "state/holos/instances/demo/web-0/seed.iso")
}
