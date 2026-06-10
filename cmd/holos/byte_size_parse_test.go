package main

import "testing"

func TestParseByteSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want int64
	}{
		{name: "bytes", raw: "1048576", want: 1 << 20},
		{name: "megabytes", raw: "30M", want: 30 << 20},
		{name: "gigabytes with b", raw: "2GB", want: 2 << 30},
		{name: "decimal", raw: "1.5G", want: 1536 << 20},
		{name: "spaces and lowercase", raw: " 4g ", want: 4 << 30},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseByteSize(tt.raw)
			if err != nil {
				t.Fatalf("parseByteSize(%q): %v", tt.raw, err)
			}
			if got != tt.want {
				t.Fatalf("parseByteSize(%q) = %d, want %d", tt.raw, got, tt.want)
			}
		})
	}
}

func TestParseByteSizeRejectsInvalidValues(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{"", "B", "tiny", "512K"} {
		if got, err := parseByteSize(raw); err == nil {
			t.Fatalf("parseByteSize(%q) = %d, nil; want error", raw, got)
		}
	}
}
