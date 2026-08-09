package main

import (
	"path/filepath"
	"testing"
)

func TestCapabilitySnapshotExposesGovernanceModules(t *testing.T) {
	snapshot := capabilitySnapshot()
	seen := map[string]bool{}
	for _, capability := range snapshot.Capabilities {
		seen[capability.ID] = true
	}
	for _, required := range []string{"snapshot", "interfaces", "routes", "dns", "traffic", "forwarding", "firewall", "virtual"} {
		if !seen[required] {
			t.Fatalf("missing capability %q", required)
		}
	}
}

func TestPlanPersistsWithStateFingerprint(t *testing.T) {
	original := userConfigDir
	root := filepath.Join(t.TempDir(), "config")
	userConfigDir = func() (string, error) { return root, nil }
	t.Cleanup(func() { userConfigDir = original })
	plan, err := createPlan(map[string]string{"operation": "open-port", "port": "17117", "protocol": "tcp"})
	if err != nil {
		t.Fatal(err)
	}
	if plan.ID == "" || plan.Fingerprint == "" || plan.Risk != "high" {
		t.Fatalf("unexpected plan: %#v", plan)
	}
	plans, err := loadPlans()
	if err != nil || len(plans) != 1 || plans[0].ID != plan.ID {
		t.Fatalf("plans = %#v, %v", plans, err)
	}
}

func TestPlanRejectsReadOnlyAction(t *testing.T) {
	if _, err := createPlan(map[string]string{"operation": "snapshot"}); err == nil {
		t.Fatal("read-only action must not create a change plan")
	}
}

func TestAuditStoresInverseForSupportedChanges(t *testing.T) {
	original := userConfigDir
	root := filepath.Join(t.TempDir(), "config")
	userConfigDir = func() (string, error) { return root, nil }
	t.Cleanup(func() { userConfigDir = original })
	if err := appendAudit("open-port", map[string]string{"port": "17117"}, "done"); err != nil {
		t.Fatal(err)
	}
	entries, err := loadAudit()
	if err != nil || len(entries) != 1 || entries[0].UndoOperation != "close-port" {
		t.Fatalf("entries = %#v, %v", entries, err)
	}
}
