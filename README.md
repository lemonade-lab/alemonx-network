# ALemonX 网络、镜像与防火墙系统插件

这是一个 Go 实现的 ALemonX Setup 系统插件，提供：

- 网络接口、默认路由、DNS 和 npm Registry 连通性诊断；
- npm Registry / HTTP(S) 代理状态和镜像可达性检查；
- 本机 TCP 端口占用探测；
- 防火墙状态查询；
- 在用户二次确认后，开放或关闭指定端口。

所有修改防火墙的动作只调用固定的系统命令，端口限制为 `1..65535`，协议仅允许 `tcp` 或 `udp`。Linux 仅支持 UFW；macOS 不会尝试修改 PF 或应用防火墙规则。

## 开发

ALemonX 识别的清单是 `alx.json`。在此仓库根目录启动 ALemonX 时，插件会通过 Go 开发执行器直接运行：

```bash
go test ./runner/...
go run ./runner/main.go <<'EOF'
{"protocol":"alx/v1","method":"run","action":"network-check"}
EOF
```

发布时构建对应平台二进制并放入 `dist/`：

```bash
GOOS=linux GOARCH=amd64 go build -o dist/alemonx-network-linux-amd64 ./runner
GOOS=windows GOARCH=amd64 go build -o dist/alemonx-network-windows-amd64.exe ./runner
GOOS=darwin GOARCH=arm64 go build -o dist/alemonx-network-darwin-arm64 ./runner
GOOS=darwin GOARCH=amd64 go build -o dist/alemonx-network-darwin-amd64 ./runner
```
