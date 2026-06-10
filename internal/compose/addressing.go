package compose

import (
	"fmt"
	"sort"
)

type serviceNetworkAllocation struct {
	Hosts      map[string]string
	Primary    serviceNetworkAttachment
	Additional []serviceNetworkAttachment
}

type serviceNetworkAttachment struct {
	Name    string
	Plan    NetworkSegmentPlan
	IPs     []string
	BaseMAC string
}

func allocateServiceNetworks(services map[string]Service, order []string, network NetworkPlan, projectName string) (map[string]string, map[string]serviceNetworkAllocation, error) {
	attached := make(map[string][]string, len(services))
	for _, name := range order {
		attached[name] = serviceNetworkNames(services[name])
	}

	segmentIPs := make(map[string]map[string][]string, len(network.Segments))
	for networkName, segment := range network.Segments {
		ipsByService := make(map[string][]string)
		ipCounter := 2
		for _, serviceName := range order {
			if !serviceAttachedToNetwork(attached[serviceName], networkName) {
				continue
			}
			if segment.Backend == "bridge" || segment.Backend == "tap" {
				ipsByService[serviceName] = nil
				continue
			}
			replicas, _ := serviceReplicas(services[serviceName])
			ips := make([]string, replicas)
			for i := 0; i < replicas; i++ {
				ips[i] = fmt.Sprintf("10.10.%d.%d", networkOctet(segment.Subnet), ipCounter)
				ipCounter++
			}
			ipsByService[serviceName] = ips
		}
		segmentIPs[networkName] = ipsByService
		hosts := make(map[string]string)
		for _, serviceName := range order {
			ips := ipsByService[serviceName]
			if len(ips) == 0 {
				continue
			}
			if err := addServiceHosts(hosts, serviceName, services[serviceName], networkName, ips); err != nil {
				return nil, nil, fmt.Errorf("network %q: %w", networkName, err)
			}
		}
		segment.Hosts = hosts
		network.Segments[networkName] = segment
	}

	projectHosts := make(map[string]string)
	allocations := make(map[string]serviceNetworkAllocation, len(services))
	for _, serviceName := range order {
		serviceNetworks := attached[serviceName]
		allocation := serviceNetworkAllocation{
			Hosts: make(map[string]string),
		}
		hosts, err := makeServiceHosts(serviceName, serviceNetworks, services, order, segmentIPs)
		if err != nil {
			return nil, nil, err
		}
		allocation.Hosts = hosts
		for i, networkName := range serviceNetworks {
			segment, ok := network.Segments[networkName]
			if !ok {
				return nil, nil, fmt.Errorf("service %q references undefined network %q", serviceName, networkName)
			}
			attachment := serviceNetworkAttachment{
				Name:    networkName,
				Plan:    segment,
				IPs:     append([]string(nil), segmentIPs[networkName][serviceName]...),
				BaseMAC: generateNetworkMAC(projectName, serviceName, networkName),
			}
			if i == 0 {
				allocation.Primary = attachment
			} else {
				allocation.Additional = append(allocation.Additional, attachment)
			}
		}
		allocations[serviceName] = allocation
		for host, ip := range allocation.Hosts {
			if _, ok := projectHosts[host]; !ok {
				projectHosts[host] = ip
			}
		}
	}

	return projectHosts, allocations, nil
}

func makeServiceHosts(serviceName string, serviceNetworks []string, services map[string]Service, order []string, segmentIPs map[string]map[string][]string) (map[string]string, error) {
	hosts := make(map[string]string)
	for _, peerName := range order {
		shared := firstSharedNetwork(serviceNetworks, serviceNetworkNames(services[peerName]))
		if shared == "" {
			continue
		}
		ips := segmentIPs[shared][peerName]
		if len(ips) == 0 {
			continue
		}
		if err := addServiceHosts(hosts, peerName, services[peerName], shared, ips); err != nil {
			return nil, fmt.Errorf("service %q: %w", serviceName, err)
		}
	}
	return hosts, nil
}

func addServiceHosts(hosts map[string]string, serviceName string, svc Service, networkName string, ips []string) error {
	for i, ip := range ips {
		if err := addHostAlias(hosts, fmt.Sprintf("%s-%d", serviceName, i), ip); err != nil {
			return fmt.Errorf("service %q instance %d: %w", serviceName, i, err)
		}
	}
	if err := addHostAlias(hosts, serviceName, ips[0]); err != nil {
		return fmt.Errorf("service %q: %w", serviceName, err)
	}
	for _, alias := range svc.Networks[networkName].Aliases {
		if err := addHostAlias(hosts, alias, ips[0]); err != nil {
			return fmt.Errorf("service %q alias %q: %w", serviceName, alias, err)
		}
	}
	return nil
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

func serviceNetworkNames(svc Service) []string {
	if len(svc.Networks) == 0 {
		return []string{defaultNetworkName}
	}
	names := make([]string, 0, len(svc.Networks))
	for name := range svc.Networks {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func firstSharedNetwork(left, right []string) string {
	for _, l := range left {
		for _, r := range right {
			if l == r {
				return l
			}
		}
	}
	return ""
}

func serviceAttachedToNetwork(networks []string, name string) bool {
	for _, network := range networks {
		if network == name {
			return true
		}
	}
	return false
}

func networkOctet(subnet string) int {
	var first, second, third int
	if _, err := fmt.Sscanf(subnet, "%d.%d.%d.0/24", &first, &second, &third); err != nil {
		return 0
	}
	return third
}
