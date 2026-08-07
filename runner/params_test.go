package main

import "testing"

func TestListenPortParam(t *testing.T) {
	for _, value := range []string{"", "0", "65536", "abc", "1;rm"} {
		if _, err := listenPortParam(map[string]string{"listenPort": value}); err == nil {
			t.Fatalf("listenPort %q must be rejected", value)
		}
	}
	if got, err := listenPortParam(map[string]string{"listenPort": "8080"}); err != nil || got != 8080 {
		t.Fatalf("listenPortParam = %d, %v", got, err)
	}
}

func TestTargetPortParamFallsBackToListenPort(t *testing.T) {
	if got, err := targetPortParam(map[string]string{"listenPort": "8080"}, 8080); err != nil || got != 8080 {
		t.Fatalf("fallback targetPort = %d, %v", got, err)
	}
	if got, err := targetPortParam(map[string]string{"listenPort": "8080", "targetPort": "9090"}, 8080); err != nil || got != 9090 {
		t.Fatalf("explicit targetPort = %d, %v", got, err)
	}
	if _, err := targetPortParam(map[string]string{"targetPort": "70000"}, 8080); err == nil {
		t.Fatal("out-of-range targetPort must be rejected")
	}
}

func TestIPParamRejectsInvalid(t *testing.T) {
	for _, value := range []string{"", "abc", "999.1.1.1", "1.2.3.4.5", "::1"} {
		if _, err := ipParam(value, "目标 IP"); err == nil {
			t.Fatalf("IP %q must be rejected (IPv6/loopback-class inputs excluded)", value)
		}
	}
	if got, err := ipParam("192.168.1.100", "目标 IP"); err != nil || got != "192.168.1.100" {
		t.Fatalf("ipParam = %q, %v", got, err)
	}
}

func TestCIDRParamNormalizes(t *testing.T) {
	if _, _, err := cidrParam("192.168.1.50/24", "目标网段"); err != nil {
		t.Fatalf("valid CIDR rejected: %v", err)
	}
	if _, _, err := cidrParam("192.168.1.50", "目标网段"); err == nil {
		t.Fatal("bare IP must be rejected")
	}
	if _, _, err := cidrParam("10.0.0.0/33", "目标网段"); err == nil {
		t.Fatal("prefix /33 must be rejected")
	}
	if _, _, err := cidrParam("2001:db8::/64", "目标网段"); err == nil {
		t.Fatal("IPv6 CIDR must be rejected for IPv4 backends")
	}
}

func TestMaskString(t *testing.T) {
	_, prefix, err := cidrParam("192.168.1.50/24", "目标网段")
	if err != nil {
		t.Fatal(err)
	}
	if got := maskString(prefix); got != "255.255.255.0" {
		t.Fatalf("maskString(/24) = %q", got)
	}
}

func TestMTUParam(t *testing.T) {
	for _, value := range []string{"", "0", "1279", "65536", "abc"} {
		if _, err := mtuParam(value); err == nil {
			t.Fatalf("MTU %q must be rejected", value)
		}
	}
	if got, err := mtuParam("1500"); err != nil || got != 1500 {
		t.Fatalf("mtuParam = %d, %v", got, err)
	}
}

func TestInterfaceParam(t *testing.T) {
	for _, value := range []string{"", "  ", "en\n0"} {
		if _, err := ifaceParam(value); err == nil {
			t.Fatalf("interface %q must be rejected", value)
		}
	}
	if got, err := ifaceParam("Wi-Fi"); err != nil || got != "Wi-Fi" {
		t.Fatalf("interface with space must be accepted: %q, %v", got, err)
	}
}

func TestLinuxIfaceNameParam(t *testing.T) {
	for _, value := range []string{"", "-eth0", "en 0", "en/0", "0123456789abcdef", "a;rm"} {
		if _, err := linuxIfaceNameParam(value, "接口"); err == nil {
			t.Fatalf("iface name %q must be rejected", value)
		}
	}
	for _, value := range []string{"en0", "eth0", "eth0.10", "wlan0", "bond0"} {
		if got, err := linuxIfaceNameParam(value, "接口"); err != nil || got != value {
			t.Fatalf("iface name %q must be accepted: %q, %v", value, got, err)
		}
	}
}

func TestVlanIDParam(t *testing.T) {
	for _, value := range []string{"0", "4095", "65536", "abc"} {
		if _, err := vlanIDParam(value); err == nil {
			t.Fatalf("vlan id %q must be rejected", value)
		}
	}
	if got, err := vlanIDParam("10"); err != nil || got != 10 {
		t.Fatalf("vlan id = %d, %v", got, err)
	}
	if got, err := vlanIDParam("4094"); err != nil || got != 4094 {
		t.Fatalf("vlan id 4094 = %d, %v", got, err)
	}
}

func TestBondModeParam(t *testing.T) {
	for _, value := range []string{"balance-rr", "active-backup", "balance-xor", "broadcast", "802.3ad", "balance-tlb", "balance-alb", ""} {
		if _, err := bondModeParam(value); err != nil {
			t.Fatalf("bond mode %q must be accepted: %v", value, err)
		}
	}
	if _, err := bondModeParam("bogus"); err == nil {
		t.Fatal("invalid bond mode must be rejected")
	}
}

func TestFirewalldZoneAndServiceParam(t *testing.T) {
	for _, value := range []string{"public", "home", "dmz"} {
		if _, err := firewalldZoneParam(value); err != nil {
			t.Fatalf("zone %q rejected: %v", value, err)
		}
	}
	if _, err := firewalldZoneParam("bad zone;x"); err == nil {
		t.Fatal("invalid zone must be rejected")
	}
	for _, value := range []string{"http", "https", "ssh"} {
		if _, err := firewalldServiceParam(value); err != nil {
			t.Fatalf("service %q rejected: %v", value, err)
		}
	}
	if _, err := firewalldServiceParam("http/1"); err == nil {
		t.Fatal("invalid service must be rejected")
	}
}
