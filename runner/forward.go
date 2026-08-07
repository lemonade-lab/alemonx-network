package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// Port mapping / DNAT. Windows uses netsh portproxy, Linux uses firewalld
// (falling back to iptables DNAT), macOS uses a userland TCP forwarder. Every
// backend command is fixed and never concatenated from user input.

func forwardList() (string, error) {
	switch runtime.GOOS {
	case "windows":
		return windowsForwardList()
	case "linux":
		return linuxForwardList()
	case "darwin":
		return userlandForwardList()
	default:
		return "", errors.New("当前系统暂不支持端口转发")
	}
}

func forwardAdd(params map[string]string) (string, error) {
	switch runtime.GOOS {
	case "windows":
		return windowsForwardAdd(params)
	case "linux":
		return linuxForwardAdd(params)
	case "darwin":
		return userlandForwardAdd(params)
	default:
		return "", errors.New("当前系统暂不支持端口转发")
	}
}

func forwardRemove(params map[string]string) (string, error) {
	switch runtime.GOOS {
	case "windows":
		return windowsForwardRemove(params)
	case "linux":
		return linuxForwardRemove(params)
	case "darwin":
		return userlandForwardRemove(params)
	default:
		return "", errors.New("当前系统暂不支持端口转发")
	}
}

const firewallHint = "提示：如入站流量仍被系统防火墙拦截，请先在「端口与防火墙」页开放对应端口。"

// firstIPv4 returns the first non-loopback IPv4 address on the host, used as
// the netsh listen address (netsh rejects 0.0.0.0).
func firstIPv4() (string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	for _, item := range interfaces {
		if item.Flags&net.FlagUp == 0 || item.Flags&net.FlagLoopback != 0 {
			continue
		}
		addresses, _ := item.Addrs()
		for _, address := range addresses {
			ip, _, parseErr := net.ParseCIDR(address.String())
			if parseErr == nil && ip.To4() != nil {
				return ip.To4().String(), nil
			}
		}
	}
	return "", errors.New("未找到可用的本机 IPv4 地址")
}

// ---------------------------------------------------------------------------
// Windows: netsh interface portproxy (system-level, TCP only, persistent)
// ---------------------------------------------------------------------------

func windowsForwardList() (string, error) {
	output := commandOutput("netsh", "interface", "portproxy", "show", "all")
	if strings.Contains(output, "未找到命令") {
		return "", errors.New("未找到 netsh")
	}
	return "✓ Windows 端口映射规则（netsh portproxy，仅 TCP）：\n" + output, nil
}

func windowsForwardAdd(params map[string]string) (string, error) {
	listenPort, err := listenPortParam(params)
	if err != nil {
		return "", err
	}
	protocol, err := protocolParam(params)
	if err != nil {
		return "", err
	}
	if protocol != "tcp" {
		return "", errors.New("Windows netsh portproxy 仅支持 TCP 映射，不支持 UDP")
	}
	targetIP, err := ipParam(param(params, "targetIP"), "目标 IP")
	if err != nil {
		return "", err
	}
	targetPort, err := targetPortParam(params, listenPort)
	if err != nil {
		return "", err
	}
	listenAddr := param(params, "listenAddress")
	if listenAddr == "" {
		if listenAddr, err = firstIPv4(); err != nil {
			return "", err
		}
	} else if _, err = ipParam(listenAddr, "监听地址"); err != nil {
		return "", err
	}
	args := []string{"interface", "portproxy", "add", "v4tov4",
		"listenaddress=" + listenAddr, "listenport=" + strconv.Itoa(listenPort),
		"connectaddress=" + targetIP, "connectport=" + strconv.Itoa(targetPort)}
	output, runErr := exec.Command("netsh", args...).CombinedOutput()
	if runErr != nil {
		return strings.TrimSpace(string(output)), fmt.Errorf("无法添加端口映射：%w", runErr)
	}
	return fmt.Sprintf("已添加端口映射：%s:%d → %s:%d（TCP）。\n%s\n%s",
		listenAddr, listenPort, targetIP, targetPort, strings.TrimSpace(string(output)), firewallHint), nil
}

func windowsForwardRemove(params map[string]string) (string, error) {
	listenPort, err := listenPortParam(params)
	if err != nil {
		return "", err
	}
	if _, err := protocolParam(params); err != nil {
		return "", err
	}
	targetIP, err := ipParam(param(params, "targetIP"), "目标 IP")
	if err != nil {
		return "", err
	}
	targetPort, err := targetPortParam(params, listenPort)
	if err != nil {
		return "", err
	}
	listenAddr := param(params, "listenAddress")
	if listenAddr == "" {
		if listenAddr, err = firstIPv4(); err != nil {
			return "", err
		}
	} else if _, err = ipParam(listenAddr, "监听地址"); err != nil {
		return "", err
	}
	args := []string{"interface", "portproxy", "delete", "v4tov4",
		"listenaddress=" + listenAddr, "listenport=" + strconv.Itoa(listenPort),
		"connectaddress=" + targetIP, "connectport=" + strconv.Itoa(targetPort)}
	output, runErr := exec.Command("netsh", args...).CombinedOutput()
	if runErr != nil {
		return strings.TrimSpace(string(output)), fmt.Errorf("无法移除端口映射：%w", runErr)
	}
	return fmt.Sprintf("已移除端口映射：%s:%d → %s:%d。\n%s",
		listenAddr, listenPort, targetIP, targetPort, strings.TrimSpace(string(output))), nil
}

// ---------------------------------------------------------------------------
// Linux: firewalld preferred, iptables DNAT fallback
// ---------------------------------------------------------------------------

func firewalldAvailable() bool {
	output, err := exec.Command("firewall-cmd", "--state").CombinedOutput()
	return err == nil && strings.Contains(strings.TrimSpace(string(output)), "running")
}

func iptablesAvailable() bool {
	_, err := exec.LookPath("iptables")
	return err == nil
}

func forwardSpec(listenPort int, protocol, targetIP string, targetPort int) string {
	return fmt.Sprintf("port=%d:proto=%s:toport=%d:toaddr=%s", listenPort, protocol, targetPort, targetIP)
}

func linuxForwardList() (string, error) {
	if firewalldAvailable() {
		permanent := commandOutput("firewall-cmd", "--permanent", "--list-forward-ports")
		runtimeRules := commandOutput("firewall-cmd", "--list-forward-ports")
		masquerade := commandOutput("firewall-cmd", "--query-masquerade")
		lines := []string{"✓ Linux 端口映射规则（firewalld）："}
		seen := map[string]bool{}
		for _, rule := range append(strings.Split(permanent, "\n"), strings.Split(runtimeRules, "\n")...) {
			rule = strings.TrimSpace(rule)
			if rule == "" || rule == "no" || seen[rule] {
				continue
			}
			seen[rule] = true
			lines = append(lines, "  "+rule)
		}
		if len(lines) == 1 {
			lines = append(lines, "? 暂无端口转发规则")
		}
		lines = append(lines, "✓ Masquerade（地址伪装）："+strings.TrimSpace(masquerade))
		return strings.Join(lines, "\n"), nil
	}
	if iptablesAvailable() {
		output := commandOutput("iptables", "-t", "nat", "-S", "PREROUTING")
		lines := []string{"✓ Linux 端口映射规则（iptables DNAT，非持久化）："}
		found := false
		for _, raw := range strings.Split(output, "\n") {
			if strings.Contains(raw, "-j DNAT") {
				lines = append(lines, "  "+strings.TrimSpace(raw))
				found = true
			}
		}
		if !found {
			lines = append(lines, "? 暂无端口转发规则")
		}
		return strings.Join(lines, "\n"), nil
	}
	return "", errors.New("未找到 firewalld 或 iptables；为避免猜测防火墙工具，本插件不会修改系统规则")
}

func linuxForwardAdd(params map[string]string) (string, error) {
	listenPort, err := listenPortParam(params)
	if err != nil {
		return "", err
	}
	protocol, err := protocolParam(params)
	if err != nil {
		return "", err
	}
	targetIP, err := ipParam(param(params, "targetIP"), "目标 IP")
	if err != nil {
		return "", err
	}
	targetPort, err := targetPortParam(params, listenPort)
	if err != nil {
		return "", err
	}
	if firewalldAvailable() {
		spec := forwardSpec(listenPort, protocol, targetIP, targetPort)
		if output, runErr := exec.Command("firewall-cmd", "--permanent", "--add-forward-port="+spec).CombinedOutput(); runErr != nil {
			return strings.TrimSpace(string(output)), fmt.Errorf("firewalld 添加转发规则失败：%w", runErr)
		}
		if output, runErr := exec.Command("firewall-cmd", "--permanent", "--add-masquerade").CombinedOutput(); runErr != nil {
			return strings.TrimSpace(string(output)), fmt.Errorf("firewalld 启用 masquerade 失败：%w", runErr)
		}
		if output, runErr := exec.Command("firewall-cmd", "--reload").CombinedOutput(); runErr != nil {
			return strings.TrimSpace(string(output)), fmt.Errorf("firewalld 重载失败：%w", runErr)
		}
		return fmt.Sprintf("已添加端口映射：%d/%s → %s:%d（firewalld，持久化）。\n%s",
			listenPort, protocol, targetIP, targetPort, firewallHint), nil
	}
	if iptablesAvailable() {
		target := net.JoinHostPort(targetIP, strconv.Itoa(targetPort))
		args := []string{"-t", "nat", "-A", "PREROUTING", "-p", protocol,
			"--dport", strconv.Itoa(listenPort), "-j", "DNAT", "--to-destination", target}
		if output, runErr := exec.Command("iptables", args...).CombinedOutput(); runErr != nil {
			return strings.TrimSpace(string(output)), fmt.Errorf("iptables 添加 DNAT 规则失败：%w", runErr)
		}
		warning := ""
		if data, readErr := os.ReadFile("/proc/sys/net/ipv4/ip_forward"); readErr == nil && strings.TrimSpace(string(data)) == "0" {
			warning = "\n注意：net.ipv4.ip_forward 为 0，DNAT 可能不生效；请先启用 IP 转发与 FORWARD 放行。"
		}
		return fmt.Sprintf("已添加端口映射：%d/%s → %s:%d（iptables DNAT，重启后失效）。\n%s%s",
			listenPort, protocol, targetIP, targetPort, firewallHint, warning), nil
	}
	return "", errors.New("未找到 firewalld 或 iptables；为避免猜测防火墙工具，本插件不会修改系统规则")
}

func linuxForwardRemove(params map[string]string) (string, error) {
	listenPort, err := listenPortParam(params)
	if err != nil {
		return "", err
	}
	protocol, err := protocolParam(params)
	if err != nil {
		return "", err
	}
	targetIP, err := ipParam(param(params, "targetIP"), "目标 IP")
	if err != nil {
		return "", err
	}
	targetPort, err := targetPortParam(params, listenPort)
	if err != nil {
		return "", err
	}
	if firewalldAvailable() {
		spec := forwardSpec(listenPort, protocol, targetIP, targetPort)
		if output, runErr := exec.Command("firewall-cmd", "--permanent", "--remove-forward-port="+spec).CombinedOutput(); runErr != nil {
			return strings.TrimSpace(string(output)), fmt.Errorf("firewalld 移除转发规则失败：%w", runErr)
		}
		if output, runErr := exec.Command("firewall-cmd", "--reload").CombinedOutput(); runErr != nil {
			return strings.TrimSpace(string(output)), fmt.Errorf("firewalld 重载失败：%w", runErr)
		}
		return fmt.Sprintf("已移除端口映射：%d/%s → %s:%d。", listenPort, protocol, targetIP, targetPort), nil
	}
	if iptablesAvailable() {
		target := net.JoinHostPort(targetIP, strconv.Itoa(targetPort))
		args := []string{"-t", "nat", "-D", "PREROUTING", "-p", protocol,
			"--dport", strconv.Itoa(listenPort), "-j", "DNAT", "--to-destination", target}
		if output, runErr := exec.Command("iptables", args...).CombinedOutput(); runErr != nil {
			return strings.TrimSpace(string(output)), fmt.Errorf("iptables 移除 DNAT 规则失败：%w", runErr)
		}
		return fmt.Sprintf("已移除端口映射：%d/%s → %s:%d。", listenPort, protocol, targetIP, targetPort), nil
	}
	return "", errors.New("未找到 firewalld 或 iptables；为避免猜测防火墙工具，本插件不会修改系统规则")
}
