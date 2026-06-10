package compose

import (
	"testing"

	"github.com/zeroecco/holos/internal/config"
	"gopkg.in/yaml.v3"
)

type testComposePortLongWant struct {
	name        string
	target      int
	published   string
	hostIP      string
	protocol    string
	appProtocol string
	mode        string
}

func assertComposePortLongFields(t *testing.T, got ComposePort, want testComposePortLongWant) {
	t.Helper()

	if got.Name != want.name ||
		got.Target != want.target ||
		got.Published != want.published ||
		got.HostIP != want.hostIP ||
		got.Protocol != want.protocol ||
		got.AppProtocol != want.appProtocol ||
		got.Mode != want.mode {
		t.Fatalf("port long fields = %+v, want %+v", got, want)
	}
}

func assertComposePortLongSyntax(t *testing.T, got composePortLongSyntax, want testComposePortLongWant) {
	t.Helper()

	if got.Name != want.name ||
		got.Target != want.target ||
		got.Published != want.published ||
		got.HostIP != want.hostIP ||
		got.Protocol != want.protocol ||
		got.AppProtocol != want.appProtocol ||
		got.Mode != want.mode {
		t.Fatalf("MarshalYAML long syntax = %+v, want %+v", got, want)
	}
}

func TestNormalizePortProtocol(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "default", want: config.DefaultProtocol},
		{name: "explicit tcp", input: config.DefaultProtocol, want: config.DefaultProtocol},
		{name: "unsupported", input: "udp", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := normalizePortProtocol(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("normalizePortProtocol error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizePortProtocol: %v", err)
			}
			if got != tt.want {
				t.Fatalf("normalizePortProtocol = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSplitPortProtocol(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		spec         string
		wantSpec     string
		wantProtocol string
		wantErr      string
	}{
		{name: "default", spec: "8080:80", wantSpec: "8080:80", wantProtocol: config.DefaultProtocol},
		{name: "explicit tcp", spec: "8080:80/" + config.DefaultProtocol, wantSpec: "8080:80", wantProtocol: config.DefaultProtocol},
		{name: "unsupported", spec: "53:53/udp", wantErr: "unsupported"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotSpec, gotProtocol, err := splitPortProtocol(tt.spec)
			if tt.wantErr != "" {
				assertErrorContains(t, err, tt.wantErr)
				return
			}
			if err != nil {
				t.Fatalf("splitPortProtocol: %v", err)
			}
			if gotSpec != tt.wantSpec || gotProtocol != tt.wantProtocol {
				t.Fatalf("splitPortProtocol = %q, %q; want %q, %q",
					gotSpec, gotProtocol, tt.wantSpec, tt.wantProtocol)
			}
		})
	}
}

func TestGuestOnlyPortForwards(t *testing.T) {
	t.Parallel()

	got := guestOnlyPortForwards([]int{80, 443}, config.DefaultProtocol)
	want := []config.PortForward{
		{GuestPort: 80, Protocol: config.DefaultProtocol},
		{GuestPort: 443, Protocol: config.DefaultProtocol},
	}
	assertPortForwards(t, "guestOnlyPortForwards", got, want)
}

func TestParseLabeledPortRanges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		parse   func(string) ([]int, error)
		input   string
		want    []int
		wantErr string
	}{
		{name: "guest only", parse: parseGuestOnlyPortRange, input: "80-81", want: []int{80, 81}},
		{name: "guest only invalid", parse: parseGuestOnlyPortRange, input: "bad", wantErr: "invalid port"},
		{name: "host invalid", parse: parseHostPortRange, input: "bad", wantErr: "invalid host port"},
		{name: "guest invalid", parse: parseGuestPortRange, input: "bad", wantErr: "invalid guest port"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := tt.parse(tt.input)
			if tt.wantErr != "" {
				assertErrorContains(t, err, tt.wantErr)
				return
			}
			if err != nil {
				t.Fatalf("parse labeled port range: %v", err)
			}
			assertIntSliceEqual(t, "parse labeled port range", got, tt.want)
		})
	}
}

func TestComposePortForward(t *testing.T) {
	t.Parallel()

	got := composePortForward(
		ComposePort{Name: "web", Target: 80},
		"127.0.0.1",
		8080,
		config.DefaultProtocol,
	)
	want := config.PortForward{
		Name:      "web",
		HostAddr:  "127.0.0.1",
		HostPort:  8080,
		GuestPort: 80,
		Protocol:  config.DefaultProtocol,
	}
	if got != want {
		t.Fatalf("composePortForward = %+v, want %+v", got, want)
	}
}

func TestRangePortForward(t *testing.T) {
	t.Parallel()

	got := rangePortForward(8080, 80, "127.0.0.1", "10.0.2.15", config.DefaultProtocol)
	want := config.PortForward{
		HostAddr:  "127.0.0.1",
		HostPort:  8080,
		GuestAddr: "10.0.2.15",
		GuestPort: 80,
		Protocol:  config.DefaultProtocol,
	}
	if got != want {
		t.Fatalf("rangePortForward = %+v, want %+v", got, want)
	}
}

func TestComposePortHostPorts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		published string
		want      []int
		wantErr   string
	}{
		{name: "empty defaults to ephemeral", want: []int{ephemeralHostPort}},
		{name: "single", published: "8080", want: []int{8080}},
		{name: "range", published: "8080-8081", want: []int{8080, 8081}},
		{name: "invalid", published: "8081-8080", wantErr: "invalid published port"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := composePortHostPorts(tt.published)
			if tt.wantErr != "" {
				assertErrorContains(t, err, tt.wantErr)
				return
			}
			if err != nil {
				t.Fatalf("composePortHostPorts: %v", err)
			}
			assertIntSliceEqual(t, "composePortHostPorts", got, tt.want)
		})
	}
}

func TestParseComposePortRequiresLongFormTarget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		port    ComposePort
		wantErr string
	}{
		{name: "missing target", wantErr: "target is required"},
		{name: "explicit zero target", port: ComposePort{hasTarget: true}},
		{name: "nonzero target", port: ComposePort{Target: 80}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := parseComposePort(tt.port)
			if tt.wantErr != "" {
				assertErrorContains(t, err, tt.wantErr)
				return
			}
			if err != nil {
				t.Fatalf("parseComposePort: %v", err)
			}
		})
	}
}

func TestComposePortUnmarshalAcceptsKnownLongFormFields(t *testing.T) {
	t.Parallel()

	var port ComposePort
	err := yaml.Unmarshal([]byte(`
name: web
target: 80
published: "8080"
host_ip: 127.0.0.1
protocol: tcp
app_protocol: http
mode: host
`), &port)
	if err != nil {
		t.Fatalf("unmarshal port: %v", err)
	}
	assertComposePortLongFields(t, port, testComposePortLongWant{
		name:        "web",
		target:      80,
		published:   "8080",
		hostIP:      "127.0.0.1",
		protocol:    config.DefaultProtocol,
		appProtocol: "http",
		mode:        "host",
	})
	if !port.hasTarget {
		t.Fatalf("hasTarget = false, want true")
	}
}

func TestComposePortRejectsUnknownLongFormField(t *testing.T) {
	t.Parallel()

	var port ComposePort
	err := yaml.Unmarshal([]byte(`
target: 80
published: "8080"
unexpected: value
`), &port)
	assertErrorContains(t, err, "field unexpected not found")
}

func TestDecodeComposePortTarget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		want    int
		wantErr string
	}{
		{name: "integer", raw: "80", want: 80},
		{name: "string", raw: `"443"`, want: 443},
		{name: "invalid", raw: "http", wantErr: "target:"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var node yaml.Node
			if err := yaml.Unmarshal([]byte(tt.raw), &node); err != nil {
				t.Fatalf("unmarshal node: %v", err)
			}
			got, err := decodeComposePortTarget(node.Content[0])
			if tt.wantErr != "" {
				assertErrorContains(t, err, tt.wantErr)
				return
			}
			if err != nil {
				t.Fatalf("decodeComposePortTarget: %v", err)
			}
			if got != tt.want {
				t.Fatalf("decodeComposePortTarget = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestDecodeComposePortPublished(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr string
	}{
		{name: "integer", raw: "8080", want: "8080"},
		{name: "string", raw: `"8090-8091"`, want: "8090-8091"},
		{name: "invalid", raw: "[8080]", wantErr: "published:"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var node yaml.Node
			if err := yaml.Unmarshal([]byte(tt.raw), &node); err != nil {
				t.Fatalf("unmarshal node: %v", err)
			}
			got, err := decodeComposePortPublished(node.Content[0])
			if tt.wantErr != "" {
				assertErrorContains(t, err, tt.wantErr)
				return
			}
			if err != nil {
				t.Fatalf("decodeComposePortPublished: %v", err)
			}
			if got != tt.want {
				t.Fatalf("decodeComposePortPublished = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDecodeComposePortString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr string
	}{
		{name: "scalar", raw: "host", want: "host"},
		{name: "invalid", raw: "{mode: host}", wantErr: "cannot unmarshal"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var node yaml.Node
			if err := yaml.Unmarshal([]byte(tt.raw), &node); err != nil {
				t.Fatalf("unmarshal node: %v", err)
			}
			got, err := decodeComposePortString(node.Content[0])
			if tt.wantErr != "" {
				assertErrorContains(t, err, tt.wantErr)
				return
			}
			if err != nil {
				t.Fatalf("decodeComposePortString: %v", err)
			}
			if got != tt.want {
				t.Fatalf("decodeComposePortString = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestComposePortMarshalYAML(t *testing.T) {
	t.Parallel()

	t.Run("short syntax", func(t *testing.T) {
		t.Parallel()

		got, err := (ComposePort{Short: "127.0.0.1:8080:80/tcp"}).MarshalYAML()
		if err != nil {
			t.Fatalf("MarshalYAML: %v", err)
		}
		if got != "127.0.0.1:8080:80/tcp" {
			t.Fatalf("MarshalYAML = %#v, want short syntax string", got)
		}
	})

	t.Run("long syntax", func(t *testing.T) {
		t.Parallel()

		got, err := (ComposePort{
			Name:        "web",
			Target:      80,
			Published:   "8080",
			HostIP:      "127.0.0.1",
			Protocol:    config.DefaultProtocol,
			AppProtocol: "http",
			Mode:        portModeHost,
		}).MarshalYAML()
		if err != nil {
			t.Fatalf("MarshalYAML: %v", err)
		}
		long, ok := got.(composePortLongSyntax)
		if !ok {
			t.Fatalf("MarshalYAML type = %T, want composePortLongSyntax", got)
		}
		assertComposePortLongSyntax(t, long, testComposePortLongWant{
			name:        "web",
			target:      80,
			published:   "8080",
			hostIP:      "127.0.0.1",
			protocol:    config.DefaultProtocol,
			appProtocol: "http",
			mode:        portModeHost,
		})
	})
}

func TestGeneratedPortName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		specIndex   int
		parsedIndex int
		parsedCount int
		want        string
	}{
		{name: "single parsed port", specIndex: 2, parsedCount: 1, want: "port-2"},
		{name: "range parsed first port", specIndex: 2, parsedIndex: 0, parsedCount: 3, want: "port-2-0"},
		{name: "range parsed later port", specIndex: 2, parsedIndex: 1, parsedCount: 3, want: "port-2-1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := generatedPortName(tt.specIndex, tt.parsedIndex, tt.parsedCount); got != tt.want {
				t.Fatalf("generatedPortName = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParsePortsGeneratesOnlyMissingNames(t *testing.T) {
	t.Parallel()

	ports, err := parsePorts([]ComposePort{
		{Target: 80},
		{Name: "web", Target: 81},
		{Target: 90, Published: "8090-8091"},
	})
	if err != nil {
		t.Fatalf("parsePorts: %v", err)
	}
	got := make([]string, 0, len(ports))
	for _, port := range ports {
		got = append(got, port.Name)
	}
	want := []string{"port-0", "web", "port-2-0", "port-2-1"}
	assertStringSliceEqual(t, "port names", got, want)
}

func TestValidatePortMode(t *testing.T) {
	t.Parallel()

	for _, mode := range []string{"", portModeHost, portModeIngress} {
		t.Run("accepted "+mode, func(t *testing.T) {
			t.Parallel()

			if err := validatePortMode(mode); err != nil {
				t.Fatalf("validatePortMode(%q): %v", mode, err)
			}
		})
	}

	err := validatePortMode("vip")
	assertErrorContains(t, err, `mode "vip" is unsupported`)
}

func TestParsePortRange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		raw  string
		want []int
	}{
		{raw: "80", want: []int{80}},
		{raw: "80-82", want: []int{80, 81, 82}},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			t.Parallel()

			got, err := parsePortRange(tt.raw)
			if err != nil {
				t.Fatalf("parsePortRange(%q): %v", tt.raw, err)
			}
			assertIntSliceEqual(t, "parsePortRange("+tt.raw+")", got, tt.want)
		})
	}

	_, err := parsePortRange("82-80")
	assertErrorContains(t, err, "range end must be >= start")
}

func TestValidatePortRangeBounds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		start   int
		end     int
		wantErr bool
	}{
		{name: "single", start: 80, end: 80},
		{name: "ascending", start: 80, end: 82},
		{name: "descending", start: 82, end: 80, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validatePortRangeBounds(tt.start, tt.end)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validatePortRangeBounds(%d, %d) error = %v, wantErr %v", tt.start, tt.end, err, tt.wantErr)
			}
		})
	}
}

func TestPortRangeValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		start int
		end   int
		want  []int
	}{
		{name: "single", start: 80, end: 80, want: []int{80}},
		{name: "range", start: 80, end: 82, want: []int{80, 81, 82}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := portRangeValues(tt.start, tt.end)
			assertIntSliceEqual(t, "portRangeValues", got, tt.want)
		})
	}
}

func TestParsePortAddressTrimsIPv6BracketsBeforeRejecting(t *testing.T) {
	t.Parallel()

	if got, err := parsePortAddress("host", "[127.0.0.1]"); err != nil || got != "127.0.0.1" {
		t.Fatalf("parsePortAddress bracketed IPv4 = %q, %v; want 127.0.0.1, nil", got, err)
	}
	_, err := parsePortAddress("host", "[::1]")
	assertErrorContains(t, err, "only IPv4 addresses are supported")
}

func TestTrimPortAddressBrackets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		raw  string
		want string
	}{
		{raw: "[127.0.0.1]", want: "127.0.0.1"},
		{raw: "127.0.0.1", want: "127.0.0.1"},
		{raw: "[::1]", want: "::1"},
		{raw: "[::1", want: "::1"},
		{raw: "::1]", want: "::1"},
	}

	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			t.Parallel()

			if got := trimPortAddressBrackets(tt.raw); got != tt.want {
				t.Fatalf("trimPortAddressBrackets(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestSplitPortSpecKeepsBracketedColonsTogether(t *testing.T) {
	t.Parallel()

	got, err := splitPortSpec("[::1]:8080:10.0.2.15:80")
	if err != nil {
		t.Fatalf("splitPortSpec: %v", err)
	}
	want := []string{"[::1]", "8080", "10.0.2.15", "80"}
	assertStringSliceEqual(t, "splitPortSpec", got, want)
}

func TestSplitPortSpecRejectsMalformedBrackets(t *testing.T) {
	t.Parallel()

	for _, spec := range []string{"[::1:8080:80", "::1]:8080:80", "[[::1]]:8080:80"} {
		t.Run(spec, func(t *testing.T) {
			t.Parallel()

			_, err := splitPortSpec(spec)
			assertErrorContains(t, err, invalidPortSpecError)
		})
	}
}

func TestParsePort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		spec      string
		hostAddr  string
		host      int
		guestAddr string
		guest     int
		protocol  string
	}{
		{"8080:80", "", 8080, "", 80, config.DefaultProtocol},
		{"443:443/tcp", "", 443, "", 443, config.DefaultProtocol},
		{"80", "", 0, "", 80, config.DefaultProtocol},
		{"127.0.0.1:8080:80", "127.0.0.1", 8080, "", 80, config.DefaultProtocol},
		{"0.0.0.0:8443:443/tcp", "0.0.0.0", 8443, "", 443, config.DefaultProtocol},
		{"127.0.0.1:8080:10.0.2.15:80", "127.0.0.1", 8080, "10.0.2.15", 80, config.DefaultProtocol},
		{"0.0.0.0:8443:10.0.2.15:443/tcp", "0.0.0.0", 8443, "10.0.2.15", 443, config.DefaultProtocol},
	}

	for _, tt := range tests {
		ports, err := parsePort(tt.spec)
		if err != nil {
			t.Fatalf("parsePort(%q): %v", tt.spec, err)
		}
		if len(ports) != 1 {
			t.Fatalf("parsePort(%q) returned %d ports, want 1", tt.spec, len(ports))
		}
		pf := ports[0]
		if pf.HostAddr != tt.hostAddr || pf.HostPort != tt.host || pf.GuestAddr != tt.guestAddr || pf.GuestPort != tt.guest || pf.Protocol != tt.protocol {
			t.Fatalf("parsePort(%q) = %+v, want host=%s:%d guest=%s:%d proto=%s",
				tt.spec, pf, tt.hostAddr, tt.host, tt.guestAddr, tt.guest, tt.protocol)
		}
	}
}

func TestParsePortRejectsInvalidAddresses(t *testing.T) {
	t.Parallel()

	for _, spec := range []string{
		"localhost:8080:10.0.2.15:80",
		"127.0.0.1:8080:localhost:80",
		"::1:8080:10.0.2.15:80",
	} {
		if _, err := parsePort(spec); err == nil {
			t.Fatalf("parsePort(%q): expected address error", spec)
		}
	}
}

func TestParsePortRejectsIPv6WithClearError(t *testing.T) {
	t.Parallel()

	for _, spec := range []string{
		"[::1]:8080:80",
		"127.0.0.1:8080:[::1]:80",
	} {
		_, err := parsePort(spec)
		assertErrorContains(t, err, "only IPv4")
	}
}

// parsePort previously accepted "/udp" and other non-TCP protocol suffixes,
// only for manifest validation to reject them later. The error must now
// surface at parse time.
func TestParsePortRejectsNonTCPProtocol(t *testing.T) {
	t.Parallel()

	for _, spec := range []string{"53:53/udp", "80/sctp"} {
		if _, err := parsePort(spec); err == nil {
			t.Fatalf("parsePort(%q): expected error for non-tcp protocol", spec)
		}
	}
}

func TestParsePortRejectsInvalidShape(t *testing.T) {
	t.Parallel()

	_, err := parsePort("127.0.0.1:8080:10.0.2.15:80:extra")
	assertErrorContains(t, err, invalidPortSpecError)
}

func TestParsePortRejectsMismatchedRanges(t *testing.T) {
	t.Parallel()

	_, err := parsePort("8080-8081:80")
	assertErrorContains(t, err, "host and guest port ranges must have the same length")
}

func TestResolveAcceptsLongPortSyntax(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeTestImage(t, dir)
	yamlDoc := `
name: longports
services:
  web:
    image: ./base.qcow2
    ports:
      - name: web
        target: "80"
        host_ip: 127.0.0.1
        published: "8080"
        protocol: tcp
        app_protocol: http
        mode: host
`
	project := resolveTestCompose(t, dir, yamlDoc)
	ports := project.Services[testComposeWebService].Ports
	if len(ports) != 1 {
		t.Fatalf("ports len = %d, want 1", len(ports))
	}
	got := ports[0]
	gotIdentity := got.Name == testComposeWebService
	gotHost := got.HostAddr == "127.0.0.1" && got.HostPort == testComposeWebHostPort
	gotGuest := got.GuestPort == testComposeWebGuestPort
	gotProtocol := got.Protocol == config.DefaultProtocol
	if !gotIdentity || !gotHost || !gotGuest || !gotProtocol {
		want := "name=web host=127.0.0.1:8080 guest=80 proto=tcp"
		t.Fatalf("resolved port = %+v, want %s", got, want)
	}
}

func TestResolveAcceptsPortRangeSyntax(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeTestImage(t, dir)
	yamlDoc := `
name: portranges
services:
  web:
    image: ./base.qcow2
    ports:
      - "8080-8081:80-81"
      - target: 90
        published: "8090-8091"
        protocol: tcp
`
	project := resolveTestCompose(t, dir, yamlDoc)
	ports := project.Services[testComposeWebService].Ports
	hostPorts := make([]int, 0, len(ports))
	guestPorts := make([]int, 0, len(ports))
	for _, port := range ports {
		hostPorts = append(hostPorts, port.HostPort)
		guestPorts = append(guestPorts, port.GuestPort)
	}
	assertIntSliceEqual(t, "port range host ports", hostPorts, []int{8080, 8081, 8090, 8091})
	assertIntSliceEqual(t, "port range guest ports", guestPorts, []int{80, 81, 90, 90})
}
