# OpenBao Go SDK 接入说明

**文档修订：2026-09-29 工程改进｜原 R3-D1 文档记录保留**

本文面向接入 SDK 的 Go 服务开发者，说明准备条件、依赖引用、认证、初始化、调用、退出和故障处理。它描述本包真实公开接口与当前实现，不是上线许可或已发布版本公告。

> **当前可用性边界**：正式 module 和现有许可已确认；主 Go 1.26.8、最低 Go 1.25.0、独立消费者、真实 OpenBao fixture 和固定扫描的当前证据见 [实施交接](implementation-handoff.md)。测试使用隔离临时资源，未发布本轮快照。本文代码不构成生产可用证明。
>
> 原验收不变：SDK 发布要求 AC-001～AC-071 满足相应门禁；业务接入另看 AC-072。文档增补不改变任务状态。

## 1. 阅读顺序与事实来源

首次操作先读 [首次使用指南](first-use-guide.md)，查方法时读 [功能手册](sdk-feature-manual.md)。完整公开签名由 [接口契约](spec/02-接口契约.md) 固定，并由 [公开接口测试](../public_contract_test.go)约束。当前源码与验收分别见根目录和 [acceptance-results.json](../acceptance-results.json)。

本文中“当前实现”来自源码；“调用方应当”是沿用原设计的集成要求；没有测试证据的条目会注明“待验证”。不把原设计目标自动写成已完成能力。

## 2. 接入边界

SDK 是一个 Go 库，不是独立 HTTP 服务。根包为 `bao`，提供 Client、KV、PKI、Transit 和诊断入口。业务项目负责加载配置、选择身份、授权调用、保存准确引用、事务与补偿。

| 放在 SDK | 留在调用项目 |
|---|---|
| OpenBao 请求、凭据快照、超时、错误分类 | 用户/租户/终端授权、业务路径命名 |
| KV v2 版本操作 | 业务数据结构与引用持久化 |
| PKI 签发和材料校验 | 终端认领、证书交付、MQTT/Webhook |
| Transit 密码学操作 | 待签字节协议、防重放、业务状态机 |

不要把 SDK 引入 Flutter、浏览器 H5 或小程序直接携带后台 Token 使用；在后端调用层集成。SDK 本身没有 Gin、Fiber、Gorm、数据库或 MQTT 依赖。

来源：[README](../README.md)、[业务接入与回滚](integration-and-rollback.md)。

## 3. 接入前准备

| 项目 | 必须明确的内容 | 当前包中的状态 |
|---|---|---|
| SDK 模块地址 | 真实代码仓库、已核验快照/版本 | `github.com/RockInMars/openbao-sdk-go` 已确认；本轮源码未推送或发布 |
| 官方依赖 | `github.com/openbao/openbao/api/v2@v2.7.0` 的完整模块及校验 | 已有模块缓存与 go.sum，固定不升级 |
| Go 工具链 | 声明下限与实际验证分开 | go.mod 1.25.0；主验证固定 1.26.8，最低版本与平台结果见兼容矩阵 |
| OpenBao 地址 | HTTPS origin，例如测试网关地址 | 不接受含 `/v1` 或其它路径前缀的 Address |
| Namespace | 明确 root，或确认存在的 named 路径 | 不根据 `xyouting-test` 名称猜测部署形态 |
| TLS 信任 | 系统信任，或受信 CA 文件/PEM | 私有 CA 必须明确配置；不关闭验证 |
| 认证材料 | 受限 Token 文件，或 AppRole | 不使用生产 Root Token 作示例 |
| 引擎挂载 | KV v2 / PKI / Transit 实际挂载名 | Client 获取功能对象不会创建或检查引擎 |
| 资源配置 | PKI Role、Transit 已有密钥和版本 | 管理端预先配置；运行时 SDK 不自动创建 |

来源：[go.mod](../go.mod)、[配置校验](../config.go)、[兼容性记录](compatibility.md)。

### 3.1 先验证源码快照

在 SDK 根目录运行；遇到失败先定位并修复，不删除检查：

```bash
python -B scripts/normal-test.py
```

依赖缓存不足时，先按授权单独准备并核对模块校验，再冻结输入。`dependency-check --prepare` 属于会更新模块的准备步骤，不能混入冻结后的验证；普通门禁默认不下载，不擅自升级工具链。

只能在另一台联网机器取得依赖时，使用 [公共依赖转移说明](dependency-transfer.md) 的 `make dependency-export` 和接收端 verify 流程。依赖 ZIP 不包含 OpenBao 服务端、Go 工具链或扫描器；消费者脚本仍有独立的离线限制。

### 3.2 正式模块引用

下面是维护者发布并核实目标版本后使用的命令模板；本轮没有创建发布版本，也未验证从远端取得当前未提交源码：

```bash
# 在调用项目根目录，按真实已发布值填写。
: "${SDK_MODULE:?设置实际 SDK 模块路径}"
: "${SDK_VERSION:?设置已经发布并核验的版本}"
GOWORK=off go get "${SDK_MODULE}@${SDK_VERSION}"
GOWORK=off go mod tidy
GOWORK=off go test ./...
```

module/import 和消费者 require 已同步为维护者确认的地址，保留 `RockInMars` 大小写。后续改变地址必须重新验证，不能沿用当前证据。

不将本地 `replace`、共享 go.work 或 `make contract-test` 的补充结果当作独立发布依赖可用的证据。正式独立消费者由 `make consumer-test` 验证。来源：[发布检查](release.md)、[Makefile](../Makefile)。

## 4. 选择认证与客户端范围

一个 `*bao.Client` 固定 **Address + Namespace + 身份**。同一服务可复用同一身份的 Client；不同空间或身份使用不同 Client，不按请求调用 `SetToken` / `SetNamespace`，当前公开 API 也没有这两个方法。

不要把客户端传入的 mount、path、keyName 或 Namespace 直接交给一个拥有广泛权限的后台身份执行。应用层授权不能由 SDK 路径校验替代。

### 4.1 外部 Token 模式

已有外部认证管理时使用 `auth.ExternalToken`。Token 的获取和续期由调用方负责，SDK 不为外部 Token 争抢续期。

| 提供者 | 构造方式 | 约束 |
|---|---|---|
| Token 文件 | `auth.NewTokenFile(path)` | 每次取快照读取文件；原子替换后读取新值；读取失败不无限复用旧值 |
| 静态 Token | `auth.NewStaticToken(token)` | 显式传入 `sensitive.Bytes`，不硬编码；不自动轮换或续期 |
| 自定义 | 实现 `auth.TokenProvider` | 返回快照、Generation、可选 ValidUntil；默认保留句柄所有权，可显式交付独立快照；遵守 Context，见[所有权说明](authentication.md#快照所有权) |

`NewTokenFile` 只检查路径字符串，实际文件内容在取快照时读取。受保护的常规文件及其父目录由部署侧管理，建议权限 `0600`；当前读取内容限制 64 KiB，trim 后必须为非空可打印 ASCII 凭据。ValidUntil 零值表示“未知”，不是永久有效。

来源：[auth/config.go](../auth/config.go)、[auth/providers.go](../auth/providers.go)、[认证说明](authentication.md)。

### 4.2 SDK 管理 AppRole 模式

以下是按现有接口组合的配置函数，不读取环境变量、不自动登录。`RoleID` 和 SecretID 应从可信配置通道获得，调用方负责其生命周期。

```go
package integration

import (
	bao "github.com/RockInMars/openbao-sdk-go"
	"github.com/RockInMars/openbao-sdk-go/auth"
	"github.com/RockInMars/openbao-sdk-go/sensitive"
)

func NewManagedClient(
	base bao.Config,
	authMount string,
	roleID sensitive.Bytes,
	secretIDFile string,
	use auth.SecretIDUse,
) (*bao.Client, error) {
	source, err := auth.NewSecretIDFile(secretIDFile, use)
	if err != nil {
		return nil, err
	}
	base.Auth = auth.Config{
		Mode: auth.ManagedAppRole,
		AppRole: &auth.AppRoleConfig{
			Mount:            authMount,
			RoleID:           roleID,
			SecretIDProvider: source,
		},
	}
	return bao.New(base)
}
```

`authMount` 填相对 `auth/` 的挂载，例如 `sdk-role`，不填 `/v1/auth/sdk-role`。`use` 显式选择 `auth.ReusableSecretID` 或 `auth.SingleUseSecretID`。

当前实现要求返回 Token 有正的 lease_duration；若返回 num_uses，则必须为 0。一次性 SecretID 的 Generation 在请求发起前登记；响应未知也不会再次消费同一 Generation。该内存记录不是跨进程持久幂等，进程重启仍需可信凭据管理和业务恢复。

续期/重新登录按实际租期进行。业务 `403` 不会触发重登并重放原业务写入。来源：[AppRole 实现](../internal/authn/approle.go)。

## 5. 最小接入：准确版本只读

**这是调用项目可以采用的完整函数示例，不是包中新增的运行时函数。**传入受信任的 `base` 配置与已授权的准确 `kv.Ref`；函数不会创建秘密或执行 CAS，也不会打印秘密正文。

```go
package integration

import (
	"context"
	"errors"
	"time"

	bao "github.com/RockInMars/openbao-sdk-go"
	"github.com/RockInMars/openbao-sdk-go/auth"
	"github.com/RockInMars/openbao-sdk-go/kv"
)

func WithExactSecret(
	serviceCtx context.Context,
	base bao.Config,
	tokenFile string,
	ref kv.Ref,
	consume func(context.Context, kv.Document) error,
) (err error) {
	if serviceCtx == nil || consume == nil {
		return errors.New("service context and secret consumer are required")
	}
	provider, err := auth.NewTokenFile(tokenFile)
	if err != nil {
		return err
	}
	base.Auth = auth.Config{Mode: auth.ExternalToken, TokenProvider: provider}
	client, err := bao.New(base)
	if err != nil {
		return err
	}
	defer func() {
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err = errors.Join(err, client.Close(shutdown))
	}()

	if err = client.Start(serviceCtx); err != nil {
		return err
	}
	store, err := client.KVv2(ref.Mount)
	if err != nil {
		return err
	}
	callCtx, cancel := context.WithTimeout(serviceCtx, 5*time.Second)
	defer cancel()
	result, err := store.ReadRef(callCtx, ref)
	if err != nil {
		return err
	}
	defer result.Data.Zero()
	// consume 必须遵守 Context；不保留秘密的别名、不记录原始内容。
	return consume(callCtx, result.Data)
}
```

base 至少配置 `Address`、`ClusterAlias`、`Namespace`；私有 CA 同时设置 TLS。Ref 的 `ClusterAlias`、`Namespace`、`Mount` 必须与 Client/功能对象匹配，Version 为正整数。root 模式下 Ref.Namespace 是空字符串，不是 `"root"`。

长驻服务在启动层创建一次 Client，再把窄接口注入业务层；上面函数便于说明首次读取，并不要求每个业务请求都创建客户端。来源：[Client](../client.go)、[KV 读取](../kv_client.go)。

## 6. 配置字段与默认值

下面是 R3 源码默认值，不是线上压测结果；零值通常使用默认值，负值非法。配置对象由调用方构造，SDK 不提供自动 YAML/环境加载器。

| 字段 | 输入规则或默认值 |
|---|---|
| `Address` | HTTPS origin；允许空路径或单个 `/`；不能带用户名密码、query、fragment、`/v1` 或网关路径前缀 |
| `ClusterAlias` | 必填稳定别名，使用合法单段路径字符 |
| `Namespace.Mode` / `Path` | `NamespaceRoot` + 空 Path，或 `NamespaceNamed` + 非空合法相对路径；零值不合法 |
| `TLS.CAFile` / `CAPEM` | 二选一；均不提供时采用系统根信任；提供自定义 CA 时使用自定义池，不自动叠加系统根 |
| `TLS.ServerName` | 可选，用于实际 TLS 校验，不是跳过主机名验证 |
| `TLS.ClientCertFile` / `ClientKeyFile` | 文件模式必须成对；用于双向 TLS，不替代 OpenBao 认证 |
| `TLS.ClientCertPEM` / `ClientKeyPEM` | 内存模式必须成对，不能与文件模式混用；私钥类型为 sensitive.Bytes |
| `TLS.MinVersion` | 默认 TLS 1.2；当前仅接受 TLS 1.2 / TLS 1.3 |
| `Network.ProxyURL` | 默认不使用环境代理；显式 HTTP/HTTPS 代理，无嵌入用户名密码、query、fragment |
| `Network.MaxIdleConnections` / `MaxIdlePerHost` | 64 / 32 |
| `Network.IdleConnTimeout` | 90 秒 |
| `Timeouts.Request` / `PKIIssue` | 10 秒 / 30 秒；后者同时用于 Issue 与 SignCSR |
| `Timeouts.Login` / `Renew` | 10 秒 / 5 秒 |
| `Timeouts.Dial` / `TLSHandshake` | 5 秒 / 5 秒 |
| `Limits.MaxConcurrentRequests` | 32，业务请求并发限制 |
| `Limits.MaxRequestBytes` | 1 MiB，最终编码后的请求也受限制 |
| `Limits.MaxResponseBytes` | 2 MiB，解压后的响应 |
| `Limits.MaxResponseHeaderBytes` | 64 KiB |
| `ReadRetry.MaxAttempts` | 默认 3，合法有效值 1～3；设 1 禁用 SDK 读取重试，设 0 是采用默认值 |
| `ReadRetry.BaseDelay` / `MaxDelay` | 100ms / 2 秒，BaseDelay 不得大于 MaxDelay |

路径单段仅接受 `A-Z a-z 0-9 _ - .`，拒绝空段、`.`、`..`、末尾点、百分号、反斜杠和其它字符。完整路径以 `/` 分段，无前导/尾随斜杠。PKI Role、Transit key name 是单段；mount 和 KV path 可以多段。`List("")` 是显式列出挂载根目录的例外。

`New` 不进行网络 I/O，但会读取配置的本地 TLS 文件并构造传输；“不联网”不等于“完全没有文件 I/O”。配置文件路径必须来自可信部署，不接受用户上传的任意本机路径。

来源：[配置类型](../config_types.go)、[配置校验](../config.go)、[TLS 实现](../network.go)、[路径规则](../internal/engine/path.go)。

## 7. 生命周期与停机

| 动作 | 语义 |
|---|---|
| `New(cfg, opts...)` | 本地校验和构造；不登录、不启用引擎 |
| `Start(serviceCtx)` | 校验凭据/登录并启动必要认证生命周期；保留服务 Context |
| 业务调用 | 每次接收请求 Context；在服务存活和操作预算内执行 |
| `State()` | 查看本地生命周期和认证状态；不是远端 ACL 成功证明 |
| `Close(shutdownCtx)` | 拒绝新请求、停止认证循环、等待/取消已准入请求，清理自有资源 |

不要在短暂的初始化函数中 `defer cancel()` 掉传给 Start 的 serviceCtx，再将 Client 返回给业务；那会结束此 Client 的服务生命周期。退出时使用新建的有限时 shutdownCtx，不复用已被取消的 HTTP 请求 Context。

Close 不撤销外部 Token、不删除 KV、不吊销证书。取消过的服务生命周期不能靠重复 Start 延长；要开始新的服务生命周期需创建新 Client。自定义 Provider/Observer 必须按约定及时返回，SDK 不承诺能强制中断任意阻塞回调。

来源：[client.go](../client.go)、[认证生命周期](authentication.md)。

## 8. 首次连通性与就绪判断

`service-bootstrap` 输出 started，只说明 Start 完成。外部 Token 的 Start 主要读取快照，不能据此判断远端地址、ACL 或引擎全部可用。

先以实际请求检验目标：ClusterHealth 是无 Token、无 Namespace 的集群请求；CheckReady 是当前身份对指定 KV 版本的读取。只能证明被探测资源的可读性，不能证明签发/签名权限。

```go
package integration

import (
	"context"
	"errors"

	bao "github.com/RockInMars/openbao-sdk-go"
	"github.com/RockInMars/openbao-sdk-go/diagnostics"
)

func RequireReadable(ctx context.Context, client *bao.Client, probe diagnostics.ReadProbe) error {
	if ctx == nil || client == nil {
		return errors.New("context and client are required")
	}
	status, err := client.CheckReady(ctx, probe)
	if err != nil {
		return err // 网络、协议、取消等：未得到正常的就绪结论。
	}
	if !status.Ready {
		return errors.New("configured KV probe is not ready")
	}
	return nil
}
```

已知的未就绪、认证、权限、资源不可见或版本不可用会返回 `Readiness{Ready:false, ErrorCode:...}` 且 `error=nil`；必须检查 Ready。用受限的非秘密探测记录，不要为就绪检查赋予管理员权限。

来源：[diagnostics_client.go](../diagnostics_client.go)。

## 9. 按最小权限接入

这里列出当前代码发送的相对 API 路径，供权限评审使用；不是已在真实环境验收的 ACL 模板。ACL 路径不包含 `/v1/` 或 HTTP host，也不把 Namespace 拼入普通引擎相对路径。

| 用途 | 当前调用所涉及的路径 | 范围控制 |
|---|---|---|
| 精确读取 KV | `<kv>/data/<path>` | 仅授权指定业务前缀的 read |
| KV 创建/CAS | `<kv>/data/<path>` | 按场景审核 create/update，不允许默认无条件覆盖 |
| 元数据/列表 | `<kv>/metadata/<path或prefix>` | read/list 与 data 权限分开 |
| 软删除/恢复 | `<kv>/delete/<path>`、`<kv>/undelete/<path>` | 维护操作，按需另配身份 |
| PKI | `<pki>/issue/<role>`、`sign/<role>`、`cert/<serial>`、`ca_chain`、`revoke` | 固定角色；吊销与普通读取分开 |
| Transit 公钥/元数据预检 | `<transit>/keys/<name>` | 签名、验签、加解密、HMAC 也需要此 read 权限 |
| Transit 密码学操作 | `<transit>/<action>/<name>` | 仅已有 key；不授予 keys create/update 或 export |
| HMAC | `<transit>/hmac/<name>/sha2-256`、`<transit>/verify/<name>/sha2-256` | 按源码完整路径核对，见下方差异 |
| AppRole 与续期 | `auth/<mount>/login`、`auth/token/renew-self` | 使用所属 Namespace 的认证配置 |

**已发现的模板差异**：当前 `transit_hmac.go` 验证请求带 `/sha2-256`，而 `deploy/test/policies/control.hcl` 和 `internal/testenv/policies.go` 的 mac verify 授权只有 `transit/verify/mac`。本次只记录该差异，不修改权限或宣称 HMAC 已集成通过；真实集成前必须核对并以失败测试修复对应代码/fixture，不能直接扩大为全部路径权限。

来源：[测试权限目录](../deploy/test/policies/)、[自动 fixture 权限](../internal/testenv/policies.go)、[Transit HMAC](../transit_hmac.go)。

## 10. 业务层错误处理

先检查**结果是否未知**，再按错误码决定业务动作。对于写请求，`DeadlineExceeded` 不等于服务端没执行；业务不应自动从头重做签发或写入。

```go
package integration

import (
	"context"
	"errors"

	"github.com/RockInMars/openbao-sdk-go/baoerr"
)

// ErrorAction 仅返回业务分类，不执行自动重试，也不改变原始 err。
func ErrorAction(err error) string {
	if err == nil {
		return "completed"
	}
	if baoerr.HasUnknownOutcome(err) {
		return "reconcile-before-any-replay"
	}
	if baoerr.IsCode(err, baoerr.CodeCASConflict) {
		return "reload-and-resolve-conflict"
	}
	if baoerr.IsCode(err, baoerr.CodePermissionDenied) {
		return "review-identity-and-permissions"
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "stop-current-call"
	}
	return "inspect-sanitized-error"
}
```

404 的 `not_found_or_hidden` 不能当成业务资源确定不存在；CAS 冲突不自动读最新后覆盖；准确版本缺失不回退 latest。只有在可信持久日志、operation_id、resource_generation 和内容核对支持时才能恢复业务步骤。

错误对象支持脱敏格式化，但不要读取并输出任意用户构造 Error.Message 的原始字段。业务生成的 errors.New 文本也必须避免拼接秘密。

来源：[错误说明](error-handling.md)、[baoerr](../baoerr/error.go)、[错误分类](../internal/engine/errors.go)。

## 11. 长驻服务、项目边界与回滚

在调用项目的启动/依赖注入层创建 Client，业务层只获得实际需要的窄接口。不要将完整 Client 作为 HTTP 入参处理器的万能 OpenBao 代理，不要 import SDK 的 `internal/` 或 `examples/internal/`。

在 saas-go 风格的独立 Go 服务中，可把配置映射与实例初始化放入 `internal/bootstrap`；将 KV Ref、凭据代次、业务唯一操作号的持久化留在业务 service/repository。此处是接入位置建议，不是已经修改 saas-go 的记录。

读迁移可以在隔离环境比较同一准确引用的旧新结果；发证和写入不能双写影子流量。回滚切换适配器，保留先前引用和历史版本，不删除新秘密、重建全部证书或替换为 latest。细节见 [业务接入与回滚](integration-and-rollback.md)。

## 12. 首次接入完成检查

| 检查 | 合格表现 |
|---|---|
| 依赖 | 完整官方模块、真实校验记录、固定工具链；没有 overlay/replace 旁路 |
| 配置 | Address、Namespace、身份、挂载与资源均显式、受信任 |
| 只读探测 | 目标准确版本实际可读；正确区分 Ready=false 和 error |
| 功能 | 实际所需 KV/PKI/Transit 真实调用及权限负例通过 |
| 秘密 | Token/私钥/明文不在日志、错误、指标标签、源码中出现 |
| 恢复 | 写结果未知、CAS 冲突、失效版本按业务协议处理 |
| 退出 | 请求取消、Client 关闭和外部凭据所有权符合预期 |
| 发布 | 正式测试、真实集成、独立消费者、扫描及验收门禁有当前证据 |

## 13. 常见接入问题

| 现象 | 首先检查 | 不应采取的做法 |
|---|---|---|
| `invalid_argument` 在 New | Namespace 零值、Address 带路径、配置互斥、CA 文件内容 | 关闭 TLS 或默认切 root |
| Start 成功但首次请求失败 | 真实连通性、Token ACL、mount 与版本；Start 不是远端全量探测 | 宣称 started 就可用 |
| `permission_denied` 在 Transit | 除签名/加密权限外，还要检查 keys read 的预检权限 | 自动授予管理员权限 |
| `cas_conflict` | 同路径是否已创建或已被并发更新 | 盲目重试覆盖 |
| `version_unavailable` | 版本保留、软删除、销毁、密钥最小版本策略 | 改读 latest |
| 超时且 `Effect=unknown` | 业务唯一操作号、已保存引用与实际结果 | 重新签发后当作同一次操作 |
| Token 文件更新不生效 | 实际文件、文件属主/权限、可信父目录、客户端身份范围 | 输出 Token 排查 |
| `json.Marshal` 秘密类型失败 | 应使用受控 Reveal/Decode，而不是普通序列化 | 保存 `[REDACTED]` 当秘密内容 |
| Client 返回 canceled | Start 的 serviceCtx 是否提前取消 | 对同一实例重复 Start 延长寿命 |
| 文档命令依赖失败 | 网络、固定工具链、真实模块及校验材料 | 伪造 go.sum 或本地假模块 |

## 14. 本次文档增补的边界

原 R3-D1 文档验证记录保留在 [历史记录](documentation-validation-R3-D1.json)；本轮 module、示例、测试和工具改动以 [实施交接](implementation-handoff.md)、原任务台账与逐项验收为准，历史记录不转记为当前 PASS。
