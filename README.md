# ALemonX 网络、端口转发与防火墙系统插件

这是一个 Go 实现的 ALemonX 系统插件，提供接近 Cockpit「网络」模块的本机网络管理能力：

- **网络接口**：接口详情（状态 / MTU / MAC / IP / 速率）、接口启停、静态 IP 添加/移除、DNS、MTU、静态路由、接口流量快照；
- **端口转发**：把本机端口收到的入站连接映射转发到**目标 IP 的端口**（端口映射 / DNAT），并列出/移除规则；
- **虚拟网络（仅 Linux）**：链路聚合（Bond）、网桥（Bridge）、VLAN 的创建 / 删除 / 列出；
- **防火墙**：端口占用探测、防火墙状态查询、开放/关闭入站端口、firewalld 区域与服务管理（仅 Linux）；
- **镜像与代理**：npm Registry / 代理状态、镜像可达性、切换官方源或 npmmirror。

所有修改系统、网络或防火墙的动作都只调用固定的系统命令，输入在插件内二次校验，端口限制为 `1..65535`，协议仅允许 `tcp` 或 `udp`。

## 端口映射到指定 IP

| 平台 | 后端 | 说明 |
| --- | --- | --- |
| Windows | `netsh interface portproxy` | 系统级、可持久；仅 TCP；监听地址留空时自动取本机非回环 IPv4 |
| Linux | `firewalld`（回退 `iptables` DNAT） | firewalld 持久化（Cockpit 同款）；iptables 为非持久化并提示开启 IP 转发 |
| macOS | 用户态 TCP 转发器 | 插件 detach 自身进程；无需 root；仅 TCP；规则持久化于用户配置目录 |

跨平台统一动作：`forward-list` / `forward-add` / `forward-remove`。添加时不会隐式修改防火墙，如入站仍被拦截会在输出中提示先开放对应端口。

macOS 用户态转发器的规则存于 `~/.config/alx-network/`（Windows 为 `%APPDATA%`，macOS 为 `~/Library/Application Support/`），移除规则时会终止对应进程。

## 开发

ALemonX 识别的清单是 `alx.json`。在此仓库根目录启动 ALemonX 时，插件会通过 Go 开发执行器直接运行：

```bash
go test ./runner/...
go vet ./runner/...
printf '{"protocol":"alx/v1","method":"run","action":"network-check"}' | go run ./runner
```

发布时构建对应平台二进制并放入 `dist/`：

```bash
GOOS=linux GOARCH=amd64 go build -o dist/alemonx-network-linux-amd64 ./runner
GOOS=windows GOARCH=amd64 go build -o dist/alemonx-network-windows-amd64.exe ./runner
GOOS=darwin GOARCH=arm64 go build -o dist/alemonx-network-darwin-arm64 ./runner
GOOS=darwin GOARCH=amd64 go build -o dist/alemonx-network-darwin-amd64 ./runner
```

## 安全与平台边界

- 所有输入参数均在执行器内校验（`net.ParseIP` / `net.ParseCIDR` / 端口 / MTU / 协议），一律 `exec.Command` 参数数组，绝不拼接 shell 字符串。
- 所有修改类动作在清单中声明 `confirm: true`，UI / CLI / MCP 三端统一二次确认。
- macOS 应用防火墙不支持按端口修改，`open-port` / `close-port` 在 macOS 上会明确拒绝；端口映射则改用用户态转发器，不触碰 PF 或应用防火墙。
- Linux 接口/DNS/路由命令依赖 `ip`（iproute2）、`resolvectl` 等工具；缺失时优雅降级提示，不猜测改写 `/etc/resolv.conf` 等系统文件。
- 平台不支持的能力一律返回明确提示，不影响其他动作。
