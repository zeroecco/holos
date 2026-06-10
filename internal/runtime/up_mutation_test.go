package runtime

import (
	"testing"

	"github.com/zeroecco/holos/internal/config"
)

func testManifest(name string) config.Manifest {
	return config.Manifest{Name: name}
}

func serviceRecordNames(records []ServiceRecord) []string {
	names := make([]string, len(records))
	for i, record := range records {
		names[i] = record.Name
	}
	return names
}

func assertServiceAuthorizedKeys(t *testing.T, name string, services map[string]config.Manifest, service string, want []string) {
	t.Helper()

	assertStringSliceEqual(t, name, services[service].CloudInit.SSHAuthorizedKeys, want)
}

// TestCarryOverUnreachedServices pins the mid-run-failure contract:
// every VM that got started this call must end up in the saved
// record, and pre-existing entries for services the loop never
// reached must survive so `holos ps` and `holos down` can still
// manage them. Without this, a failing healthcheck on service B
// in a three-service project would quietly erase A's record from
// the previous Up while A's qemu process was still running.
//
// Prior records for services no longer in the compose file must
// ALSO survive a failed Up. The happy-path teardown sweep is gated
// behind `upErr == nil`, so if we dropped those records on failure
// their QEMU processes would keep running with nothing to track
// them; `holos ps` would show empty and `holos down` would have no
// target. We only reconcile the disappeared services on the next
// successful Up.
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
		{Name: "removed-but-running", DesiredReplicas: 9,
			Instances: []InstanceRecord{{Name: "removed-but-running-0", PID: 12345}}},
	}
	got := carryOverUnreachedServices(started, prior)

	want := []string{"a", "b", "c", "removed-but-running"}
	assertStringSliceEqual(t, "carryOverUnreachedServices names", serviceRecordNames(got), want)

	// The fresh `started` entry for `a` must win. If the helper
	// preferred `prior` it would silently discard the new state.
	if got[0].DesiredReplicas != 2 {
		t.Fatalf("carry-over preferred prior record for `a`; want DesiredReplicas=2, got %d", got[0].DesiredReplicas)
	}

	// The instance from the "removed-but-running" service must ride
	// through intact so `holos down` knows what PID to stop.
	last := got[len(got)-1]
	if len(last.Instances) != 1 {
		t.Fatalf("removed-but-running service lost its instance record: %+v", last)
	}
	if last.Instances[0].PID != 12345 {
		t.Fatalf("removed-but-running PID = %d, want 12345", last.Instances[0].PID)
	}
}

// TestCarryOverUnreachedServices_NoError returns the started slice
// untouched when the loop never aborted. The happy path must not pay
// for the carry-over logic.
func TestCarryOverUnreachedServices_NoError(t *testing.T) {
	t.Parallel()

	started := []ServiceRecord{{Name: "a"}, {Name: "b"}}
	out := carryOverUnreachedServices(started, nil)
	assertStringSliceEqual(t, "carryOverUnreachedServices names", serviceRecordNames(out), []string{"a", "b"})
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
	assertServiceAuthorizedKeys(t, "input authorized keys", original, "web", []string{"user-key"})

	// Contract 2: the output has both keys exactly once.
	assertServiceAuthorizedKeys(t, "output authorized keys", out1, "web", []string{"user-key", "exec-key"})

	// Contract 3: calling again with the same input is idempotent.
	// If augmentation leaked back into the input the second call
	// would produce [user-key, exec-key, exec-key].
	out2 := augmentServicesWithExecKey(original, pubKey)
	assertServiceAuthorizedKeys(t, "second output authorized keys", out2, "web", []string{"user-key", "exec-key"})

	// Contract 4: outputs are independent. Appending to one must not
	// show up in the other (catches shared backing arrays created by
	// a missing copy).
	mod := out1["web"]
	mod.CloudInit.SSHAuthorizedKeys = append(mod.CloudInit.SSHAuthorizedKeys, "tampered")
	assertServiceAuthorizedKeys(t, "independent output authorized keys", out2, "web", []string{"user-key", "exec-key"})
}
