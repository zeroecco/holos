package runtime

import (
	"testing"

	"github.com/zeroecco/holos/internal/config"
)

// TestAugmentServicesWithExecKey_DoesNotMutateInput pins the Manager.Up
// "no side effects on caller's Project" contract. Before the fix the
// loop wrote manifests back into the shared map, so a second Up() on
// the same *compose.Project (test harnesses, a long-lived daemon, a
// future watch-mode reload) would see the public key already present
// and append it again. Over N calls authorized_keys would grow from
// [user] to [user, exec, exec, exec, ...], bloating cloud-init and
// churning the spec hash.
func TestAugmentServicesWithExecKey_DoesNotMutateInput(t *testing.T) {
	t.Parallel()

	original := map[string]config.Manifest{
		"web": {
			Name: "web",
			CloudInit: config.CloudInit{
				SSHAuthorizedKeys: []string{"user-key"},
			},
		},
	}

	pubKey := "exec-key"
	out1 := augmentServicesWithExecKey(original, pubKey)

	// Contract 1: the input is untouched.
	if got := original["web"].CloudInit.SSHAuthorizedKeys; len(got) != 1 || got[0] != "user-key" {
		t.Fatalf("input map was mutated: %v", got)
	}

	// Contract 2: the output has both keys exactly once.
	if got := out1["web"].CloudInit.SSHAuthorizedKeys; len(got) != 2 || got[0] != "user-key" || got[1] != "exec-key" {
		t.Fatalf("output missing expected keys: %v", got)
	}

	// Contract 3: calling again with the same input is idempotent.
	// If augmentation leaked back into the input the second call
	// would produce [user-key, exec-key, exec-key].
	out2 := augmentServicesWithExecKey(original, pubKey)
	if got := out2["web"].CloudInit.SSHAuthorizedKeys; len(got) != 2 {
		t.Fatalf("second call saw a mutated input; keys=%v", got)
	}

	// Contract 4: outputs are independent. Appending to one must not
	// show up in the other (catches shared backing arrays created by
	// a missing copy).
	mod := out1["web"]
	mod.CloudInit.SSHAuthorizedKeys = append(mod.CloudInit.SSHAuthorizedKeys, "tampered")
	if got := out2["web"].CloudInit.SSHAuthorizedKeys; len(got) != 2 {
		t.Fatalf("outputs share backing array; got %v", got)
	}
}
