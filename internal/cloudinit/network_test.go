package cloudinit

import (
	"testing"

	"github.com/zeroecco/holos/internal/config"
)

func TestRenderNetworkConfigInternalOnly(t *testing.T) {
	t.Parallel()

	manifest := config.Manifest{InternalNetwork: &config.InternalNetworkConfig{
		InstanceIPs: []string{"10.10.0.2"},
		BaseMAC:     "52:54:00:ab:cd:00",
	}}

	got := renderNetworkConfig(manifest, 0)
	assertContains(t, got,
		"version: 2",
		internalNetworkInterface+":",
		"10.10.0.2"+internalNetworkAddressCIDR,
		"52:54:00:ab:cd:00",
	)
	assertOmits(t, got, externalNetworkInterface+":")
}

func TestRenderNetworkConfigAddsExternalDHCP(t *testing.T) {
	t.Parallel()

	manifest := config.Manifest{InternalNetwork: &config.InternalNetworkConfig{
		InstanceIPs: []string{"10.10.0.2"},
		BaseMAC:     "52:54:00:ab:cd:00",
		UserBaseMAC: "52:54:00:ab:ef:00",
	}}

	got := renderNetworkConfig(manifest, 0)
	assertContains(t, got,
		externalNetworkInterface+":",
		"dhcp4: true",
		"52:54:00:ab:ef:00",
	)
}

func TestRenderNetworkConfigAddsDNSSearch(t *testing.T) {
	t.Parallel()

	manifest := config.Manifest{InternalNetwork: &config.InternalNetworkConfig{
		InstanceIPs: []string{"10.10.0.2"},
		BaseMAC:     "52:54:00:ab:cd:00",
		DNSSearch:   []string{"svc.local", "example.test"},
	}}

	got := renderNetworkConfig(manifest, 0)
	assertContains(t, got,
		"nameservers:",
		"search:",
		"- svc.local",
		"- example.test",
	)
}

func TestRenderNetworkConfigSkipsMissingInstance(t *testing.T) {
	t.Parallel()

	manifest := config.Manifest{InternalNetwork: &config.InternalNetworkConfig{
		InstanceIPs: []string{"10.10.0.2"},
		BaseMAC:     "52:54:00:ab:cd:00",
	}}

	if got := renderNetworkConfig(manifest, 1); got != "" {
		t.Fatalf("renderNetworkConfig missing instance = %q, want empty", got)
	}
}
