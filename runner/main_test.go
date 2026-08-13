package main

import "testing"

func TestPortParamRejectsUnsafePorts(t *testing.T) {
	for _, value := range []string{"", "0", "65536", "abc", "22; rm -rf /"} {
		if _, err := portParam(map[string]string{"port": value}); err == nil {
			t.Fatalf("port %q must be rejected", value)
		}
	}
	if got, err := portParam(map[string]string{"port": "17117"}); err != nil || got != 17117 {
		t.Fatalf("portParam = %d, %v", got, err)
	}
}

func TestProtocolParamAllowsOnlyDeclaredProtocols(t *testing.T) {
	for _, value := range []string{"tcp", "udp", ""} {
		if _, err := protocolParam(map[string]string{"protocol": value}); err != nil {
			t.Fatalf("protocol %q must be accepted: %v", value, err)
		}
	}
	if _, err := protocolParam(map[string]string{"protocol": "tcp;cmd"}); err == nil {
		t.Fatal("unsafe protocol must be rejected")
	}
}
