package compose

import "testing"

func TestResolveAcceptsLabelsSyntax(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeTestImage(t, dir)
	yamlDoc := `
name: labels
services:
  map:
    image: ./base.qcow2
    labels:
      com.example.role: api
      com.example.empty: ""
  list:
    image: ./base.qcow2
    labels:
      - com.example.role=worker
      - com.example.flag
`
	project := resolveTestCompose(t, dir, yamlDoc)
	if got := project.Services["map"].Labels; got["com.example.role"] != "api" || got["com.example.empty"] != "" {
		t.Fatalf("map labels = %#v", got)
	}
	if got := project.Services["list"].Labels; got["com.example.role"] != "worker" || got["com.example.flag"] != "" {
		t.Fatalf("list labels = %#v", got)
	}
}

func TestParseComposeLabel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		raw       string
		wantKey   string
		wantValue string
	}{
		{name: "assignment", raw: "com.example.role=worker", wantKey: "com.example.role", wantValue: "worker"},
		{name: "flag", raw: "com.example.flag", wantKey: "com.example.flag", wantValue: ""},
		{name: "keeps equals in value", raw: "com.example.expr=a=b", wantKey: "com.example.expr", wantValue: "a=b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			key, value := parseComposeLabel(tt.raw)
			if key != tt.wantKey || value != tt.wantValue {
				t.Fatalf("parseComposeLabel(%q) = (%q, %q), want (%q, %q)",
					tt.raw, key, value, tt.wantKey, tt.wantValue)
			}
		})
	}
}

func TestResolveAcceptsLabelFileSyntax(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeTestImage(t, dir)
	writeTestFile(t, dir, "labels.env", "com.example.role=file\ncom.example.file=true\n")
	yamlDoc := `
name: labelfile
services:
  api:
    image: ./base.qcow2
    label_file:
      - ./labels.env
    labels:
      com.example.role: inline
`
	project := resolveTestCompose(t, dir, yamlDoc)
	labels := project.Services["api"].Labels
	if labels["com.example.role"] != "inline" || labels["com.example.file"] != "true" {
		t.Fatalf("labels = %#v", labels)
	}
}

func TestLabelFileValue(t *testing.T) {
	t.Parallel()

	if got := labelFileValue(nil); got != "" {
		t.Fatalf("labelFileValue(nil) = %q, want empty", got)
	}
	if got := labelFileValue(stringPtr("worker")); got != "worker" {
		t.Fatalf("labelFileValue = %q, want worker", got)
	}
}

func TestMergeLabelFileValues(t *testing.T) {
	t.Parallel()

	out := map[string]string{"com.example.role": "inline"}
	mergeLabelFileValues(out, Environment{
		"com.example.role": stringPtr("file"),
		"com.example.flag": nil,
	})

	if out["com.example.role"] != "file" || out["com.example.flag"] != "" {
		t.Fatalf("merged labels = %#v", out)
	}
}

func TestResolveAcceptsExtraHostsSyntax(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeTestImage(t, dir)
	yamlDoc := `
name: extrahosts
services:
  map:
    image: ./base.qcow2
    extra_hosts:
      db.local: 10.0.0.10
  list:
    image: ./base.qcow2
    extra_hosts:
      - cache.local=10.0.0.11
      - api.local:10.0.0.12
`
	project := resolveTestCompose(t, dir, yamlDoc)
	if got := project.Services["map"].ExtraHosts["db.local"]; got != "10.0.0.10" {
		t.Fatalf("map extra host = %q, want 10.0.0.10", got)
	}
	hosts := project.Services["list"].ExtraHosts
	if hosts["cache.local"] != "10.0.0.11" || hosts["api.local"] != "10.0.0.12" {
		t.Fatalf("list extra hosts = %#v", hosts)
	}
	if hosts["map"] == "" {
		t.Fatalf("project service hosts should still be present: %#v", hosts)
	}
}

func TestParseExtraHost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		raw      string
		wantHost string
		wantAddr string
	}{
		{name: "equals separator", raw: "cache.local=10.0.0.11", wantHost: "cache.local", wantAddr: "10.0.0.11"},
		{name: "colon separator", raw: "api.local:10.0.0.12", wantHost: "api.local", wantAddr: "10.0.0.12"},
		{name: "bracketed IPv6", raw: "v6.local=[::1]", wantHost: "v6.local", wantAddr: "::1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			host, addr, err := parseExtraHost(tt.raw)
			if err != nil {
				t.Fatalf("parseExtraHost(%q): %v", tt.raw, err)
			}
			if host != tt.wantHost || addr != tt.wantAddr {
				t.Fatalf("parseExtraHost(%q) = (%q, %q), want (%q, %q)",
					tt.raw, host, addr, tt.wantHost, tt.wantAddr)
			}
		})
	}
}

func TestParseExtraHostRejectsInvalidEntries(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{"missing-separator", "=10.0.0.1", "host="} {
		_, _, err := parseExtraHost(raw)
		assertErrorContains(t, err, "invalid extra_hosts entry")
	}
}

func TestSplitExtraHost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		raw      string
		wantHost string
		wantAddr string
		wantOK   bool
	}{
		{name: "equals separator", raw: "cache.local=10.0.0.11", wantHost: "cache.local", wantAddr: "10.0.0.11", wantOK: true},
		{name: "colon separator", raw: "api.local:10.0.0.12", wantHost: "api.local", wantAddr: "10.0.0.12", wantOK: true},
		{name: "equals wins", raw: "host=addr:still-addr", wantHost: "host", wantAddr: "addr:still-addr", wantOK: true},
		{name: "missing separator", raw: "host", wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			host, addr, ok := splitExtraHost(tt.raw)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if host != tt.wantHost || addr != tt.wantAddr {
				t.Fatalf("splitExtraHost(%q) = (%q, %q), want (%q, %q)",
					tt.raw, host, addr, tt.wantHost, tt.wantAddr)
			}
		})
	}
}

func TestTrimBracketedIPv6(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		addr        string
		want        string
		wantBracket bool
	}{
		{name: "bracketed", addr: "[::1]", want: "::1", wantBracket: true},
		{name: "plain", addr: "::1", want: "::1", wantBracket: false},
		{name: "missing close", addr: "[::1", want: "[::1", wantBracket: false},
		{name: "missing open", addr: "::1]", want: "::1]", wantBracket: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := isBracketedIPv6(tt.addr); got != tt.wantBracket {
				t.Fatalf("isBracketedIPv6(%q) = %v, want %v", tt.addr, got, tt.wantBracket)
			}
			if got := trimBracketedIPv6(tt.addr); got != tt.want {
				t.Fatalf("trimBracketedIPv6(%q) = %q, want %q", tt.addr, got, tt.want)
			}
		})
	}
}

func TestResolveServiceExtraHostsPreservesServicePrecedence(t *testing.T) {
	t.Parallel()

	got := resolveServiceExtraHosts(
		map[string]string{
			"api":   "10.10.0.2",
			"cache": "10.10.0.3",
		},
		ExtraHosts{
			"api":      "192.0.2.10",
			"external": "192.0.2.20",
		},
	)

	if got["api"] != "192.0.2.10" {
		t.Fatalf("api host = %q, want service override", got["api"])
	}
	if got["cache"] != "10.10.0.3" || got["external"] != "192.0.2.20" {
		t.Fatalf("merged extra hosts = %#v", got)
	}
}

func TestCopyExtraHosts(t *testing.T) {
	t.Parallel()

	dst := map[string]string{
		"api": "10.10.0.2",
	}
	copyExtraHosts(dst, map[string]string{
		"api":      "192.0.2.10",
		"external": "192.0.2.20",
	})

	if dst["api"] != "192.0.2.10" || dst["external"] != "192.0.2.20" {
		t.Fatalf("copied extra hosts = %#v", dst)
	}
}

func TestResolveAcceptsHostnameSyntax(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeTestImage(t, dir)
	yamlDoc := `
name: names
services:
  api:
    image: ./base.qcow2
    hostname: api
    domainname: example.internal
`
	project := resolveTestCompose(t, dir, yamlDoc)
	if got := project.Services["api"].CloudInit.Hostname; got != "api.example.internal" {
		t.Fatalf("hostname = %q, want api.example.internal", got)
	}
}

func TestComposeHostname(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		hostname   string
		domainName string
		want       string
	}{
		{name: "empty hostname"},
		{name: "hostname only", hostname: "api", want: "api"},
		{name: "with domain", hostname: "api", domainName: "example.internal", want: "api.example.internal"},
		{name: "already qualified", hostname: "api.example.internal", domainName: "ignored.internal", want: "api.example.internal"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := composeHostname(tt.hostname, tt.domainName); got != tt.want {
				t.Fatalf("composeHostname = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveServiceHostname(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		svc  Service
		want string
	}{
		{name: "empty"},
		{
			name: "compose hostname",
			svc:  Service{Hostname: "api", DomainName: "example.internal"},
			want: "api.example.internal",
		},
		{
			name: "cloud init hostname wins",
			svc: Service{
				Hostname:   "api",
				DomainName: "example.internal",
				CloudInit:  CloudInit{Hostname: "custom.internal"},
			},
			want: "custom.internal",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := resolveServiceHostname(tt.svc); got != tt.want {
				t.Fatalf("resolveServiceHostname = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveAcceptsScaleSyntax(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeTestImage(t, dir)
	yamlDoc := `
name: scale
services:
  worker:
    image: ./base.qcow2
    scale: "3"
`
	project := resolveTestCompose(t, dir, yamlDoc)
	if got := project.Services["worker"].Replicas; got != 3 {
		t.Fatalf("replicas = %d, want 3", got)
	}
}
