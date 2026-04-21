package runtime

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
)

// sshDir returns the per-project directory that owns the `holos exec`
// keypair. One key per project is sufficient because all instances in a
// project share a trust domain: they already sit on the same socket-
// multicast L2 segment and cannot be isolated from each other.
func sshDir(stateDir, project string) string {
	return filepath.Join(stateDir, "ssh", project)
}

// privateKeyPath / publicKeyPath are the on-disk locations of the
// generated keypair. Naming follows OpenSSH conventions so operators can
// inspect or rotate them with standard tools.
func privateKeyPath(stateDir, project string) string {
	return filepath.Join(sshDir(stateDir, project), "id_ed25519")
}

func publicKeyPath(stateDir, project string) string {
	return privateKeyPath(stateDir, project) + ".pub"
}

// ensureProjectSSHKey creates an ed25519 keypair for the project at
// state_dir/ssh/<project>/ if one does not exist. It returns the path
// to the private key (for `holos exec` to consume) and the
// OpenSSH-format public key text (for cloud-init injection).
//
// Generation is idempotent: if either file already exists, the stored
// bytes win. This lets operators rotate a key by deleting the files and
// re-running `holos up`: new instances pick up the new key while
// already-booted VMs keep their existing authorized_keys entry.
func ensureProjectSSHKey(stateDir, project string) (privatePath string, publicKey string, err error) {
	dir := sshDir(stateDir, project)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", "", fmt.Errorf("create ssh dir: %w", err)
	}

	privPath := privateKeyPath(stateDir, project)
	pubPath := publicKeyPath(stateDir, project)

	if _, errPriv := os.Stat(privPath); errPriv == nil {
		pubBytes, err := os.ReadFile(pubPath)
		if err != nil {
			return "", "", fmt.Errorf("read public key: %w", err)
		}
		return privPath, strings.TrimSpace(string(pubBytes)), nil
	}

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("generate ed25519 key: %w", err)
	}

	// Encode the private key in OpenSSH's format so the standard `ssh`
	// binary can consume it directly via -i without any conversion.
	pemBlock, err := ssh.MarshalPrivateKey(priv, "holos-"+project)
	if err != nil {
		return "", "", fmt.Errorf("marshal ssh private key: %w", err)
	}
	if err := os.WriteFile(privPath, pem.EncodeToMemory(pemBlock), 0o600); err != nil {
		return "", "", fmt.Errorf("write private key: %w", err)
	}

	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		return "", "", fmt.Errorf("derive public key: %w", err)
	}
	authorizedLine := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub))) +
		" holos-" + project + "\n"
	if err := os.WriteFile(pubPath, []byte(authorizedLine), 0o644); err != nil {
		return "", "", fmt.Errorf("write public key: %w", err)
	}
	return privPath, strings.TrimSpace(authorizedLine), nil
}
