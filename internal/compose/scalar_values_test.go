package compose

import "testing"

func TestComposeInt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   any
		want    int
		wantErr string
	}{
		{name: "nil", value: nil},
		{name: "int", value: 2, want: 2},
		{name: "int64", value: int64(3), want: 3},
		{name: "whole float", value: 4.0, want: 4},
		{name: "string", value: "5", want: 5},
		{name: "blank string", value: "  "},
		{name: "fractional float", value: 1.5, wantErr: "replicas must be an integer"},
		{name: "bad string", value: "many", wantErr: "replicas:"},
		{name: "unsupported", value: true, wantErr: "replicas has unsupported type bool"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := composeInt(tt.value, "replicas")
			if tt.wantErr != "" {
				assertErrorContains(t, err, tt.wantErr)
				return
			}
			if err != nil {
				t.Fatalf("composeInt: %v", err)
			}
			if got != tt.want {
				t.Fatalf("composeInt = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestComposeIntString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   string
		want    int
		wantErr string
	}{
		{name: "blank", value: "  "},
		{name: "valid", value: "5", want: 5},
		{name: "invalid", value: "many", wantErr: "replicas:"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := composeIntString(tt.value, "replicas")
			if tt.wantErr != "" {
				assertErrorContains(t, err, tt.wantErr)
				return
			}
			if err != nil {
				t.Fatalf("composeIntString: %v", err)
			}
			if got != tt.want {
				t.Fatalf("composeIntString = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestComposeFloat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   any
		want    float64
		wantErr string
	}{
		{name: "nil", value: nil},
		{name: "int", value: 2, want: 2},
		{name: "int64", value: int64(3), want: 3},
		{name: "float", value: 2.5, want: 2.5},
		{name: "string", value: "1.5", want: 1.5},
		{name: "blank string", value: "\t"},
		{name: "bad string", value: "fast", wantErr: "cpus:"},
		{name: "unsupported", value: false, wantErr: "cpus has unsupported type bool"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := composeFloat(tt.value, "cpus")
			if tt.wantErr != "" {
				assertErrorContains(t, err, tt.wantErr)
				return
			}
			if err != nil {
				t.Fatalf("composeFloat: %v", err)
			}
			if got != tt.want {
				t.Fatalf("composeFloat = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestComposeFloatString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   string
		want    float64
		wantErr string
	}{
		{name: "blank", value: "  "},
		{name: "valid", value: "2.5", want: 2.5},
		{name: "invalid", value: "fast", wantErr: "cpus:"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := composeFloatString(tt.value, "cpus")
			if tt.wantErr != "" {
				assertErrorContains(t, err, tt.wantErr)
				return
			}
			if err != nil {
				t.Fatalf("composeFloatString: %v", err)
			}
			if got != tt.want {
				t.Fatalf("composeFloatString = %v, want %v", got, tt.want)
			}
		})
	}
}
