package compose

import (
	"fmt"
	"sort"
)

func (f *File) validate() error {
	if !namePattern.MatchString(f.Name) {
		return fmt.Errorf("project name %q must match %s", f.Name, namePattern.String())
	}
	if len(f.Services) == 0 {
		return fmt.Errorf("at least one service is required")
	}
	for name, network := range f.Networks {
		if network.Driver == "tap" && tapBridgeName(network) == "" {
			return fmt.Errorf("network %q driver tap requires driver_opts.holos.tap.bridge", name)
		}
	}
	for name, svc := range f.Services {
		if !namePattern.MatchString(name) {
			return fmt.Errorf("service name %q must match %s", name, namePattern.String())
		}
		if !serviceHasRunnableSource(svc) {
			return fmt.Errorf("service %q requires an image (or a dockerfile/build with FROM)", name)
		}
		for _, dep := range svc.DependsOn {
			if _, ok := f.Services[dep]; !ok {
				return fmt.Errorf("service %q depends on unknown service %q", name, dep)
			}
		}
	}
	return nil
}

func serviceHasRunnableSource(svc Service) bool {
	return svc.Image != "" || svc.Dockerfile != "" || svc.Build.isSet()
}

func (f *File) topoSort() ([]string, error) {
	inDegree := make(map[string]int)
	dependents := make(map[string][]string)

	for name := range f.Services {
		inDegree[name] = 0
	}
	for name, svc := range f.Services {
		for _, dep := range svc.DependsOn {
			inDegree[name]++
			dependents[dep] = append(dependents[dep], name)
		}
	}

	var queue []string
	for name, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, name)
		}
	}
	sort.Strings(queue)

	var order []string
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		order = append(order, node)

		deps := dependents[node]
		sort.Strings(deps)
		for _, dep := range deps {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				queue = append(queue, dep)
			}
		}
	}

	if len(order) != len(f.Services) {
		return nil, fmt.Errorf("circular dependency detected among services")
	}
	return order, nil
}
