package main

import (
	"fmt"

	"github.com/zeroecco/holos/internal/compose"
	"github.com/zeroecco/holos/internal/config"
	"github.com/zeroecco/holos/internal/qemu"
	"github.com/zeroecco/holos/internal/runtime"
)

type inspectDocument struct {
	Kind      string                     `json:"kind"`
	Project   string                     `json:"project"`
	Record    *runtime.ProjectRecord     `json:"record,omitempty"`
	Service   *runtime.ServiceRecord     `json:"service,omitempty"`
	Instance  *runtime.InstanceRecord    `json:"instance,omitempty"`
	Manifests map[string]config.Manifest `json:"manifests,omitempty"`
	Manifest  *config.Manifest           `json:"manifest,omitempty"`
	QEMUArgs  []string                   `json:"qemu_args,omitempty"`
	Volumes   []runtime.VolumeInfo       `json:"volumes,omitempty"`
}

func runInspect(args []string) error {
	flags := newFlagSet("inspect")
	projectFlags := addProjectFlags(flags, "path to holos.yaml (adds resolved manifests and QEMU args)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 1 {
		return fmt.Errorf("usage: holos inspect [-f holos.yaml] [project|instance]")
	}

	manager := runtime.NewManager(*projectFlags.stateDir)
	project, err := loadOptionalInspectProject(*projectFlags.filePath, *projectFlags.stateDir)
	if err != nil {
		return err
	}
	target := inspectTarget(flags.Args(), project)
	if target == "" {
		return fmt.Errorf("usage: holos inspect [-f holos.yaml] [project|instance]")
	}

	doc, err := inspectTargetDocument(manager, project, target)
	if err != nil {
		return err
	}
	return printJSON(doc)
}

func loadOptionalInspectProject(filePath, stateDir string) (*compose.Project, error) {
	if filePath == "" {
		return nil, nil
	}
	return loadProject(filePath, stateDir)
}

func inspectTarget(args []string, project *compose.Project) string {
	if len(args) > 0 {
		return args[0]
	}
	if project != nil {
		return project.Name
	}
	return ""
}

func inspectTargetDocument(manager *runtime.Manager, project *compose.Project, target string) (inspectDocument, error) {
	if project != nil {
		record, err := manager.ProjectStatus(project.Name)
		if err != nil {
			return inspectDocument{}, err
		}
		if target == project.Name {
			return inspectProjectDocument(manager, project, record)
		}
		if inst, svc, ok := findInstanceInRecord(record, target); ok {
			return inspectInstanceDocument(project, record, svc, inst)
		}
		return inspectDocument{}, fmt.Errorf("inspect target %q not found in project %q", target, project.Name)
	}

	if record, ok := lookupProjectRecord(manager, target); ok {
		return inspectProjectDocument(manager, project, record)
	}
	records, err := manager.ListProjects()
	if err != nil {
		return inspectDocument{}, err
	}
	for _, record := range records {
		if inst, svc, ok := findInstanceInRecord(record, target); ok {
			return inspectInstanceDocument(project, record, svc, inst)
		}
	}
	return inspectDocument{}, fmt.Errorf("inspect target %q not found", target)
}

func inspectProjectDocument(manager *runtime.Manager, project *compose.Project, record *runtime.ProjectRecord) (inspectDocument, error) {
	volumes, err := manager.ListVolumes()
	if err != nil {
		return inspectDocument{}, err
	}
	doc := inspectDocument{
		Kind:    "project",
		Project: record.Name,
		Record:  record,
		Volumes: filterVolumesByProject(volumes, record.Name),
	}
	if project != nil && project.Name == record.Name {
		doc.Manifests = project.Services
	}
	return doc, nil
}

func inspectInstanceDocument(project *compose.Project, record *runtime.ProjectRecord, service runtime.ServiceRecord, instance runtime.InstanceRecord) (inspectDocument, error) {
	doc := inspectDocument{
		Kind:     "instance",
		Project:  record.Name,
		Service:  &service,
		Instance: &instance,
	}
	manifest, ok := inspectManifest(project, service.Name)
	if !ok {
		return doc, nil
	}
	doc.Manifest = &manifest
	args, err := inspectQEMUArgs(manifest, instance)
	if err != nil {
		return inspectDocument{}, err
	}
	doc.QEMUArgs = args
	return doc, nil
}

func inspectManifest(project *compose.Project, service string) (config.Manifest, bool) {
	if project == nil {
		return config.Manifest{}, false
	}
	manifest, ok := project.Services[service]
	return manifest, ok
}

func inspectQEMUArgs(manifest config.Manifest, instance runtime.InstanceRecord) ([]string, error) {
	spec := runtime.InspectLaunchSpec(manifest, instance)
	return qemu.BuildArgs(manifest, spec)
}
