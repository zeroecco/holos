package runtime

import "fmt"

const (
	qemuImgCreateSubcommand   = "create"
	qemuImgSnapshotSubcommand = "snapshot"
	qemuImgSnapshotCreateFlag = "-c"
	qemuImgFormatFlag         = "-f"
	qemuImgBackingFormatFlag  = "-F"
	qemuImgBackingFileFlag    = "-b"
)

func byteSizeArg(sizeBytes int64) string {
	return fmt.Sprintf("%d", sizeBytes)
}
