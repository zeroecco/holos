package compose

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

// These fields have runtime behavior or deliberately retained metadata (labels).
// Nested options are checked separately. Keep this list aligned with service
// resolution when implementing additional Compose behavior.
var supportedServiceFields = []string{
	"image", "image_format", "image_os", "build", "dockerfile",
	"command", "entrypoint", "working_dir", "user", "deploy", "replicas", "scale",
	"hostname", "domainname", "cpus", "mem_limit", "vm", "ports", "volumes",
	"devices", "depends_on", "labels", "label_file", "extra_hosts", "dns_search",
	"env_file", "environment", "networks", "configs", "secrets", "mac_address",
	"post_start", "pre_stop", "cloud_init", "stop_grace_period", "healthcheck",
}

var supportedNetworkOptions = []string{
	"holos.bridge.name", "holos.bridge", "holos.tap.bridge", "bridge",
	"com.docker.network.bridge.name", "holos.import.type", "holos.import.source",
}

// CompatibilityWarnings names explicitly supplied settings whose behavior is
// ignored. Inspect source fields to preserve explicit false/0 values and options
// discarded by custom decoders. Never include values: they may contain secrets.
func (f *File) CompatibilityWarnings() []string {
	var warnings compatibilityWarnings
	for name, service := range sourceObject(f.sourceFields["services"]) {
		warnings.service(sourceObject(service), "services."+name)
	}
	warnings.project(f.sourceFields)
	sort.Strings(warnings)
	return warnings
}

type compatibilityWarnings []string

func (warnings *compatibilityWarnings) ignored(path string) {
	*warnings = append(*warnings, path+": accepted for compatibility but ignored by Holos")
}

func (warnings *compatibilityWarnings) fields(value any, path string, supported ...string) {
	for field := range sourceObject(value) {
		if !slices.Contains(supported, field) && !strings.HasPrefix(field, "x-") {
			warnings.ignored(path + "." + field)
		}
	}
}

func (warnings *compatibilityWarnings) service(service map[string]any, path string) {
	warnings.fields(service, path, supportedServiceFields...)
	warnings.fields(service["build"], path+".build", "context", "dockerfile", "dockerfile_inline")

	deploy := sourceObject(service["deploy"])
	warnings.fields(deploy, path+".deploy", "replicas", "resources")
	resources := sourceObject(deploy["resources"])
	warnings.fields(resources, path+".deploy.resources", "limits")
	warnings.fields(resources["limits"], path+".deploy.resources.limits", "cpus", "memory")

	for name, network := range sourceObject(service["networks"]) {
		warnings.fields(network, path+".networks."+name, "aliases", "mac_address")
	}
	for name, dependency := range sourceObject(service["depends_on"]) {
		warnings.fields(dependency, path+".depends_on."+name)
	}
	warnings.serviceVolumes(service["volumes"], path+".volumes")
	warnings.serviceDevices(service["devices"], path+".devices")
	for _, hook := range []string{"post_start", "pre_stop"} {
		for index, value := range sourceSequence(service[hook]) {
			warnings.fields(value, fmt.Sprintf("%s.%s[%d]", path, hook, index), "command", "working_dir", "environment")
		}
	}
	for index, port := range sourceSequence(service["ports"]) {
		warnings.fields(port, fmt.Sprintf("%s.ports[%d]", path, index), "target", "published", "host_ip", "protocol", "name")
	}
}

func (warnings *compatibilityWarnings) serviceVolumes(value any, path string) {
	for index, entry := range sourceSequence(value) {
		volume := sourceObject(entry)
		volumePath := fmt.Sprintf("%s[%d]", path, index)
		kind, _ := volume["type"].(string)
		if kind != "" && kind != "bind" && kind != "volume" {
			warnings.ignored(volumePath)
			continue
		}
		warnings.fields(volume, volumePath, "type", "source", "target", "read_only")
	}
}

func (warnings *compatibilityWarnings) serviceDevices(value any, path string) {
	for index, entry := range sourceSequence(value) {
		device := sourceObject(entry)
		devicePath := fmt.Sprintf("%s[%d]", path, index)
		if pci, _ := device["pci"].(string); pci == "" {
			warnings.ignored(devicePath)
			continue
		}
		warnings.fields(device, devicePath, "pci", "rom_file")
	}
}

func (warnings *compatibilityWarnings) project(root map[string]any) {
	for name, volume := range sourceObject(root["volumes"]) {
		path := "volumes." + name
		warnings.fields(volume, path, "size", "driver_opts")
		warnings.fields(sourceObject(volume)["driver_opts"], path+".driver_opts", "source")
	}
	for name, value := range sourceObject(root["networks"]) {
		path := "networks." + name
		network := sourceObject(value)
		warnings.fields(network, path, "driver", "driver_opts")
		if driver, ok := network["driver"].(string); ok && driver != "bridge" && driver != "tap" {
			warnings.ignored(path + ".driver")
		}
		warnings.fields(network["driver_opts"], path+".driver_opts", supportedNetworkOptions...)
	}
	if _, present := root["models"]; present {
		warnings.ignored("models")
	}
	for name, config := range sourceObject(root["configs"]) {
		warnings.fields(config, "configs."+name, "file", "content", "environment", "external")
	}
	for name, secret := range sourceObject(root["secrets"]) {
		warnings.fields(secret, "secrets."+name, "file", "environment", "external")
	}
}

// Scalars and sequences have no mapping fields to check. Typed decoding has
// already validated each field's accepted YAML forms before diagnostics run.
func sourceObject(value any) map[string]any {
	object, _ := value.(map[string]any)
	return object
}

func sourceSequence(value any) []any {
	sequence, _ := value.([]any)
	return sequence
}
