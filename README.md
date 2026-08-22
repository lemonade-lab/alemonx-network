# 网络、端口转发与防火墙

[![CI](https://github.com/lemonade-lab/alemonx-network/actions/workflows/ci.yml/badge.svg)](https://github.com/lemonade-lab/alemonx-network/actions/workflows/ci.yml)

这是 ALemonX 的一个插件，帮你**治理电脑的网络**。装好后，在 ALemonX 里打开它，就能在一个统一控制台中观察网络状态、预演变更并安全执行，不用记任何命令：

- **网络概览**：网卡、默认路由、流量、能力与近期变更一屏查看。
- **连接与接口**：查看并管理网卡、IP、DNS、MTU、路由；Linux 支持 VLAN、网桥与 Bond。
- **服务发布**：把端口转发作为受控服务发布，先预演影响，再申请授权。
- **安全策略**：查看防火墙开关状态，开放/关闭端口；Windows 与 UFW 环境还提供防火墙总开关。
- **分层诊断与审计**：按 DNS、TCP、路由定位问题，保留本机变更记录并支持可用的撤销操作。

## 安装

1. 打开 [Releases 页面](https://github.com/lemonade-lab/alemonx-network/releases)，下载和你系统匹配的文件：
   - **Windows** → 下载 `alemonx-network-windows-amd64.zip`
   - **Mac（M 系列芯片）** → 下载 `alemonx-network-darwin-arm64.zip`
   - **Mac（Intel 芯片）** → 下载 `alemonx-network-darwin-amd64.zip`
   - **Linux** → 下载 `alemonx-network-linux-amd64.zip`
2. 解压，得到一个叫 `alemonx-network` 的文件夹。
3. 把整个文件夹放进当前 ALemonX 工作区的 `plugins/` 目录，即
   `<workspace>/plugins/alemonx-network/`。工作区由启动参数 `--workspace`
   或 `ALX_WORKSPACE` 决定；未设置时使用 ALemonX 默认工作区。通过应用内
   插件页安装时会自动放到此处。
4. 打开 ALemonX，点左侧「插件」，就能看到「网络、端口转发与防火墙」。点它，进界面。

> 只要文件夹被放对了位置，ALemonX 会在约 1 秒内自动认出它，不用重启程序。

端口转发的配置、日志和状态会保存在
`<workspace>/store/alemonx-network/`，因此挂载整个工作区运行 Docker 时可随
容器重启保留。首次使用新目录时会复制旧版用户配置中的网络插件数据，原目录不
会被删除。

## 开始使用

点开插件后，界面包含五个模块：

| 页签 | 能做什么 |
| --- | --- |
| **概览** | 查看网络健康、默认路由、平台能力、风险提示和近期变更。 |
| **连接与接口** | 管理接口、IP、DNS、MTU 与路由；Linux 显示虚拟网络能力。 |
| **服务与流量** | 查看流量、现有转发规则并预演服务发布。 |
| **安全策略** | 查看防火墙状态，并按最小范围预演端口规则。 |
| **诊断与历史** | 分层检查 DNS/TCP/路由，查看审计历史并撤销支持的最近变更。 |

改动系统设置时会先生成有效期十分钟的计划，展示风险、影响和验证项目；确认后才会调用系统原生管理员授权。应用前如果网络状态已变化，计划会被拒绝并要求重新预演。

## 平台限制

- **端口转发**：Windows / Linux / Mac 都支持。
- **开放/关闭端口**：Windows / Linux 支持；**Mac 不支持**（系统限制，会提示你）。
- **网卡、IP、DNS、路由管理**：三个系统都支持，个别能力视系统而定。

## 给开发者

- 插件逻辑在 `runner/`（Go），界面在 `frontend/`（React + Vite + Tailwind，视觉与 ALemonX 一致）。
- 常用命令：`make check`（测试）、`make web`（构建界面）、`make build`（构建各平台二进制）、`make dist`（打包成发布 zip）。
- 技术细节见 [开发文档](https://github.com/lemonade-lab/alemonjs.dev/blob/main/docs/plugin-development.md)。

## 发布

维护者打标签推送，CI 会自动构建并发布：

```bash
git tag v0.6.0
git push origin v0.6.0
```

## 安全

- 所有修改系统的操作都只调用固定的系统命令，输入会先校验，绝不执行你输入的任意命令。
- 只在你明确操作时才会改动系统，动手前都会二次确认。
