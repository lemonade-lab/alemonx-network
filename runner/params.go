package main

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
)

// All user-supplied values are re-validated here before any system command
// runs. The frontend is never trusted.

func param(params map[string]string, key string) string {
	return strings.TrimSpace(params[key])
}

func portParam(params map[string]string) (int, error) {
	value := param(params, "port")
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("端口必须是 1 到 65535 的整数")
	}
	return port, nil
}

func listenPortParam(params map[string]string) (int, error) {
	value := param(params, "listenPort")
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("监听端口必须是 1 到 65535 的整数")
	}
	return port, nil
}

func targetPortParam(params map[string]string, fallback int) (int, error) {
	value := param(params, "targetPort")
	if value == "" {
		return fallback, nil
	}
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("目标端口必须是 1 到 65535 的整数")
	}
	return port, nil
}

func protocolParam(params map[string]string) (string, error) {
	value := strings.ToLower(param(params, "protocol"))
	if value == "" {
		value = "tcp"
	}
	if value != "tcp" && value != "udp" {
		return "", fmt.Errorf("协议仅支持 tcp 或 udp")
	}
	return value, nil
}

// ipParam validates an IPv4 address (the forwarding backends are v4-only).
func ipParam(value string, label string) (string, error) {
	parsed := net.ParseIP(value)
	if parsed == nil || parsed.To4() == nil {
		return "", fmt.Errorf("%s必须是有效的 IPv4 地址", label)
	}
	return parsed.To4().String(), nil
}

// cidrParam validates an IPv4 CIDR and returns the normalized form and the
// parsed prefix. Non-canonical input such as "192.168.1.5/24" is normalized
// to "192.168.1.5/24" after validation.
func cidrParam(value string, label string) (string, net.IPNet, error) {
	ip, prefix, err := net.ParseCIDR(value)
	if err != nil || ip.To4() == nil {
		return "", net.IPNet{}, fmt.Errorf("%s必须是有效的 IPv4 CIDR，例如 192.168.1.50/24", label)
	}
	ones, _ := prefix.Mask.Size()
	return ip.To4().String() + "/" + strconv.Itoa(ones), *prefix, nil
}

func mtuParam(value string) (int, error) {
	mtu, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || mtu < 1280 || mtu > 65535 {
		return 0, fmt.Errorf("MTU 必须是 1280 到 65535 的整数")
	}
	return mtu, nil
}

func ifaceParam(value string) (string, error) {
	name := strings.TrimSpace(value)
	if name == "" {
		return "", errors.New("请输入网络接口名称")
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return "", errors.New("网络接口名称包含无效字符")
		}
	}
	return name, nil
}

// maskString converts an IPNet mask to dotted-decimal form for netsh.
func maskString(prefix net.IPNet) string {
	if mask := prefix.Mask; mask != nil {
		if v4 := net.IP(mask).To4(); v4 != nil {
			return v4.String()
		}
	}
	return "255.255.255.0"
}

var linuxIfaceName = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,15}$`)
var firewalldName = regexp.MustCompile(`^[A-Za-z0-9_-]{1,31}$`)

// linuxIfaceNameParam validates a name that will be passed to `ip link`. A
// leading '-' is rejected so a crafted value can never be parsed as an option.
func linuxIfaceNameParam(value, label string) (string, error) {
	name := strings.TrimSpace(value)
	if name == "" {
		return "", fmt.Errorf("请输入%s", label)
	}
	if strings.HasPrefix(name, "-") || !linuxIfaceName.MatchString(name) {
		return "", fmt.Errorf("%s无效：Linux 接口名仅允许字母、数字和 _ . : -，且不超过 15 字符，不能以 - 开头", label)
	}
	return name, nil
}

func vlanIDParam(value string) (int, error) {
	id, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || id < 1 || id > 4094 {
		return 0, fmt.Errorf("VLAN ID 必须是 1 到 4094 的整数")
	}
	return id, nil
}

var bondModes = map[string]bool{
	"balance-rr":   true,
	"active-backup": true,
	"balance-xor":  true,
	"broadcast":    true,
	"802.3ad":      true,
	"balance-tlb":  true,
	"balance-alb":  true,
}

func bondModeParam(value string) (string, error) {
	mode := strings.TrimSpace(value)
	if mode == "" {
		mode = "802.3ad"
	}
	if !bondModes[mode] {
		return "", fmt.Errorf("不支持的 Bond 模式：%s（可选 802.3ad、active-backup、balance-rr 等）", mode)
	}
	return mode, nil
}

// memberListParam parses a comma-separated list and verifies each name is a
// currently existing network interface.
func memberListParam(value, label string) ([]string, error) {
	existing := map[string]bool{}
	if interfaces, err := net.Interfaces(); err == nil {
		for _, item := range interfaces {
			existing[item.Name] = true
		}
	}
	members := []string{}
	for _, raw := range strings.Split(value, ",") {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		if !existing[name] {
			return nil, fmt.Errorf("%s中的接口 %q 不存在", label, name)
		}
		members = append(members, name)
	}
	if len(members) == 0 {
		return nil, fmt.Errorf("请输入%s（逗号分隔）", label)
	}
	return members, nil
}

func firewalldZoneParam(value string) (string, error) {
	zone := strings.TrimSpace(value)
	if zone == "" {
		zone = "public"
	}
	if !firewalldName.MatchString(zone) {
		return "", fmt.Errorf("firewalld 区域名无效")
	}
	return zone, nil
}

func firewalldServiceParam(value string) (string, error) {
	service := strings.TrimSpace(value)
	if !firewalldName.MatchString(service) {
		return "", fmt.Errorf("firewalld 服务名无效")
	}
	return service, nil
}
