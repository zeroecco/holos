package runtime

import "path/filepath"

const (
	seedDirName           = "seed"
	seedUserDataFilename  = "user-data"
	seedMetaDataFilename  = "meta-data"
	seedNetworkConfigName = "network-config"
	seedCloudLocalDSName  = "seed.img"
	seedISOName           = "seed.iso"
)

type seedPaths struct {
	dir               string
	userData          string
	metaData          string
	networkConfig     string
	cloudLocalDSImage string
	isoImage          string
}

func newSeedPaths(workDir string) seedPaths {
	dir := filepath.Join(workDir, seedDirName)
	return seedPaths{
		dir:               dir,
		userData:          filepath.Join(dir, seedUserDataFilename),
		metaData:          filepath.Join(dir, seedMetaDataFilename),
		networkConfig:     filepath.Join(dir, seedNetworkConfigName),
		cloudLocalDSImage: filepath.Join(workDir, seedCloudLocalDSName),
		isoImage:          filepath.Join(workDir, seedISOName),
	}
}
