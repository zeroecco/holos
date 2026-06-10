package runtime

import "testing"

func TestNextTestEphemeralTCPPortSequence(t *testing.T) {
	t.Setenv(testEphemeralPortsEnv, "10001, 10002")
	resetTestEphemeralTCPPorts()

	port, ok, err := nextTestEphemeralTCPPort()
	if err != nil || !ok || port != 10001 {
		t.Fatalf("first override = (%d, %v, %v), want (10001, true, nil)", port, ok, err)
	}

	port, ok, err = nextTestEphemeralTCPPort()
	if err != nil || !ok || port != 10002 {
		t.Fatalf("second override = (%d, %v, %v), want (10002, true, nil)", port, ok, err)
	}

	_, ok, err = nextTestEphemeralTCPPort()
	if !ok {
		t.Fatalf("exhausted override ok = false, want true")
	}
	assertErrorContains(t, err, "exhausted")
}

func TestNextTestEphemeralTCPPortRejectsInvalidEntry(t *testing.T) {
	t.Setenv(testEphemeralPortsEnv, "65536")
	resetTestEphemeralTCPPorts()

	_, ok, err := nextTestEphemeralTCPPort()
	if !ok {
		t.Fatalf("invalid override ok = false, want true")
	}
	assertErrorContains(t, err, "invalid")
}

func TestParseTestEphemeralPorts(t *testing.T) {
	t.Parallel()

	got := parseTestEphemeralPorts("10001, 10002,")
	want := []string{"10001", "10002", ""}
	assertStringSliceEqual(t, "parseTestEphemeralPorts", got, want)
}

func TestParseTestEphemeralPort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		value   string
		want    int
		wantErr bool
	}{
		{value: "1", want: 1},
		{value: "65535", want: 65535},
		{value: "0", wantErr: true},
		{value: "65536", wantErr: true},
		{value: "not-a-port", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			got, err := parseTestEphemeralPort(tt.value)
			if tt.wantErr {
				if err == nil {
					t.Fatal("parseTestEphemeralPort returned nil error, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseTestEphemeralPort returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("parseTestEphemeralPort = %d, want %d", got, tt.want)
			}
		})
	}
}

func resetTestEphemeralTCPPorts() {
	testEphemeralPortsMu.Lock()
	defer testEphemeralPortsMu.Unlock()

	testEphemeralPortsValue = ""
	testEphemeralPortsIndex = 0
}
