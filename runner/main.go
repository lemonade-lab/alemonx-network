package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const protocol = "alx/v1"

type request struct {
	Protocol string            `json:"protocol"`
	Method   string            `json:"method"`
	Action   string            `json:"action"`
	Params   map[string]string `json:"params"`
	Confirm  bool              `json:"confirm,omitempty"`
	StateDir string            `json:"stateDir,omitempty"`
}
type response struct {
	Output string `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
	Data   any    `json:"data,omitempty"`
}

func main() {
	// Detached forwarder entry point used by the macOS port-mapping feature.
	if len(os.Args) >= 2 && os.Args[1] == "serve-forward" {
		os.Exit(serveForwardCommand(os.Args[2:]))
	}
	var input request
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		write(response{Error: "请求格式无效：" + err.Error()})
		return
	}
	if input.Protocol != protocol || input.Method != "run" {
		write(response{Error: fmt.Sprintf("不支持的 ALX 插件协议（protocol=%q method=%q）", input.Protocol, input.Method)})
		return
	}
	result, err := runAction(input.Action, input.Params)
	write(response{Output: result.Output, Data: result.Data, Error: errorText(err)})
}

func write(result response) { _ = json.NewEncoder(os.Stdout).Encode(result) }
func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func run(action string, params map[string]string) (string, error) {
	result, err := runAction(action, params)
	return result.Output, err
}

type actionResult struct {
	Output string
	Data   any
}

func runAction(action string, params map[string]string) (actionResult, error) {
	var output string
	var err error
	switch action {
	case "capabilities":
		data := capabilitySnapshot()
		return actionResult{Output: "已识别当前系统网络治理能力。", Data: data}, nil
	case "snapshot":
		data := networkSnapshot()
		return actionResult{Output: "已刷新网络快照。", Data: data}, nil
	case "diagnose":
		data := networkDiagnosis(params)
		return actionResult{Output: diagnosisSummary(data), Data: data}, nil
	case "plan":
		plan, planErr := createPlan(params)
		if planErr != nil {
			return actionResult{}, planErr
		}
		return actionResult{Output: "变更计划已生成，请确认差异后应用。", Data: plan}, nil
	case "apply-plan":
		return actionResult{}, fmt.Errorf("网络变更计划必须由 ALemonX 宿主批准后执行")
	case "apply-approved-plan":
		return applyApprovedPlan(params)
	case "undo-approved":
		return undoApproved(params)
	case "audit-list":
		return actionResult{Output: "权限审计由 ALemonX 宿主保存。"}, nil
	case "undo-last":
		return actionResult{}, fmt.Errorf("网络撤销必须经过系统授权后执行")
	case "network-check":
		output = networkCheck()
	case "mirror-check":
		output = mirrorCheck()
	case "set-npm-registry":
		output, err = setNPMRegistry(params)
	case "reset-npm-registry":
		output, err = resetNPMRegistry()
	case "port-check":
		port, err := portParam(params)
		if err != nil {
			return actionResult{}, err
		}
		output = portCheck(port)
	case "firewall-status":
		output = firewallStatus()
	case "open-port", "close-port":
		port, err := portParam(params)
		if err != nil {
			return actionResult{}, err
		}
		transport, err := protocolParam(params)
		if err != nil {
			return actionResult{}, err
		}
		output, err = changeFirewall(action, port, transport)
	case "iface-up", "iface-down":
		output, err = ifaceAction(action, params)
	case "ip-add", "ip-remove":
		output, err = ipAddress(action, params)
	case "dns-set":
		output, err = dnsSet(params)
	case "mtu-set":
		output, err = mtuSet(params)
	case "route-add", "route-remove":
		output, err = routeAction(action, params)
	case "traffic":
		output = trafficSnapshot()
	case "forward-list":
		output, err = forwardList()
	case "forward-add":
		output, err = forwardAdd(params)
	case "forward-remove":
		output, err = forwardRemove(params)
	case "virtual-list":
		output, err = virtualList()
	case "bond-create":
		output, err = bondCreate(params)
	case "bond-delete":
		output, err = virtualDelete("Bond", params)
	case "bridge-create":
		output, err = bridgeCreate(params)
	case "bridge-delete":
		output, err = virtualDelete("网桥", params)
	case "vlan-create":
		output, err = vlanCreate(params)
	case "vlan-delete":
		output, err = virtualDelete("VLAN", params)
	case "firewalld-zones":
		output, err = firewalldZones()
	case "firewalld-service-add":
		output, err = firewalldServiceAction("firewalld-service-add", params)
	case "firewalld-service-remove":
		output, err = firewalldServiceAction("firewalld-service-remove", params)
	case "firewalld-zone-set-default":
		output, err = firewalldSetDefaultZone(params)
	default:
		return actionResult{}, fmt.Errorf("未知操作：%s", action)
	}
	if err != nil {
		return actionResult{Output: output}, err
	}
	return actionResult{Output: output}, nil
}

func mirrorCheck() string {
	registry := strings.TrimSpace(commandOutput("npm", "config", "get", "registry"))
	unconfigured := false
	if registry == "" || strings.Contains(registry, "未找到命令") {
		registry = "https://registry.npmjs.org/"
		unconfigured = true
	}
	registryLine := "✓ npm 下载源：" + redactURL(registry)
	if unconfigured {
		registryLine = "? npm 下载源：未配置，使用官方默认 " + redactURL(registry)
	}
	lines := []string{registryLine, "✓ HTTP_PROXY：" + displayEnv("HTTP_PROXY"), "✓ HTTPS_PROXY：" + displayEnv("HTTPS_PROXY"), "✓ NO_PROXY：" + displayEnv("NO_PROXY")}
	parsed, err := url.Parse(registry)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return strings.Join(lines, "\n") + "\n? Registry 格式无效，未发起连接测试。"
	}
	client := &http.Client{Timeout: 6 * time.Second}
	response, err := client.Get(strings.TrimRight(parsed.String(), "/") + "/-/ping")
	if err != nil {
		return strings.Join(lines, "\n") + "\n! 镜像连通性：失败（" + err.Error() + "）"
	}
	_ = response.Body.Close()
	status := response.Status
	if response.StatusCode >= 200 && response.StatusCode < 400 {
		status = "可访问（" + response.Status + "）"
	} else {
		status = "异常（" + response.Status + "）"
	}
	return strings.Join(lines, "\n") + "\n✓ 镜像连通性：" + status
}

func setNPMRegistry(params map[string]string) (string, error) {
	registry := strings.TrimSpace(params["registry"])
	allowed := map[string]bool{
		"https://registry.npmjs.org/":     true,
		"https://registry.npmmirror.com/": true,
	}
	if !allowed[registry] {
		return "", fmt.Errorf("仅允许切换到插件内置的 npm 官方源或 npmmirror")
	}
	output, err := exec.Command("npm", "config", "set", "registry", registry).CombinedOutput()
	if err != nil {
		return strings.TrimSpace(string(output)), fmt.Errorf("无法设置 npm Registry：%w", err)
	}
	return "npm Registry 已切换为：" + registry, nil
}

func resetNPMRegistry() (string, error) {
	output, err := exec.Command("npm", "config", "delete", "registry").CombinedOutput()
	if err != nil {
		return strings.TrimSpace(string(output)), fmt.Errorf("无法恢复 npm 官方源：%w", err)
	}
	return "已删除自定义 Registry 设置；npm 将使用官方默认源。", nil
}

func portCheck(port int) string {
	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err == nil {
		_ = listener.Close()
		return fmt.Sprintf("✓ 端口 %d 当前未被 TCP 服务监听。", port)
	}
	return fmt.Sprintf("! 端口 %d 可能正在被 TCP 服务监听或被系统保留：%v", port, err)
}

func firewallStatus() string {
	switch runtime.GOOS {
	case "windows":
		return "✓ Windows 防火墙状态：\n" + commandOutput("netsh", "advfirewall", "show", "allprofiles")
	case "darwin":
		return "✓ macOS PF 防火墙状态：\n" + commandOutput("pfctl", "-s", "info")
	case "linux":
		if output := commandOutput("ufw", "status", "verbose"); !strings.Contains(output, "未找到命令") {
			return "✓ UFW 防火墙状态：\n" + output
		}
		if output := commandOutput("firewall-cmd", "--state"); !strings.Contains(output, "未找到命令") {
			return "✓ firewalld 状态：\n" + output
		}
		return "? 未检测到 UFW 或 firewalld。"
	default:
		return "? 当前系统暂不支持读取防火墙状态。"
	}
}

func changeFirewall(action string, port int, transport string) (string, error) {
	if runtime.GOOS == "darwin" {
		return "", fmt.Errorf("macOS 应用防火墙不支持按端口安全修改；请在系统设置或受管 PF 配置中处理")
	}
	name := fmt.Sprintf("ALemonX %d/%s", port, strings.ToUpper(transport))
	var command *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		if action == "open-port" {
			command = exec.Command("netsh", "advfirewall", "firewall", "add", "rule", "name="+name, "dir=in", "action=allow", "protocol="+strings.ToUpper(transport), "localport="+strconv.Itoa(port))
		} else {
			command = exec.Command("netsh", "advfirewall", "firewall", "delete", "rule", "name="+name, "dir=in", "protocol="+strings.ToUpper(transport), "localport="+strconv.Itoa(port))
		}
	case "linux":
		if _, err := exec.LookPath("ufw"); err != nil {
			return "", fmt.Errorf("未找到 UFW；为避免猜测防火墙工具，本插件不会修改系统规则")
		}
		if action == "open-port" {
			command = exec.Command("ufw", "allow", strconv.Itoa(port)+"/"+transport)
		} else {
			command = exec.Command("ufw", "delete", "allow", strconv.Itoa(port)+"/"+transport)
		}
	default:
		return "", fmt.Errorf("当前系统暂不支持修改防火墙规则")
	}
	output, err := command.CombinedOutput()
	if err != nil {
		return strings.TrimSpace(string(output)), fmt.Errorf("防火墙操作失败：%w；通常需要以管理员身份运行 ALemonX", err)
	}
	verb := map[string]string{"open-port": "已开放", "close-port": "已关闭"}[action]
	return fmt.Sprintf("%s %d/%s 入站端口规则。\n%s", verb, port, transport, strings.TrimSpace(string(output))), nil
}

func defaultRoute() string {
	switch runtime.GOOS {
	case "windows":
		return commandOutput("route", "print", "-4")
	case "darwin":
		return commandOutput("route", "-n", "get", "default")
	case "linux":
		return commandOutput("ip", "route", "show", "default")
	default:
		return "当前系统未知"
	}
}
func commandOutput(name string, args ...string) string {
	output, err := exec.Command(name, args...).CombinedOutput()
	text := strings.TrimSpace(string(output))
	if err != nil {
		if text == "" {
			return name + " 未找到命令或执行失败：" + err.Error()
		}
		return text
	}
	if text == "" {
		return "未返回信息"
	}
	return text
}
func displayEnv(key string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return redactURL(value)
	}
	return "未设置"
}

func redactURL(value string) string {
	parsed, err := url.Parse(value)
	if err != nil || parsed.User == nil {
		return value
	}
	username := parsed.User.Username()
	parsed.User = url.UserPassword(username, "******")
	return parsed.String()
}
