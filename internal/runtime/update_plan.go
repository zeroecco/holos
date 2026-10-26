package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/zeroecco/holos/internal/compose"
	"github.com/zeroecco/holos/internal/config"
)

// UpOptions controls preview and explicit consent to discard instance disks.
type UpOptions struct {
	// PreserveOtherServices scopes start to selected services without removing peers.
	PreserveOtherServices bool
	DryRun                bool
	Recreate              bool
	Report                func([]ServiceChange)
}

// ChangeAction is the planned effect on a service's instances.
type ChangeAction string

const (
	ChangeCreate  ChangeAction = "create"
	ChangeKeep    ChangeAction = "keep"
	ChangeReplace ChangeAction = "replace"
	ChangeRemove  ChangeAction = "remove"
)

// IsDestructive reports whether the action discards existing root disks.
func (action ChangeAction) IsDestructive() bool {
	return action == ChangeReplace || action == ChangeRemove
}

// ServiceChange describes the effect of up on one service.
type ServiceChange struct {
	Service string
	Action  ChangeAction
	Reason  string
}

func serviceFingerprint(manifest config.Manifest) string {
	// Runtime-only metadata does not require replacing a guest.
	manifest.Labels = nil
	manifest.Healthcheck = nil
	manifest.DependsOn = nil
	manifest.PreStopCommands = nil
	payload, _ := json.Marshal(manifest) // Manifest contains only JSON-safe types.
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

func projectChanges(project *compose.Project, record *ProjectRecord) []ServiceChange {
	existing := existingServicesByName(record.Services)
	var changes []ServiceChange
	for _, name := range project.ServiceOrder {
		prior := existing[name]
		change := ServiceChange{Service: name, Action: ChangeKeep, Reason: "configuration unchanged; retain disks and start missing/stopped instances"}
		switch {
		case prior == nil || len(prior.Instances) == 0:
			change.Action, change.Reason = ChangeCreate, "no existing instances"
		case prior.SpecHash == "":
			change.Action, change.Reason = ChangeReplace, "legacy state has no service fingerprint"
		case prior.SpecHash != serviceFingerprint(project.Services[name]):
			change.Action, change.Reason = ChangeReplace, "resolved VM configuration changed"
		}
		changes = append(changes, change)
	}
	var removed []string
	for name := range existing {
		if _, ok := project.Services[name]; !ok {
			removed = append(removed, name)
		}
	}
	sort.Strings(removed)
	for _, name := range removed {
		changes = append(changes, ServiceChange{Service: name, Action: ChangeRemove, Reason: "service removed from compose"})
	}
	return changes
}

// prepareUpdates runs under the project lock. Checkpoint each removal before
// launching replacements, so failures never restore a stale fingerprint.
func (m *Manager) prepareUpdates(record *ProjectRecord, changes []ServiceChange, allowRecreate bool) error {
	removeServices := make(map[string]bool)
	var destructive []string
	for _, change := range changes {
		if !change.Action.IsDestructive() {
			continue
		}
		removeServices[change.Service] = true
		destructive = append(destructive, fmt.Sprintf("%s (%s)", change.Service, change.Action))
	}
	if len(destructive) > 0 && !allowRecreate {
		return fmt.Errorf("up would discard root disks for %s; inspect with up --dry-run, then use up --recreate to apply (named volumes are preserved)", strings.Join(destructive, ", "))
	}

	// Stop consumers before dependencies using the previous startup order.
	for i := len(record.Services) - 1; i >= 0; i-- {
		service := &record.Services[i]
		if !removeServices[service.Name] {
			continue
		}
		if err := m.stopServiceInstances(record.Name, service); err != nil {
			if saveErr := m.saveUpdatedProject(record); saveErr != nil {
				return fmt.Errorf("%w (persist state: %v)", err, saveErr)
			}
			return err
		}
		// Preserve stopped records until deletion succeeds; errors remain recoverable.
		if err := m.saveUpdatedProject(record); err != nil {
			return err
		}
		for _, instance := range service.Instances {
			if instance.WorkDir == "" {
				continue
			}
			if err := os.RemoveAll(instance.WorkDir); err != nil {
				return err
			}
		}
		record.Services = append(record.Services[:i], record.Services[i+1:]...)
		if err := m.saveUpdatedProject(record); err != nil {
			return err
		}
	}
	return nil
}
