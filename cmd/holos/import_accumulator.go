package main

import (
	"fmt"

	"github.com/zeroecco/holos/internal/compose"
	"github.com/zeroecco/holos/internal/virtimport"
)

type importAccumulator struct {
	file     compose.File
	order    []string
	warnings []string
}

func newImportAccumulator() *importAccumulator {
	return &importAccumulator{
		file: compose.File{Services: map[string]compose.Service{}},
	}
}

func (a *importAccumulator) addDomain(label string, data []byte) error {
	name, svc, volumes, warns, err := virtimport.ConvertWithVolumes(data)
	if err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	if _, exists := a.file.Services[name]; exists {
		return fmt.Errorf("%s: service name %q already imported (rename the source domain)", label, name)
	}
	a.file.Services[name] = svc
	a.addImportedVolumeDeclarations(volumes)
	a.order = append(a.order, name)
	for _, w := range warns {
		a.warnings = append(a.warnings, fmt.Sprintf("%s: %s", name, w))
	}
	return nil
}

func (a *importAccumulator) addImportedVolumeDeclarations(volumes []virtimport.ImportedVolume) {
	if len(volumes) == 0 {
		return
	}
	if a.file.Volumes == nil {
		a.file.Volumes = map[string]compose.Volume{}
	}
	for _, volume := range volumes {
		a.file.Volumes[volume.Name] = compose.Volume{
			DriverOpts: map[string]string{"source": volume.SourcePath},
		}
	}
}

func (a *importAccumulator) composeFile(projectName string) compose.File {
	file := a.file
	switch {
	case projectName != "":
		file.Name = projectName
	case len(a.order) > 0:
		file.Name = a.order[0]
	default:
		file.Name = "imported"
	}
	return file
}
