package dockerfile

import (
	"fmt"
	"strings"
)

func supportedInstructionList() string {
	return strings.Join(supportedInstructionNames, ", ")
}

func unsupportedInstructionError(cmd string) error {
	switch cmd {
	case "ARG":
		return fmt.Errorf("Dockerfile ARG is not supported; set concrete values with ENV or cloud_init.runcmd")
	case "CMD", "ENTRYPOINT":
		return fmt.Errorf("Dockerfile %s is not supported; use cloud_init.runcmd or a systemd unit inside the guest to start long-running processes", cmd)
	case "EXPOSE":
		return fmt.Errorf("Dockerfile EXPOSE is not supported; publish ports in holos.yaml with services.<name>.ports")
	case "LABEL", "ONBUILD", "SHELL", "STOPSIGNAL", "USER", "VOLUME":
		return fmt.Errorf("Dockerfile %s is not supported by holos's cloud-init provisioning model", cmd)
	default:
		return fmt.Errorf("Dockerfile instruction %s is not supported; supported instructions are %s", cmd, supportedInstructionList())
	}
}
