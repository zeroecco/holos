package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os/exec"

	"github.com/zeroecco/holos/internal/config"
	"github.com/zeroecco/holos/internal/qemu"
)

const (
	tapBackendName = "tap"
	tapNamePrefix  = "ht"
	tapNameHexLen  = 12
)

type tapAttachment struct {
	NetdevID string
	IfName   string
	Bridge   string
}

func (m *Manager) prepareManagedTaps(project string, manifest config.Manifest, spec *qemu.LaunchSpec) ([]tapAttachment, error) {
	taps := managedTapAttachments(project, manifest, spec.Name)
	if len(taps) == 0 {
		return nil, nil
	}
	ip, err := m.ipBinary()
	if err != nil {
		return nil, err
	}
	created := make([]tapAttachment, 0, len(taps))
	for _, tap := range taps {
		if err := runIP(ip, "tuntap", "add", "dev", tap.IfName, "mode", "tap"); err != nil {
			cleanupManagedTaps(ip, created)
			return nil, fmt.Errorf("create tap %s: %w", tap.IfName, err)
		}
		created = append(created, tap)
		if err := runIP(ip, "link", "set", tap.IfName, "master", tap.Bridge); err != nil {
			cleanupManagedTaps(ip, created)
			return nil, fmt.Errorf("attach tap %s to bridge %s: %w", tap.IfName, tap.Bridge, err)
		}
		if err := runIP(ip, "link", "set", tap.IfName, "up"); err != nil {
			cleanupManagedTaps(ip, created)
			return nil, fmt.Errorf("bring tap %s up: %w", tap.IfName, err)
		}
		if spec.TapIfNames == nil {
			spec.TapIfNames = map[string]string{}
		}
		spec.TapIfNames[tap.NetdevID] = tap.IfName
	}
	return created, nil
}

func managedTapAttachments(project string, manifest config.Manifest, instanceName string) []tapAttachment {
	if manifest.InternalNetwork == nil {
		return nil
	}
	var taps []tapAttachment
	if manifest.InternalNetwork.Backend == tapBackendName && manifest.InternalNetwork.BridgeName != "" {
		taps = append(taps, tapAttachment{
			NetdevID: "net1",
			IfName:   managedTapName(project, instanceName, "net1"),
			Bridge:   manifest.InternalNetwork.BridgeName,
		})
	}
	for i, segment := range manifest.InternalNetwork.Segments {
		if segment.Backend != tapBackendName || segment.BridgeName == "" {
			continue
		}
		netdevID := fmt.Sprintf("net%d", i+2)
		taps = append(taps, tapAttachment{
			NetdevID: netdevID,
			IfName:   managedTapName(project, instanceName, netdevID),
			Bridge:   segment.BridgeName,
		})
	}
	return taps
}

func managedTapName(project string, instanceName string, netdevID string) string {
	sum := sha256.Sum256([]byte(project + "/" + instanceName + "/" + netdevID))
	return tapNamePrefix + hex.EncodeToString(sum[:])[:tapNameHexLen]
}

func cleanupManagedTaps(ip string, taps []tapAttachment) {
	for _, tap := range taps {
		_ = runIP(ip, "link", "del", tap.IfName)
	}
}

func (m *Manager) cleanupInstanceTaps(instance InstanceRecord) {
	if len(instance.TapIfNames) == 0 {
		return
	}
	ip, err := m.ipBinary()
	if err != nil {
		return
	}
	for _, ifName := range instance.TapIfNames {
		_ = runIP(ip, "link", "del", ifName)
	}
}

func runIP(binary string, args ...string) error {
	output, err := exec.Command(binary, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %w", string(output), err)
	}
	return nil
}

func tapIfNameMap(taps []tapAttachment) map[string]string {
	if len(taps) == 0 {
		return nil
	}
	names := make(map[string]string, len(taps))
	for _, tap := range taps {
		names[tap.NetdevID] = tap.IfName
	}
	return names
}

func copyTapIfNames(names map[string]string) map[string]string {
	if len(names) == 0 {
		return nil
	}
	m := make(map[string]string, len(names))
	for netdevID, name := range names {
		m[netdevID] = name
	}
	return m
}
