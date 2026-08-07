package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

// Linux virtual networking: bond (link aggregation), bridge and VLAN. All
// mutations run fixed `ip` commands built from validated names — never a shell
// string. On non-Linux hosts every action degrades gracefully.

func virtualList() (string, error) {
	if runtime.GOOS != "linux" {
		return "", fmt.Errorf("当前系统仅 Linux 支持虚拟网络（Bond/网桥/VLAN）")
	}
	if _, err := exec.LookPath("ip"); err != nil {
		return "", fmt.Errorf("未找到 ip 命令（iproute2）；无法枚举虚拟网络")
	}
	lines := []string{"✓ 虚拟网络概览："}
	bonds, bridges, vlans := classifyVirtual()
	interfaces, _ := net.Interfaces()
	ips := map[string][]string{}
	for _, item := range interfaces {
		if addresses, err := item.Addrs(); err == nil {
			for _, address := range addresses {
				ips[item.Name] = append(ips[item.Name], address.String())
			}
		}
	}
	if len(bonds) == 0 && len(bridges) == 0 && len(vlans) == 0 {
		return strings.Join(append(lines, "? 未创建任何虚拟网络"), "\n"), nil
	}
	if len(bonds) > 0 {
		lines = append(lines, "Bond（链路聚合）：")
		for _, name := range bonds {
			line := fmt.Sprintf("  %s", name)
			if mode := readSysFile("/sys/class/net/" + name + "/bonding/mode"); mode != "" {
				line += "  模式 " + mode
			}
			if slaves := readSysFile("/sys/class/net/" + name + "/bonding/slaves"); slaves != "" {
				line += "  成员 " + strings.Join(strings.Fields(slaves), ", ")
			}
			if addrs := ips[name]; len(addrs) > 0 {
				line += "  IP " + strings.Join(addrs, ", ")
			}
			lines = append(lines, line)
		}
	}
	if len(bridges) > 0 {
		lines = append(lines, "网桥（Bridge）：")
		for _, name := range bridges {
			line := fmt.Sprintf("  %s", name)
			if members := bridgeMembers(name); len(members) > 0 {
				line += "  成员 " + strings.Join(members, ", ")
			}
			if addrs := ips[name]; len(addrs) > 0 {
				line += "  IP " + strings.Join(addrs, ", ")
			}
			lines = append(lines, line)
		}
	}
	if len(vlans) > 0 {
		lines = append(lines, "VLAN：")
		for _, name := range vlans {
			vid, parent := vlanInfo(name)
			line := fmt.Sprintf("  %s", name)
			if vid != "" {
				line += "  VLAN " + vid
			}
			if parent != "" {
				line += "（父接口 " + parent + "）"
			}
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n"), nil
}

// classifyVirtual returns sorted bond/bridge/vlan interface names using sysfs
// markers, which are deterministic and avoid parsing `ip -d` text.
func classifyVirtual() (bonds, bridges, vlans []string) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, nil, nil
	}
	for _, item := range interfaces {
		name := item.Name
		switch {
		case fileExists("/sys/class/net/" + name + "/bonding/mode"):
			bonds = append(bonds, name)
		case dirExists("/sys/class/net/" + name + "/bridge"):
			bridges = append(bridges, name)
		case fileExists("/proc/net/vlan/" + name):
			vlans = append(vlans, name)
		}
	}
	sort.Strings(bonds)
	sort.Strings(bridges)
	sort.Strings(vlans)
	return bonds, bridges, vlans
}

func bridgeMembers(name string) []string {
	entries, err := os.ReadDir("/sys/class/net/" + name + "/brif")
	if err != nil {
		return nil
	}
	members := make([]string, 0, len(entries))
	for _, entry := range entries {
		members = append(members, entry.Name())
	}
	sort.Strings(members)
	return members
}

func vlanInfo(name string) (vid, parent string) {
	data, err := os.ReadFile("/proc/net/vlan/" + name)
	if err != nil {
		return "", ""
	}
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "VID:") {
			vid = strings.TrimSpace(strings.TrimPrefix(line, "VID:"))
		}
		if strings.HasPrefix(line, "DEVICE:") {
			parent = strings.TrimSpace(strings.TrimPrefix(line, "DEVICE:"))
		}
	}
	return vid, parent
}

func readSysFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func requireLinuxVirtual() error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("当前系统仅 Linux 支持虚拟网络（Bond/网桥/VLAN）")
	}
	return nil
}

// linkAction builds and runs a single `ip link ...` command, returning a
// beginner-facing result string.
func linkAction(args []string, success string) (string, error) {
	if err := requireLinuxVirtual(); err != nil {
		return "", err
	}
	if _, err := exec.LookPath("ip"); err != nil {
		return "", fmt.Errorf("未找到 ip 命令（iproute2）；请先安装后再试")
	}
	output, err := exec.Command("ip", args...).CombinedOutput()
	if err != nil {
		return strings.TrimSpace(string(output)), fmt.Errorf("ip 命令执行失败：%w", err)
	}
	if success == "" {
		return "操作已完成。", nil
	}
	return success, nil
}

func bondCreate(params map[string]string) (string, error) {
	if err := requireLinuxVirtual(); err != nil {
		return "", err
	}
	name, err := linuxIfaceNameParam(param(params, "name"), "Bond 名称")
	if err != nil {
		return "", err
	}
	mode, err := bondModeParam(param(params, "mode"))
	if err != nil {
		return "", err
	}
	members, err := memberListParam(param(params, "slaves"), "成员网卡")
	if err != nil {
		return "", err
	}
	if _, err := linkAction([]string{"link", "add", name, "type", "bond", "mode", mode}, ""); err != nil {
		return "", err
	}
	for _, member := range members {
		if _, err := linkAction([]string{"link", "set", member, "master", name}, ""); err != nil {
			_, _ = linkAction([]string{"link", "del", name}, "")
			return "", fmt.Errorf("加入成员 %s 失败，已回滚：%w", member, err)
		}
		if _, err := linkAction([]string{"link", "set", member, "up"}, ""); err != nil {
			return "", err
		}
	}
	if _, err := linkAction([]string{"link", "set", name, "up"}, ""); err != nil {
		return "", err
	}
	return fmt.Sprintf("✓ 已创建 Bond %s（模式 %s，成员 %s）。可在「添加静态 IP」中为它配置地址。",
		name, mode, strings.Join(members, ", ")), nil
}

func bridgeCreate(params map[string]string) (string, error) {
	if err := requireLinuxVirtual(); err != nil {
		return "", err
	}
	name, err := linuxIfaceNameParam(param(params, "name"), "网桥名称")
	if err != nil {
		return "", err
	}
	members, err := memberListParam(param(params, "members"), "成员网卡")
	if err != nil {
		return "", err
	}
	if _, err := linkAction([]string{"link", "add", name, "type", "bridge"}, ""); err != nil {
		return "", err
	}
	for _, member := range members {
		if _, err := linkAction([]string{"link", "set", member, "master", name}, ""); err != nil {
			_, _ = linkAction([]string{"link", "del", name}, "")
			return "", fmt.Errorf("加入成员 %s 失败，已回滚：%w", member, err)
		}
		if _, err := linkAction([]string{"link", "set", member, "up"}, ""); err != nil {
			return "", err
		}
	}
	if _, err := linkAction([]string{"link", "set", name, "up"}, ""); err != nil {
		return "", err
	}
	return fmt.Sprintf("✓ 已创建网桥 %s（成员 %s）。", name, strings.Join(members, ", ")), nil
}

func vlanCreate(params map[string]string) (string, error) {
	if err := requireLinuxVirtual(); err != nil {
		return "", err
	}
	parent, err := linuxIfaceNameParam(param(params, "parent"), "父接口")
	if err != nil {
		return "", err
	}
	id, err := vlanIDParam(param(params, "id"))
	if err != nil {
		return "", err
	}
	interfaces, listErr := net.Interfaces()
	parentExists := false
	for _, item := range interfaces {
		if item.Name == parent {
			parentExists = true
			break
		}
	}
	if listErr != nil || !parentExists {
		return "", fmt.Errorf("父接口 %q 不存在", parent)
	}
	name := fmt.Sprintf("%s.%d", parent, id)
	if _, err := linkAction([]string{"link", "add", "link", parent, "name", name, "type", "vlan", "id", strconv.Itoa(id)}, ""); err != nil {
		return "", err
	}
	if _, err := linkAction([]string{"link", "set", name, "up"}, ""); err != nil {
		return "", err
	}
	return fmt.Sprintf("✓ 已创建 VLAN %s（父接口 %s，ID %d）。", name, parent, id), nil
}

func virtualDelete(kind string, params map[string]string) (string, error) {
	if err := requireLinuxVirtual(); err != nil {
		return "", err
	}
	name, err := linuxIfaceNameParam(param(params, "name"), kind+"名称")
	if err != nil {
		return "", err
	}
	if _, err := linkAction([]string{"link", "del", name}, ""); err != nil {
		return "", err
	}
	return fmt.Sprintf("✓ 已删除 %s %s。", kind, name), nil
}
