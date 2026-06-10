package compose

import "fmt"

func allocateServiceIPs(services map[string]Service, order []string) (map[string]string, map[string][]string) {
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
		hosts[name] = ips[0]
		serviceIPs[name] = ips
	}

	return hosts, serviceIPs
}
