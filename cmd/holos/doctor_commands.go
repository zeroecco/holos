package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type doctorCommand struct {
	name string
	args []string
}

const (
	doctorNameSeparator    = ", "
	doctorFailureSeparator = "; "
	executableModeBits     = os.FileMode(0o111)
)

func checkCommand(name, envVar string, args []string, purpose string) doctorCheck {
	path := ""
	if override := os.Getenv(envVar); override != "" {
		if err := checkExecutable(override); err != nil {
			return doctorCheck{Name: name, Status: doctorStatusFail, Message: doctorOverrideNotExecutableMessage(envVar, override, err)}
		}
		path = override
	} else if found, err := exec.LookPath(name); err == nil {
		path = found
	} else {
		return doctorCheck{Name: name, Status: doctorStatusFail, Message: missingCommandMessage(name, envVar, purpose)}
	}

	out, err := runDoctorProbe(path, args)
	if err != nil {
		return doctorCheck{Name: name, Status: doctorStatusFail, Message: doctorProbeFailureMessage(name, path, err)}
	}
	return doctorCheck{Name: name, Status: doctorStatusOK, Message: doctorProbeSuccessMessage(path, out)}
}

func checkAnyCommand(label string, commands []doctorCommand, purpose string) doctorCheck {
	var failures []string
	for _, candidate := range commands {
		path, err := exec.LookPath(candidate.name)
		if err != nil {
			failures = append(failures, candidate.name+": not found")
			continue
		}
		out, err := runDoctorProbe(path, candidate.args)
		if err != nil {
			failures = append(failures, candidate.name+": "+err.Error())
			continue
		}
		return doctorCheck{Name: label, Status: doctorStatusOK, Message: doctorCommandSuccessMessage(candidate.name, path, out)}
	}
	var names []string
	for _, candidate := range commands {
		names = append(names, candidate.name)
	}
	detail := purpose + "; install one of " + strings.Join(names, doctorNameSeparator)
	if len(failures) > 0 {
		detail += " (" + strings.Join(failures, doctorFailureSeparator) + ")"
	}
	return doctorCheck{Name: label, Status: doctorStatusFail, Message: detail}
}

func doctorCommandSuccessMessage(name, path, output string) string {
	if output != "" {
		return fmt.Sprintf("%s at %s (%s)", name, path, output)
	}
	return fmt.Sprintf("%s at %s", name, path)
}

func doctorProbeSuccessMessage(path, output string) string {
	if output != "" {
		return fmt.Sprintf("%s (%s)", path, output)
	}
	return path
}

func doctorOverrideNotExecutableMessage(envVar, path string, err error) string {
	return fmt.Sprintf("%s points to %s, but it is not executable: %v", envVar, path, err)
}

func doctorProbeFailureMessage(name, path string, err error) string {
	return fmt.Sprintf("%s found at %s but probe failed: %v", name, path, err)
}

func missingCommandMessage(name, envVar, purpose string) string {
	if envVar != "" {
		return purpose + "; install it or set " + envVar
	}
	return purpose + "; install " + name
}

func checkExecutable(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("is a directory")
	}
	if info.Mode().Perm()&executableModeBits == 0 {
		return fmt.Errorf("execute bit is not set")
	}
	return nil
}
