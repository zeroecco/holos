package compose

import (
	"fmt"

	"github.com/zeroecco/holos/internal/config"
)

func serviceReplicas(svc Service) (int, error) {
	scale, err := composeInt(svc.Scale, "scale")
	if err != nil {
		return 0, err
	}
	if err := ensureReplicaSettingsAgree(svc.Replicas, scale, svc.Deploy.Replicas); err != nil {
		return 0, err
	}
	replicas := config.DefaultReplicas
	for _, value := range []int{svc.Replicas, scale, svc.Deploy.Replicas} {
		if value != 0 {
			replicas = value
			break
		}
	}
	if err := validateReplicaCount(replicas); err != nil {
		return 0, err
	}
	return replicas, nil
}

// projectInstanceCount validates every service's replica settings before IP
// allocation. Invalid counts must fail here instead of reaching
// make([]string, replicas) in allocateServiceIPs.
func projectInstanceCount(services map[string]Service, order []string) (int, error) {
	totalReplicas := 0
	for _, name := range order {
		replicas, err := serviceReplicas(services[name])
		if err != nil {
			return 0, fmt.Errorf("service %q: %w", name, err)
		}
		totalReplicas += replicas
	}
	return totalReplicas, nil
}

func validateProjectInstanceCapacity(instances int) error {
	if instances <= maxProjectInstances {
		return nil
	}
	return fmt.Errorf(
		"project requires %d instances but the internal network %s only has %d usable addresses",
		instances, subnetCIDR, maxProjectInstances)
}

func networkInstanceCount(services map[string]Service, order []string, networkName string) int {
	total := 0
	for _, name := range order {
		if !serviceAttachedToNetwork(serviceNetworkNames(services[name]), networkName) {
			continue
		}
		replicas, _ := serviceReplicas(services[name])
		total += replicas
	}
	return total
}

func validateReplicaCount(replicas int) error {
	if replicas < 1 {
		return fmt.Errorf("replicas must be >= 1")
	}
	if replicas > maxReplicas {
		return fmt.Errorf("replicas %d exceeds maximum of %d", replicas, maxReplicas)
	}
	return nil
}

func ensureReplicaSettingsAgree(replicas int, scale int, deployReplicas int) error {
	if replicas != 0 && scale != 0 && replicas != scale {
		return fmt.Errorf("replicas and scale disagree (%d != %d)", replicas, scale)
	}
	if replicas != 0 && deployReplicas != 0 && replicas != deployReplicas {
		return fmt.Errorf("replicas and deploy.replicas disagree (%d != %d)", replicas, deployReplicas)
	}
	if scale != 0 && deployReplicas != 0 && scale != deployReplicas {
		return fmt.Errorf("scale and deploy.replicas disagree (%d != %d)", scale, deployReplicas)
	}
	return nil
}
