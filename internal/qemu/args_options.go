package qemu

import "strings"

const (
	qemuArgDrive  = "-drive"
	qemuArgDevice = "-device"

	qemuOptIfNone         = "if=none"
	qemuOptCacheWriteback = "cache=writeback"
	qemuOptDiscardUnmap   = "discard=unmap"
	qemuOptReadonly       = "readonly=on"
	qemuOptIfPflash       = "if=pflash"
	qemuOptIfVirtio       = "if=virtio"
	qemuOptMediaCDROM     = "media=cdrom"

	qemuOptKeyID     = "id"
	qemuOptKeyFormat = "format"
	qemuOptKeyFile   = "file"
)

func qemuKeyValue(key, value string) string {
	return key + "=" + value
}

func qemuOptions(options ...string) string {
	return strings.Join(options, qemuOptionSeparator)
}
