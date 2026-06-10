package compose

import (
	"testing"

	"github.com/zeroecco/holos/internal/config"
)

func TestDebian13AddsVGABootWorkaround(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	stateDir := testStateDir(dir)

	file := &File{
		Name:          "debian13",
		imageResolver: composeTestImages,
		Services: map[string]Service{
			"vm": {Image: "debian:13"},
		},
	}
	project, err := file.Resolve(dir, stateDir)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	got := project.Services["vm"].VM.ExtraArgs
	assertStringSlicePrefix(t, "debian:13 extra args", got, []string{vgaDeviceArg, vgaDeviceName})

	file.Services["vm"] = Service{Image: "debian:13", VM: VM{UEFI: true}}
	project, err = file.Resolve(dir, stateDir)
	if err != nil {
		t.Fatalf("resolve uefi: %v", err)
	}
	if got := project.Services["vm"].VM.ExtraArgs; len(got) != 0 {
		t.Fatalf("uefi debian:13 extra args = %v, want none", got)
	}
}

func TestRocky10AddsVGABootWorkaround(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	stateDir := testStateDir(dir)

	file := &File{
		Name:          "rocky10",
		imageResolver: composeTestImages,
		Services: map[string]Service{
			"vm": {Image: "rocky:10"},
		},
	}
	project, err := file.Resolve(dir, stateDir)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	got := project.Services["vm"].VM.ExtraArgs
	assertStringSlicePrefix(t, "rocky:10 extra args", got, []string{vgaDeviceArg, vgaDeviceName})
}

func TestResolveVMExtraArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		extraArgs   []string
		uefi        bool
		requiresVGA bool
		want        []string
	}{
		{name: "does not mutate existing args", extraArgs: []string{"-m", "2G"}, want: []string{"-m", "2G"}},
		{name: "prepends vga when image requires it under bios", extraArgs: []string{"-m", "2G"}, requiresVGA: true, want: []string{vgaDeviceArg, vgaDeviceName, "-m", "2G"}},
		{name: "skips vga under uefi", extraArgs: []string{"-m", "2G"}, uefi: true, requiresVGA: true, want: []string{"-m", "2G"}},
		{name: "skips vga when image does not require it", extraArgs: []string{"-m", "2G"}, want: []string{"-m", "2G"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := resolveVMExtraArgs(tt.extraArgs, tt.uefi, tt.requiresVGA)
			assertStringSliceEqual(t, "resolveVMExtraArgs()", got, tt.want)
			if len(tt.extraArgs) > 0 && tt.extraArgs[0] != "-m" {
				t.Fatalf("input extraArgs mutated: %v", tt.extraArgs)
			}
		})
	}
}

func TestResolveVMUEFI(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		explicit bool
		devices  []config.Device
		want     bool
	}{
		{name: "default off"},
		{name: "explicit on", explicit: true, want: true},
		{name: "device passthrough forces on", devices: []config.Device{{PCI: "0000:01:00.0"}}, want: true},
		{name: "explicit and device", explicit: true, devices: []config.Device{{PCI: "0000:01:00.0"}}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := resolveVMUEFI(tt.explicit, tt.devices); got != tt.want {
				t.Fatalf("resolveVMUEFI(%v, %+v) = %v, want %v", tt.explicit, tt.devices, got, tt.want)
			}
		})
	}
}

func TestResolveVMMachine(t *testing.T) {
	t.Parallel()

	if got := resolveVMMachine(""); got != config.DefaultMachine {
		t.Fatalf("resolveVMMachine(empty) = %q, want %q", got, config.DefaultMachine)
	}
	const explicitMachine = "pc"
	if got := resolveVMMachine(explicitMachine); got != explicitMachine {
		t.Fatalf("resolveVMMachine(explicit) = %q, want %q", got, explicitMachine)
	}
}

func TestResolveVMCPUModel(t *testing.T) {
	t.Parallel()

	if got := resolveVMCPUModel(""); got != config.DefaultCPUModel {
		t.Fatalf("resolveVMCPUModel(empty) = %q, want %q", got, config.DefaultCPUModel)
	}
	const explicitCPUModel = "max"
	if got := resolveVMCPUModel(explicitCPUModel); got != explicitCPUModel {
		t.Fatalf("resolveVMCPUModel(explicit) = %q, want %q", got, explicitCPUModel)
	}
}

func TestCentOSStreamUsesImageMinimumMemory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	stateDir := testStateDir(dir)

	file := &File{
		Name:          "centos",
		imageResolver: composeTestImages,
		Services: map[string]Service{
			"vm": {Image: "centos-stream"},
		},
	}
	project, err := file.Resolve(dir, stateDir)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got := project.Services["vm"].VM.MemoryMB; got != 1024 {
		t.Fatalf("implicit centos-stream memory = %d, want 1024", got)
	}

	file.Services["vm"] = Service{Image: "centos-stream", VM: VM{MemoryMB: 768}}
	project, err = file.Resolve(dir, stateDir)
	if err != nil {
		t.Fatalf("resolve explicit memory: %v", err)
	}
	if got := project.Services["vm"].VM.MemoryMB; got != 768 {
		t.Fatalf("explicit centos-stream memory = %d, want 768", got)
	}
}

func TestResolveVMMemoryMB(t *testing.T) {
	t.Parallel()

	got, err := resolveVMMemoryMB(Service{Image: "centos-stream"}, composeTestImages)
	if err != nil {
		t.Fatalf("resolveVMMemoryMB implicit: %v", err)
	}
	if got != 1024 {
		t.Fatalf("implicit memory = %d, want image minimum 1024", got)
	}

	got, err = resolveVMMemoryMB(Service{Image: "centos-stream", MemLimit: "768M"}, composeTestImages)
	if err != nil {
		t.Fatalf("resolveVMMemoryMB explicit: %v", err)
	}
	if got != 768 {
		t.Fatalf("explicit memory = %d, want 768", got)
	}
}

func TestApplyImageMinimumMemory(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		memMB    int
		memLimit string
		image    string
		want     int
	}{
		{name: "implicit raised to image minimum", memMB: config.DefaultMemoryMB, image: "centos-stream", want: 1024},
		{name: "implicit keeps larger value", memMB: 2048, image: "centos-stream", want: 2048},
		{name: "explicit keeps lower value", memMB: 768, memLimit: "768M", image: "centos-stream", want: 768},
		{name: "no image minimum", memMB: config.DefaultMemoryMB, image: "custom", want: config.DefaultMemoryMB},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := applyImageMinimumMemory(tt.memMB, tt.memLimit, tt.image, composeTestImages)
			if got != tt.want {
				t.Fatalf("applyImageMinimumMemory() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestResolveVMDiskSizeBytes(t *testing.T) {
	t.Parallel()

	got, err := resolveVMDiskSizeBytes(" ")
	if err != nil {
		t.Fatalf("resolveVMDiskSizeBytes blank: %v", err)
	}
	if got != 0 {
		t.Fatalf("blank disk size = %d, want 0", got)
	}

	got, err = resolveVMDiskSizeBytes("2G")
	if err != nil {
		t.Fatalf("resolveVMDiskSizeBytes 2G: %v", err)
	}
	if got != 2*(1<<30) {
		t.Fatalf("disk size = %d, want 2G", got)
	}

	_, err = resolveVMDiskSizeBytes("huge")
	assertErrorContains(t, err, "vm.disk_size:")
}
