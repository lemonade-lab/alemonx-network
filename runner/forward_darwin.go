//go:build darwin

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// macOS has no simple system-level port mapping, so forward rules run as
// detached userland TCP forwarder processes. Rules are persisted in the user
// config directory; processes are spawned with a new process group so killing
// the alx parent never strands the forwarder.

func userlandForwardList() (string, error) {
	entries, err := loadForwardState()
	if err != nil {
		return "", err
	}
	lines := []string{"✓ macOS 端口映射规则（用户态 TCP 转发器）："}
	if len(entries) == 0 {
		lines = append(lines, "? 暂无端口转发规则")
	}
	for _, entry := range entries {
		state := "运行中"
		if !processAlive(entry.PID) {
			state = "已停止"
		}
		lines = append(lines, fmt.Sprintf("  %s:%d → %s:%d（%s，PID %d，%s）",
			entry.ListenAddress, entry.ListenPort, entry.TargetIP, entry.TargetPort,
			entry.Protocol, entry.PID, state))
	}
	lines = append(lines, firewallHint)
	return strings.Join(lines, "\n"), nil
}

func userlandForwardAdd(params map[string]string) (string, error) {
	listenPort, err := listenPortParam(params)
	if err != nil {
		return "", err
	}
	protocol, err := protocolParam(params)
	if err != nil {
		return "", err
	}
	if protocol != "tcp" {
		return "", errors.New("macOS 用户态转发仅支持 TCP；UDP 端口转发当前不支持")
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
		listenAddr = "0.0.0.0"
	} else if _, err = ipParam(listenAddr, "监听地址"); err != nil {
		return "", err
	}
	probe, err := net.Listen("tcp", net.JoinHostPort(listenAddr, strconv.Itoa(listenPort)))
	if err != nil {
		return "", fmt.Errorf("监听端口 %d 已被占用：%v", listenPort, err)
	}
	_ = probe.Close()

	rule := ForwardRule{
		ID:            "fwd-" + strconv.FormatInt(time.Now().UnixNano(), 36),
		ListenAddress: listenAddr,
		ListenPort:    listenPort,
		Protocol:      protocol,
		TargetIP:      targetIP,
		TargetPort:    targetPort,
		CreatedAt:     time.Now().Format(time.RFC3339),
	}
	configPath, err := forwardConfigPath(rule.ID)
	if err != nil {
		return "", err
	}
	logPath, err := forwardLogPath(rule.ID)
	if err != nil {
		return "", err
	}
	if err := writeForwardConfig(configPath, rule); err != nil {
		return "", err
	}
	pid, err := spawnDetachedForwarder(configPath, logPath)
	if err != nil {
		_ = os.Remove(configPath)
		return "", fmt.Errorf("无法启动转发器：%w", err)
	}
	rule.PID = pid
	entries, err := loadForwardState()
	if err != nil {
		return "", err
	}
	entries = append(entries, rule)
	if err := saveForwardState(entries); err != nil {
		return "", err
	}
	time.Sleep(150 * time.Millisecond)
	status := "运行中"
	if !processAlive(pid) {
		status = "已启动但进程未存活（请查看日志 " + logPath + "）"
	}
	return fmt.Sprintf("已添加端口映射：%s:%d → %s:%d（TCP，PID %d，%s）。\n%s",
		listenAddr, listenPort, targetIP, targetPort, pid, status, firewallHint), nil
}

func userlandForwardRemove(params map[string]string) (string, error) {
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
	entries, err := loadForwardState()
	if err != nil {
		return "", err
	}
	remaining := make([]ForwardRule, 0, len(entries))
	removed := false
	for _, entry := range entries {
		if entry.ListenPort == listenPort && entry.TargetIP == targetIP && entry.TargetPort == targetPort {
			stopForwarder(entry.PID)
			if configPath, pathErr := forwardConfigPath(entry.ID); pathErr == nil {
				_ = os.Remove(configPath)
			}
			if logPath, pathErr := forwardLogPath(entry.ID); pathErr == nil {
				_ = os.Remove(logPath)
			}
			removed = true
			continue
		}
		remaining = append(remaining, entry)
	}
	if !removed {
		return "", fmt.Errorf("未找到匹配 %d → %s:%d 的端口映射", listenPort, targetIP, targetPort)
	}
	if err := saveForwardState(remaining); err != nil {
		return "", err
	}
	return fmt.Sprintf("已移除端口映射：%d → %s:%d。", listenPort, targetIP, targetPort), nil
}

func forwardStateDir() (string, error) {
	config, err := userConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(config, "alx-network"), nil
}

func forwardConfigPath(id string) (string, error) {
	dir, err := forwardStateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "forward-"+id+".json"), nil
}

func forwardLogPath(id string) (string, error) {
	dir, err := forwardStateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "forward-"+id+".log"), nil
}

func writeForwardConfig(path string, rule ForwardRule) error {
	config := proxyConfig{
		ListenAddress: rule.ListenAddress,
		ListenPort:    rule.ListenPort,
		TargetIP:      rule.TargetIP,
		TargetPort:    rule.TargetPort,
	}
	data, err := json.Marshal(config)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func spawnDetachedForwarder(configPath, logPath string) (int, error) {
	executable, err := os.Executable()
	if err != nil {
		return 0, err
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return 0, err
	}
	command := exec.Command(executable, "serve-forward", "--config", configPath)
	command.Stdin = nil
	command.Stdout = logFile
	command.Stderr = logFile
	// New process group keeps the forwarder alive if the alx parent dies.
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := command.Start(); err != nil {
		_ = logFile.Close()
		return 0, err
	}
	pid := command.Process.Pid
	_ = command.Process.Release()
	return pid, nil
}

// processAlive reports whether a pid exists. A signal-0 probe is a snapshot,
// not a guarantee; the caller may also re-check the bound port.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return process.Signal(syscall.Signal(0)) == nil
}

func stopForwarder(pid int) {
	if pid <= 0 {
		return
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	_ = process.Signal(syscall.SIGTERM)
	time.Sleep(500 * time.Millisecond)
	_ = process.Signal(syscall.SIGKILL)
}

// serveForwardCommand is the detached forwarder entry point.
func serveForwardCommand(args []string) int {
	config := ""
	for index := 0; index < len(args); index++ {
		if args[index] == "--config" && index+1 < len(args) {
			config = args[index+1]
			index++
		}
	}
	if config == "" {
		fmt.Fprintln(os.Stderr, "serve-forward 需要 --config <path>")
		return 2
	}
	if err := serveForwardFromConfig(config); err != nil {
		fmt.Fprintln(os.Stderr, "转发器退出："+err.Error())
		return 1
	}
	return 0
}
