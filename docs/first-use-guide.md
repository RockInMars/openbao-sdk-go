# OpenBao Go SDK 首次使用指南

**文档修订：R3-D1｜2026-09-29｜按现有示例实际输入编写**

目标是让接入者从环境准备开始，先理解客户端生命周期，再依次进行准确版本 KV 操作、Transit 签名和 PKI 签发保存。所有真实调用只面向已授权的隔离测试资源，不能使用生产 Root Token、生产秘密路径或真实设备证书。

> **先确认状态**：R3 尚未完成官方依赖和正式 SDK 验证，以下是恢复条件后执行的首次使用流程，不是本轮运行成功记录。`go run` 会执行网络请求，有的步骤会产生写入或证书签发。先阅读每步的副作用，再执行。

完整接入原理见 [接入说明](sdk-integration-guide.md)，方法参数见 [功能手册](sdk-feature-manual.md)。

## 1. 使用者和环境准备

下列命令使用包内 Makefile 对应的 Bash 环境。当前历史测试记录是 Linux/amd64，不宣称 Windows/macOS 已验证。Windows 接入需要准备等价的 Bash/Go/Python 执行环境，并在目标平台补验证。

| 需要的材料 | 谁准备 | 本文不会做什么 |
|---|---|---|
| 源码快照和可信依赖 | 开发者 | 不把源码 ZIP 当依赖 ZIP |
| 符合实际上游要求的固定 Go | 开发者/运维 | 不猜最低版本或自动升级其它项目 |
| 隔离 OpenBao、HTTPS 与 CA | 测试环境负责人 | 不自动连接生产地址 |
| 指定 Namespace、引擎与角色/密钥 | 测试环境负责人 | 不通过 SDK 创建 Namespace/引擎/根 CA/密钥 |
| 受限 Token 文件 | 凭据管理流程 | 不在命令行、日志或文档写入真实 Token |

来源：[兼容性](compatibility.md)、[测试环境说明](../deploy/test/README.md)、[示例配置层](../examples/internal/bootstrap/config.go)。

## 2. 第一次先处理依赖，不运行示例碰运气

在解压后的 SDK 根目录执行：

```bash
go version
make dependency-check
make normal-test
```

`make normal-test` 不通过时先修复并保留证据；不能通过 `make contract-test` 的补充模式跳过正式依赖要求。`go.mod` 中临时域名不是远端仓库，不能直接对它 `go get`。

需要从联网开发机转移依赖时执行：

```bash
make dependency-export
```

只有成功后才有 `.artifacts/openbao-public-dependencies.zip` 及 `.sha256`；接收端必须按 [依赖转移说明](dependency-transfer.md) 独立 verify。依赖恢复成功不代表服务端程序和扫描器也已经准备好。

## 3. 配置示例共用输入

以下地址、别名、文件路径和 Namespace 都是**占位示例**，执行前替换为真实测试配置。不要直接粘贴后把连不上占位地址当 SDK 缺陷。

```bash
# SDK_BAO_* 是 examples 的配置输入，不是 SDK 本体自动读取的变量。
export SDK_BAO_ADDRESS='https://bao-test.example.com:8200'
export SDK_BAO_CLUSTER='bao-test'
export SDK_BAO_NAMESPACE_MODE='named'
export SDK_BAO_NAMESPACE='sdk-demo'
export SDK_BAO_TOKEN_FILE='/absolute/test-secrets/runtime-token'
export SDK_BAO_CA_FILE='/absolute/test-trust/openbao-https-ca.pem'
```

使用已明确授权的 root Namespace 时改为：

```bash
export SDK_BAO_NAMESPACE_MODE='root'
unset SDK_BAO_NAMESPACE
```

HTTPS 证书已经由系统信任且无需自定义信任池时可以 unset SDK_BAO_CA_FILE，不是关闭证书验证。Named Namespace 不存在时不能自动改 root；`xyouting-test` 只有实际确认为 Namespace 后才能填入。

四个 main 示例均使用 `auth.NewTokenFile`，不是 AppRole。ManagedAppRole 的配置代码见 [接入说明第4节](sdk-integration-guide.md)。

### 不要混淆两套环境变量

`SDK_BAO_*` 是示例访问一个已准备好的**隔离测试实例**的输入；`BAO_TEST_*` 是 `make integration-test` **新建并拥有临时实例**的输入。后者拒绝外部 `BAO_TEST_ADDRESS/BAO_TEST_TOKEN`，管理员身份只用于 fixture。

来源：[示例配置](../examples/internal/bootstrap/config.go)、[测试环境](../deploy/test/README.md)。

## 4. 第一次启动与退出

```bash
GOWORK=off go run ./examples/service-bootstrap
```

源码在 Start 成功后打印：

```text
SDK service started; awaiting cancellation
```

随后保持运行，按 Ctrl+C 触发退出，以独立5秒关闭 Context 清理客户端。

**这一步不是完整连通性或权限测试**：外部 Token 的 Start 读取本地凭据快照，不会遍历所有远端资源。必须继续真实调用；只想先只读，可采用 [准确版本读取函数](sdk-integration-guide.md) 或其中的 CheckReady 例子，不需要先做写入。

来源：[service-bootstrap/main.go](../examples/service-bootstrap/main.go)、[bootstrap.Run](../examples/internal/bootstrap/config.go)、[Client.Start](../client.go)。

## 5. 第一次 KV 操作：创建 → 准确读取 → CAS

**副作用：产生一份测试秘密及新版本。**为每次独立演示使用受控、唯一、未用过的 path，Token 只对该测试前缀有权限。

```bash
export SDK_BAO_KV_MOUNT='kv'
# 路径为示例，需要选择本次未使用过且有授权的测试路径。
export SDK_BAO_SECRET_PATH='fixture/first-run-001'
GOWORK=off go run ./examples/kv-versioned
```

实际示例写入包含大整数 `9007199254740993` 的非生产演示文档，然后 ReadRef 和 CAS。输出模式是：

```text
Created version <n> and CAS version <n+1>; secret values omitted
```

这只是源码定义的预期输出形式，不是本次执行结果。CAS 演示写入相同文档，也会形成新版本。结果未知时先核对原路径，不盲目重复 Create；同一路径再次启动遇 cas_conflict 是保护行为，不要改成无条件覆盖。

首次应核对 `ReadResult.Ref` 的五个字段、Version 固定、秘密不出现在 stdout。历史引用保存完整 Ref，不能只存 path。演示本身不自动清理版本；需要清理时由授权测试流程使用显式 DeleteVersions，避免误删其它数据。

来源：[kv-versioned/main.go](../examples/kv-versioned/main.go)、[KV 实现](../kv_client.go)。

## 6. 第一次 Transit 签名与验签

**前提：密钥已存在，版本已确认，算法与 Profile 匹配。**除了 sign/verify 权限，还要有指定 `transit/keys/<name>` 的 read 权限。

```bash
export SDK_BAO_TRANSIT_MOUNT='transit'
export SDK_BAO_KEY_NAME='ecdsa'
export SDK_BAO_KEY_VERSION='1'
export SDK_BAO_SIGN_PROFILE='ecdsa-p256-sha256-asn1'
GOWORK=off go run ./examples/transit-sign
```

该示例对固定的非秘密演示消息执行签名、验签、准确版本公钥读取，不创建或轮换密钥。成功输出包含实际 key version/type，不输出消息或签名。

其它已有算法的 profile 必须使用精确字符串：`rsa-pss-sha256-saltlen-hash`、`ed25519-message`，同时切换到匹配类型的实际密钥。不要只改 profile 而继续使用不匹配的 key。

此示例不覆盖加解密或 HMAC；这些用法见 [功能手册](sdk-feature-manual.md)。HMAC 验证的当前测试权限后缀差异也在手册注明，未在本次文档中静默修复。

来源：[transit-sign/main.go](../examples/transit-sign/main.go)、[transit/types.go](../transit/types.go)。

## 7. 第一次 PKI 签发并保存

**副作用：签发真实测试证书和私钥，并保存一份 KV 秘密。**只对专用测试角色和唯一凭据代次运行一次。

```bash
export SDK_BAO_PKI_MOUNT='pki'
export SDK_BAO_PKI_ROLE='device'
export SDK_BAO_KV_MOUNT='kv'
export SDK_BAO_SECRET_PATH='fixture/credential-run-001'
export SDK_BAO_GENERATION='credential-run-001'
export SDK_BAO_OPERATION_ID='prepare-run-001'
export SDK_BAO_CERT_CN='terminal-test.example.com'
export SDK_BAO_ISSUER_TRUST_FILE='/absolute/test-trust/device-root-ca.pem'
GOWORK=off go run ./examples/pki-issue-store
```

`SDK_BAO_ISSUER_TRUST_FILE` 是业务证书校验信任根，不是 `SDK_BAO_CA_FILE` 的简单别名；两者分别验证设备证书与 HTTPS 服务。角色必须允许演示 CN、客户端用途，以及申请10分钟有效期；示例要求至少剩余5分钟。实际允许值由测试角色决定。

代码顺序是：Issue → ValidateBundle → KV.Create → ReadRef → 核对操作号与代次。任一步失败都不说明前面没执行。特别是 Issue 已成功、KV 保存失败时，**不要重跑整个命令当作保存重试**。

当前一次性 main 没有持久任务日志，不能跨进程恢复；保存字段与回读校验也只是示例。请先阅读 [功能手册的 PKI 存储说明](sdk-feature-manual.md)，生产编排要由调用项目实现持久唯一占位、完整材料存储、准确引用和未知结果恢复。

来源：[pki-issue-store/main.go](../examples/pki-issue-store/main.go)、[业务恢复示例](../examples/credentialworkflow/workflow.go)。

## 8. 真实集成测试不能由示例成功替代

示例只验证所选调用路径；完整测试需要新建临时 OpenBao。按 [测试说明](../deploy/test/README.md) 选择经过来源核验的程序或固定 digest 镜像，明确 BAO_TEST_VERSION，运行：

```bash
make integration-test
make consumer-test
make tooling-test
make fuzz-test
make security-test
make release-check
```

不要在不同资源之间复用管理员凭据；不要把环境缺失导致测试未运行说成无失败；不要把 HMAC/PKI 演示路径之外的能力自动标成通过。原验收和实际业务迁移分别记录。

## 9. 首次结果记录模板

把以下内容记录到调用项目的测试记录中，不粘贴 Token、SecretID、私钥或秘密正文：

| 记录项 | 填写内容 |
|---|---|
| 源码 | SDK 归档 SHA-256 / 版本 / 提交标识 |
| 依赖 | 实际官方模块版本、Go 工具链、go.sum 与预检结果 |
| 服务 | 经核验的 OpenBao 版本，程序校验和或镜像 digest |
| 范围 | 测试集群别名、Namespace、受限身份用途、挂载 |
| 调用 | 具体示例、命令、退出码、允许的非秘密结果 |
| 数据 | 测试资源路径/准确版本，必要的受控引用 |
| 失败 | 错误码、Effect、是否已发生前置步骤、恢复决定 |
| 清理 | 哪些测试资源保留，哪些由授权流程清理 |
| 结论 | 实际验证了什么，哪些能力未运行 |

## 10. 本次文档检查范围

本文按 R3 的四个 main 和配置层逐个核对环境变量与输出形式，检查相对链接及命令目标。没有运行这些真实示例，没有请求生产或测试 OpenBao，没有改变 OB-001 或 AC-001～AC-072 状态。
