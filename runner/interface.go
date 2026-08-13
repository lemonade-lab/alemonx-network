package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// Interface listing and per-interface management. Mutating actions validate
// their parameters in params.go, never concatenate shell strings, and degrade
// gracefully on platforms without the needed tooling.

func interfaceList() []string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return []string{"! 读取网卡信息失败：" + err.Error()}
	}
	lines := []string{}
	for _, item := range interfaces {
		if item.Flags&net.FlagLoopback != 0 {
			continue
		}
		state := "down"
		if item.Flags&net.FlagUp != 0 {
			state = "up"
		}
		header := fmt.Sprintf("接口 %s（%s，MTU %d", item.Name, state, item.MTU)
		if len(item.HardwareAddr) > 0 {
			header += "，MAC " + item.HardwareAddr.String()
		}
		header += "）"
		if state == "up" && runtime.GOOS == "linux" {
			if speed := linuxLinkSpeed(item.Name); speed != "" {
				header += "，速率 " + speed
			}
		}
		lines = append(lines, header)
		addresses, _ := item.Addrs()
		if len(addresses) == 0 {
			lines = append(lines, "  （无地址）")
			continue
		}
		for _, address := range addresses {
			lines = append(lines, "  "+address.String())
		}
	}
	if len(lines) == 0 {
		lines = append(lines, "! 未找到可用网卡")
	}
	return lines
}

func linuxLinkSpeed(name string) string {
	data, err := os.ReadFile("/sys/class/net/" + name + "/speed")
	if err != nil {
		return ""
	}
	value := strings.TrimSpace(string(data))
	if value == "-1" {
		return ""
	}
	return value + " Mb/s"
}

// ifaceAction enables or disables an interface.
func ifaceAction(action string, params map[string]string) (string, error) {
	name, err := ifaceParam(param(params, "interface"))
	if err != nil {
		return "", err
	}
	up := action == "iface-up"
	verb := map[bool]string{true: "启用", false: "停用"}[up]
	switch runtime.GOOS {
	case "linux":
		state := "down"
		if up {
			state = "up"
		}
		output, runErr := exec.Command("ip", "link", "set", name, state).CombinedOutput()
		if runErr != nil {
			return strings.TrimSpace(string(output)), fmt.Errorf("无法%s接口 %s：%w", verb, name, runErr)
		}
		return fmt.Sprintf("已%s接口 %s。", verb, name), nil
	case "darwin":
		state := "down"
		if up {
			state = "up"
		}
		output, runErr := exec.Command("ifconfig", name, state).CombinedOutput()
		if runErr != nil {
			return strings.TrimSpace(string(output)), fmt.Errorf("无法%s接口 %s：%w", verb, name, runErr)
		}
		return fmt.Sprintf("已%s接口 %s。", verb, name), nil
	case "windows":
		admin := "disable"
		if up {
			admin = "enable"
		}
		output, runErr := exec.Command("netsh", "interface", "set", "interface", "name="+name, "admin="+admin).CombinedOutput()
		if runErr != nil {
			return strings.TrimSpace(string(output)), fmt.Errorf("无法%s接口 %s：%w", verb, name, runErr)
		}
		return fmt.Sprintf("已%s接口 %s。", verb, name), nil
	default:
		return "", fmt.Errorf("当前系统暂不支持接口启停")
	}
}

// ipAddress adds or removes a static IPv4 address on an interface.
func ipAddress(action string, params map[string]string) (string, error) {
	name, err := ifaceParam(param(params, "interface"))
	if err != nil {
		return "", err
	}
	normalized, prefix, err := cidrParam(param(params, "cidr"), "IP/CIDR")
	if err != nil {
		return "", err
	}
	add := action == "ip-add"
	verb := map[bool]string{true: "添加", false: "移除"}[add]
	switch runtime.GOOS {
	case "linux":
		verbArg := "del"
		if add {
			verbArg = "add"
		}
		output, runErr := exec.Command("ip", "addr", verbArg, normalized, "dev", name).CombinedOutput()
		if runErr != nil {
			return strings.TrimSpace(string(output)), fmt.Errorf("无法%s地址 %s：%w", verb, normalized, runErr)
		}
		return fmt.Sprintf("已在接口 %s %s地址 %s。", name, verb, normalized), nil
	case "darwin":
		verbArg := "del"
		if add {
			verbArg = "add"
		}
		output, runErr := exec.Command("ifconfig", name, verbArg, normalized).CombinedOutput()
		if runErr != nil {
			return strings.TrimSpace(string(output)), fmt.Errorf("无法%s地址 %s：%w", verb, normalized, runErr)
		}
		return fmt.Sprintf("已在接口 %s %s地址 %s（仅临时生效）。", name, verb, normalized), nil
	case "windows":
		verbArg := "delete"
		if add {
			verbArg = "add"
		}
		args := []string{"interface", "ipv4", verbArg, "address", "name=" + name, normalized}
		if add {
			args = append(args, maskString(prefix))
		}
		output, runErr := exec.Command("netsh", args...).CombinedOutput()
		if runErr != nil {
			return strings.TrimSpace(string(output)), fmt.Errorf("无法%s地址 %s：%w", verb, normalized, runErr)
		}
		return fmt.Sprintf("已在接口 %s %s地址 %s。", name, verb, normalized), nil
	default:
		return "", fmt.Errorf("当前系统暂不支持静态 IP 管理")
	}
}

// dnsSet sets static DNS servers for an interface or network service.
func dnsSet(params map[string]string) (string, error) {
	name, err := ifaceParam(param(params, "interface"))
	if err != nil {
		return "", err
	}
	servers := strings.Fields(param(params, "dns"))
	if len(servers) == 0 {
		return "", fmt.Errorf("请输入至少一个 DNS 服务器")
	}
	for _, server := range servers {
		if _, err := ipParam(server, "DNS 服务器"); err != nil {
			return "", err
		}
	}
	switch runtime.GOOS {
	case "linux":
		if _, err := exec.LookPath("resolvectl"); err != nil {
			return "", fmt.Errorf("未找到 resolvectl；为避免猜测网络管理器，本插件不会直接改写 /etc/resolv.conf")
		}
		args := append([]string{"dns", name}, servers...)
		output, runErr := exec.Command("resolvectl", args...).CombinedOutput()
		if runErr != nil {
			return strings.TrimSpace(string(output)), fmt.Errorf("无法为接口 %s 设置 DNS：%w", name, runErr)
		}
		return fmt.Sprintf("已为接口 %s 设置 DNS：%s。", name, strings.Join(servers, "、")), nil
	case "darwin":
		args := append([]string{"-setdnsservers", name}, servers...)
		output, runErr := exec.Command("networksetup", args...).CombinedOutput()
		if runErr != nil {
			return strings.TrimSpace(string(output)), fmt.Errorf("无法为网络服务 %s 设置 DNS：%w", name, runErr)
		}
		return fmt.Sprintf("已为网络服务 %s 设置 DNS：%s。", name, strings.Join(servers, "、")), nil
	case "windows":
		args := []string{"interface", "ipv4", "set", "dnsservers", "name=" + name, "static", servers[0], "validate=no"}
		for _, server := range servers[1:] {
			args = append(args, "add="+server)
		}
		output, runErr := exec.Command("netsh", args...).CombinedOutput()
		if runErr != nil {
			return strings.TrimSpace(string(output)), fmt.Errorf("无法为接口 %s 设置 DNS：%w", name, runErr)
		}
		return fmt.Sprintf("已为接口 %s 设置 DNS：%s。", name, strings.Join(servers, "、")), nil
	default:
		return "", fmt.Errorf("当前系统暂不支持 DNS 管理")
	}
}

// mtuSet sets the MTU of an interface.
func mtuSet(params map[string]string) (string, error) {
	name, err := ifaceParam(param(params, "interface"))
	if err != nil {
		return "", err
	}
	mtu, err := mtuParam(param(params, "mtu"))
	if err != nil {
		return "", err
	}
	switch runtime.GOOS {
	case "linux":
		output, runErr := exec.Command("ip", "link", "set", name, "mtu", strconv.Itoa(mtu)).CombinedOutput()
		if runErr != nil {
			return strings.TrimSpace(string(output)), fmt.Errorf("无法设置接口 %s 的 MTU：%w", name, runErr)
		}
		return fmt.Sprintf("已将接口 %s 的 MTU 设置为 %d。", name, mtu), nil
	case "darwin":
		output, runErr := exec.Command("ifconfig", name, "mtu", strconv.Itoa(mtu)).CombinedOutput()
		if runErr != nil {
			return strings.TrimSpace(string(output)), fmt.Errorf("无法设置接口 %s 的 MTU：%w", name, runErr)
		}
		return fmt.Sprintf("已将接口 %s 的 MTU 设置为 %d（仅临时生效）。", name, mtu), nil
	case "windows":
		output, runErr := exec.Command("netsh", "interface", "ipv4", "set", "subinterface", "name="+name, "mtu="+strconv.Itoa(mtu), "store=persistent").CombinedOutput()
		if runErr != nil {
			return strings.TrimSpace(string(output)), fmt.Errorf("无法设置接口 %s 的 MTU：%w", name, runErr)
		}
		return fmt.Sprintf("已将接口 %s 的 MTU 设置为 %d。", name, mtu), nil
	default:
		return "", fmt.Errorf("当前系统暂不支持 MTU 管理")
	}
}

// routeAction adds or removes a static route.
func routeAction(action string, params map[string]string) (string, error) {
	destination, prefix, err := cidrParam(param(params, "destination"), "目标网段")
	if err != nil {
		return "", err
	}
	gateway, err := ipParam(param(params, "gateway"), "网关")
	if err != nil {
		return "", err
	}
	iface := param(params, "interface")
	iface = strings.TrimSpace(iface)
	if iface != "" {
		if _, err := ifaceParam(iface); err != nil {
			return "", err
		}
	}
	add := action == "route-add"
	verb := map[bool]string{true: "添加", false: "移除"}[add]
	switch runtime.GOOS {
	case "linux":
		verbArg := "del"
		if add {
			verbArg = "add"
		}
		args := []string{"route", verbArg, destination, "via", gateway}
		if iface != "" {
			args = append(args, "dev", iface)
		}
		output, runErr := exec.Command("ip", args...).CombinedOutput()
		if runErr != nil {
			return strings.TrimSpace(string(output)), fmt.Errorf("无法%s路由 %s 经 %s：%w", verb, destination, gateway, runErr)
		}
		return fmt.Sprintf("已%s路由 %s → %s。", verb, destination, gateway), nil
	case "darwin":
		if add {
			args := []string{"-n", "add", "-net", destination, gateway}
			output, runErr := exec.Command("route", args...).CombinedOutput()
			if runErr != nil {
				return strings.TrimSpace(string(output)), fmt.Errorf("无法%s路由 %s 经 %s：%w", verb, destination, gateway, runErr)
			}
			return fmt.Sprintf("已%s路由 %s → %s。", verb, destination, gateway), nil
		}
		output, runErr := exec.Command("route", "-n", "delete", "-net", destination, gateway).CombinedOutput()
		if runErr != nil {
			return strings.TrimSpace(string(output)), fmt.Errorf("无法%s路由 %s 经 %s：%w", verb, destination, gateway, runErr)
		}
		return fmt.Sprintf("已%s路由 %s → %s。", verb, destination, gateway), nil
	case "windows":
		ones, _ := prefix.Mask.Size()
		if add {
			args := []string{"add", destination, "mask", maskString(prefix), gateway}
			if iface != "" {
				args = append(args, "if", iface)
			}
			args = append(args, "-p")
			output, runErr := exec.Command("route", args...).CombinedOutput()
			if runErr != nil {
				return strings.TrimSpace(string(output)), fmt.Errorf("无法%s路由 %s 经 %s：%w", verb, destination, gateway, runErr)
			}
			return fmt.Sprintf("已%s路由 %s/%d → %s。", verb, destination, ones, gateway), nil
		}
		output, runErr := exec.Command("route", "delete", destination, "mask", maskString(prefix), gateway).CombinedOutput()
		if runErr != nil {
			return strings.TrimSpace(string(output)), fmt.Errorf("无法%s路由 %s 经 %s：%w", verb, destination, gateway, runErr)
		}
		return fmt.Sprintf("已%s路由 %s/%d → %s。", verb, destination, ones, gateway), nil
	default:
		return "", fmt.Errorf("当前系统暂不支持静态路由管理")
	}
}

// trafficSnapshot reads cumulative RX/TX byte and packet counters.
func trafficSnapshot() string {
	switch runtime.GOOS {
	case "linux":
		return linuxTraffic()
	case "darwin":
		return darwinTraffic()
	case "windows":
		return windowsTraffic()
	default:
		return "? 当前系统暂不支持流量统计。"
	}
}

func linuxTraffic() string {
	data, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return "! 读取 /proc/net/dev 失败：" + err.Error()
	}
	lines := []string{"接口\t\t接收 (字节/报文)\t\t发送 (字节/报文)"}
	for _, raw := range strings.Split(string(data), "\n") {
		raw = strings.TrimSpace(raw)
		if raw == "" || !strings.Contains(raw, ":") {
			continue
		}
		parts := strings.SplitN(raw, ":", 2)
		name := strings.TrimSpace(parts[0])
		fields := strings.Fields(parts[1])
		if len(fields) < 9 {
			continue
		}
		rxBytes, _ := strconv.ParseUint(fields[0], 10, 64)
		rxPackets, _ := strconv.ParseUint(fields[1], 10, 64)
		txBytes, _ := strconv.ParseUint(fields[8], 10, 64)
		txPackets, _ := strconv.ParseUint(fields[9], 10, 64)
		lines = append(lines, fmt.Sprintf("%s\t%s / %d\t\t%s / %d", name, humanBytes(rxBytes), rxPackets, humanBytes(txBytes), txPackets))
	}
	return strings.Join(lines, "\n")
}

func darwinTraffic() string {
	output := commandOutput("netstat", "-ib")
	lines := []string{"接口\t\t接收 (字节/报文)\t\t发送 (字节/报文)"}
	for _, raw := range strings.Split(output, "\n") {
		fields := strings.Fields(raw)
		if len(fields) < 10 || !strings.HasPrefix(fields[2], "<Link") {
			continue
		}
		name := fields[0]
		rxBytes, _ := strconv.ParseUint(fields[6], 10, 64)
		rxPackets, _ := strconv.ParseUint(fields[4], 10, 64)
		txBytes, _ := strconv.ParseUint(fields[9], 10, 64)
		txPackets, _ := strconv.ParseUint(fields[7], 10, 64)
		lines = append(lines, fmt.Sprintf("%s\t\t%s / %d\t\t%s / %d", name, humanBytes(rxBytes), rxPackets, humanBytes(txBytes), txPackets))
	}
	if len(lines) == 1 {
		return "? 未读取到接口流量：" + output
	}
	return strings.Join(lines, "\n")
}

func windowsTraffic() string {
	output := commandOutput("powershell", "-NoProfile", "-Command",
		"Get-NetAdapterStatistics | Where-Object Name -ne '' | Select-Object Name,ReceivedBytes,SentBytes,ReceivedPackets,SentPackets | Format-Table -AutoSize")
	if strings.Contains(output, "未找到命令") {
		return "? 当前系统暂不支持流量统计。"
	}
	return output
}

func humanBytes(value uint64) string {
	const unit = 1024
	if value < unit {
		return strconv.FormatUint(value, 10) + " B"
	}
	div, exp := uint64(unit), 0
	for n := value / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(value)/float64(div), "KMGTPE"[exp])
}
