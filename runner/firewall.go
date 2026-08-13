package main

import (
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// FirewallStatus is the structured, read-only firewall state that feeds the
// Security tab's status card. The master switch probes the same backends.
type FirewallStatus struct {
	Available bool   `json:"available"`
	Enabled   *bool  `json:"enabled,omitempty"`
	Backend   string `json:"backend,omitempty"`
	Detail    string `json:"detail,omitempty"`
}

// firewallStatus probes the active platform backend without modifying anything.
func firewallStatus() FirewallStatus {
	switch runtime.GOOS {
	case "windows":
		return windowsFirewallStatus()
	case "linux":
		return linuxFirewallStatus()
	case "darwin":
		return FirewallStatus{Available: false, Detail: "macOS 应用防火墙不支持由本插件读取或修改；请在系统设置中管理。"}
	default:
		return FirewallStatus{Available: false, Detail: "当前系统暂不支持读取防火墙状态。"}
	}
}

func windowsFirewallStatus() FirewallStatus {
	output := commandOutput("netsh", "advfirewall", "show", "allprofiles")
	if strings.Contains(output, "未找到命令") || strings.Contains(output, "Operation not permitted") {
		return FirewallStatus{Available: false, Detail: "无法读取 Windows 防火墙：" + output}
	}
	status := FirewallStatus{Available: true, Backend: "netsh"}
	switch parseWindowsFirewallState(output) {
	case "on":
		status.Enabled = boolPtr(true)
		status.Detail = "Windows 防火墙已启用。"
	case "off":
		status.Enabled = boolPtr(false)
		status.Detail = "Windows 防火墙已停用。"
	default:
		status.Detail = "未能确定 Windows 防火墙开关状态。"
	}
	return status
}

// parseWindowsFirewallState reads the per-profile State lines printed by
// `netsh advfirewall show allprofiles`. Any enabled profile keeps the system
// meaningfully protected, so "on" wins over "off".
func parseWindowsFirewallState(output string) string {
	on, off := false, false
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || !strings.EqualFold(fields[0], "State") {
			continue
		}
		switch strings.ToUpper(fields[1]) {
		case "ON":
			on = true
		case "OFF":
			off = true
		}
	}
	switch {
	case on:
		return "on"
	case off:
		return "off"
	default:
		return "unknown"
	}
}

func linuxFirewallStatus() FirewallStatus {
	if output := commandOutput("ufw", "status", "verbose"); !strings.Contains(output, "未找到命令") {
		enabled := strings.Contains(output, "Status: active")
		status := FirewallStatus{Available: true, Backend: "ufw", Enabled: boolPtr(enabled)}
		if enabled {
			status.Detail = "UFW 防火墙已启用。"
		} else {
			status.Detail = "UFW 防火墙未启用。"
		}
		return status
	}
	if output := commandOutput("firewall-cmd", "--state"); !strings.Contains(output, "未找到命令") {
		running := strings.Contains(strings.TrimSpace(output), "running")
		status := FirewallStatus{Available: true, Backend: "firewalld", Enabled: boolPtr(running)}
		if running {
			status.Detail = "firewalld 正在运行。"
		} else {
			status.Detail = "firewalld 未运行。"
		}
		return status
	}
	return FirewallStatus{Available: false, Detail: "未检测到 UFW 或 firewalld。"}
}

func boolPtr(value bool) *bool { return &value }

// setFirewall toggles the whole system firewall. It is deliberately a
// high-risk, plan-based privileged operation: Windows flips all profiles,
// Linux uses UFW when present and refuses to guess for firewalld, which has
// no safe global off switch.
func setFirewall(params map[string]string) (string, error) {
	state := strings.ToLower(strings.TrimSpace(params["state"]))
	if state != "on" && state != "off" {
		return "", errors.New("防火墙开关仅支持 on 或 off")
	}
	switch runtime.GOOS {
	case "windows":
		output, err := exec.Command("netsh", "advfirewall", "set", "allprofiles", "state", state).CombinedOutput()
		if err != nil {
			return strings.TrimSpace(string(output)), fmt.Errorf("切换 Windows 防火墙失败：%w；通常需要以管理员身份运行 ALemonX", err)
		}
		verb := map[string]string{"on": "已启用", "off": "已停用"}[state]
		return fmt.Sprintf("%s Windows 防火墙。\n%s", verb, strings.TrimSpace(string(output))), nil
	case "linux":
		if _, err := exec.LookPath("ufw"); err == nil {
			action, verb := "enable", "已启用"
			if state == "off" {
				action, verb = "disable", "已停用"
			}
			output, err := exec.Command("ufw", action).CombinedOutput()
			if err != nil {
				return strings.TrimSpace(string(output)), fmt.Errorf("切换 UFW 失败：%w；通常需要以管理员身份运行 ALemonX", err)
			}
			detail := strings.TrimSpace(string(output))
			if detail == "" {
				detail = "完成。"
			}
			return fmt.Sprintf("%s UFW 防火墙。\n%s", verb, detail), nil
		}
		if _, err := exec.LookPath("firewall-cmd"); err == nil {
			return "", errors.New("firewalld 没有安全的全局开关；请保持启用，或使用系统服务管理")
		}
		return "", errors.New("未检测到 UFW 或 firewalld；为避免猜测防火墙工具，本插件不会修改系统规则")
	default:
		return "", errors.New("当前系统暂不支持开关防火墙")
	}
}
