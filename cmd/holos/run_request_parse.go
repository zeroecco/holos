package main

import (
	"errors"
	"strings"

	"github.com/zeroecco/holos/internal/compose"
)

const (
	runCommandSeparator     = "--"
	runCommandArgSeparator  = " "
	runMissingImageErrorMsg = "run requires an image (e.g. `holos run ubuntu:noble`)"
)

func parseRunCommand(args []string, dockerfile string, runCmd []string) (string, []string, error) {
	for i, arg := range args {
		if arg == runCommandSeparator {
			args = append(args[:i], args[i+1:]...)
			break
		}
	}

	var image string
	var trailing []string
	if dockerfile != "" {
		if len(args) > 0 {
			image = args[0]
			trailing = args[1:]
		}
	} else {
		if len(args) == 0 {
			return "", nil, errors.New(runMissingImageErrorMsg)
		}
		image = args[0]
		trailing = args[1:]
	}
	if len(trailing) > 0 {
		runCmd = append(runCmd, strings.Join(trailing, runCommandArgSeparator))
	}
	return image, runCmd, nil
}

func parseRunMemory(raw string) (int, error) {
	if raw == "" {
		return 0, nil
	}
	return parseMemoryMB(raw)
}

func composeDevices(devices []string) []compose.ComposeDevice {
	out := make([]compose.ComposeDevice, len(devices))
	for i, device := range devices {
		out[i] = compose.ComposeDevice{PCI: device}
	}
	return out
}
