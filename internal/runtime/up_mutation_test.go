package runtime

import (
	"testing"

	"github.com/zeroecco/holos/internal/config"
)

// TestCarryOverUnreachedServices pins the mid-run-failure contract:
// every VM that got started this call must end up in the saved
// record, and pre-existing entries for services the loop never
// reached must survive so `holos ps` and `holos down` can still
// manage them. Without this, a failing healthcheck on service B
// in a three-service project would quietly erase A's record from
// the previous Up while A's qemu process was still running.
//
// DesiredReplicas is used as a "which copy won" marker: the fresh
// `started` record for service a carries a different count from its
// prior entry, and the helper must prefer the fresh one.
func TestCarryOverUnreachedServices(t *testing.T) {
	t.Parallel()

	started := []ServiceRecord{{Name: "a", DesiredReplicas: 2}}
	prior := []ServiceRecord{
		{Name: "a", DesiredReplicas: 1},
		{Name: "b", DesiredReplicas: 3},
		{Name: "c", DesiredReplicas: 4},
		{Name: "stale", DesiredReplicas: 9},
	}
	desired := map[string]config.Manifest{
		"a": {Name: "a"},
		"b": {Name: "b"},
		"c": {Name: "c"},
	}

	got := carryOverUnreachedServices(started, prior, desired)

	gotNames := make([]string, len(got))
	for i, s := range got {
		gotNames[i] = s.Name
	}
	want := []string{"a", "b", "c"}
	if len(gotNames) != len(want) {
		t.Fatalf("want %v, got %v", want, gotNames)
	}
	for i, n := range want {
		if gotNames[i] != n {
			t.Fatalf("want[%d]=%q, got[%d]=%q (full: %v)", i, n, i, gotNames[i], gotNames)
		}
	}

	// The fresh `started` entry for `a` must win. If the helper
	// preferred `prior` it would silently discard the new state.
	if got[0].DesiredReplicas != 2 {
		t.Fatalf("carry-over preferred prior record for `a`; want DesiredReplicas=2, got %d", got[0].DesiredReplicas)
	}

	// `stale` was in prior but not in desired, so it must be dropped
	// (the operator removed it from the compose file; the next
	// successful Up will garbage collect it).
	for _, n := range gotNames {
		if n == "stale" {
			t.Fatalf("stale service leaked through: %v", gotNames)
		}
	}
}

// TestCarryOverUnreachedServices_NoError returns the started slice
// untouched when the loop never aborted. The happy path must not pay
// for the carry-over logic.
func TestCarryOverUnreachedServices_NoError(t *testing.T) {
	t.Parallel()

	started := []ServiceRecord{{Name: "a"}, {Name: "b"}}
	out := carryOverUnreachedServices(started, nil, map[string]config.Manifest{
		"a": {}, "b": {},
	})
	if len(out) != 2 || out[0].Name != "a" || out[1].Name != "b" {
		t.Fatalf("unexpected output: %v", out)
	}
}

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
