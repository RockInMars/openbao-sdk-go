# OB-001 真实依赖核验与恢复入口（R2）

## 当前事实

R2 在隔离目录恢复 R1，没有修改任何 `.go`、`go.mod` 或 `go.sum`。实际 Go 工具链仍是 `go1.23.2 linux/amd64`，不是已确认的 api/v2 最低版本。

实际 `go mod download -json github.com/openbao/openbao/api/v2@v2.7.0` 返回 1；模块代理与 GitHub 的 DNS 解析失败。保留 TLS 验证的公共 HTTPS DNS 探测也连接失败。Codex 注册环境列表为空。浏览器能看到包文档及部分精确 tag 的源文件，但没有取得完整模块、go.mod 或校验和；这些浏览器资料不能用于制造一个“可编译的本地官方模块”。

## 新入口

在能够访问模块代理、checksum database 的隔离 SDK 目录中执行：

```bash
make dependency-check
```

该目标运行 `scripts/dependency-check.py --prepare`，在当前 SDK 中完成：

1. 记录实际 Go 版本、GOOS/GOARCH；检查根 `go.mod` 保持精确候选 pin、无 replace。
2. 执行真实 `go mod download -json github.com/openbao/openbao/api/v2@v2.7.0`。
3. 必须实际取得 `GoMod`、`Dir`、`Zip`、`Sum`、`GoModSum`；记录文件 SHA-256，读取真实模块的 `go` 指令，不猜最低版本。
4. 仅在步骤 2～3 成功后，执行 `go mod tidy`；检查没有改变候选 pin，没有引入 replace。
5. 下载依赖图中的模块、执行 `go list -m -json all`、拒绝替换/解析错误、执行 `go mod verify`；核对 `go.sum` 内的两条直接依赖记录。
6. 写入 `.artifacts/dependency-report.json`，任何命令、材料或检查失败均返回非零状态。

这是 **OFFICIAL_DEPENDENCY_PREFLIGHT**，通过也只代表此依赖子检查，不是 OB-001 完整通过，更不是 SDK 编译、服务器兼容或发布通过。原19项任务依赖和72项验收不变。

`--prepare` 允许 Go 在本 SDK 内更新 go.mod/go.sum，执行后需要审查差异；它不会更改其他项目、创建远端发布或更新任务状态。需要不执行 tidy 时，可运行 `python3 scripts/dependency-check.py`；空 go.sum 仍会失败，不会被补成猜测值。

为避免得到被替换或跳过校验的证据，子进程设置 `GOENV=off`、`GOWORK=off`、清空 GOFLAGS 和公有模块的 checksum 例外、强制 `GOSUMDB=sum.golang.org`。保留显式 `GOPROXY` 以支持可信代理或已验证离线代理；普通 SDK 运行时配置没有因此改变。

工具链固定为当前 PATH 上的 Go，`GOTOOLCHAIN=local` 禁止自动追踪工具链。如果实际依赖要求更高版本，先根据它的真实 go.mod 核实并安装一个经过来源校验的固定版本，再通过 PATH 切换本 SDK 的工具链；不要擅自升级消费项目。

## 成功后的后续命令

```bash
make normal-test
make integration-test
make consumer-test
make security-test
make release-check
```

每一步均按原方案处理真实失败、记录当前源码证据。真实集成仍需要 `deploy/test/README.md` 中指定的受校验程序或固定 digest 镜像；不能连接生产或复用现有业务凭据。

## 当前缺失的执行条件

本会话没有能够下载完整公有 Go 模块的执行网络。重复在相同条件下发送“继续”，不会产生新的完整模块。解除这个阻塞需要可联网的隔离开发环境，或仅包含本 SDK 所需公开依赖、原始模块包、真实 go.mod/go.sum 与来源校验记录的依赖材料；不要上传整个个人 module cache、生产 Token 或私钥。

当前8个新增Python测试仅验证预检解析与拒绝逻辑。测试中人为构造的模块元数据和 h1 字符串明确属于单测 fixture，不会写入真实 go.sum，也不作为上游校验通过的证据。
