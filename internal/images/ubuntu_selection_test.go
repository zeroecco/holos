package images

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestUbuntuSelectionValidation(t *testing.T) {
	t.Parallel()

	entry := Image{Name: "ubuntu", Tag: "noble", UbuntuRelease: "noble", URL: "https://example.test/", Format: "qcow2"}
	valid := ubuntuSelection{
		Version: 1, SourceURL: entry.URL, Release: entry.UbuntuRelease,
		URL:    entry.URL + "server/releases/noble/release-20260911/ubuntu-24.04-server-cloudimg-amd64.img",
		SHA256: strings.Repeat("a", sha256HexLength),
	}
	cases := []struct {
		name   string
		mutate func(*ubuntuSelection)
	}{
		{"unknown version", func(s *ubuntuSelection) { s.Version++ }},
		{"wrong source", func(s *ubuntuSelection) { s.SourceURL = "https://other.test/" }},
		{"wrong release", func(s *ubuntuSelection) { s.Release = "jammy" }},
		{"missing hash", func(s *ubuntuSelection) { s.SHA256 = "" }},
		{"invalid hash", func(s *ubuntuSelection) { s.SHA256 = strings.Repeat("g", sha256HexLength) }},
		{"external artifact", func(s *ubuntuSelection) { s.URL = strings.ReplaceAll(s.URL, "example.test", "other.test") }},
		{"mutable artifact", func(s *ubuntuSelection) { s.URL = strings.ReplaceAll(s.URL, "release-20260911", "current") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			selection := valid
			tc.mutate(&selection)
			data, err := json.Marshal(selection)
			if err != nil {
				t.Fatal(err)
			}
			cacheDir := t.TempDir()
			if err := os.WriteFile(ubuntuSelectionPath(cacheDir, &entry), data, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := loadUbuntuSelection(cacheDir, &entry); err == nil {
				t.Fatal("accepted invalid persisted selection")
			}
			// A bad record must not silently select a new build from the network.
			if _, _, err := ubuntuImageForPull(&entry, cacheDir); err == nil {
				t.Fatal("pull accepted invalid persisted selection")
			}
		})
	}
}

func TestSaveUbuntuSelectionAtomic(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	entry := Image{Name: "ubuntu", Tag: "noble", UbuntuRelease: "noble", URL: "https://example.test/", Format: "qcow2"}
	resolved := entry
	resolved.URL += "server/releases/noble/release-20260911/ubuntu-24.04-server-cloudimg-amd64.img"
	resolved.SHA256 = strings.Repeat("a", sha256HexLength)
	if err := saveUbuntuSelection(cacheDir, &entry, &resolved); err != nil {
		t.Fatal(err)
	}
	got, err := loadUbuntuSelection(cacheDir, &entry)
	if err != nil || *got != resolved {
		t.Fatalf("loaded selection = %+v, %v; want %+v", got, err, resolved)
	}
	path := ubuntuSelectionPath(cacheDir, &entry)
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("selection permissions: %v, %v", info, err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := saveUbuntuSelection(cacheDir, &entry, &resolved); err == nil {
		t.Fatal("expected selection promotion to fail over directory")
	}
	partials, err := filepath.Glob(filepath.Join(cacheDir, ".ubuntu-selection-*"))
	if err != nil || len(partials) != 0 {
		t.Fatalf("temporary selection files left behind: %v, %v", partials, err)
	}
}

func TestCommitUbuntuSelectionRechecksAfterLock(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	entry := Image{Name: "ubuntu", Tag: "noble", UbuntuRelease: "noble", URL: "https://example.test/", Format: "qcow2"}
	older := ubuntuCacheBuild("20260911", "older Ubuntu image")
	newer := ubuntuCacheBuild("20260912", "newer Ubuntu image")
	candidate, winner := entry, entry
	candidate.URL, candidate.SHA256 = entry.URL+older.path, older.hash
	winner.URL, winner.SHA256 = entry.URL+newer.path, newer.hash
	for _, build := range []testUbuntuCacheBuild{older, newer} {
		if err := os.WriteFile(ubuntuResolvedCachePath(cacheDir, entry, build), []byte(build.payload), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	file, err := os.OpenFile(ubuntuSelectionPath(cacheDir, &entry)+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	done := make(chan struct{})
	var selected *Image
	var commitErr error
	go func() {
		close(started)
		selected, commitErr = commitUbuntuSelection(cacheDir, &entry, &candidate)
		close(done)
	}()
	<-started
	select {
	case <-done:
		t.Fatalf("selection commit bypassed held lock: %+v, %v", selected, commitErr)
	case <-time.After(100 * time.Millisecond):
	}
	// Simulate another process committing a winner before releasing its lock.
	if err := saveUbuntuSelection(cacheDir, &entry, &winner); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_UN); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
		if commitErr != nil || selected == nil || *selected != winner {
			t.Fatalf("selection after waiting = %+v, %v; want %+v", selected, commitErr, winner)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("selection commit did not finish after lock release")
	}
	stored, err := loadUbuntuSelection(cacheDir, &entry)
	if err != nil || *stored != winner {
		t.Fatalf("stored selection = %+v, %v; want %+v", stored, err, winner)
	}
}
