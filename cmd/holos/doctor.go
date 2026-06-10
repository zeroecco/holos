package main

import (
	"errors"
)

type doctorReport struct {
	OS       string        `json:"os"`
	Arch     string        `json:"arch"`
	StateDir string        `json:"state_dir"`
	Checks   []doctorCheck `json:"checks"`
}

type doctorCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

const (
	doctorStatusOK   = "ok"
	doctorStatusWarn = "warn"
	doctorStatusFail = "fail"
)

func runDoctor(args []string) error {
	flags := newFlagSet("doctor")
	stateDir := addStateDirFlag(flags)
	jsonOut := flags.Bool("json", false, "emit JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}

	report := buildDoctorReport(*stateDir)
	if *jsonOut {
		if err := printJSON(report); err != nil {
			return err
		}
	} else {
		printDoctorReport(report)
	}

	if doctorHasFailure(report) {
		return errors.New("doctor found failed checks")
	}
	return nil
}
