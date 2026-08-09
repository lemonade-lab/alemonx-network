package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
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

type AuditEntry struct {
	ID            string            `json:"id"`
	Operation     string            `json:"operation"`
	Params        map[string]string `json:"params"`
	Output        string            `json:"output"`
	UndoOperation string            `json:"undoOperation,omitempty"`
	CreatedAt     string            `json:"createdAt"`
}

// governanceStateDir is supplied only by the ALemonX host to an elevated
// process, keeping its plan and audit files owned by the initiating user.
var governanceStateDir string

func capabilitySnapshot() CapabilitySnapshot {
	all := []Capability{
		{ID: "snapshot", Label: "网络快照", Available: true},
		{ID: "interfaces", Label: "接口与 IP 配置", Available: true, Elevated: true},
		{ID: "routes", Label: "路由与 MTU", Available: true, Elevated: true},
		{ID: "dns", Label: "DNS 配置", Available: true, Elevated: true},
		{ID: "traffic", Label: "流量快照", Available: true},
		{ID: "forwarding", Label: "端口转发", Available: true, Elevated: true},
		{ID: "firewall", Label: "防火墙规则", Available: runtime.GOOS != "darwin", Elevated: true},
		{ID: "virtual", Label: "Bond、网桥与 VLAN", Available: runtime.GOOS == "linux", Elevated: true},
	}
	for index := range all {
		if !all[index].Available {
			all[index].Reason = "当前系统不提供由本插件安全管理的此项能力。"
		}
	}
	return CapabilitySnapshot{Platform: runtime.GOOS + "/" + runtime.GOARCH, Capabilities: all}
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
	id, err := randomID()
	if err != nil {
		return ChangePlan{}, err
	}
	risk := "medium"
	impact := "将修改本机网络配置。"
	if operation == "open-port" || operation == "forward-add" || strings.HasPrefix(operation, "firewalld-") {
		risk, impact = "high", "可能扩大设备可被网络访问的范围。"
	}
	now := time.Now()
	plan := ChangePlan{ID: id, Operation: operation, Params: copyParams, Fingerprint: networkSnapshot().Fingerprint, Risk: risk, Impact: impact, Verification: []string{"重新读取网络快照", "执行 DNS 与 TCP 连通性检查"}, CreatedAt: now.Format(time.RFC3339), ExpiresAt: now.Add(10 * time.Minute).Format(time.RFC3339)}
	plans, _ := loadPlans()
	plans = append(plans, plan)
	if err := savePlans(plans); err != nil {
		return ChangePlan{}, err
	}
	return plan, nil
}

func applyPlan(params map[string]string) (actionResult, error) {
	id := strings.TrimSpace(params["planID"])
	plans, err := loadPlans()
	if err != nil {
		return actionResult{}, err
	}
	for _, plan := range plans {
		if plan.ID != id {
			continue
		}
		expires, _ := time.Parse(time.RFC3339, plan.ExpiresAt)
		if time.Now().After(expires) {
			return actionResult{}, fmt.Errorf("变更计划已过期，请重新预演")
		}
		if networkSnapshot().Fingerprint != plan.Fingerprint {
			return actionResult{}, fmt.Errorf("当前网络状态已变化，请重新预演后再应用")
		}
		return runAction(plan.Operation, plan.Params)
	}
	return actionResult{}, fmt.Errorf("未找到变更计划")
}

func isMutatingAction(action string) bool {
	switch action {
	case "set-npm-registry", "reset-npm-registry", "open-port", "close-port", "iface-up", "iface-down", "ip-add", "ip-remove", "dns-set", "mtu-set", "route-add", "route-remove", "forward-add", "forward-remove", "bond-create", "bond-delete", "bridge-create", "bridge-delete", "vlan-create", "vlan-delete", "firewalld-service-add", "firewalld-service-remove", "firewalld-zone-set-default":
		return true
	default:
		return false
	}
}

func governancePath(name string) (string, error) {
	if governanceStateDir != "" {
		return filepath.Join(governanceStateDir, name), nil
	}
	config, err := userConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(config, "alx-network", name), nil
}

func loadPlans() ([]ChangePlan, error) {
	var value []ChangePlan
	return value, loadGovernance("plans.json", &value)
}
func savePlans(value []ChangePlan) error { return saveGovernance("plans.json", value) }
func loadAudit() ([]AuditEntry, error) {
	var value []AuditEntry
	return value, loadGovernance("audit.json", &value)
}

func loadGovernance(name string, value any) error {
	path, err := governancePath(name)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}

func saveGovernance(name string, value any) error {
	path, err := governancePath(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	temporary := path + ".new"
	if err := os.WriteFile(temporary, data, 0600); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

func appendAudit(operation string, params map[string]string, output string) error {
	entries, err := loadAudit()
	if err != nil {
		return err
	}
	id, err := randomID()
	if err != nil {
		return err
	}
	copyParams := map[string]string{}
	for key, value := range params {
		copyParams[key] = value
	}
	entry := AuditEntry{ID: id, Operation: operation, Params: copyParams, Output: output, UndoOperation: inverseOperation(operation), CreatedAt: time.Now().Format(time.RFC3339)}
	entries = append([]AuditEntry{entry}, entries...)
	if len(entries) > 100 {
		entries = entries[:100]
	}
	return saveGovernance("audit.json", entries)
}

func inverseOperation(operation string) string {
	values := map[string]string{"open-port": "close-port", "close-port": "open-port", "iface-up": "iface-down", "iface-down": "iface-up", "ip-add": "ip-remove", "ip-remove": "ip-add", "route-add": "route-remove", "route-remove": "route-add", "forward-add": "forward-remove", "forward-remove": "forward-add", "bond-create": "bond-delete", "bridge-create": "bridge-delete", "vlan-create": "vlan-delete", "firewalld-service-add": "firewalld-service-remove", "firewalld-service-remove": "firewalld-service-add"}
	return values[operation]
}

func undoLastChange() (actionResult, error) {
	entries, err := loadAudit()
	if err != nil {
		return actionResult{}, err
	}
	for _, entry := range entries {
		if entry.UndoOperation == "" {
			continue
		}
		return runAction(entry.UndoOperation, entry.Params)
	}
	return actionResult{}, fmt.Errorf("没有可撤销的本机变更")
}

func randomID() (string, error) {
	value := make([]byte, 12)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}
