package runtime

import "fmt"

const (
	qemuImgCreateSubcommand   = "create"
	qemuImgResizeSubcommand   = "resize"
	qemuImgResizeShrinkFlag   = "--shrink"
	qemuImgSnapshotSubcommand = "snapshot"
	qemuImgSnapshotCreateFlag = "-c"
	qemuImgSnapshotListFlag   = "-l"
	qemuImgSnapshotDeleteFlag = "-d"
	qemuImgSnapshotApplyFlag  = "-a"
	qemuImgConvertSubcommand  = "convert"
	qemuImgOutputFormatFlag   = "-O"
	qemuImgSnapshotInputFlag  = "-l"
	qemuImgFormatFlag         = "-f"
	qemuImgBackingFormatFlag  = "-F"
	qemuImgBackingFileFlag    = "-b"
)

func byteSizeArg(sizeBytes int64) string {
	return fmt.Sprintf("%d", sizeBytes)
}

func diskSnapshotListArgs(path string) []string {
	return []string{qemuImgSnapshotSubcommand, qemuImgSnapshotListFlag, path}
}

func diskSnapshotDeleteArgs(snapshotName, path string) []string {
	return []string{qemuImgSnapshotSubcommand, qemuImgSnapshotDeleteFlag, snapshotName, path}
}

func diskSnapshotApplyArgs(snapshotName, path string) []string {
	return []string{qemuImgSnapshotSubcommand, qemuImgSnapshotApplyFlag, snapshotName, path}
}

func diskSnapshotExportArgs(snapshotName, source, destination string) []string {
	return []string{qemuImgConvertSubcommand, qemuImgOutputFormatFlag, "qcow2", qemuImgSnapshotInputFlag, snapshotName, source, destination}
}
