# 网络、端口转发与防火墙

[![CI](https://github.com/lemonade-lab/alemonx-network/actions/workflows/ci.yml/badge.svg)](https://github.com/lemonade-lab/alemonx-network/actions/workflows/ci.yml)

这是 ALemonX 的一个插件，帮你**管理电脑的网络**。装好后，在 ALemonX 里打开它，就能在一个简洁的界面上做下面这些事，不用记任何命令：

- **检查网络**：网卡、DNS、外网是否正常，一键查看。
- **管理网卡**：看网卡信息、启用/停用、手动设 IP、改 DNS。
- **端口转发**：让别人能通过你的电脑访问到另一台设备（比如家里那台服务器）。
- **防火墙**：查看/开放/关闭端口。
- **换 npm 下载源**：国内网络慢时切到国内镜像。

## 安装

1. 打开 [Releases 页面](https://github.com/lemonade-lab/alemonx-network/releases)，下载和你系统匹配的文件：
   - **Windows** → 下载 `alemonx-network-windows-amd64.zip`
   - **Mac（M 系列芯片）** → 下载 `alemonx-network-darwin-arm64.zip`
   - **Mac（Intel 芯片）** → 下载 `alemonx-network-darwin-amd64.zip`
   - **Linux** → 下载 `alemonx-network-linux-amd64.zip`
2. 解压，得到一个叫 `alemonx-network` 的文件夹。
3. 把整个文件夹放进 ALemonX 的插件目录。任选其一：
   - 你电脑的「用户配置」目录下 `alx/plugins/`：
     - Windows：`C:\Users\你的用户名\AppData\Roaming\alx\plugins\`
     - Mac：`~/Library/Application Support/alx/plugins/`
     - Linux：`~/.config/alx/plugins/`
4. 打开 ALemonX，点左侧「插件」，就能看到「网络、端口转发与防火墙」。点它，进界面。

> 只要文件夹被放对了位置，ALemonX 会在约 1 秒内自动认出它，不用重启程序。

## 开始使用

点开插件后，界面顶部有三个页签：

| 页签 | 能做什么 |
| --- | --- |
| **诊断** | 点「检查网络」看网络是否正常；「查看防火墙」「检查下载源」也是点一下就行。 |
| **端口转发** | 把本机一个端口收到的连接，转发到另一台设备的端口。点「刷新」看现有规则，点「添加」新建一条，填：本机端口、目标设备 IP、目标端口、协议。 |
| **防火墙** | 开放或关闭某个入站端口（填端口号和协议即可）。 |

改动系统设置的操作用前都会再弹窗确认一次，不用担心误点。

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
