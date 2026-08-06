package main

import (
	"context"
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
}
type response struct {
	Output string `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}

func main() {
	var input request
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		write(response{Error: "请求格式无效：" + err.Error()})
		return
	}
	if input.Protocol != protocol || input.Method != "run" {
		write(response{Error: fmt.Sprintf("不支持的 ALX Setup 插件协议（protocol=%q method=%q）", input.Protocol, input.Method)})
		return
	}
	output, err := run(input.Action, input.Params)
	write(response{Output: output, Error: errorText(err)})
}

func write(result response) { _ = json.NewEncoder(os.Stdout).Encode(result) }
func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func run(action string, params map[string]string) (string, error) {
	switch action {
	case "network-check":
		return networkCheck(), nil
	case "mirror-check":
		return mirrorCheck(), nil
	case "port-check":
		port, err := portParam(params)
		if err != nil {
			return "", err
		}
		return portCheck(port), nil
	case "firewall-status":
		return firewallStatus(), nil
	case "open-port", "close-port":
		port, err := portParam(params)
		if err != nil {
			return "", err
		}
		transport, err := protocolParam(params)
		if err != nil {
			return "", err
		}
		return changeFirewall(action, port, transport)
	default:
		return "", fmt.Errorf("未知操作：%s", action)
	}
}

func networkCheck() string {
	lines := []string{"系统：" + runtime.GOOS + "/" + runtime.GOARCH}
	interfaces, err := net.Interfaces()
	if err != nil {
		lines = append(lines, "网卡：读取失败（"+err.Error()+"）")
	} else {
		active := []string{}
		for _, item := range interfaces {
			if item.Flags&net.FlagUp == 0 || item.Flags&net.FlagLoopback != 0 {
				continue
			}
			addresses, _ := item.Addrs()
			for _, address := range addresses {
				if ip, _, splitErr := net.ParseCIDR(address.String()); splitErr == nil && !ip.IsLoopback() {
					active = append(active, item.Name+"="+ip.String())
				}
			}
		}
		if len(active) == 0 {
			lines = append(lines, "网卡：未找到已启用的非回环地址")
		} else {
			lines = append(lines, "网卡："+strings.Join(active, "，"))
		}
	}
	lines = append(lines, "默认路由："+defaultRoute())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	addresses, err := net.DefaultResolver.LookupHost(ctx, "registry.npmjs.org")
	if err != nil {
		lines = append(lines, "DNS：registry.npmjs.org 解析失败（"+err.Error()+"）")
	} else {
		lines = append(lines, "DNS：registry.npmjs.org → "+strings.Join(addresses, ", "))
	}
	connection, err := (&net.Dialer{}).DialContext(ctx, "tcp", "registry.npmjs.org:443")
	if err != nil {
		lines = append(lines, "HTTPS 连通性：失败（"+err.Error()+"）")
	} else {
		_ = connection.Close()
		lines = append(lines, "HTTPS 连通性：registry.npmjs.org:443 可连接")
	}
	return strings.Join(lines, "\n")
}

func mirrorCheck() string {
	registry := strings.TrimSpace(commandOutput("npm", "config", "get", "registry"))
	if registry == "" || strings.Contains(registry, "未找到命令") {
		registry = "https://registry.npmjs.org/（npm 未安装或未配置）"
	}
	lines := []string{"npm Registry：" + redactURL(registry), "HTTP_PROXY：" + displayEnv("HTTP_PROXY"), "HTTPS_PROXY：" + displayEnv("HTTPS_PROXY"), "NO_PROXY：" + displayEnv("NO_PROXY")}
	parsed, err := url.Parse(registry)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return strings.Join(lines, "\n") + "\nRegistry 格式无效，未发起连接测试。"
	}
	client := &http.Client{Timeout: 6 * time.Second}
	response, err := client.Get(strings.TrimRight(parsed.String(), "/") + "/-/ping")
	if err != nil {
		return strings.Join(lines, "\n") + "\n镜像连通性：失败（" + err.Error() + "）"
	}
	_ = response.Body.Close()
	return strings.Join(lines, "\n") + "\n镜像连通性：" + response.Status
}

func portCheck(port int) string {
	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err == nil {
		_ = listener.Close()
		return fmt.Sprintf("端口 %d 当前未被 TCP 服务监听。", port)
	}
	return fmt.Sprintf("端口 %d 可能正在被 TCP 服务监听或被系统保留：%v", port, err)
}

func firewallStatus() string {
	switch runtime.GOOS {
	case "windows":
		return commandOutput("netsh", "advfirewall", "show", "allprofiles")
	case "darwin":
		return commandOutput("pfctl", "-s", "info")
	case "linux":
		if output := commandOutput("ufw", "status", "verbose"); !strings.Contains(output, "未找到命令") {
			return output
		}
		return commandOutput("firewall-cmd", "--state")
	default:
		return "当前系统暂不支持读取防火墙状态。"
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

func portParam(params map[string]string) (int, error) {
	value := strings.TrimSpace(params["port"])
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("端口必须是 1 到 65535 的整数")
	}
	return port, nil
}
func protocolParam(params map[string]string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(params["protocol"]))
	if value == "" {
		value = "tcp"
	}
	if value != "tcp" && value != "udp" {
		return "", fmt.Errorf("协议仅支持 tcp 或 udp")
	}
	return value, nil
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
