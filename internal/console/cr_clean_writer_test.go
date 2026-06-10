package console

import (
	"bytes"
	"testing"
)

func TestCRCleanWriter(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "preserves CRLF",
			input: "one\r\ntwo\n",
			want:  "one\r\ntwo\n",
		},
		{
			name:  "erases after bare carriage return",
			input: "progress 100%\rdone\n",
			want:  "progress 100%\r\x1b[Kdone\n",
		},
		{
			name:  "erases after each bare carriage return",
			input: "10%\r20%\r30%\n",
			want:  "10%\r\x1b[K20%\r\x1b[K30%\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			writer := crCleanWriter{w: &out}

			n, err := writer.Write([]byte(tt.input))
			if err != nil {
				t.Fatalf("Write() error = %v", err)
			}
			if n != len(tt.input) {
				t.Fatalf("Write() n = %d, want %d", n, len(tt.input))
			}
			if got := out.String(); got != tt.want {
				t.Fatalf("output = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCRCleanWriterDefersChunkBoundaryCR(t *testing.T) {
	var out bytes.Buffer
	writer := crCleanWriter{w: &out}

	if n, err := writer.Write([]byte("progress\r")); err != nil || n != len("progress\r") {
		t.Fatalf("first Write() n = %d, err = %v", n, err)
	}
	if got := out.String(); got != "progress" {
		t.Fatalf("output after pending CR = %q, want %q", got, "progress")
	}

	if n, err := writer.Write([]byte("done\n")); err != nil || n != len("done\n") {
		t.Fatalf("second Write() n = %d, err = %v", n, err)
	}
	if got, want := out.String(), "progress\r\x1b[Kdone\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestCRCleanWriterPreservesChunkBoundaryCRLF(t *testing.T) {
	var out bytes.Buffer
	writer := crCleanWriter{w: &out}

	if _, err := writer.Write([]byte("one\r")); err != nil {
		t.Fatalf("first Write() error = %v", err)
	}
	if _, err := writer.Write([]byte("\ntwo\n")); err != nil {
		t.Fatalf("second Write() error = %v", err)
	}
	if got, want := out.String(), "one\r\ntwo\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}
