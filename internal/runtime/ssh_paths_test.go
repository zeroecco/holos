package runtime

import (
	"crypto/ed25519"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

const sshTestProjectName = "demo"

func TestSSHKeyPaths(t *testing.T) {
	stateDir := filepath.FromSlash("state/holos")
	project := sshTestProjectName

	keyDir := filepath.FromSlash("state/holos/ssh/demo")
	if got := sshDir(stateDir, project); got != keyDir {
		t.Fatalf("sshDir = %q, want %q", got, keyDir)
	}
	privatePath := filepath.FromSlash("state/holos/ssh/demo/id_ed25519")
	if got := privateKeyPath(stateDir, project); got != privatePath {
		t.Fatalf("privateKeyPath = %q, want %q", got, privatePath)
	}
	publicPath := filepath.FromSlash("state/holos/ssh/demo/id_ed25519.pub")
	if got := publicKeyPath(stateDir, project); got != publicPath {
		t.Fatalf("publicKeyPath = %q, want %q", got, publicPath)
	}
}

func TestSSHKeyComment(t *testing.T) {
	t.Parallel()

	if got, want := sshKeyComment(sshTestProjectName), "holos-demo"; got != want {
		t.Fatalf("sshKeyComment = %q, want %q", got, want)
	}
}

func TestAuthorizedKeyLine(t *testing.T) {
	t.Parallel()

	pub, err := ssh.NewPublicKey(ed25519.PublicKey(make([]byte, ed25519.PublicKeySize)))
	if err != nil {
		t.Fatalf("NewPublicKey: %v", err)
	}

	got := authorizedKeyLine(pub, sshTestProjectName)
	wantPrefix := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(pub))) + " "
	if !strings.HasPrefix(got, wantPrefix) {
		t.Fatalf("authorizedKeyLine = %q, want prefix %q", got, wantPrefix)
	}
	wantSuffix := "holos-demo\n"
	if !strings.HasSuffix(got, wantSuffix) {
		t.Fatalf("authorizedKeyLine = %q, want comment suffix %q", got, wantSuffix)
	}
}

func TestEnsureProjectSSHKeyCreatesKeyPair(t *testing.T) {
	stateDir := t.TempDir()
	project := sshTestProjectName

	privatePath, publicKey, err := ensureProjectSSHKey(stateDir, project)
	if err != nil {
		t.Fatalf("ensureProjectSSHKey: %v", err)
	}
	wantPrivatePath := filepath.Join(stateDir, "ssh", "demo", "id_ed25519")
	if privatePath != wantPrivatePath {
		t.Fatalf("private path = %q, want %q", privatePath, wantPrivatePath)
	}
	if !strings.HasSuffix(publicKey, " holos-demo") {
		t.Fatalf("public key = %q, want comment suffix %q", publicKey, " holos-demo")
	}

	assertFileMode(t, sshDir(stateDir, project), sshDirPerm)
	assertFileMode(t, privateKeyPath(stateDir, project), sshPrivateKeyPerm)
	assertFileMode(t, publicKeyPath(stateDir, project), sshPublicKeyPerm)

	privatePathAgain, publicKeyAgain, err := ensureProjectSSHKey(stateDir, project)
	if err != nil {
		t.Fatalf("second ensureProjectSSHKey: %v", err)
	}
	if privatePathAgain != privatePath {
		t.Fatalf("second private path = %q, want %q", privatePathAgain, privatePath)
	}
	if publicKeyAgain != publicKey {
		t.Fatalf("second public key = %q, want %q", publicKeyAgain, publicKey)
	}
}

func assertFileMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s mode = %o, want %o", path, got, want)
	}
}
