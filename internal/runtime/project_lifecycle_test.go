package runtime

import (
	"errors"
	"os"
	"testing"
)

const (
	testLegacySpecHash = ""
	testOldSpecHash    = "old"
	testNextSpecHash   = "new"
	testSameSpecHash   = "same"
)

func testProjectRecordWithHash(hash string) *ProjectRecord {
	return &ProjectRecord{Name: testPathProject, SpecHash: hash}
}

func TestShouldResetProjectState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		record       *ProjectRecord
		nextSpecHash string
		want         bool
	}{
		{
			name:         "new project",
			record:       nil,
			nextSpecHash: testNextSpecHash,
			want:         false,
		},
		{
			name:         "legacy record without hash",
			record:       testProjectRecordWithHash(testLegacySpecHash),
			nextSpecHash: testNextSpecHash,
			want:         false,
		},
		{
			name:         "same hash",
			record:       testProjectRecordWithHash(testSameSpecHash),
			nextSpecHash: testSameSpecHash,
			want:         false,
		},
		{
			name:         "changed hash",
			record:       testProjectRecordWithHash(testOldSpecHash),
			nextSpecHash: testNextSpecHash,
			want:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := shouldResetProjectState(tt.record, tt.nextSpecHash); got != tt.want {
				t.Fatalf("shouldResetProjectState(%+v, %q) = %v, want %v", tt.record, tt.nextSpecHash, got, tt.want)
			}
		})
	}
}

func TestProjectLoadErrorBlocksUp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil"},
		{name: "missing project record", err: os.ErrNotExist},
		{name: "other load error", err: errors.New("permission denied"), want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := projectLoadErrorBlocksUp(tt.err); got != tt.want {
				t.Fatalf("projectLoadErrorBlocksUp(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
