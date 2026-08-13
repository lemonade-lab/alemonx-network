package main

import "testing"

func TestParseWindowsFirewallState(t *testing.T) {
	on := "Domain Profile Settings:\nState                                 ON\nPrivate Profile Settings:\nState                                 OFF\nPublic Profile Settings:\nState                                 ON\n"
	if got := parseWindowsFirewallState(on); got != "on" {
		t.Fatalf("parseWindowsFirewallState = %q, want on", got)
	}
	off := "Domain Profile Settings:\nState                                 OFF\nPrivate Profile Settings:\nState                                 OFF\n"
	if got := parseWindowsFirewallState(off); got != "off" {
		t.Fatalf("parseWindowsFirewallState = %q, want off", got)
	}
	unknown := "Firewall Policy                       BlockInbound,AllowOutbound\n"
	if got := parseWindowsFirewallState(unknown); got != "unknown" {
		t.Fatalf("parseWindowsFirewallState = %q, want unknown", got)
	}
}

func TestSetFirewallRejectsInvalidState(t *testing.T) {
	for _, state := range []string{"", "banana", "ON", "enabled"} {
		if _, err := setFirewall(map[string]string{"state": state}); err == nil {
			t.Fatalf("setFirewall(%q) must be rejected", state)
		}
	}
}

func TestFirewallStatusIsStructured(t *testing.T) {
	status := firewallStatus()
	if status.Detail == "" {
		t.Fatal("firewallStatus detail must not be empty")
	}
	if !status.Available && status.Enabled != nil {
		t.Fatal("unavailable firewall must not report an enabled state")
	}
}
