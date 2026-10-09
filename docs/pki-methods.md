# PKI 全部公开方法详解

本文按当前 SDK 解释 **1 个挂载入口、5 个网络方法、2 个本地校验函数和 9 个公开结构体**。这里不是 OpenBao 管理 API 的完整清单；创建根 CA、管理角色或任意指定 issuer 不属于本 SDK 的运行时范围。

接口以当前 HEAD `80f758eb3625cf0af1de2cb1d410ae5f847cb3db` 的 [签发实现](../pki_client.go)、[查询与吊销实现](../pki_read.go)、[本地校验](../pki/validate.go)、[公开类型](../pki/types.go) 为准。本文是使用说明，不是新的真实服务验证、兼容认证或发布批准。

## 1. 先理解 PKI 的几种材料

| 名称 | 意义 | 不要混淆 |
|---|---|---|
| CA / issuer | 签发证书的机构与签发者 | PKI 挂载不等于一个不可变的历史 issuer |
| 叶证书 | 设备、用户或服务持有的证书 | 包含公钥，不包含私钥 |
| 私钥 | 与证书公钥配对的秘密 | 证书和 serial 不能恢复私钥 |
| CSR | 包含公钥、名称等信息，并由对应私钥签名的申请 | 不是已签发证书，也不是业务授权证明 |
| CN | Subject 的 CommonName | 不是通用的 TLS 主机名或租户身份校验 |
| SAN | DNS、邮箱、IP、URI 等替代名称 | SDK 的各种校验覆盖范围不同 |
| 证书链 | 叶证书通往信任锚的签名材料 | 响应附带根证书不等于调用方信任它 |
| TTL | 请求的有效时长 | 实际有效期看返回证书，不是可以自动续期的业务租约 |

OpenBao 服务连接的 TLS CA 与**业务证书签发体系的可信根**是两套用途。配置 Client 能连接 OpenBao，并不意味着业务应信任该服务返回的任何业务证书。

### 1.1 Issue 与 SignCSR 的核心区别

| 选择 | 私钥在哪里生成 | 网络返回 | 谁保管私钥 |
|---|---|---|---|
| `Issue` | 服务端签发流程 | 证书、CA 材料、终端私钥 | 调用方必须及时、安全保存 |
| `SignCSR` | 调用方或其密钥设施 | 证书、CA 材料；没有私钥 | 调用方一直保管原私钥 |

官方明确指出 Issue 生成的终端私钥不会作为可回读材料存储在 PKI 中；丢掉响应里的私钥，ReadCertificate 也无法补回。见 [官方签发说明](https://openbao.org/docs/2.6.x/api/secret/pki/#generate-certificate-and-key)。

### 1.2 方法总览

| 入口 / 方法 | 参数 | 返回 | 副作用 |
|---|---|---|---|
| `Client.PKI` | `mount string` | `*bao.PKIClient, error` | 无网络 |
| `Issue` | `ctx, pki.IssueRequest` | `*pki.IssuedCertificate, error` | 签发与生成终端私钥 |
| `SignCSR` | `ctx, pki.SignCSRRequest` | `*pki.SignedCertificate, error` | 签发 |
| `ReadCertificate` | `ctx, serial string` | `*pki.CertificateRecord, error` | 只读 |
| `ReadIssuerChain` | `ctx` | `*pki.ChainResult, error` | 只读 |
| `Revoke` | `ctx, serial string` | `*pki.RevokeResult, error` | 吊销 |
| `pki.ValidateCSR` | `raw []byte` | `error` | 本地校验 |
| `pki.ValidateBundle` | `pki.IssuedCertificate, pki.VerifyPolicy` | `error` | 本地校验 |

当前 pki 包没有其他公开函数，也没有这些结果结构体自带的网络方法。固定签名见 [公共契约测试](../public_contract_test.go)。

## 2. 挂载入口与共同规则

### 2.1 `Client.PKI`

签名：`func (c *Client) PKI(mount string) (*PKIClient, error)`。

- 绑定已有挂载，例如 `pki` 或 `team/pki`；返回客户端沿用外层 Client 的固定地址、集群、Namespace 和身份。
- 只进行本地绑定和路径校验，不创建挂载、CA、issuer 或 role，也不探测权限。
- nil Client 或不合法挂载返回参数错误。
- 挂载路径各段只允许 ASCII 字母、数字、`_`、`-`、`.`；不能为空、使用 `.` / `..`、尾部为点或含空段、前后斜杠。详见 [路径实现](../internal/engine/path.go)。
- `IssueRequest.Role` 和 `SignCSRRequest.Role` 必须是**合法单段名称**，不能把完整 `pki/issue/role` 路径当作 Role。
- 网络调用前，外层 Client 需成功 `Start(ctx)`；不要自行构造零值 PKIClient。
- PKIClient 没有独立 Close；外层 Client 关闭不会吊销业务证书或删除秘密。

认证、配置和生命周期见 [接入说明](sdk-integration-guide.md)、[认证说明](authentication.md)。

### 2.2 预算、大小与重试

所有网络方法的 `ctx` 都必须非 nil；两个本地校验函数没有 ctx，也不访问 OpenBao。

当前默认值及其区别：

| 边界 | 当前默认 / 实现限制 |
|---|---|
| Issue、SignCSR 的总操作预算 | 30 秒 |
| ReadCertificate、ReadIssuerChain、Revoke 的普通预算 | 10 秒 |
| 编码后的请求体 | 1 MiB |
| 网络响应体 | 2 MiB |
| 一份本地 PEM 材料解析 | 最多 4 MiB |
| 一份证书 PEM 中的证书块 | 最多 64 个 |

前四项可受 Client 配置影响；后两项是当前材料解析器的固定边界。签名请求的 CSR 原文大小通过检查，也不表示加上 JSON 包装与字符串转义后的请求一定能发送。

预算覆盖等待认证、并发、传输、读取响应与允许的重试，并服从更早的调用方 deadline。ReadCertificate 和 ReadIssuerChain 默认最多 **3 次总尝试**；Issue、SignCSR、Revoke 默认只有 **1 次 SDK 业务请求**。

SDK 不承诺代理没有隐藏重放，不把 403 当成“自动重登后重发原签发请求”，也不承诺服务端 exactly-once。见 [执行器操作分类](../internal/engine/operations.go)、[错误处理](error-handling.md)。

## 3. `Issue`：签发证书并返回终端私钥

签名：`func (p *PKIClient) Issue(ctx context.Context, req pki.IssueRequest) (*pki.IssuedCertificate, error)`。

发送 `POST /v1/{mount}/issue/{role}`，服务端角色与签发者应已由管理员配置好。

### 3.1 IssueRequest 全部字段

| 字段 | 类型 | 要求 / 语义 |
|---|---|---|
| `Role` | string | 必填，合法单段角色名，不是挂载或 issuer ID |
| `CommonName` | string | 可空；CN 与所有 SAN 不能同时全部为空 |
| `DNSNames` | []string | DNS SAN 列表；每项非空，不允许逗号、空格、tab 或控制字符 |
| `EmailNames` | []string | 邮箱 SAN；必须能解析为同一个纯邮箱地址，不接受显示名称包装 |
| `IPAddresses` | []netip.Addr | 每项必须有效且没有 zone |
| `URISANs` | []string | URI SAN；必须可解析且有 scheme，不允许逗号与控制字符 |
| `TTL` | time.Duration | 必须 `>0`；零值不会自动使用服务端默认 TTL |
| `ExcludeCNFromSANs` | bool | 控制 CN 是否由签发接口自动加入 SAN，按服务端策略执行 |

CN 和文本 SAN 还受当前实现的 UTF-8、单项 1024 字节与控制字符检查。这里不是所有 DNS、URI 或业务身份规则的完整验证；服务端角色决定允许的名称和签发范围，业务仍须先授权资源与身份。

请求固定使用 `format=pem`、`private_key_format=pkcs8`。DNS 与邮箱会合并到 `alt_names`，IP 使用 `ip_sans`，URI 使用 `uri_sans`；SDK 负责编码，调用者不用手工拼逗号字符串。

**没有** KeyType、KeyBits、Issuer、自动落库或 KV 路径字段。Issue 的终端密钥算法由服务端角色决定；当前 SDK 不让调用者随意覆盖 issuer。

### 3.2 成功时 SDK 已检查什么

收到成功 HTTP 响应后，仍会检查材料，不是把字符串原样标为成功：

1. certificate 必须是可解析的证书 PEM；第一个证书作为叶证书，后面的证书块可加入返回链材料。
2. 返回 serial 必须合法，并与叶证书中真实序列号匹配。
3. issuing_ca 必须是恰好一个可解析的证书 PEM。
4. 可选 ca_chain 必须为合法证书字符串集合；聚合的 CAChainPEM 不能超过当前 64 项边界。
5. 可选 expiration 存在时必须与叶证书 NotAfter 的 Unix 秒一致。
6. private_key 必须是未加密的 **PKCS#8 PEM**，且对应的公钥与叶证书公钥相同。
7. 非空请求 CN 必须精确匹配；请求的 DNS 忽略大小写匹配，邮箱/URI 精确匹配，IP 按地址匹配。

名称核对要求每个请求名称存在，不要求证书只能包含这些名称。ExcludeCNFromSANs 也不是“SDK 保证返回证书绝不含额外 SAN”的白名单。

### 3.3 成功不等于通过业务信任策略

上述检查主要确认响应形态、配对关系和请求名称。**Issue 本身不替你完成独立可信根链验证、业务 EKU 策略、当前有效性或最短剩余寿命校验。**

应按业务策略调用 ValidateBundle，而不是把响应中的 IssuingCAPEM 或 CAChainPEM 自动装进信任根。TTL 只是申请值，实际 NotBefore/NotAfter 以证书为准；不要自己用 `time.Now().Add(req.TTL)` 代替真实有效期。

成功返回的 PrivateKey 由调用方所有；保存、交付或使用结束后应 Zero。SDK 不自动保存到 KV、数据库、文件或 TLS 配置。

已发送后超时、响应损坏、私钥不匹配等情况可能返回 `EffectUnknown`。这不证明服务端没发证，不能通过反复 Issue 来“直到拿到一份能用的结果”。

## 4. `SignCSR`：签署调用方已有公钥的申请

签名：`func (p *PKIClient) SignCSR(ctx context.Context, req pki.SignCSRRequest) (*pki.SignedCertificate, error)`。

发送 `POST /v1/{mount}/sign/{role}`。

### 4.1 SignCSRRequest 只有三个字段

| 字段 | 类型 | 含义 |
|---|---|---|
| `Role` | string | 合法单段角色名称 |
| `CSRPEM` | sensitive.Bytes | 调用方已有的 PEM CSR |
| `TTL` | time.Duration | 必须为正；请求有效期由服务端策略处理 |

调用方创建私钥和 CSR，把 CSR 封装进 sensitive.Bytes；**不把私钥塞进 CSRPEM**。该私钥应一直由调用方或其密钥设施保管。

发送前 SDK 会验证 CSR PEM 结构、ASN.1 和 CSR 自签名。发送正文包括 csr、format=pem、ttl；CSR 的编码后 JSON 请求仍受整体请求体边界限制。

### 4.2 与 Issue 不同的校验范围

成功响应与 Issue 共用证书、serial、issuer、链和可选 expiration 检查；额外要求返回证书公钥与 CSR 公钥相同。

但 SignCSR **没有 IssueRequest 的名称列表**，也不会像 Issue 那样逐项核对请求 CN、DNS、邮箱、IP、URI。服务端可能根据角色与 CSR 策略处理名称；不能仅因公钥相同就认定租户身份或所有 SAN 都符合业务要求。

SDK 不生成新私钥，SignedCertificate 也没有 PrivateKey 字段。SDK 清理自身揭示 CSR 的临时字节；原 CSRPEM 句柄和本地私钥仍由调用方管理。

### 4.3 签好的 CSR 如何继续校验

ValidateBundle 接受的是 **IssuedCertificate**，不是 SignedCertificate。若调用方已有可导出的 PKCS#8 私钥，可显式组成一个用于本地校验的 IssuedCertificate：

- Certificate 使用 signed.Certificate。
- PrivateKey 使用调用方原有私钥句柄。
- VerifyPolicy 使用独立可信根和业务预期身份。

这只是调用方组合现有类型，不表示服务端返回了私钥，也不是新的 SDK 接口。第 11 节有可编译示例。

对于 HSM 等不可导出私钥，当前 ValidateBundle 的 sensitive.Bytes 契约不能直接接收任意 crypto.Signer；调用方需用与其密钥设施和业务协议一致的验证流程，不为了适配此函数强制导出秘密。

SignCSR 同样是单次签发操作，未知结果先核对，不自动重签。官方角色约束说明见 [CSR 签发接口](https://openbao.org/docs/2.6.x/api/secret/pki/#sign-certificate)。

## 5. `ReadCertificate`：按序列号读取已存证书

签名：`func (p *PKIClient) ReadCertificate(ctx context.Context, serial string) (*pki.CertificateRecord, error)`。

### 5.1 序列号校验与规范化

当前实现接受十六进制序列号：

| 输入示例 | 规范化结果 |
|---|---|
| `0A:0B` | `0a:0b` |
| `0A-0B` | `0a:0b` |
| `0a0b` | `0a:0b` |
| `a0b` | `0a:0b` |
| `00:01` | `01` |

不接受空值、前后空白、非十六进制、混用冒号/连字符、分隔后非两位的段或全零。输入最长 128 字节，去分隔符与补齐后最多 20 个数值字节。

SDK 将规范结果中的冒号转为连字符构造 `GET /v1/{mount}/cert/{serial-with-dashes}`。调用方通常直接保存并使用 Issue/SignCSR 返回的 SerialNumber，不必手工拼 URL。

### 5.2 返回内容

- 提取 certificate 中第一个叶证书，校验其真实 serial 与请求相同。
- certificate 可以含多个合法 PEM 块；CertificateRecord 只保留叶证书，不返回整份附带链。
- NotBefore、NotAfter 来自解析出的叶证书。
- revocation_time 缺失或为 0 时，RevokedAt 为 nil；为有效正数时转换为 UTC 时间。
- 非整数、负数或超出当前支持时间范围的 revocation_time 会导致响应错误。

**RevokedAt 为 nil 仅表示没有对应的正数返回证据，不独立保证证书未吊销。** 这不是实时 CRL/OCSP 验证器，也不能替代 TLS 对端的完整证书认证。

证书记录可以受服务端存储、保留和清理策略影响。404 不证明从未签发；读到证书也不恢复私钥。ReadCertificate 不是一个“签发失败就读私钥补救”的方法。

OpenBao 官方把此证书读取接口列为可匿名访问端点；证书虽非私钥，仍可能包含业务名称等信息，不能把该接口的 ACL 当作这些信息的保密承诺。SDK 自身仍走固定 Client 生命周期和身份执行路径。见 [官方证书读取说明](https://openbao.org/docs/2.6.x/api/secret/pki/#read-certificate)。

## 6. `ReadIssuerChain`：当前默认签发者的链

签名：`func (p *PKIClient) ReadIssuerChain(ctx context.Context) (*pki.ChainResult, error)`。

实际调用 `GET /v1/{mount}/ca_chain`。这是返回 PEM 的端点，SDK 按证书材料解析原始响应，不要求调用者处理 JSON 包装。

- 返回 `CertificatesPEM [][]byte`，不是 Certificate.CAChainPEM 字段。
- 解析 PEM、证书结构和材料边界；**不因此把链中的根设为业务可信根**。
- 查询的是**当前默认 issuer 的链**；不会按某个历史证书自动找原始 issuer。
- 多个读取操作之间可能发生 issuer 更新；ReadCertificate + ReadIssuerChain 不是原子历史快照。
- 没有任意 issuer ID、issuer 路径覆盖或历史链版本参数。

业务持久化某次签发材料时，应保留该次 Certificate 中的必要 IssuingCAPEM / CAChainPEM，不能将未来读取的当前链当成当时原始链。

官方默认链端点也可匿名访问。链可用于补充验证材料，但信任锚应来自独立受控配置。见 [官方默认签发者链说明](https://openbao.org/docs/2.6.x/api/secret/pki/#read-default-issuer-certificate-chain)。

## 7. `Revoke`：按序列号请求吊销

签名：`func (p *PKIClient) Revoke(ctx context.Context, serial string) (*pki.RevokeResult, error)`。

输入采用与 ReadCertificate 相同的序列号规范化规则，发送 `POST /v1/{mount}/revoke`：

```json
{"serial_number":"0a:0b"}
```

### 7.1 什么才算 SDK 接受的吊销成功

响应必须具有可解析的 data 和**有效的正数 revocation_time**，才能返回 RevokeResult.RevokedAt。

空响应、缺失字段、0、负值、非法时间或损坏响应不是明确的成功回执。即便服务端可能已执行吊销，SDK 也不会用“HTTP 看起来成功”制造时间；这种已发送的模糊响应可能仍是 EffectUnknown。

返回 RequestID 和 RevokedAt，但不返回 serial、逐步骤业务状态或 Attempts。业务应另行记录本次目标 serial 和任务身份。

### 7.2 吊销不会自动做什么

- 不擦除调用方、KV 或设备上的私钥。
- 不删除业务数据库记录，也不调用 MQTT Broker 断开连接。
- 不给设备自动补发新证书。
- 不保证所有 TLS 验证者已经获得最新吊销状态或立即断开现有会话。
- 不在本 SDK 中提供 CRL/OCSP 拉取、轮询或校验。
- 不通过这个终端证书接口管理 CA issuer 吊销。

若响应丢失，应结合可获得的证书吊销信息与业务审计核对，不能只因一次读取的 RevokedAt 为 nil 就断言原请求未执行。

官方说明该吊销端点仅凭 serial 操作，**不证明调用者曾签发它或持有它的私钥**。因此拥有 `{mount}/revoke` 权限的业务仍必须验证目标证书的资源归属，而不是把客户端输入的 serial 直接透传。见 [官方吊销说明](https://openbao.org/docs/2.6.x/api/secret/pki/#revoke-certificate)。

## 8. `pki.ValidateCSR`：本地验证申请结构和自签名

签名：`func ValidateCSR(raw []byte) error`。

成功只表示这份 CSR 符合当前解析与签名校验要求：

1. 非空、最多 4 MiB。
2. 恰好一个 `CERTIFICATE REQUEST` 或 `NEW CERTIFICATE REQUEST` PEM 块。
3. 不含 PEM headers、多余块、非空尾随内容或被解码器跳过的损坏前导块；外侧空白允许。
4. ASN.1 CSR 可以解析。
5. `csr.CheckSignature()` 成功，即申请中的公钥可以校验 CSR 自签名。

这个函数：

- 不联网，不依赖 Client.Start，不调用 Issue 或 SignCSR。
- 不证明 CN/SAN 合法、租户归属、算法强度或服务端角色允许签发。
- 不证明持有人经业务认证，也不构成 CA 签名。
- 不清理调用方的 raw；不会主动 Zero 输入句柄。

失败返回安全的 `*baoerr.Error`：`CodeInvalidArgument`、`Operation="PKI_VALIDATE"`、`EffectNone`。不会透出原始 CSR 或解析器的敏感错误正文。

## 9. `pki.ValidateBundle`：私钥配对、独立信任链与业务策略

签名：`func ValidateBundle(bundle IssuedCertificate, policy VerifyPolicy) error`。

这是本 SDK 的**离线材料校验**，不是另一次发证、查询或吊销。它返回 error，不返回链详情、自动修复结果或网络证据。

### 9.1 VerifyPolicy 全部字段

| 字段 | 类型 | 规则 |
|---|---|---|
| `TrustedRootsPEM` | [][]byte | 调用方独立受控的信任锚，必须非空，最多 64 个输入项 |
| `CurrentTime` | time.Time | 零值使用校验时的当前时间；非零按该时间校验 |
| `RequiredEKUs` | []x509.ExtKeyUsage | 必须非空，最多 16 项，业务明确选择用途 |
| `ExpectedCN` | string | 非空时要求 CN 精确相等；空值不检查 CN |
| `ExpectedDNS` | []string | 每个预期 DNS 都必须出现在 SAN，忽略大小写 |
| `ExpectedURI` | []string | 每个预期 URI 都必须精确出现在 URI SAN |
| `MinRemainingTTL` | time.Duration | 不能为负，剩余有效时间不得小于该值 |

ExpectedDNS 不是通配符主机名匹配器：SAN 中的 `*.example` 不会仅因为它可代表某个主机，就通过 `ExpectedDNS=["device.example"]` 的字面存在检查。ExpectedCN 也不是 TLS 主机名验证的替代品。

当前策略**没有 ExpectedIP、ExpectedEmail、允许算法、最小密钥位数或最大 TTL 字段**。不要为这个函数编造这些检查；协议需要它们时，由业务补充明确验证。

### 9.2 私钥与证书的检查

- 解析 CertificatePEM 的第一个证书为叶证书。
- 从 bundle.PrivateKey 揭示独立临时副本，只接受 `PRIVATE KEY` PEM，即未加密 PKCS#8。
- 私钥必须可解析为 crypto.Signer，且其公钥与叶证书公钥相同。
- 不接受 PKCS#1 的 `RSA PRIVATE KEY`、SEC1 的 `EC PRIVATE KEY` 或加密私钥 PEM；调用方需按自己的受控流程准备正确格式。
- 临时私钥字节会清理；原 bundle.PrivateKey 仍归调用方，函数不会将其 Zero。

它按值接收 bundle，但 sensitive.Bytes 的值复制共享清理句柄；按值传递不意味着克隆出另一份长期私钥。

### 9.3 独立信任锚与链

- 只使用 policy.TrustedRootsPEM 建立 roots pool，不自动用系统根，也不把响应中的 CAChainPEM 当作信任锚。
- 每个根输入必须是合法证书 PEM，其中证书需标记 IsCA。当前实现并不额外要求这些信任锚必须是自签名根；把某个中间 CA 放进去也是调用方显式授予信任。
- CertificatePEM 里的后续证书、CAChainPEM 与非空 IssuingCAPEM 作为中间材料加入链验证。
- CAChainPEM 与非空 IssuingCAPEM 合计最多 64 个材料输入项；每份 PEM 的材料大小和证书块数量另有解析边界。
- 链验证通过与否还取决于 Go x509 的签名、链约束、有效期和用途判断；“材料能解析”不等于“可信”。

因此 ReadIssuerChain 可以提供链材料，但不解决“哪些根可信”这个独立配置问题。

### 9.4 有效期与元数据

校验时间 now 必须满足：

```text
NotBefore <= now < NotAfter
NotAfter - now >= MinRemainingTTL
```

Certificate.NotBefore / NotAfter 元数据非零时，必须分别与叶证书真实值相等；SerialNumber 非空时，也必须与叶证书序列号相同。零值时间或空 serial 不会替代证书本身的实际内容。

**FingerprintSHA256 元数据不属于当前 ValidateBundle 的核对项。** Issue/SignCSR 会计算叶证书真实 DER 的 SHA-256，但手工组成 bundle 时，不能仅靠填上一个 fingerprint 字符串宣称它已被此函数校验。

### 9.5 EKU 与名称不是“有一个就行”

SDK 针对 RequiredEKUs 的**每一项**单独调用叶证书链验证。因此请求 ClientAuth 与 ServerAuth 时，不能仅通过其中一个用途就认为满足整个 policy。

- 客户端证书常见用途为 `x509.ExtKeyUsageClientAuth`。
- 服务端证书应按协议使用 `x509.ExtKeyUsageServerAuth`。
- 不要用 `ExtKeyUsageAny` 代替原本应明确的用途；它表示放宽用途要求。
- 这里是 x509 的用途链判断，不额外强制叶证书必须显式包含某个 EKU 扩展字段。
- CN/DNS/URI 条件是所要求身份的匹配检查，不是拒绝所有额外名称的完整白名单。
- 当前函数也不额外强制叶证书 IsCA=false 或所有业务证书 profile 约束；需要这些约束的接入方须补充。

### 9.6 不检查吊销，也不把后续失败改写成未签发

ValidateBundle 不进行 CRL/OCSP 查询或吊销判断；即便材料通过本地信任、用途和有效期检查，也不独立证明证书未被吊销。Go 官方也明确说明 x509.Verify 不做吊销检查。见 [Go x509.Verify](https://pkg.go.dev/crypto/x509#Certificate.Verify)。

任何校验失败统一返回 `CodeInvalidArgument`、`Operation="PKI_VALIDATE"`、`EffectNone` 的安全错误，没有网络 HTTP 回执。

**EffectNone 只描述这次本地校验没有副作用。** 如果前面 Issue/SignCSR 已成功，后续 ValidateBundle 失败并不抹去已经发生的签发，也不能作为重新发证的理由。组合函数必须单独保留签发阶段的事实和恢复责任。

来源：[ValidateBundle 实现](../pki/validate.go)、[严格材料解析](../internal/pkiutil/parse.go)、[严格 PEM 解码](../internal/pemutil/decode.go)。

## 10. 全部 9 个公开结构体与所有权

IssueRequest、SignCSRRequest 与 VerifyPolicy 的字段已分别在第 3、4、9 节解释，其余六个结构体如下。

### 10.1 `pki.Certificate`

| 字段 | 类型 | 含义 |
|---|---|---|
| `CertificatePEM` | []byte | 提取并规范编码后的叶证书 PEM |
| `IssuingCAPEM` | []byte | 签发响应中的 issuer 证书 PEM |
| `CAChainPEM` | [][]byte | 叶证书后附带的链与 ca_chain 聚合材料；不承诺去重或构建好的唯一可信链 |
| `SerialNumber` | string | 规范化的小写十六进制、冒号分隔序列号 |
| `NotBefore` | time.Time | 叶证书真实起始时间 |
| `NotAfter` | time.Time | 叶证书真实截止时间 |
| `FingerprintSHA256` | string | 叶证书 DER 的 SHA-256 小写十六进制，不是 PEM 文本哈希 |

Certificate 没有私钥字段，也没有专用网络、Zero 或持久化方法。

### 10.2 `pki.IssuedCertificate` 与 `pki.SignedCertificate`

| 字段 | IssuedCertificate | SignedCertificate |
|---|---|---|
| `Certificate` | pki.Certificate | pki.Certificate |
| `PrivateKey` | sensitive.Bytes，PKCS#8 PEM | 没有这个字段 |
| `RequestID` | string | string |

SDK 没有将 lease_id、lease_duration、renewable 或 Attempts 暴露在这些签发结果中。证书过期后的轮换由业务流程负责，不能假设存在 `RenewCertificate` 方法。

### 10.3 `pki.CertificateRecord`

字段为 `CertificatePEM []byte`、`SerialNumber string`、`NotBefore time.Time`、`NotAfter time.Time`、`RevokedAt *time.Time`、`RequestID string`。

它不是完整签发材料包：没有 PrivateKey、IssuingCAPEM、CAChainPEM 或 FingerprintSHA256 字段。不能把一次查询结果当作原 IssueResult 的等价替代物。

### 10.4 `pki.ChainResult` 与 `pki.RevokeResult`

- ChainResult：`CertificatesPEM [][]byte`、`RequestID string`。
- RevokeResult：`RevokedAt time.Time`、`RequestID string`。

全部 PKI 结果都没有 Attempts 字段；需要了解错误尝试次数时，应检查 `baoerr.Error.Attempts` 或受限观察事件，而不是套用 KV WriteResult 的字段。

### 10.5 私钥、CSR 与普通证书字节

PrivateKey 和 CSRPEM 使用 sensitive.Bytes；相关通用能力见 [秘密字节实现](../sensitive/bytes.go)：

| 操作 | 行为与责任 |
|---|---|
| `sensitive.NewBytes(raw)` | 独立复制；调用方仍负责原 raw |
| `RevealCopy()` | 显式明文副本；用完 clear，不打印或写日志 |
| `Zero()` | 清理共享句柄；值复制的其他句柄也变空 |
| `Len()` | 当前句柄中的字节数，不是有效密钥证明 |
| 普通 fmt/slog | 默认脱敏 |
| 普通 JSON 序列化 | 返回错误，不能隐式导出秘密 |

Issue 的返回私钥归调用方；SignCSR 的输入 CSR 与调用方私钥也归调用方；ValidateBundle 仅借用句柄并清理自身临时副本。

Zero 不追踪 RevealCopy 的副本，也不能保证 Go 已解析私钥对象、string、TLS 栈、GC 或运行时复制彻底擦除。不要用值赋值制造所谓“独立私钥备份”。

CertificatePEM、链及查询记录中的普通 []byte **没有相同的自动输出脱敏保护**；证书可以带业务身份，日志仍应限制。序列化含 PrivateKey 的 IssuedCertificate 会触发秘密字段的 JSON 错误；普通 []byte / [][]byte 的 JSON 表示则是 Base64 字符串/数组，不是字面 PEM。

持久化时应明确版本化 schema，保存必要证书、issuer、链和私钥材料及业务任务身份；若使用 KV，应保存准确 kv.Ref。不能直接 json.Marshal 整个 IssuedCertificate 当作完整、隐式安全的保存方案。见 [KV 方法说明](kv-methods.md)、[接入与回滚](integration-and-rollback.md)。

## 11. 可编译组合示例

下面只有函数定义，没有 main、Client 初始化或自动执行网络操作。调用者提供已 Start 的 Client、受授权的角色/serial、独立可信根与有效 context。

- ExampleIssueRequest 仅演示字段组合；示例名称、IP 和 SAN 都需要服务端角色允许，不意味着已有该角色或可在生产执行。
- ClientAuthPolicy 显式设置客户端用途；CurrentTime 留零以使用真正校验时的时间。需要 URI 约束时，业务再明确设置 ExpectedURI。
- IssueWithPolicy 保留签发阶段结果：后续本地校验失败时清理私钥，但返回 outcome.Bundle 的公开材料供业务记录 serial。
- **有 error 时不可将 outcome.Bundle 交给正常使用路径；PolicyValidated=true 才是这个示例的组合成功。**
- 如果 Issue 本身返回未知结果，outcome.Bundle 为 nil 仍不证明服务端没有签发。
- GenerateCSR 在本地使用 ECDSA P-256，返回两个独立句柄：CSR 和 PKCS#8 私钥。调用方分别负责 Zero；已解析 Go 私钥对象的彻底擦除仍不受保证。
- SignExistingCSR 不接收或发送私钥；ValidateSignedWithKey 借用原私钥完成现有 ValidateBundle 的组合。
- ReadState 的证书与当前默认链是两次独立读取，不是历史证书完整材料恢复。
- RevokeExplicitly 不自动重试、补发或更新数据库；外层业务必须先核对 serial 归属。
- 这些辅助函数是本文示例，不是额外 SDK 方法。

```go
package pkiguide

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"net/netip"
	"time"

	bao "github.com/RockInMars/openbao-sdk-go"
	"github.com/RockInMars/openbao-sdk-go/baoerr"
	"github.com/RockInMars/openbao-sdk-go/pki"
	"github.com/RockInMars/openbao-sdk-go/sensitive"
)

type IssueOutcome struct {
	Bundle          *pki.IssuedCertificate
	PolicyValidated bool
}

func BindIssuer(client *bao.Client, mount string) (*bao.PKIClient, error) {
	if client == nil {
		return nil, errors.New("client is required")
	}
	return client.PKI(mount)
}

func requireInputs(ctx context.Context, issuer *bao.PKIClient) error {
	if ctx == nil || issuer == nil {
		return errors.New("context and issuer are required")
	}
	return nil
}

func ExampleIssueRequest(role string) pki.IssueRequest {
	return pki.IssueRequest{
		Role:              role,
		CommonName:        "device.example",
		DNSNames:          []string{"device.example"},
		EmailNames:        []string{"device@example.invalid"},
		IPAddresses:       []netip.Addr{netip.MustParseAddr("192.0.2.10")},
		URISANs:           []string{"spiffe://example.invalid/device/demo"},
		TTL:               30 * time.Minute,
		ExcludeCNFromSANs: true,
	}
}

func ClientAuthPolicy(roots [][]byte, commonName string, dnsNames []string) pki.VerifyPolicy {
	return pki.VerifyPolicy{
		TrustedRootsPEM: roots,
		RequiredEKUs:    []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		ExpectedCN:      commonName,
		ExpectedDNS:     append([]string(nil), dnsNames...),
		MinRemainingTTL: 5 * time.Minute,
	}
}

func IssueWithPolicy(ctx context.Context, issuer *bao.PKIClient, request pki.IssueRequest, policy pki.VerifyPolicy) (IssueOutcome, error) {
	if err := requireInputs(ctx, issuer); err != nil {
		return IssueOutcome{}, err
	}
	if len(policy.TrustedRootsPEM) == 0 || len(policy.RequiredEKUs) == 0 {
		return IssueOutcome{}, errors.New("explicit trust roots and EKUs are required")
	}
	issued, err := issuer.Issue(ctx, request)
	if err != nil {
		return IssueOutcome{}, err
	}
	outcome := IssueOutcome{Bundle: issued}
	if err := pki.ValidateBundle(*issued, policy); err != nil {
		issued.PrivateKey.Zero()
		return outcome, err
	}
	outcome.PolicyValidated = true
	return outcome, nil
}

func GenerateCSR(commonName string, dnsNames []string) (sensitive.Bytes, sensitive.Bytes, error) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return sensitive.Bytes{}, sensitive.Bytes{}, err
	}
	template := &x509.CertificateRequest{
		Subject:  pkix.Name{CommonName: commonName},
		DNSNames: append([]string(nil), dnsNames...),
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, template, privateKey)
	if err != nil {
		return sensitive.Bytes{}, sensitive.Bytes{}, err
	}
	defer clear(csrDER)
	csrPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER})
	defer clear(csrPEM)
	if err := pki.ValidateCSR(csrPEM); err != nil {
		return sensitive.Bytes{}, sensitive.Bytes{}, err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return sensitive.Bytes{}, sensitive.Bytes{}, err
	}
	defer clear(keyDER)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	defer clear(keyPEM)
	return sensitive.NewBytes(csrPEM), sensitive.NewBytes(keyPEM), nil
}

func SignExistingCSR(ctx context.Context, issuer *bao.PKIClient, role string, csr sensitive.Bytes, ttl time.Duration) (*pki.SignedCertificate, error) {
	if err := requireInputs(ctx, issuer); err != nil {
		return nil, err
	}
	return issuer.SignCSR(ctx, pki.SignCSRRequest{
		Role: role, CSRPEM: csr, TTL: ttl,
	})
}

func ValidateSignedWithKey(signed *pki.SignedCertificate, localKey sensitive.Bytes, policy pki.VerifyPolicy) error {
	if signed == nil {
		return errors.New("signed certificate is required")
	}
	return pki.ValidateBundle(pki.IssuedCertificate{
		Certificate: signed.Certificate,
		PrivateKey:  localKey,
		RequestID:   signed.RequestID,
	}, policy)
}

func ReadState(ctx context.Context, issuer *bao.PKIClient, serial string) (*pki.CertificateRecord, *pki.ChainResult, error) {
	if err := requireInputs(ctx, issuer); err != nil {
		return nil, nil, err
	}
	record, err := issuer.ReadCertificate(ctx, serial)
	if err != nil {
		return nil, nil, err
	}
	chain, err := issuer.ReadIssuerChain(ctx)
	if err != nil {
		return record, nil, err
	}
	return record, chain, nil
}

func RevokeExplicitly(ctx context.Context, issuer *bao.PKIClient, serial string) (*pki.RevokeResult, error) {
	if err := requireInputs(ctx, issuer); err != nil {
		return nil, err
	}
	return issuer.Revoke(ctx, serial)
}

func ClassifyFailure(err error) string {
	switch {
	case err == nil:
		return "success"
	case baoerr.HasUnknownOutcome(err):
		return "reconcile-before-any-retry"
	case baoerr.IsCode(err, baoerr.CodeNotFoundOrHidden):
		return "investigate-without-reissuing"
	case baoerr.IsCode(err, baoerr.CodePermissionDenied):
		return "check-authorized-identity-and-policy"
	default:
		return "check-the-failed-step-and-issuance-evidence"
	}
}
```

IssueWithPolicy 的简单前置检查不覆盖所有 policy、根证书、role 或业务规则。即使提前检查配置，后续校验失败仍可能发生；不能把这些步骤包装成跨 OpenBao、KV 与数据库的原子事务。

示例的私钥保存、业务授权、任务持久化、幂等协调、补偿和回读验证需由接入方实现。完整现有演示见 [pki-issue-store](../examples/pki-issue-store/main.go) 与 [credentialworkflow](../examples/credentialworkflow/workflow.go)，其边界说明见 [功能手册](sdk-feature-manual.md)。

## 12. API 路径与最小权限对照

以挂载 pki、角色 device 为例：

| 操作 | 实际 API 路径 | ACL / 安全边界 |
|---|---|---|
| Issue | `POST /v1/pki/issue/device` | policy 路径 `pki/issue/device`，`update` |
| SignCSR | `POST /v1/pki/sign/device` | policy 路径 `pki/sign/device`，`update` |
| Revoke | `POST /v1/pki/revoke` | policy 路径 `pki/revoke`，`update`；必须另做 serial 归属授权 |
| ReadCertificate | `GET /v1/pki/cert/{serial-with-dashes}` | 官方允许匿名读取，不当作秘密访问控制 |
| ReadIssuerChain | `GET /v1/pki/ca_chain` | 官方允许匿名读取，不当作可信根证明 |

ACL 路径不含 `/v1/`。签发角色约束决定证书名称、算法、TTL 等范围，ACL 只控制是否能调用相应路径；两者也不能替代调用应用的业务身份认证。

按任务分离签发、吊销与只读身份，不为解决权限错误授予整个 `pki/*` 或 root。官方公开读取端点的匿名能力不表示 SDK 绕过自己的固定身份、Namespace 或生命周期。

## 13. 错误与副作用：不要盲目重发证书

| 结果 / 稳定判断 | 解释与下一步 |
|---|---|
| `CodeInvalidArgument` | 输入、编码后请求体或明确 API 参数拒绝；本地 Validate 函数也用此码 |
| `CodePermissionDenied` | 确认的权限拒绝；检查授权，不扩大身份或自动重发 |
| `CodeNotFoundOrHidden` | 404 类结果；不能证明证书从未签发或私钥不存在于业务侧 |
| `CodeInvalidResponse` | 材料、serial、公钥或回执不满足契约；签发/吊销请求仍可能产生副作用 |
| `CodeResponseTooLarge` | 响应超限；对已发送签发仍须考虑未知结果 |
| `CodeNotReady / CodeClosed` | 外层 Client 状态不允许本次操作 |
| `CodeCanceled / CodeDeadlineExceeded` | 取消或预算耗尽；代码本身不说明是否已执行写入 |
| `baoerr.HasUnknownOutcome(err)` | 可能已发生签发/吊销，先核对业务任务和服务端证据 |

使用 `baoerr.IsCode`、`HasUnknownOutcome` 或 `errors.As`，不解析 Error() 字符串，也不输出原始错误正文、CSR 或私钥。

错误的 Code 与 Effect 是两个维度：

- `EffectNone`：本次操作没有副作用或具有确认拒绝证据；本地校验始终无网络副作用。
- `EffectUnknown`：已发送写可能执行，结果无法确认。
- `EffectConfirmed`：公共错误模型中的确认效果值，不是签发结果的额外字段。

尤其要区分 **SDK 的 Issue 返回错误** 与 **Issue 成功后业务调用 ValidateBundle/保存数据失败**。后者不能仅因本地错误没有 unknown 标记，就重新 Issue；签发事实应单独记录。

SDK 不替业务撤销所有失败组合中的证书，也不自动补发、换 serial 或重新生成私钥。恢复流程见 [错误处理](error-handling.md)、[业务接入与回滚](integration-and-rollback.md)。

## 14. 当前 SDK 不提供什么

- 创建/导入根 CA、生成中间 CA、管理 issuer、角色、URL 或挂载配置。
- 指定任意 issuer 覆盖角色选择、越过角色政策签发。
- 自动续期证书、失败后自动重新签发、签发并存 KV/数据库的一键事务。
- 从 ReadCertificate 或 serial 恢复私钥。
- 自动选择/信任响应随附的 CA、历史 issuer 链恢复。
- 通用私钥格式转换或任意 crypto.Signer 的 ValidateBundle 入口。
- 自动 CRL/OCSP 下载、吊销检查、Broker 断开或客户端会话终止。
- 全部 SAN 类型、算法强度、业务资产归属或所有 profile 条款的通用验证。
- 永久管理接口、issuer 吊销、任意 RawRequest。

这些是当前运行时范围限制，不是文档漏列的公开方法；维护操作应走单独授权和审计流程。

## 15. 最重要的选择规则

1. **要让服务端生成终端私钥，用 Issue；已有私钥，只发 CSR，用 SignCSR。**
2. **ReadCertificate 读证书，不恢复私钥；ReadIssuerChain 读当前默认链，不恢复历史链。**
3. **ValidateCSR 验证申请结构与自签名，不证明业务身份；ValidateBundle 校验配对和独立信任策略，不查询吊销。**
4. **必须显式提供可信根与 EKU；CN/DNS/URI 的覆盖范围与匹配规则不能混用。**
5. **吊销用 Revoke，但私钥清理、凭据轮换和现有会话处置仍是业务职责。**
6. **本地校验失败不抹去前面的签发事实；未知签发结果也不等于失败。**
7. **私钥与 CSR 归属明确、显式出口受控；成功后及时安全保存，结束后尽力清理。**

## 参考入口

- [PKI 签发源码](../pki_client.go)、[查询与吊销源码](../pki_read.go)、[本地校验源码](../pki/validate.go)、[所有请求和结果类型](../pki/types.go)。
- [材料解析](../internal/pkiutil/parse.go)、[PEM 边界](../internal/pemutil/decode.go)、[签发响应边界回归](../pki_response_boundary_test.go)、[本地信任校验回归](../pki/validate_test.go)。
- [接口契约](spec/02-接口契约.md)、[总体设计](spec/01-总体设计.md)、[功能手册](sdk-feature-manual.md)、[安全说明](../SECURITY.md)。
- [OpenBao PKI 概览](https://openbao.org/docs/2.6.x/secrets/pki/) 与 [PKI API](https://openbao.org/docs/2.6.x/api/secret/pki/)。

官方 OpenBao 链接使用与当前固定 2.6.3 fixture 同系列的 2.6.x 文档，不代表最新服务端版本或所有部署兼容性。Go 文档用于解释底层校验语义，不替代本 SDK 固定工具链验证。当前工程与正式证据仍见 [当前状态](current-status.md)、[兼容性](compatibility.md)、[发布检查](release.md)。
