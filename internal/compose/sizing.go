package compose

import (
	"fmt"
	"math"
	"strconv"

	"github.com/zeroecco/holos/internal/config"
)

func composeCPUs(svc Service) (float64, error) {
	cpus, err := composeFloat(svc.CPUs, "cpus")
	if err != nil {
		return 0, err
	}
	if cpus > 0 {
		return cpus, nil
	}

	for _, candidate := range []struct {
		label string
		raw   string
	}{
		{label: "deploy.resources.limits.cpus", raw: svc.Deploy.Resources.Limits.CPUs},
		{label: "deploy.resources.reservations.cpus", raw: svc.Deploy.Resources.Reservations.CPUs},
	} {
		if isBlankScalarString(candidate.raw) {
			continue
		}
		value, err := strconv.ParseFloat(candidate.raw, 64)
		if err != nil {
			return 0, fmt.Errorf("%s: %w", candidate.label, err)
		}
		return value, nil
	}
	return 0, nil
}

func composeVCPU(cpus float64) int {
	if cpus <= 0 {
		return config.DefaultVCPU
	}
	return int(math.Ceil(cpus))
}

func composeMemLimit(svc Service) string {
	for _, value := range []string{
		svc.MemLimit,
		svc.Deploy.Resources.Limits.Memory,
		svc.Deploy.Resources.Reservations.Memory,
	} {
		if !isBlankScalarString(value) {
			return value
		}
	}
	return ""
}

func composeMemoryMB(memLimit string) (int, error) {
	if isBlankScalarString(memLimit) {
		return config.DefaultMemoryMB, nil
	}
	bytes, err := parseVolumeSize(memLimit)
	if err != nil {
		return 0, fmt.Errorf("mem_limit: %w", err)
	}
	return bytesToMiBRoundedUp(bytes), nil
}

func bytesToMiBRoundedUp(bytes int64) int {
	mb := int(bytes / (1 << 20))
	if bytes%(1<<20) != 0 {
		mb++
	}
	return mb
}
