package runtime

import "path/filepath"

const (
	instanceOverlayFilename      = "root.qcow2"
	instanceConsoleLogFilename   = "console.log"
	instanceSerialSocketFilename = "serial.sock"
	instanceQMPSocketFilename    = "qmp.sock"
	instanceQEMULogFilename      = "qemu.log"
	instanceOVMFVarsFilename     = "OVMF_VARS.fd"
)

type instancePaths struct {
	overlay      string
	consoleLog   string
	serialSocket string
	qmpSocket    string
	qemuLog      string
	ovmfVars     string
}

func newInstancePaths(workDir string) instancePaths {
	return instancePaths{
		overlay:      filepath.Join(workDir, instanceOverlayFilename),
		consoleLog:   filepath.Join(workDir, instanceConsoleLogFilename),
		serialSocket: filepath.Join(workDir, instanceSerialSocketFilename),
		qmpSocket:    filepath.Join(workDir, instanceQMPSocketFilename),
		qemuLog:      filepath.Join(workDir, instanceQEMULogFilename),
		ovmfVars:     filepath.Join(workDir, instanceOVMFVarsFilename),
	}
}
