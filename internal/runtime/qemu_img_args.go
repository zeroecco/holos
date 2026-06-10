package runtime

import "fmt"

const (
	qemuImgCreateSubcommand  = "create"
	qemuImgFormatFlag        = "-f"
	qemuImgBackingFormatFlag = "-F"
	qemuImgBackingFileFlag   = "-b"
)

func byteSizeArg(sizeBytes int64) string {
	return fmt.Sprintf("%d", sizeBytes)
}
