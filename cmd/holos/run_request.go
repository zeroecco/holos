package main

import (
	"fmt"

	"github.com/zeroecco/holos/internal/compose"
	"github.com/zeroecco/holos/internal/config"
	"github.com/zeroecco/holos/internal/images"
)

const runServiceName = "vm"

type runOptions struct {
	name       string
	vcpu       int
	memory     string
	user       string
	imageOS    string
	dockerfile string
	uefi       bool
	ports      []string
	volumes    []string
	devices    []string
	packages   []string
	runcmd     []string
}

type runRequest struct {
	projectName string
	serviceName string
	file        compose.File
}

func newRunRequest(args []string, opts runOptions) (runRequest, error) {
	image, runCmd, err := parseRunCommand(args, opts.dockerfile, opts.runcmd)
	if err != nil {
		return runRequest{}, err
	}

	memMB, err := parseRunMemory(opts.memory)
	if err != nil {
		return runRequest{}, err
	}

	devices := composeDevices(opts.devices)
	projectName := opts.name
	if projectName == "" {
		projectName = generateRunName(image)
	}
	if !runNamePattern.MatchString(projectName) {
		return runRequest{}, fmt.Errorf("project name %q must be a DNS label (lowercase letters, digits, hyphens)", projectName)
	}

	user := opts.user
	if user == "" {
		user = images.DefaultUser(image)
	}
	if err := config.ValidateUserName(user); err != nil {
		return runRequest{}, fmt.Errorf("--user: %w", err)
	}

	return runRequest{
		projectName: projectName,
		serviceName: runServiceName,
		file:        runComposeFile(projectName, image, user, runCmd, devices, memMB, opts),
	}, nil
}
