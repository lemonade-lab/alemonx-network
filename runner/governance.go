package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"
)

type Capability struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Available bool   `json:"available"`
	Elevated  bool   `json:"elevated"`
	Reason    string `json:"reason,omitempty"`
}

type CapabilitySnapshot struct {
	Platform     string       `json:"platform"`
	Capabilities []Capability `json:"capabilities"`
}

type InterfaceSnapshot struct {
	Name      string   `json:"name"`
	Up        bool     `json:"up"`
	MTU       int      `json:"mtu"`
	MAC       string   `json:"mac,omitempty"`
	Addresses []string `json:"addresses"`
}

type NetworkSnapshot struct {
	Platform     string              `json:"platform"`
	CapturedAt   string              `json:"capturedAt"`
	Interfaces   []InterfaceSnapshot `json:"interfaces"`
	DefaultRoute string              `json:"defaultRoute"`
	Traffic      string              `json:"traffic"`
	Fingerprint  string              `json:"fingerprint"`
}

type DiagnosticStep struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Status  string `json:"status"`
	Detail  string `json:"detail"`
	Latency int64  `json:"latencyMs,omitempty"`
}

type Diagnosis struct {
	Target string           `json:"target"`
	Steps  []DiagnosticStep `json:"steps"`
}

type ChangePlan struct {
	ID           string            `json:"id"`
	Operation    string            `json:"operation"`
	Params       map[string]string `json:"params"`
	Fingerprint  string            `json:"fingerprint"`
	Risk         string            `json:"risk"`
	Impact       string            `json:"impact"`
	Verification []string          `json:"verification"`
	CreatedAt    string            `json:"createdAt"`
	ExpiresAt    string            `json:"expiresAt"`
}

func capabilitySnapshot() CapabilitySnapshot {
	all := []Capability{
		{ID: "snapshot", Label: "网络快照", Available: true},
		{ID: "interfaces", Label: "接口与 IP 配置", Available: true, Elevated: true},
		{ID: "routes", Label: "路由与 MTU", Available: true, Elevated: true},
		{ID: "dns", Label: "DNS 配置", Available: true, Elevated: true},
		{ID: "traffic", Label: "流量快照", Available: true},
		{ID: "forwarding", Label: "端口转发", Available: true, Elevated: true},
		{ID: "firewall", Label: "防火墙规则", Available: runtime.GOOS != "darwin", Elevated: true},
		{ID: "firewall-toggle", Label: "防火墙总开关", Available: firewallToggleAvailable(), Elevated: true},
		{ID: "virtual", Label: "Bond、网桥与 VLAN", Available: runtime.GOOS == "linux", Elevated: true},
	}
	for index := range all {
		if !all[index].Available {
			all[index].Reason = capabilityReason(all[index].ID)
		}
	}
	return CapabilitySnapshot{Platform: runtime.GOOS + "/" + runtime.GOARCH, Capabilities: all}
}

func firewallToggleAvailable() bool {
	return runtime.GOOS == "windows" || (runtime.GOOS == "linux" && commandExists("ufw"))
}

func capabilityReason(id string) string {
	switch id {
	case "firewall-toggle":
		switch runtime.GOOS {
		case "darwin":
			return "macOS 应用防火墙不支持由本插件控制。"
		case "linux":
			return "未检测到 UFW；firewalld 没有安全的全局开关。"
		default:
			return "当前系统暂不支持开关防火墙。"
		}
	case "firewall":
		return "macOS 应用防火墙不支持按端口安全修改；请在系统设置中管理。"
	default:
		return "当前系统不提供由本插件安全管理的此项能力。"
	}
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func networkSnapshot() NetworkSnapshot {
	items, _ := net.Interfaces()
	interfaces := make([]InterfaceSnapshot, 0, len(items))
	fingerprintParts := make([]string, 0, len(items)+1)
	for _, item := range items {
		if item.Flags&net.FlagLoopback != 0 {
			continue
		}
		addresses, _ := item.Addrs()
		values := make([]string, 0, len(addresses))
		for _, address := range addresses {
			values = append(values, address.String())
		}
		sort.Strings(values)
		interfaces = append(interfaces, InterfaceSnapshot{Name: item.Name, Up: item.Flags&net.FlagUp != 0, MTU: item.MTU, MAC: item.HardwareAddr.String(), Addresses: values})
		fingerprintParts = append(fingerprintParts, item.Name+"|"+strings.Join(values, ","))
	}
	sort.Slice(interfaces, func(i, j int) bool { return interfaces[i].Name < interfaces[j].Name })
	route := defaultRoute()
	fingerprintParts = append(fingerprintParts, route)
	hash := sha256.Sum256([]byte(strings.Join(fingerprintParts, "\n")))
	return NetworkSnapshot{Platform: runtime.GOOS + "/" + runtime.GOARCH, CapturedAt: time.Now().Format(time.RFC3339), Interfaces: interfaces, DefaultRoute: route, Traffic: trafficSnapshot(), Fingerprint: hex.EncodeToString(hash[:])}
}

func networkDiagnosis(params map[string]string) Diagnosis {
	target := strings.TrimSpace(params["target"])
	if target == "" {
		target = "registry.npmjs.org"
	}
	steps := []DiagnosticStep{}
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	addresses, err := net.DefaultResolver.LookupHost(ctx, target)
	if err != nil {
		steps = append(steps, DiagnosticStep{ID: "dns", Label: "DNS 解析", Status: "fail", Detail: err.Error(), Latency: time.Since(start).Milliseconds()})
		return Diagnosis{Target: target, Steps: steps}
	}
	steps = append(steps, DiagnosticStep{ID: "dns", Label: "DNS 解析", Status: "ok", Detail: strings.Join(addresses, ", "), Latency: time.Since(start).Milliseconds()})
	start = time.Now()
	connection, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(target, "443"))
	if err != nil {
		steps = append(steps, DiagnosticStep{ID: "tcp", Label: "TCP 443 连通性", Status: "fail", Detail: err.Error(), Latency: time.Since(start).Milliseconds()})
	} else {
		_ = connection.Close()
		steps = append(steps, DiagnosticStep{ID: "tcp", Label: "TCP 443 连通性", Status: "ok", Detail: "连接成功", Latency: time.Since(start).Milliseconds()})
	}
	steps = append(steps, DiagnosticStep{ID: "route", Label: "默认路由", Status: "ok", Detail: defaultRoute()})
	return Diagnosis{Target: target, Steps: steps}
}

func diagnosisSummary(result Diagnosis) string {
	for _, step := range result.Steps {
		if step.Status == "fail" {
			return "网络诊断发现异常：" + step.Label
		}
	}
	return "网络诊断完成，目标连通正常。"
}

func createPlan(params map[string]string) (ChangePlan, error) {
	operation := strings.TrimSpace(params["operation"])
	if !isMutatingAction(operation) {
		return ChangePlan{}, fmt.Errorf("不支持预演此操作：%s", operation)
	}
	copyParams := map[string]string{}
	for key, value := range params {
		if key != "operation" {
			copyParams[key] = value
		}
	}
	risk := "medium"
	impact := "将修改服务器网络配置；请确认远程管理通道仍可用。"
	if operation == "open-port" || operation == "forward-add" || strings.HasPrefix(operation, "firewalld-") {
		risk, impact = "high", "可能扩大设备可被网络访问的范围。"
	} else if operation == "firewall-set" {
		risk, impact = "high", "将启用或停用整个系统防火墙，可能阻断所有入站连接，或让设备暴露在公网。"
	}
	now := time.Now()
	// The unprivileged runner may preview a plan but must never persist the
	// capability that later authorizes root work. The host replaces ID and owns
	// plan/audit persistence in its SQLite journal.
	return ChangePlan{Operation: operation, Params: copyParams, Fingerprint: networkSnapshot().Fingerprint, Risk: risk, Impact: impact, Verification: []string{"重新读取服务器网络快照", "确认 SSH、远程桌面或其他管理通道仍可用", "执行 DNS 与 TCP 连通性检查"}, CreatedAt: now.Format(time.RFC3339), ExpiresAt: now.Add(10 * time.Minute).Format(time.RFC3339)}, nil
}

func applyApprovedPlan(params map[string]string) (actionResult, error) {
	operation := strings.TrimSpace(params["operation"])
	if !isMutatingAction(operation) {
		return actionResult{}, fmt.Errorf("宿主批准的网络变更无效")
	}
	if expected := strings.TrimSpace(params["__alxFingerprint"]); expected != "" && networkSnapshot().Fingerprint != expected {
		return actionResult{}, fmt.Errorf("当前网络状态已变化，请重新预演后再应用")
	}
	values := map[string]string{}
	for key, value := range params {
		if key != "operation" && !strings.HasPrefix(key, "__alx") {
			values[key] = value
		}
	}
	return runAction(operation, values)
}

func undoApproved(params map[string]string) (actionResult, error) {
	operation := inverseOperation(strings.TrimSpace(params["operation"]))
	if operation == "" {
		return actionResult{}, fmt.Errorf("最近操作不支持自动撤销")
	}
	values := map[string]string{}
	for key, value := range params {
		if key != "operation" && !strings.HasPrefix(key, "__alx") {
			values[key] = value
		}
	}
	return runAction(operation, values)
}

func inverseOperation(operation string) string {
	return map[string]string{"open-port": "close-port", "close-port": "open-port", "iface-up": "iface-down", "iface-down": "iface-up", "ip-add": "ip-remove", "ip-remove": "ip-add", "route-add": "route-remove", "route-remove": "route-add", "forward-add": "forward-remove", "forward-remove": "forward-add", "bond-create": "bond-delete", "bridge-create": "bridge-delete", "vlan-create": "vlan-delete", "firewalld-service-add": "firewalld-service-remove", "firewalld-service-remove": "firewalld-service-add"}[operation]
}

func isMutatingAction(action string) bool {
	switch action {
	case "firewall-set", "open-port", "close-port", "iface-up", "iface-down", "ip-add", "ip-remove", "dns-set", "mtu-set", "route-add", "route-remove", "forward-add", "forward-remove", "bond-create", "bond-delete", "bridge-create", "bridge-delete", "vlan-create", "vlan-delete", "firewalld-service-add", "firewalld-service-remove", "firewalld-zone-set-default":
		return true
	default:
		return false
	}
}
