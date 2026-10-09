# OpenBao Go SDK 功能手册与调用示例

**文档修订：2026-09-29 工程改进｜不是软件发布版本**

本文是运行时接口手册，面向需要查询“做什么、怎么调用、返回什么、失败时怎么办”的开发者。安装与启动见 [接入说明](sdk-integration-guide.md)，按顺序试用见 [首次使用指南](first-use-guide.md)。

> **状态说明**：“有实现”、本地验证、真实 OpenBao 验收和发布是不同结论。本轮已同步正式 module 地址，当前逐项状态及证据见 [验收结果](../acceptance-results.json) 与 [实施交接](implementation-handoff.md)。R3 的 8 PASS / 59 PARTIAL / 5 NOT_RUN 保留为历史；本文代码组合不新增 SDK API。

## 1. 功能目录和证据入口

| 模块 | 内容 | 实现入口 | 原验收范围 |
|---|---|---|---|
| Core | 配置、生命周期、固定身份和挂载 | `config.go`、`client.go`、`options.go` | AC-001～002、008～026 |
| Auth | 外部 Token、AppRole、续期和重新登录 | `auth/`、`internal/authn/` | AC-024～034 |
| KV v2 | 9 个业务方法、Document、版本引用 | `kv_client.go`、`kv_metadata.go`、`kv/` | AC-003～007、035～042 |
| PKI | 5 个网络方法、2 个本地校验函数 | `pki_client.go`、`pki_read.go`、`pki/` | AC-043～050 |
| Transit | 11 个网络方法，含元数据预检 | `transit_*.go`、`transit/` | AC-051～058 |
| 诊断 | 集群健康、准确 KV 版本就绪探测 | `diagnostics_client.go` | AC-059～060 |
| 观察 | 脱敏事件钩子 | `observe/`、`client.go` | AC-061 |
| 工程 | 并发/模糊/真实集成/消费者/发布 | `tests/`、`scripts/`、`Makefile` | AC-062～072 |

准确状态逐项读取 [acceptance-results.json](../acceptance-results.json)，原要求见 [验收矩阵](spec/04-验收矩阵.md)。接口签名依据 [公开契约测试](../public_contract_test.go) 和 [原接口契约](spec/02-接口契约.md)，输入限制按本文列出的实现文件说明。

## 2. 通用调用约定

所有网络方法接收非 nil Context；先创建 Client 并 Start，ClusterHealth 是可在 Start 前调用的例外。获取 KVv2/PKI/Transit 对象只校验 mount，不联网，也不创建资源。

返回 `(*Result, error)` 的方法应先判断 error：只有 error 为 nil 才使用完整结果。失败不会通过“结果加错误”夹带私钥。SDK 的 Err 是业务层恢复的输入，不提供跨 OpenBao 与数据库的事务。

只读操作按白名单最多 3 次 SDK 总尝试，写入、签发、认证和密码学 POST 默认单次。Transit 的一个高层调用可能先做一次可重试的 keys 读取，再发送一次密码学 POST；“密码学不重试”不代表总共只有一个 HTTP 请求。整个组合共享操作预算。

来源：[执行器](../internal/engine/client.go)、[操作注册表](../internal/engine/operations.go)、[Transit 预检](../transit_keys.go)。

## 3. Core 与认证

### 3.1 Client 入口

| 方法 | 输入 | 返回 | 关键行为 |
|---|---|---|---|
| `bao.New(cfg, opts...)` | `bao.Config`、`...bao.Option` | `*bao.Client, error` | 配置/TLS 本地校验；不登录；可能读本地 TLS 文件 |
| `client.Start(ctx)` | 服务生命周期 Context | `error` | 凭据准备、AppRole 登录和必要续期任务；并发启动合并 |
| `client.Close(ctx)` | 独立关闭预算 Context | `error` | 幂等；停止新请求并等待/取消在途调用；不撤销外部资源 |
| `client.State()` | 无 | `diagnostics.ClientState` | 本地状态，不是远端 ACL 验证 |
| `client.KVv2(mount)` | 合法相对挂载路径 | `*bao.KVClient, error` | 绑定 KV v2 |
| `client.PKI(mount)` | 合法相对挂载路径 | `*bao.PKIClient, error` | 绑定 PKI |
| `client.Transit(mount)` | 合法相对挂载路径 | `*bao.TransitClient, error` | 绑定 Transit |
| `bao.WithObserver(observer)` | 非 nil `observe.Observer` | `bao.Option` | 仅 SDK 定义的可选参数，无 RawClient 逃生入口 |

`ClientState`：`Lifecycle`、`AuthState`、`Ready`、`TokenExpiresAt`、`LastErrorCode`。不要只比较 Lifecycle 为 READY 而忽略 Ready 或实际请求结果。

来源：[client.go](../client.go)、[options.go](../options.go)、[diagnostics/types.go](../diagnostics/types.go)。

### 3.2 认证提供者

| 函数/接口 | 输入与返回 | 适用范围 |
|---|---|---|
| `auth.NewStaticToken(token)` | `sensitive.Bytes` → `auth.TokenProvider, error` | 可信来源已取得的固定 Token；不续期 |
| `auth.NewTokenFile(path)` | `string` → `auth.TokenProvider, error` | 读取受保护的常规文件；可信原子替换 |
| `auth.NewSecretIDFile(path, use)` | `string, auth.SecretIDUse` → `auth.SecretIDProvider, error` | ManagedAppRole 的 SecretID 来源 |
| `TokenProvider.Snapshot(ctx)` | → `auth.TokenSnapshot, error` | 自定义外部身份提供器 |
| `SecretIDProvider.Current(ctx)` | → `auth.SecretIDSnapshot, error` | 自定义可更新 SecretID 来源 |

TokenSnapshot 字段为 `Token`、`Generation`、`ValidUntil`；SecretIDSnapshot 字段为 `SecretID`、`Generation`、`Use`。Generation 不是秘密内容。内建 provider 的独立快照交付消费者清理；自定义 provider 默认保留句柄所有权，SDK 不直接 Zero，只有显式所有权协议允许转移。共享与包装场景、直接调用和错误返回的责任见[快照所有权](authentication.md#快照所有权)。调用必须响应 Context，异常不得带出 Token 或文件内容。

`auth.Config` 的 ExternalToken 与 ManagedAppRole 互斥；两种模式不得同时配置。ManagedAppRole 中 RoleID 是 `sensitive.Bytes`，Mount 相对 auth/，SecretID 的复用模式必须明确。SDK 没有公开 `Login()`、`RenewToken()` 或 `SetToken()` 方法，认证由 Start 和内部生命周期管理。

来源：[auth/config.go](../auth/config.go)、[auth/providers.go](../auth/providers.go)、[认证说明](authentication.md)。

## 4. KV v2：秘密数据与版本

逐个方法的用途、完整字段、版本与 CAS 规则、精确 ACL 路径、Document 所有权和组合示例见 [KV v2 全部公开方法详解](kv-methods.md)。

### 4.1 九个公开方法

以下 `ctx` 为 `context.Context`，路径相对 KV 挂载，不能自行拼 `/data/` 前缀。

| 方法 | 参数 | 返回 | 使用规则 |
|---|---|---|---|
| `Create` | `ctx, path string, data kv.Document` | `*kv.WriteResult, error` | 发送 cas=0，仅创建；路径已存在时不覆盖 |
| `CompareAndSwap` | `ctx, path string, expected int, data kv.Document` | `*kv.WriteResult, error` | expected 为正整数且非 MaxInt；返回版本必须为 expected+1 |
| `ReadVersion` | `ctx, path string, version int` | `*kv.ReadResult, error` | version>0；返回版本必须匹配，无 latest 回退 |
| `ReadLatest` | `ctx, path string` | `*kv.ReadResult, error` | 明确表达要读最新；不要用于固定历史凭据引用 |
| `ReadRef` | `ctx, ref kv.Ref` | `*kv.ReadResult, error` | 集群、Namespace、挂载先匹配，再准确读取 |
| `ReadMetadata` | `ctx, path string` | `*kv.Metadata, error` | 获取当前键级配置和各版本状态，不读取秘密正文 |
| `List` | `ctx, prefix string` | `*kv.ListResult, error` | 空前缀表示根；非空不能尾随 `/`；不是递归列表 |
| `DeleteVersions` | `ctx, path string, versions []int` | `error` | 非空、正整数、不重复；软删除，不是 destroy |
| `UndeleteVersions` | `ctx, path string, versions []int` | `error` | 恢复明确版本；不能恢复已永久销毁的数据 |

`WriteResult`：`Ref, CreatedAt, RequestID, Attempts`。`ReadResult` 在这些字段外增加 `Data kv.Document`。RequestID 来自受控响应，Attempts 是该次 SDK 操作的尝试次数，不证明下游代理没有重放。

`kv.Ref` 的 JSON 字段：`cluster_alias, namespace, mount, path, version`。它不包含秘密正文，适合业务持久化，但路径本身也可能透露业务信息，应限制访问。Ref 并不是永不失效的引用，版本保留策略仍需运维保证。

来源：[kv_client.go](../kv_client.go)、[kv_metadata.go](../kv_metadata.go)、[kv/types.go](../kv/types.go)。

### 4.2 Document 的正确使用

| 方法 | 功能 | 注意 |
|---|---|---|
| `kv.ParseDocument(raw []byte)` | 校验并保留原始 JSON 对象 | 重复键、非法根、超深结构、有损 Unicode 转义会被拒绝 |
| `kv.NewDocument(value any)` | 先 JSON 编码再校验 | 不会恢复调用方在 float64 中已经丢失的数字精度 |
| `doc.Decode(dst any)` | 解码到业务结构；启用 UseNumber | 传入有效指针，目标字段类型仍决定精度/范围 |
| `doc.RevealJSON()` | 返回显式明文 JSON 副本 | 不打印、不直接发日志；用毕清理 |
| `doc.Zero()` | 尽力清理所持有的秘密 | 已解码字符串/副本和运行时复制不受完全保证 |

普通 `json.Marshal(doc)` 会返回错误，不会序列化出秘密。格式化输出为脱敏内容。值复制共享清理句柄，所以不能在一个副本 Zero 后继续依赖另一个副本。

以下文档示例用 Create→ReadRef→CAS 展示版本关系。它产生两次真实写入，不可在生产使用演示路径，也不能在失败后盲目重新运行整个函数。示例将大整数保存为字符串，以保留经过真实后端往返后的精度；本地 Document 对 JSON 数字的精度保证不代表后端具有相同保证，详见[兼容性边界](compatibility.md)。

```go
package integration

import (
	"context"
	"errors"

	bao "github.com/RockInMars/openbao-sdk-go"
	"github.com/RockInMars/openbao-sdk-go/kv"
)

func CreateReadUpdate(ctx context.Context, store *bao.KVClient, path string) (kv.Ref, error) {
	if ctx == nil || store == nil {
		return kv.Ref{}, errors.New("context and store are required")
	}
	initial, err := kv.ParseDocument([]byte(`{"schema_version":1,"large_integer":"9007199254740993"}`))
	if err != nil {
		return kv.Ref{}, err
	}
	defer initial.Zero()
	created, err := store.Create(ctx, path, initial)
	if err != nil {
		return kv.Ref{}, err
	}
	read, err := store.ReadRef(ctx, created.Ref)
	if err != nil {
		return kv.Ref{}, err
	}
	defer read.Data.Zero()
	updated, err := kv.ParseDocument([]byte(`{"schema_version":2,"large_integer":"9007199254740993"}`))
	if err != nil {
		return kv.Ref{}, err
	}
	defer updated.Zero()
	saved, err := store.CompareAndSwap(ctx, path, read.Ref.Version, updated)
	if err != nil {
		return kv.Ref{}, err
	}
	return saved.Ref, nil // 调用方持久化新的准确引用，而不是只保存 path。
}
```

来源：[Document](../kv/document.go)、[JSON 校验](../internal/jsondoc/json.go)、[Unicode 校验](../internal/jsondoc/unicode.go)。

### 4.3 元数据、删除和恢复

`Metadata` 包含 `CurrentVersion`、`OldestVersion`、`MaxVersions`、`CASRequired`、`DeleteVersionAfter`、`CustomMetadata`、按版本升序排列的 `Versions`、`RequestID`。每个版本包含 `Version, CreatedAt, DeletedAt, Destroyed`。

`DeletedAt` 保存原始 deletion_time，可能是未来自动删除截止时间；非空不等于已经删除。当前读取实现只在销毁或删除时间已到时拒绝该版本。CustomMetadata 是键级可变信息，不能当作某个历史版本的不可变业务事实。

List 的条目为 `Name` 和 `IsFolder`，Name 不保留尾随斜杠；404 不自动解释为空列表。删除/恢复返回 error 而非完整结果，业务需要确认目标版本状态时应再次做明确读取或元数据核对。

## 5. PKI：证书签发与校验

逐个方法的用途、全部字段、CSR 与私钥所有权、独立信任链、EKU、吊销边界和组合示例见 [PKI 全部公开方法详解](pki-methods.md)。

### 5.1 五个网络方法

| 方法 | 参数 | 返回 | 关键边界 |
|---|---|---|---|
| `Issue` | `ctx, pki.IssueRequest` | `*pki.IssuedCertificate, error` | 返回证书和私钥；不写 KV，不保存业务状态 |
| `SignCSR` | `ctx, pki.SignCSRRequest` | `*pki.SignedCertificate, error` | 校验 CSR 自签名及返回公钥匹配；不生成调用方私钥 |
| `ReadCertificate` | `ctx, serial string` | `*pki.CertificateRecord, error` | 读取证书和可用吊销信息，不恢复私钥 |
| `ReadIssuerChain` | `ctx` | `*pki.ChainResult, error` | 当前默认签发者链，不是历史证书原始链 |
| `Revoke` | `ctx, serial string` | `*pki.RevokeResult, error` | 要求明确吊销回执；不调用 Broker 断开设备 |

IssueRequest 字段：

| 字段 | 语义 |
|---|---|
| `Role string` | 必填合法单段角色名，角色已预置 |
| `CommonName string` | 可空；CN 与各 SAN 不能同时全部为空 |
| `DNSNames []string` | DNS SAN 请求值 |
| `EmailNames []string` | 邮箱 SAN 请求值 |
| `IPAddresses []netip.Addr` | 合法 IP 地址；不带 zone |
| `URISANs []string` | 有 scheme 的 URI SAN |
| `TTL time.Duration` | 必须 >0；实际有效期以返回证书为准 |
| `ExcludeCNFromSANs bool` | 传给签发接口的 CN/SAN 控制 |

SignCSRRequest 只有 `Role, CSRPEM sensitive.Bytes, TTL`。不要为它编造 `KeyType`、`KeyBits` 或自动保存字段。当前 Issue 请求固定 `format=pem, private_key_format=pkcs8`，算法由服务端角色决定。

来源：[PKI 请求实现](../pki_client.go)、[PKI 查询实现](../pki_read.go)、[pki/types.go](../pki/types.go)。

### 5.2 返回类型

`pki.Certificate` 包含：`CertificatePEM`、`IssuingCAPEM`、`CAChainPEM`、`SerialNumber`、`NotBefore`、`NotAfter`、`FingerprintSHA256`。IssuedCertificate 外层字段为 `Certificate`、`PrivateKey sensitive.Bytes`、`RequestID`；SignedCertificate 没有 PrivateKey。

CertificateRecord 字段为 `CertificatePEM, SerialNumber, NotBefore, NotAfter, RevokedAt, RequestID`。RevokedAt 为 nil 仅表示没有对应返回证据，不独立保证证书当前未吊销。

ChainResult 字段是 `CertificatesPEM`（与 Certificate.CAChainPEM 名称不同）；RevokeResult 是 `RevokedAt, RequestID`。

### 5.3 本地校验

| 函数 | 输入 | 返回 |
|---|---|---|
| `pki.ValidateCSR(raw)` | `[]byte` PEM | `error` |
| `pki.ValidateBundle(bundle, policy)` | `pki.IssuedCertificate, pki.VerifyPolicy` | `error` |

VerifyPolicy 中 TrustedRootsPEM 必须由调用方提供且非空，RequiredEKUs 必须明确且非空；它还支持 CurrentTime、ExpectedCN、ExpectedDNS、ExpectedURI、MinRemainingTTL。

当前策略对象没有 ExpectedIP 或 ExpectedEmail 字段；不能称它提供任意 SAN 的通用策略检查。Issue 会核对请求的 IP/Email 等名称，SignCSR 主要校验公钥对应；接收外部证书时更完整的业务身份匹配要由调用方按协议补足。ExpectedDNS 是预期名称存在检查，不能当作通配符主机名匹配器。

```go
package integration

import (
	"context"
	"crypto/x509"
	"errors"
	"time"

	bao "github.com/RockInMars/openbao-sdk-go"
	"github.com/RockInMars/openbao-sdk-go/pki"
)

func IssueForClient(ctx context.Context, issuer *bao.PKIClient, role, cn string, roots [][]byte) (*pki.IssuedCertificate, error) {
	if ctx == nil || issuer == nil {
		return nil, errors.New("context and issuer are required")
	}
	issued, err := issuer.Issue(ctx, pki.IssueRequest{
		Role: role, CommonName: cn, TTL: 10 * time.Minute,
	})
	if err != nil {
		return nil, err
	}
	err = pki.ValidateBundle(*issued, pki.VerifyPolicy{
		TrustedRootsPEM: roots,
		RequiredEKUs:    []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		ExpectedCN:      cn,
		MinRemainingTTL: 5 * time.Minute,
	})
	if err != nil {
		issued.PrivateKey.Zero()
		// 签发已经发生；此校验错误不是“服务端从未签发”的证明。
		return nil, err
	}
	// 所有权交给调用方：保存或交付完成后必须清理 PrivateKey。
	return issued, nil
}
```

该组合函数遇到 ValidateBundle 失败只返回本地校验错误；它不是 SDK 的 Issue 结果。业务任务必须记录已经发生过签发，不能因为这个本地错误没有 unknown 标记就重新发证。SDK 不提供跨步骤事务。

完整单次签发与保存示例见 [pki-issue-store](../examples/pki-issue-store/main.go)，持久恢复边界见 [credentialworkflow](../examples/credentialworkflow/workflow.go)。当前示例用 map 编码，`[][]byte` 的 ca_chain_pem 在 JSON 中是 Base64 字符串数组，不是字面 PEM 数组；示例也未把独立 IssuingCAPEM 字段写入文档，回读只核对操作号与代次。业务落地前要明确自己的存储 schema、所需证书材料和回读校验，不能把这个演示当作完整跨进程恢复实现。

## 6. Transit：签名、加密和 HMAC

逐个方法的用途、完整字段、版本与派生规则、精确 ACL 路径和组合示例见 [Transit 全部公开方法详解](transit-methods.md)。

### 6.1 版本化接口

| 方法 | 请求类型或参数 | 返回 |
|---|---|---|
| `Sign` | `ctx, transit.SignRequest` | `*transit.SignResult, error` |
| `SignDigest` | `ctx, transit.SignDigestRequest` | `*transit.SignResult, error` |
| `Verify` | `ctx, transit.VerifyRequest` | `*transit.VerifyResult, error` |
| `VerifyDigest` | `ctx, transit.VerifyDigestRequest` | `*transit.VerifyResult, error` |
| `ReadKeyMetadata` | `ctx, name string` | `*transit.KeyMetadata, error` |
| `ReadPublicKey` | `ctx, name string, version int` | `*transit.PublicKeyResult, error` |
| `Encrypt` | `ctx, transit.EncryptRequest` | `*transit.CipherResult, error` |
| `Decrypt` | `ctx, transit.DecryptRequest` | `*transit.DecryptResult, error` |
| `Rewrap` | `ctx, transit.RewrapRequest` | `*transit.CipherResult, error` |
| `HMAC` | `ctx, transit.HMACRequest` | `*transit.HMACResult, error` |
| `HMACVerify` | `ctx, transit.HMACVerifyRequest` | `*transit.VerifyResult, error` |

key name 是合法单段，签名/加密/HMAC 的版本必须明确为正整数；验签要传 ExpectedVersion，Decrypt 使用密文中的版本。SDK 没有 CreateKey、Rotate、ExportPrivateKey 接口。

每个操作会读取 keys 元数据或公钥，不缓存这些读取；ACL 必须同时允许目标 keys read。返回 `KeyRef` 字段为 `ClusterAlias, Namespace, Mount, Name, Version`，不是 KV Ref。

来源：[Transit 签名](../transit_client.go)、[密钥预检](../transit_keys.go)、[加解密](../transit_cipher.go)、[HMAC](../transit_hmac.go)。

### 6.2 签名配置和输入

| Go 常量 | 实际字符串 | 支持消息 | 支持摘要 |
|---|---|---|---|
| `transit.ECDSAP256SHA256ASN1` | `ecdsa-p256-sha256-asn1` | 是 | SHA-256 的 32 字节原始摘要 |
| `transit.RSAPSSSHA256` | `rsa-pss-sha256-saltlen-hash` | 是 | SHA-256 的 32 字节原始摘要 |
| `transit.Ed25519Message` | `ed25519-message` | 是 | 否 |

消息/摘要放入 sensitive.Bytes；SDK 处理 Base64，不要先把十六进制或 Base64 文本当摘要传入。Sign 返回前对签名做本地公钥验证，且返回版本必须与请求相同。

SignRequest 字段：`KeyName, KeyVersion, Profile, Message`。SignDigestRequest 将 Message 替换为 Digest。VerifyRequest 字段：`KeyName, ExpectedVersion, Profile, Message, Signature`；VerifyDigestRequest 使用 Digest。Signature 为 `Wrapped, Version, Profile`，三个字段要一致，不能只填写 Wrapped。

```go
package integration

import (
	"context"
	"errors"

	bao "github.com/RockInMars/openbao-sdk-go"
	"github.com/RockInMars/openbao-sdk-go/sensitive"
	"github.com/RockInMars/openbao-sdk-go/transit"
)

func SignAndVerify(ctx context.Context, tr *bao.TransitClient, name string, version int, payload []byte) (transit.Signature, error) {
	if ctx == nil || tr == nil {
		return transit.Signature{}, errors.New("context and transit client are required")
	}
	msg := sensitive.NewBytes(payload)
	defer msg.Zero()
	profile := transit.ECDSAP256SHA256ASN1
	signed, err := tr.Sign(ctx, transit.SignRequest{
		KeyName: name, KeyVersion: version, Profile: profile, Message: msg,
	})
	if err != nil {
		return transit.Signature{}, err
	}
	verified, err := tr.Verify(ctx, transit.VerifyRequest{
		KeyName: name, ExpectedVersion: version, Profile: profile,
		Message: msg, Signature: signed.Signature,
	})
	if err != nil {
		return transit.Signature{}, err
	}
	if !verified.Valid {
		return transit.Signature{}, errors.New("signature did not verify")
	}
	return signed.Signature, nil
}
```

验签 `Valid=false, error=nil` 是正常的否定结果；网络失败是 error，不可吞成 false。业务负责待签字节的确定性序列化、域标识、唯一编号和防重放；SDK 不定义 MQTT 或控制命令协议。

公钥返回 `Key, KeyType, SPKIDER, PEM, RequestID`，SPKIDER 统一为 SubjectPublicKeyInfo DER。元数据返回 Name、Type、LatestVersion、MinEncryptionVersion、MinDecryptionVersion、Exportable、DeletionAllowed、SupportsSigning、SupportsEncryption、VersionNumbers、RequestID。读取这些字段不会修改任何密钥配置。

### 6.3 加解密与重新加密

当前代码接受 `aes128-gcm96`、`aes256-gcm96`；不接受收敛加密。非 derived key 的 Context 必须为空，derived key 的 Context 必须非空，并在加密/解密/重新加密时保持正确上下文。这里的 Context 是派生字节，不是 Go 的请求 Context。

| 请求 | 字段 |
|---|---|
| `EncryptRequest` | KeyName、KeyVersion、Plaintext、Context |
| `DecryptRequest` | KeyName、Ciphertext、Context |
| `RewrapRequest` | KeyName、TargetVersion、Ciphertext、Context |

Ciphertext 是 `Wrapped, Version`；SDK 会核对包装内的版本与字段一致。CipherResult 包含 Ciphertext 和 RequestID；DecryptResult 包含敏感 Plaintext 和 RequestID。

```go
package integration

import (
	"context"
	"errors"

	bao "github.com/RockInMars/openbao-sdk-go"
	"github.com/RockInMars/openbao-sdk-go/sensitive"
	"github.com/RockInMars/openbao-sdk-go/transit"
)

func EncryptAndRead(ctx context.Context, tr *bao.TransitClient, name string, version int, plain []byte, consume func([]byte) error) error {
	if ctx == nil || tr == nil || consume == nil {
		return errors.New("context, transit client and consumer are required")
	}
	value := sensitive.NewBytes(plain)
	defer value.Zero()
	encrypted, err := tr.Encrypt(ctx, transit.EncryptRequest{
		KeyName: name, KeyVersion: version, Plaintext: value,
		// 本示例仅用于预先创建的非 derived AES-GCM key。
	})
	if err != nil {
		return err
	}
	decrypted, err := tr.Decrypt(ctx, transit.DecryptRequest{
		KeyName: name, Ciphertext: encrypted.Ciphertext,
	})
	if err != nil {
		return err
	}
	defer decrypted.Plaintext.Zero()
	result := decrypted.Plaintext.RevealCopy()
	defer clear(result)
	return consume(result) // 不记录明文；consume 不保留切片别名。
}
```

Rewrap 只改变密文对应的加密版本，不向业务返回明文，也不等于密钥轮换。TargetVersion 必须已存在且符合最小加密版本，原密文版本需仍可解密。业务只在拿到新密文并持久化成功后改变引用；遇结果未知不盲重放。

### 6.4 HMAC

当前固定 SHA-256。HMACRequest 为 `KeyName, KeyVersion, Message`，返回 `Wrapped, Version, RequestID`。HMACVerifyRequest 为 `KeyName, ExpectedVersion, Message, WrappedHMAC`，返回 VerifyResult。

当前 HMAC 请求类型没有派生 Context 字段，代码拒绝 derived key；返回 MAC 解包后要求32字节。服务端元数据未提供历史版本列表时，只允许使用明确的 latest 版本，不猜测旧版本可用。验证请求与 control fixture 均使用精确的 `transit/verify/mac/sha2-256` 权限；真实集成结果以当前正式报告为准。来源：[transit_hmac.go](../transit_hmac.go)、[control 权限](../deploy/test/policies/control.hcl)、[集成诊断](evidence/OB-018/release-validation-2026-10-01/integration-diagnosis.md)。

## 7. Diagnostics 与 Observe

### 7.1 健康与就绪

| 方法 | 参数 | 返回 | 解释 |
|---|---|---|---|
| `ClusterHealth` | `ctx` | `*diagnostics.Health, error` | 无 Token/Namespace 的集群请求，可在 Start 前调用 |
| `CheckReady` | `ctx, diagnostics.ReadProbe` | `*diagnostics.Readiness, error` | 当前身份读取指定挂载、路径、版本，不写入 |

Health 字段为 Initialized、Sealed、Standby、HTTPStatus、Version、ClusterID。健康端点中的 429/472/473/501/503 会按专用状态解析，error=nil 不保证 Initialized=true 或 Sealed=false，需读取字段。它不是“业务接口限流”的统一判断。

ReadProbe 字段为 Mount、Path、Version（必须>0）。Readiness 字段为 Ready、ErrorCode。已知认证/权限/版本等否定结果可以 Ready=false、error=nil；协议/网络/取消仍返回 error。CheckReady 读取完会清理秘密内容，不向调用方返回探测文档。

来源：[诊断实现](../diagnostics_client.go)、[健康状态处理](../internal/engine/client.go)。

### 7.2 脱敏观察钩子

Observer 只有 `Observe(ctx context.Context, event observe.Event)`。Event 字段为 Operation、ClusterAlias、MountLabel、Duration、Attempts、HTTPStatus、ErrorCode、RequestID，没有秘密正文。

回调同步执行，应迅速返回；SDK恢复 panic，不因此把成功写入变成失败。但无限阻塞回调仍会阻塞调用者，不能认为 Context 到期会自动杀死回调。需要异步指标时由业务提供有界、非阻塞投递，不递归调用同一 Client。

指标标签只用有限的 operation/error 等维度，不加入完整秘密路径、每个终端编号、证书序列号或 Token。来源：[observe/observer.go](../observe/observer.go)、[client.go](../client.go)、[观察说明](observability.md)。

## 8. Sensitive 与错误字典

### 8.1 sensitive.Bytes

公开入口为 `sensitive.NewBytes(raw)`；提供 `RevealCopy()`、`Len()`、`Zero()` 以及脱敏格式化。构造和揭示做复制，Zero 是尽力清理共享句柄。原始入参切片、解码后的字符串、网络库复制和调用方保留的副本仍需单独管理，不能承诺物理内存完全擦除。

源码：[sensitive/bytes.go](../sensitive/bytes.go)。

### 8.2 稳定错误码

| Code | 意义 | 业务侧处理 |
|---|---|---|
| `invalid_argument` | 参数、配置或本地校验错误 | 修正输入；组合工作流中另判断之前步骤是否已发生 |
| `authentication_failed` | 没有可用认证或服务端明确认证拒绝 | 检查凭据来源，勿自动重放原写入 |
| `permission_denied` | 明确权限拒绝 | 检查身份和精确资源授权 |
| `not_found_or_hidden` | 不存在或不可见 | 不推断资源肯定未创建 |
| `version_unavailable` | 目标版本缺失、不可读或策略限制 | 核对历史引用；不回退最新 |
| `cas_conflict` | 已识别的 CAS 冲突 | 业务协调并发，不无条件覆盖 |
| `canceled` | 调用取消 | 结束当前调用，同时检查 Effect |
| `deadline_exceeded` | 超过操作截止时间 | 不把超时当未执行 |
| `unavailable` | 传输或服务暂不可用等 | 按操作语义处理，不能统一重试写入 |
| `invalid_response` | 响应不满足协议或关键字段约束 | 写操作可能已经执行，保留未知结果 |
| `response_too_large` | 响应超过限制 | 排查响应和配置，不关闭所有限制 |
| `redirect_blocked` | 重定向被禁止 | 使用正确固定入口，不绕过身份安全边界 |
| `not_ready` | 客户端或凭据未就绪 | 核对生命周期 |
| `closed` | 已关闭或正在关闭 | 不继续使用此 Client |

辅助函数：`baoerr.IsCode(err, code)`、`baoerr.HasUnknownOutcome(err)`。`errors.Is(err, context.Canceled/DeadlineExceeded)` 可用。不要按错误字符串包含关系判断。

Effect 独立于 Code：`none` 表示当前单次操作可以确认无效果，`unknown` 表示不能确定，`confirmed` 是契约保留的确认值。当前网络实现主要产生 none/unknown；成功通常是 result + nil，不需要期待 Error.EffectConfirmed。

来源：[baoerr/error.go](../baoerr/error.go)、[响应分类](../internal/engine/errors.go)。

## 9. 当前不提供的能力

不提供引擎启用、Namespace 创建、ACL 修改、根 CA 创建、PKI Role 管理、Transit 创建/轮换/私钥导出/删除、KV 永久销毁、OpenBao 初始化/解封、业务数据库事务、MQTT/Webhook 或终端注册。

签发私钥不自动存入 KV；读取证书不能恢复私钥；签名私钥不交付给终端；没有默认 latest 回退、自动扩大权限、任意 URL/RawClient 入口。需要这些业务动作时按原架构由管理工具或调用项目负责，不能为了首次运行就隐式加入高权限操作。

来源：[边界测试](../public_contract_test.go)、[总体设计](spec/01-总体设计.md)。

## 10. 工程入口与已知限制

| 命令 | 用途 | 不能证明什么 |
|---|---|---|
| `make dependency-check` | 获取并核对真实依赖 | 不等于整个 OB-001 或发布通过 |
| `make dependency-export` | 联网导出可复验公共依赖 | 不包括服务端/扫描器/工具链，不等于 SDK 测试通过 |
| `make normal-test` | 官方依赖下全库测试/编译/覆盖率 | 不等于真实 OpenBao 已集成 |
| `make integration-test` | 新建临时真实 OpenBao 与受限身份测试 | 不等于已有业务迁移完成 |
| `make consumer-test` | 两个独立模块引用 | 不等于远端已经发布 |
| `make security-test` | 固定工具扫描 | 工具下载失败不等于无漏洞 |
| `make tooling-test`、`make fuzz-test` | 工具和模糊测试 | 不替代功能验收 |
| `make release-check` | 检查台账和当前正式证据 | 不自动把记录改为通过 |

`make contract-test` 是历史补充模式，不用于首次业务接入或发布门禁。消费者脚本通过显式 `--offline-proxy` 使用含签名 sumdb 镜像的本地代理，不回退公网；联网模式需显式 `--allow-network`，结果见当前正式消费者报告。TLS 文件在 New 时加载，当前没有证书热加载 API；运行中更改 CA 文件不等于现存 Client 已更新信任。

HMAC verify 夹具权限后缀已按接口契约同步。PKI 演示存储材料及回读检查仍不等于完整业务交付方案；业务接入需按自身持久化和恢复要求验收。

更多来源：[R3报告](../IMPLEMENTATION_REPORT_R3.md)、[依赖转移限制](dependency-transfer.md)、[发布检查](release.md)。
