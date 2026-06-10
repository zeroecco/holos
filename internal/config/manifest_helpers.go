package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// SpecHash returns the full hex-encoded SHA-256 of the JSON-marshaled manifest.
func (m Manifest) SpecHash() (string, error) {
	payload, err := json.Marshal(m)
	if err != nil {
		return "", fmt.Errorf("marshal manifest for hash: %w", err)
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

// InstanceName returns the name for the replica at the given index
// (e.g. "web-0", "web-1").
func (m Manifest) InstanceName(index int) string {
	return fmt.Sprintf("%s-%d", m.Name, index)
}
