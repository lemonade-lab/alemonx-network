package main

import (
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// firewalld zone and service management. Linux-only; requires a running
// firewalld. All mutations use --permanent + --reload so rules survive a
// restart, mirroring Cockpit. Commands are fixed argument arrays.

func requireFirewalld() error {
	if runtime.GOOS != "linux" {
		return errors.New("当前系统仅 Linux 支持 firewalld 区域/服务管理")
	}
	if _, err := exec.LookPath("firewall-cmd"); err != nil {
		return errors.New("未找到 firewall-cmd；请先安装 firewalld")
	}
	output, err := exec.Command("firewall-cmd", "--state").CombinedOutput()
	if err != nil || !strings.Contains(strings.TrimSpace(string(output)), "running") {
		return errors.New("firewalld 未在运行，无法管理区域/服务")
	}
	return nil
}

func firewalldRun(args ...string) (string, error) {
	output, err := exec.Command("firewall-cmd", args...).CombinedOutput()
	if err != nil {
		text := strings.TrimSpace(string(output))
		if text == "" {
			text = err.Error()
		}
		return "", fmt.Errorf("firewall-cmd 执行失败：%s", text)
	}
	return strings.TrimSpace(string(output)), nil
}

func firewalldZones() (string, error) {
	if err := requireFirewalld(); err != nil {
		return "", err
	}
	defaultZone, err := firewalldRun("--get-default-zone")
	if err != nil {
		return "", err
	}
	zonesRaw, err := firewalldRun("--get-zones")
	if err != nil {
		return "", err
	}
	zones := strings.Fields(zonesRaw)
	lines := []string{"✓ firewalld 已运行", "✓ 默认区域：" + defaultZone}
	if len(zones) == 0 {
		lines = append(lines, "? 未找到任何区域")
		return strings.Join(lines, "\n"), nil
	}
	lines = append(lines, "✓ 区域："+strings.Join(zones, ", "))
	for _, zone := range zones {
		services, err := firewalldRun("--zone="+zone, "--list-services")
		if err != nil {
			continue
		}
		if services == "" {
			services = "（无）"
		}
		lines = append(lines, "  "+zone+" 服务："+services)
	}
	return strings.Join(lines, "\n"), nil
}

func firewalldServiceAction(action string, params map[string]string) (string, error) {
	if err := requireFirewalld(); err != nil {
		return "", err
	}
	zone, err := firewalldZoneParam(param(params, "zone"))
	if err != nil {
		return "", err
	}
	service, err := firewalldServiceParam(param(params, "service"))
	if err != nil {
		return "", err
	}
	flag := "--add-service=" + service
	verb := "已添加"
	if action == "firewalld-service-remove" {
		flag = "--remove-service=" + service
		verb = "已移除"
	}
	if _, err := firewalldRun("--permanent", "--zone="+zone, flag); err != nil {
		return "", err
	}
	if _, err := firewalldRun("--reload"); err != nil {
		return "", err
	}
	return fmt.Sprintf("✓ 已%s：%s 区域的 %s 服务（持久化）。", verb, zone, service), nil
}

func firewalldSetDefaultZone(params map[string]string) (string, error) {
	if err := requireFirewalld(); err != nil {
		return "", err
	}
	zone, err := firewalldZoneParam(param(params, "zone"))
	if err != nil {
		return "", err
	}
	if _, err := firewalldRun("--set-default-zone=" + zone); err != nil {
		return "", err
	}
	return fmt.Sprintf("✓ 已将默认区域切换为 %s。", zone), nil
}
