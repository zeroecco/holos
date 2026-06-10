package compose

import "fmt"

func allocateServiceIPs(services map[string]Service, order []string) (map[string]string, map[string][]string, error) {
	hosts := make(map[string]string)
	serviceIPs := make(map[string][]string)
	ipCounter := 2

	for _, name := range order {
		svc := services[name]
		replicas, _ := serviceReplicas(svc)

		ips := make([]string, replicas)
		for i := 0; i < replicas; i++ {
			ip := fmt.Sprintf("10.10.0.%d", ipCounter)
			instanceName := fmt.Sprintf("%s-%d", name, i)
			hosts[instanceName] = ip
			ips[i] = ip
			ipCounter++
		}
		if err := addHostAlias(hosts, name, ips[0]); err != nil {
			return nil, nil, err
		}
		for _, alias := range serviceNetworkAliases(svc) {
			if err := addHostAlias(hosts, alias, ips[0]); err != nil {
				return nil, nil, fmt.Errorf("service %q alias %q: %w", name, alias, err)
			}
		}
		serviceIPs[name] = ips
	}

	return hosts, serviceIPs, nil
}

func serviceNetworkAliases(svc Service) []string {
	var aliases []string
	for _, network := range svc.Networks {
		aliases = append(aliases, network.Aliases...)
	}
	return aliases
}

func addHostAlias(hosts map[string]string, name string, ip string) error {
	if name == "" {
		return nil
	}
	if existing, ok := hosts[name]; ok && existing != ip {
		return fmt.Errorf("conflicts with existing host %q at %s", name, existing)
	}
	hosts[name] = ip
	return nil
}
