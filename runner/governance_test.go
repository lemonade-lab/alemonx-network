package main

import "testing"

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

func TestPlanReturnsPreviewWithoutPersistingPrivilegeState(t *testing.T) {
	plan, err := createPlan(map[string]string{"operation": "open-port", "port": "17117", "protocol": "tcp"})
	if err != nil {
		t.Fatal(err)
	}
	if plan.ID != "" || plan.Fingerprint == "" || plan.Risk != "high" {
		t.Fatalf("unexpected plan: %#v", plan)
	}
}

func TestPlanRejectsReadOnlyAction(t *testing.T) {
	if _, err := createPlan(map[string]string{"operation": "snapshot"}); err == nil {
		t.Fatal("read-only action must not create a change plan")
	}
}

func TestApprovedPlanRequiresHostBoundOperation(t *testing.T) {
	if _, err := applyApprovedPlan(map[string]string{"operation": "snapshot"}); err == nil {
		t.Fatal("read-only action must not be accepted as host-approved mutation")
	}
}
