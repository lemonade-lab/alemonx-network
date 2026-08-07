package main

import "testing"

func TestForwardStateRoundTrip(t *testing.T) {
	original := userConfigDir
	dir := t.TempDir()
	userConfigDir = func() (string, error) { return dir, nil }
	defer func() { userConfigDir = original }()

	entries, err := loadForwardState()
	if err != nil {
		t.Fatalf("empty state must load without error: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("empty state = %d entries", len(entries))
	}

	rules := []ForwardRule{
		{ID: "fwd-a", ListenAddress: "0.0.0.0", ListenPort: 8080, Protocol: "tcp", TargetIP: "10.0.0.5", TargetPort: 8080, PID: 1234},
		{ID: "fwd-b", ListenAddress: "127.0.0.1", ListenPort: 9090, Protocol: "tcp", TargetIP: "10.0.0.6", TargetPort: 9091},
	}
	if err := saveForwardState(rules); err != nil {
		t.Fatalf("save failed: %v", err)
	}
	reloaded, err := loadForwardState()
	if err != nil {
		t.Fatalf("reload failed: %v", err)
	}
	if len(reloaded) != 2 {
		t.Fatalf("reloaded = %d entries", len(reloaded))
	}
	if reloaded[0].ID != "fwd-a" || reloaded[0].TargetIP != "10.0.0.5" || reloaded[0].PID != 1234 {
		t.Fatalf("rule a round-trip mismatch: %+v", reloaded[0])
	}
	if reloaded[1].TargetPort != 9091 {
		t.Fatalf("rule b round-trip mismatch: %+v", reloaded[1])
	}

	if err := saveForwardState(nil); err != nil {
		t.Fatalf("overwrite with empty failed: %v", err)
	}
	emptied, err := loadForwardState()
	if err != nil || len(emptied) != 0 {
		t.Fatalf("expected empty after overwrite, got %v / %v", emptied, err)
	}
}
