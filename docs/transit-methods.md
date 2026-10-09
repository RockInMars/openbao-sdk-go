# Transit 全部公开方法详解

本文解释当前 `openbao-sdk-go` 的 **1 个门面绑定方法和 11 个网络方法**，不是 OpenBao 服务端全部管理 API 的清单。依据本地提交 `80f758eb3625cf0af1de2cb1d410ae5f847cb3db` 的源码与公开契约编写；服务端背景参考 OpenBao 2.6.x。以下“SDK 限制”与“OpenBao 原生能力”明确分开，不代表增加了新接口。

## 1. Transit 解决什么问题

Transit 是远程密码学服务，不是另一套 KV 数据库。应用把需要处理的数据交给 OpenBao，使用指定密钥完成签名、验证、加密、解密或 HMAC；密文和业务引用通常由应用自己保存。本 SDK 不返回 Transit 私钥或对称密钥原料。加密时明文仍会发送给服务端，所以必须使用可信地址、身份及经过校验的 TLS。[OpenBao Transit 概念](https://openbao.org/docs/2.6.x/secrets/transit/)

| 需求 | 使用的方法 | 不应混淆的概念 |
|---|---|---|
| 用私钥证明消息未被篡改，持有公钥者能够验证 | `Sign` / `Verify` | 数字签名不隐藏消息 |
| 已经计算了 SHA-256 摘要，避免再散列一次 | `SignDigest` / `VerifyDigest` | 摘要不是原始消息，也不是其十六进制文本 |
| 查询指定版本公钥或密钥状态 | `ReadPublicKey` / `ReadKeyMetadata` | 公钥不是私钥，密钥元数据不是业务秘密内容 |
| 隐藏数据内容，再按权限恢复 | `Encrypt` / `Decrypt` | Base64 只是编码，不是加密 |
| 把旧密文换成同一命名密钥的另一版本 | `Rewrap` | 不创建新密钥版本，不向应用返回明文 |
| 使用共享密钥校验消息完整性和真实性 | `HMAC` / `HMACVerify` | HMAC 不是公钥签名，不能用公钥验证 |

HMAC 的生成和验证都依赖共享秘密；能够生成 MAC 的一方也能够构造有效 MAC，不能把它当作数字签名式的第三方归属证明。

### 方法与 HTTP 路径总览

下表是当前源码实际使用的路径。假设挂载名是 `transit`；绑定其他挂载时替换该段。签名、加解密和 HMAC 的高层调用还会先读取 `GET /v1/transit/keys/<name>`。

| SDK 方法 | 主要 HTTP 请求 | 主要作用 |
|---|---|---|
| `Client.Transit` | 无网络请求 | 绑定挂载门面 |
| `ReadKeyMetadata` | `GET /v1/transit/keys/<name>` | 读取元数据 |
| `ReadPublicKey` | `GET /v1/transit/keys/<name>` | 从指定版本记录提取和校验公钥 |
| `Sign` | `POST /v1/transit/sign/<name>` | 签原始消息 |
| `SignDigest` | `POST /v1/transit/sign/<name>` | 签已有摘要，设置 `prehashed=true` |
| `Verify` | `POST /v1/transit/verify/<name>` | 验原始消息签名 |
| `VerifyDigest` | `POST /v1/transit/verify/<name>` | 验已有摘要签名 |
| `Encrypt` | `POST /v1/transit/encrypt/<name>` | 加密明文 |
| `Decrypt` | `POST /v1/transit/decrypt/<name>` | 解密密文 |
| `Rewrap` | `POST /v1/transit/rewrap/<name>` | 对密文重新加密 |
| `HMAC` | `POST /v1/transit/hmac/<name>/sha2-256` | 生成 SHA-256 HMAC |
| `HMACVerify` | `POST /v1/transit/verify/<name>/sha2-256` | 验证 SHA-256 HMAC |

## 2. 所有方法共用的规则

### 2.1 客户端、挂载、名称和认证

- 先创建并 `Start` 根 `bao.Client`，再调用网络方法；绑定 Transit 门面本身不联网。
- 根 Client 固定集群地址、Namespace 和身份；`mount` 是合法相对挂载路径，不是完整 URL 或 `/v1/transit`。
- `KeyName` 是合法的单段名称，例如 `order-signing`，不是 `transit/keys/order-signing`。业务必须先决定当前身份允许使用哪个密钥，不能把任意外部输入直接当作密钥名。
- 每次使用密钥都读元数据或公钥，不缓存该预检。因此仅有 `sign`、`decrypt` 等操作权限还不够，还需要相应 `keys/<name>` 的读取权限。
- 使用最小权限 token 即可，不需要永久 root token。认证续期和托管 AppRole 的差别见 [认证说明](authentication.md)。

### 2.2 显式版本，不隐式选择 latest

生成签名、密文、HMAC，以及读取公钥，必须传正整数版本；`0` 不表示“自动最新版本”。验签和验 HMAC 使用正整数 `ExpectedVersion`。解密版本来自 `Ciphertext.Version`，还必须与包装字符串内部版本一致。

可以先读取 `LatestVersion` 决定本次使用哪个版本，但读取和后续操作不是事务；运维可能在中间轮换密钥或改变允许版本。明确版本的意义是可追溯，不是保证版本永远可用。SDK 不会遇到版本不可用就偷偷切换另一个版本。

### 2.3 原始字节、包装与敏感材料

- `Message`、`Digest`、`Plaintext` 和派生 `Context` 使用 `sensitive.Bytes`。传原始字节，SDK 负责 HTTP 所需的 Base64 编码。
- `Signature.Wrapped`、`Ciphertext.Wrapped`、HMAC 的 `Wrapped` 保存服务端原始 `vault:vN:<Base64>` 包装。不要改成 `bao:` 前缀，不要剥离版本，也不要再做一层 Base64。
- 包装中的版本要求规范的正整数；编码要求可严格解码并重新编码为同一字符串。畸形输入不是普通的“验证不通过”。
- 调用者管理自己创建的 `sensitive.Bytes`，使用完毕调用 `Zero()`。解密返回的 `Plaintext` 也由调用者清理；`RevealCopy()` 返回的新切片需要单独 `clear`。
- 不把明文、摘要、签名、密文、MAC 或原始响应默认写入日志。清理敏感包装不代表 Go 运行时中的所有临时副本都能被彻底擦除。

### 2.4 限制、超时与结果使用

当前默认 `Limits.MaxRequestBytes=1 MiB`，`MaxResponseBytes=2 MiB`（解压后）。Transit 不仅检查原始字节，还检查实际 JSON 请求体：Base64 膨胀、签名/密文包装、Context 和 JSON 字段都占用请求预算，不能认为“原文不到 1 MiB 就一定能发送”。

所有网络方法需要非 nil `context.Context`。高层密码学调用中的元数据预检和 POST 共用 `Timeouts.Request` 操作预算；取消、等待凭据和并发槽、重试、读取响应都会消耗预算。不要传 nil 门面。

所有返回 `(*Result, error)` 的方法，只有 `error == nil` 时才能使用完整结果。来源：[请求体校验与验证结果](../transit_client.go)、[配置限制](../config_types.go)、[公共类型](../transit/types.go)。

## 3. 门面与密钥读取

### 3.1 `Client.Transit`

**签名：** `Transit(mount string) (*bao.TransitClient, error)`

绑定一个 Transit 挂载，供后续操作复用。它只校验根 Client 和挂载路径，不登录、不探测服务、不启用引擎、不创建密钥。绑定成功不代表挂载已经存在或 token 已有访问权限。

例如 `client.Transit("transit")`。调用者复用返回门面；生命周期仍由根 Client 的 `Start` / `Close` 管理，不存在单独的 Transit 启动或关闭流程。来源：[门面绑定](../transit_keys.go#L22)。

### 3.2 `ReadKeyMetadata`

**签名：** `ReadKeyMetadata(context.Context, string) (*transit.KeyMetadata, error)`

读取命名密钥的能力、当前版本和最低允许版本，不执行任何密码学运算，也不改变密钥配置。适合展示状态、选择后续的明确版本，以及排查轮换后版本不可用。

| 返回字段 | 含义 |
|---|---|
| `Name` | 必须与请求的名称一致 |
| `Type` | 服务端密钥类型；类型存在不等于 SDK 支持所有相应操作 |
| `LatestVersion` | 服务端报告的最新正整数版本 |
| `MinEncryptionVersion` | SDK 对加密、Rewrap 目标和 HMAC 生成使用的最低版本限制 |
| `MinDecryptionVersion` | SDK 对解密、Rewrap 源和 HMAC 验证使用的最低版本限制 |
| `Exportable` | 服务端是否允许导出；本 SDK 仍没有导出方法 |
| `DeletionAllowed` | 服务端是否允许删除；本 SDK 仍没有删除方法 |
| `SupportsSigning` / `SupportsEncryption` | 服务端能力标记，不代替具体算法及参数的校验 |
| `VersionNumbers` | 实际版本表中的版本号，升序排列 |
| `RequestID` | 服务端请求标识，用于关联诊断，不是业务操作的幂等标识 |

元数据会严格检查名称、类型、正整数最新版本、最低版本、布尔字段和版本表。不合法响应返回 `invalid_response`，不会猜测或修补缺失字段。

**HMAC 专用密钥的特殊情况：** 服务端可能不返回 `keys`，或返回空对象。这时 `VersionNumbers` 为空，但保留 `LatestVersion`；HMAC 操作只能使用这一可确认版本，不能从最大版本号推造历史版本列表。若 `keys` 是 null、非对象或含畸形版本号，仍会拒绝响应。来源：[密钥读取](../transit_keys.go#L39)。

### 3.3 `ReadPublicKey`

**签名：** `ReadPublicKey(context.Context, string, int) (*transit.PublicKeyResult, error)`

按命名密钥和准确版本读取公钥，供离线验签、分发给验证端或检查公钥格式。它是 SDK 组合操作：读取 `keys/<name>` 后提取指定版本的 `public_key`，**不是**请求一个不存在的 `keys/<name>/<version>` 路径。

| 返回字段 | 含义 |
|---|---|
| `Key` | `KeyRef`：`ClusterAlias, Namespace, Mount, Name, Version` |
| `KeyType` | 经过解析校验的密钥类型 |
| `SPKIDER` | 统一的 DER SubjectPublicKeyInfo，可交给 `x509.ParsePKIXPublicKey` |
| `PEM` | 统一的 `PUBLIC KEY` PEM，不是证书或私钥 |
| `RequestID` | 本次元数据读取请求标识 |

当前解析范围是非 derived 的 `ecdsa-p256`、`rsa-2048/3072/4096` 和 `ed25519`，且元数据必须支持签名。Ed25519 的原始响应可以是 Base64 公钥，SDK 会统一成 SPKI/PEM；不是所有类型的原始响应都能直接当 PEM 读取。

版本不存在返回 `version_unavailable`。对称密钥、derived 签名密钥或不满足能力要求的读取会被拒绝；公钥响应畸形或不在当前解析范围内会得到 `invalid_response`。该方法不会导出私钥，也不会缓存公钥。

拿到公钥后自行离线验签，需要自己解析签名包装、选择正确算法、摘要方式及版本，并维护公钥信任来源和业务防重放规则。SDK 未暴露一个额外的本地验签公开方法。来源：[公钥读取](../transit_keys.go#L127)、[公钥解析](../internal/transitutil/encoding.go#L93)。

## 4. 数字签名与验签

### 4.1 三种固定 Profile

| Profile 常量 | 支持的密钥类型 | 消息签名语义 | 摘要模式 |
|---|---|---|---|
| `ECDSAP256SHA256ASN1` | `ecdsa-p256` | SHA-256 后做 ECDSA，ASN.1 签名格式 | 32 字节原始 SHA-256 摘要 |
| `RSAPSSSHA256` | `rsa-2048/3072/4096` | SHA-256、RSA-PSS，salt length 与摘要长度一致 | 32 字节原始 SHA-256 摘要 |
| `Ed25519Message` | 非 derived `ed25519` | 按 Ed25519 消息语义签原始消息 | 不支持 |

Profile 不是任意算法字符串，也没有自动选择的默认值。当前不支持 ECDSA P-384/P-521、RSA PKCS#1 v1.5 或调用者任意指定 SHA-512。

消息模式发送 `prehashed=false`；摘要模式发送 `prehashed=true`。ECDSA 固定 `hash_algorithm=sha2-256, marshaling_algorithm=asn1`，RSA 固定 `hash_algorithm=sha2-256, signature_algorithm=pss, salt_length=hash`。Ed25519 不预散列，SDK 不显式发送 `hash_algorithm=none`，以兼容其实际服务端语义。来源：[Profile 编码与兼容性](../internal/transitutil/encoding.go#L49)。

### 4.2 `Sign`

**签名：** `Sign(context.Context, transit.SignRequest) (*transit.SignResult, error)`

使用指定版本的私钥对**原始消息**签名。适合指令、声明、业务凭证等需要由独立验证端使用公钥验真的数据。签名不加密消息，也不证明当前请求者的业务权限已经校验。

| 请求字段 | 填什么 |
|---|---|
| `KeyName string` | 签名密钥名称 |
| `KeyVersion int` | 明确的正整数版本 |
| `Profile transit.Profile` | 与密钥类型匹配的固定 Profile |
| `Message sensitive.Bytes` | 确定性序列化后的原始消息，不是预先散列的摘要 |

流程是本地校验和请求体编码、读取该版本公钥、校验类型、发送签名 POST、解析签名、核对版本、使用读取到的公钥**本地验签**。最后一步不通过即 `invalid_response`，不会把仅有正确格式的服务端输出当成可信签名。

返回 `SignResult` 包含 `Key`、`Signature`、`RequestID`。`Signature` 三字段是 `Wrapped`、`Version`、`Profile`，需要一起保留。服务端返回的包装版本，以及存在时的 `key_version`，都必须与请求一致。来源：[签名实现](../transit_client.go#L16)。

### 4.3 `SignDigest`

**签名：** `SignDigest(context.Context, transit.SignDigestRequest) (*transit.SignResult, error)`

对调用者已经算好的 SHA-256 摘要签名，避免把摘要再散列一次。请求字段与 `SignRequest` 一致，但 `Message` 换成 `Digest sensitive.Bytes`。

只能用于 ECDSA P-256 或 RSA-PSS Profile；`Digest` 必须恰好 **32 字节**。例如 `sha256.Sum256(payload)` 后传入数组切片，而不是 64 字符十六进制字符串或 Base64 文本。把摘要传给 `Sign` 会按“消息”再次散列，语义不同。

返回类型、准确版本校验和返回前本地验签与 `Sign` 相同。`Ed25519Message` 调用该方法是参数错误。ECDSA/RSA-PSS 的消息模式与相应摘要模式可针对同一 SHA-256 语义验证，但不应要求两次生成的签名字节完全一致。来源：[摘要签名入口](../transit_client.go#L22)。

### 4.4 `Verify`

**签名：** `Verify(context.Context, transit.VerifyRequest) (*transit.VerifyResult, error)`

让服务端验证指定版本的签名是否对应**原始消息**。

| 请求字段 | 填什么 |
|---|---|
| `KeyName string` | 签名使用的密钥名称 |
| `ExpectedVersion int` | 业务明确接受的正整数版本 |
| `Profile transit.Profile` | 原签名使用的 Profile |
| `Message sensitive.Bytes` | 待验证的原始消息字节 |
| `Signature transit.Signature` | 包装字符串、版本和 Profile 都齐全的签名对象 |

在网络请求前检查包装、签名字节形状，以及 `ExpectedVersion == Signature.Version == 包装中的版本`、请求 Profile 与签名 Profile 一致。然后读公钥/元数据检查该版本与类型，再发送验证 POST；最终结果来自服务端 `valid` 布尔值。

- `error == nil && Valid == true`：本次验证通过。
- `error == nil && Valid == false`：完成验证，但签名与消息不匹配，是正常否定结果。
- `error != nil`：参数畸形、权限、版本、网络或响应错误，**不是**普通的验证不通过。

例如篡改消息可能得到 false；把版本改成不匹配、只填 `Wrapped` 或使用畸形 ASN.1 签名会被当作参数错误拒绝。调用成功不意味着消息新鲜、未重放或已获得业务授权。来源：[验签实现](../transit_client.go#L87)。

### 4.5 `VerifyDigest`

**签名：** `VerifyDigest(context.Context, transit.VerifyDigestRequest) (*transit.VerifyResult, error)`

验签流程及结果语义与 `Verify` 相同，但输入为 `Digest sensitive.Bytes`，并按预散列模式验证。其他字段仍是 `KeyName, ExpectedVersion, Profile, Signature`。

Digest 只能是 32 字节原始 SHA-256 摘要，Profile 只能是 ECDSA P-256 或 RSA-PSS。计算摘要的算法、原始业务字节以及签名算法必须匹配；不能把消息和摘要模式任意混用，也不支持 Ed25519 摘要验签。来源：[摘要验签入口](../transit_client.go#L90)。

## 5. 加密、解密与 Rewrap

### 5.1 支持范围与两个 Context

当前加解密只接受 `aes128-gcm96`、`aes256-gcm96`，且元数据必须支持加密；拒绝收敛加密。OpenBao 支持的其他类型不等于当前 SDK 已支持。

| 名称 | 类型 | 作用 |
|---|---|---|
| 方法的 `ctx` | `context.Context` | 取消、截止时间和调用预算 |
| 请求的 `Context` | `sensitive.Bytes` | 服务端 derived 密钥的派生上下文字节 |

非 derived 密钥的派生 Context 必须为空；derived 密钥必须非空。加密、解密和 Rewrap 同一数据要使用相同的派生 Context。它不是密钥名、token、Namespace、Go Context，也不是 AAD；当前请求类型没有暴露 AAD、自定义 nonce 或批量输入字段。

派生 Context 不是业务授权机制。即使用租户 ID 作为派生输入，也仍需要业务鉴权、正确路径与 OpenBao ACL。来源：[加解密预检](../transit_cipher.go#L15)。

### 5.2 `Encrypt`

**签名：** `Encrypt(context.Context, transit.EncryptRequest) (*transit.CipherResult, error)`

将明文加密为 Transit 包装密文。

| 请求字段 | 填什么 |
|---|---|
| `KeyName string` | 已存在的 AES-GCM 密钥名称 |
| `KeyVersion int` | 明确的目标加密版本 |
| `Plaintext sensitive.Bytes` | 原始明文字节，SDK 负责 Base64 |
| `Context sensitive.Bytes` | 仅 derived 密钥需要的派生上下文 |

预检该版本实际存在、未超过最新版本、不低于 `MinEncryptionVersion`，并检查类型、加密能力与派生规则。因为会先读取密钥，不能把该方法当作“缺少密钥时自动创建”的接口。

返回 `CipherResult{Ciphertext, RequestID}`。`Ciphertext` 包含 `Wrapped` 和 `Version`；SDK 校验包装版本与请求版本一致，并检查解包后的 AES-GCM 密文结构下限。此检查不是业务自行解密，也不证明数据已持久化。

业务自行保存密文，同时保存集群/Namespace/挂载/密钥名及派生 Context 的重建规则。仅有密文字符串不包含完整的业务密钥定位信息。来源：[加密实现](../transit_cipher.go#L39)。

### 5.3 `Decrypt`

**签名：** `Decrypt(context.Context, transit.DecryptRequest) (*transit.DecryptResult, error)`

恢复密文对应的原始明文。

| 请求字段 | 填什么 |
|---|---|
| `KeyName string` | 原加密密钥名称 |
| `Ciphertext transit.Ciphertext` | 原始 `Wrapped` 与一致的 `Version` |
| `Context sensitive.Bytes` | 原加密使用的派生 Context；非 derived 时为空 |

没有单独的 `KeyVersion`：使用密文包装中的版本，不能把旧密文的 Version 字段改成新版本来“升级”。预检该版本实际存在且不低于 `MinDecryptionVersion`，然后发送解密请求。

返回 `DecryptResult{Plaintext sensitive.Bytes, RequestID}`。SDK 严格解析服务端的 Base64 明文，调用者只在 error 为 nil 时读取它，并在用完后清理。密文内容被篡改、Context 不正确或服务端解密拒绝会返回 error，不使用 `Valid=false` 表示。来源：[解密实现](../transit_cipher.go#L68)。

### 5.4 `Rewrap`

**签名：** `Rewrap(context.Context, transit.RewrapRequest) (*transit.CipherResult, error)`

让服务端用旧版本解开密文，再使用**同一命名密钥**的指定目标版本重新加密；应用得到新密文，不得到明文。它适合密钥轮换后的批量密文迁移，但不是轮换密钥的方法。[OpenBao Rewrap 背景](https://openbao.org/docs/2.6.x/secrets/transit/#usage)

| 请求字段 | 填什么 |
|---|---|
| `KeyName string` | 原加密密钥名称，不用于跨名称迁移 |
| `TargetVersion int` | 已经存在的明确目标版本 |
| `Ciphertext transit.Ciphertext` | 原包装及准确源版本 |
| `Context sensitive.Bytes` | 原派生 Context |

目标版本必须存在并满足 `MinEncryptionVersion`；源版本也必须存在并满足 `MinDecryptionVersion`。结果的包装版本必须精确匹配 `TargetVersion`。返回类型与 Encrypt 相同。

**容易误解的四点：**

1. 不创建版本，不等于 `RotateKey`；目标版本由运维提前创建。
2. 不修改数据库里的原密文，也不删除旧密文；业务拿到结果并持久化成功后再更新引用。
3. SDK 没有额外强制 `TargetVersion > 源版本`。如果业务禁止回退，需要自行限制目标版本。
4. 只有 Rewrap 权限可以处理密文而不取得解密明文权限；仍需该密钥的元数据读取权限。

来源：[Rewrap 实现](../transit_cipher.go#L120)。

## 6. HMAC 生成和验证

### 6.1 `HMAC`

**签名：** `HMAC(context.Context, transit.HMACRequest) (*transit.HMACResult, error)`

使用指定密钥版本生成 SHA-256 消息认证码。适合双方共享一个认证能力、需要检查消息完整性的场景；它不是加密，也不输出普通的无密钥 SHA-256 摘要。

| 请求字段 | 填什么 |
|---|---|
| `KeyName string` | 支持 HMAC 的密钥名称，建议明确区分业务用途 |
| `KeyVersion int` | 实际可确认的正整数版本 |
| `Message sensitive.Bytes` | 原始消息字节 |

当前固定 SHA-256，不能通过请求选择 SHA-512。拒绝 derived 密钥，因为 HMAC 请求没有派生 Context 字段。SDK 不把密钥 Type 强制限制为 `hmac`；密钥是否支持该能力仍由服务端最终裁决。

版本应在元数据版本表中；如果专用 `hmac` 类型的版本表缺失或为空，只允许 `LatestVersion`。生成版本不能低于 `MinEncryptionVersion`。

返回 `HMACResult{Wrapped, Version, RequestID}`，SDK 要求包装版本匹配请求、解包后的 MAC 恰好 32 字节。保存原包装和版本，不能直接换成无版本的十六进制 MAC。来源：[HMAC 实现](../transit_hmac.go#L37)。

### 6.2 `HMACVerify`

**签名：** `HMACVerify(context.Context, transit.HMACVerifyRequest) (*transit.VerifyResult, error)`

验证消息与原 MAC 是否匹配，固定使用 SHA-256。

| 请求字段 | 填什么 |
|---|---|
| `KeyName string` | 原 HMAC 密钥名称 |
| `ExpectedVersion int` | 业务接受的明确版本 |
| `Message sensitive.Bytes` | 待验证原始消息 |
| `WrappedHMAC string` | 原 `HMACResult.Wrapped` |

要求包装中的版本与 ExpectedVersion 一致，解包 MAC 为 32 字节，密钥非 derived，版本可确认且不低于 `MinDecryptionVersion`。元数据缺失版本表时仍只能使用明确的 latest 版本。

其 HTTP 路径是 **`verify/<name>/sha2-256`，不是 `hmac/verify`**；请求体使用 `hmac` 字段，而普通验签使用 `signature` 字段。

返回 `VerifyResult{Valid, RequestID}`。合法消息/MAC 不匹配可以是 `Valid=false, error=nil`；畸形包装、版本冲突、权限、网络或响应错误返回 error。该方法由服务端检查，不是 SDK 下载共享秘密后本地比较。来源：[HMAC 验证实现](../transit_hmac.go#L83)。

## 7. 最小权限与失败处理

### 7.1 对应 ACL 能力

除了密钥读取，密码学 POST 需要精确路径上的 `update`；“只验证、不修改业务数据”不意味着 HTTP ACL 只需要 `read`。

| 能力 | keys 读取 | 额外操作路径，capability 为 update |
|---|---|---|
| 只签名 | `transit/keys/sign-key`，read | `transit/sign/sign-key` |
| 只验签 | 同上 | `transit/verify/sign-key` |
| 只加密 | `transit/keys/data-key`，read | `transit/encrypt/data-key` |
| 只解密 | 同上 | `transit/decrypt/data-key` |
| 只重新加密 | 同上 | `transit/rewrap/data-key` |
| 只生成 HMAC | `transit/keys/mac-key`，read | `transit/hmac/mac-key/sha2-256` |
| 只验证 HMAC | 同上 | `transit/verify/mac-key/sha2-256` |

例如仅允许验签的策略，不授予签名、解密或密钥配置权限：

```hcl
path "transit/keys/sign-key" {
  capabilities = ["read"]
}
path "transit/verify/sign-key" {
  capabilities = ["update"]
}
```

这里的密钥名和挂载是示例，必须替换成已授权的准确业务范围。不要为了修复 403 就授予 root、`transit/*` 的全权限或密钥管理能力。

### 7.2 错误与副作用

| 观察 | 如何理解和处理 |
|---|---|
| `invalid_argument` | 先检查名称、Profile、版本、包装、派生 Context、摘要长度和编码后请求大小；也可能来自明确服务端参数拒绝 |
| `version_unavailable` | 指定版本未在预检中被确认或低于允许版本；不自动换 latest |
| `permission_denied` | 核对元数据读权限与精确 POST 路径，尤其 HMAC 验证的 `/sha2-256` |
| `not_found_or_hidden` | 404 不能证明资源不存在，更不能据此在运行时创建密钥 |
| `invalid_response` | 服务端/代理返回格式或关键字段不符合契约；签名或版本不可信时不会交付结果 |
| `Valid=false, error=nil` | 仅表示已完成的验签/验 HMAC 否定结果，不是网络异常 |

只读元数据/公钥 GET 按白名单最多 3 次 SDK 总尝试。**所有这些密码学 POST 默认单次**，包括 Verify、Decrypt 和 HMACVerify；“读后预检重试”与“密码学 POST 重放”不是同一件事。

执行器把 Sign、SignDigest、Encrypt、Rewrap、HMAC 归为 Crypto。它们在已发送后响应丢失、超时或成功响应无法确认时可能标记 `Effect=UNKNOWN`，不要直接当作“服务端没有执行”而重放。Verify、VerifyDigest、Decrypt、HMACVerify 属于 Verification，其失败的副作用分类不同，但仍会返回实际错误并不自动重试。

本地参数拒绝或发送前取消可以是 `Effect=NONE`。预检成功不是后续 POST 成功的证明，SDK 也不保证 exactly-once。业务应分别记录确定完成、确定未执行和结果未知；不要用验签 false 吞掉网络错误。来源：[操作分类](../internal/engine/operations.go#L53)、[错误说明](error-handling.md)。

## 8. 选择方法与保存结果

| 业务场景 | 推荐流程 |
|---|---|
| 服务端签业务指令，终端/其他系统验真 | 固定序列化与域标识 → Sign → 保存 Signature 与 KeyRef → 分发可信版本公钥 |
| 已有 SHA-256 内容摘要 | 原始 32 字节摘要 → SignDigest；验证端使用对应摘要语义 |
| 加密业务秘密并落库 | Encrypt → 保存 Wrapped、Version、密钥定位与 Context 重建规则 |
| 恢复秘密 | 读取完整密文引用 → Decrypt → 使用明文 → 清理敏感缓冲区 |
| 运维已轮换密钥，业务迁移旧密文 | 明确目标版本 → Rewrap → 持久化新密文 → 受控更新原引用 |
| 共享密钥消息认证 | HMAC → 保存/传输包装和版本 → HMACVerify |

签名/消息应包含应用自己定义的用途或域标识、唯一编号、主体、有效期等防重放信息；SDK 不规定这些字段，也不保证 JSON 的任意序列化方式产生同一待签字节。

`KeyRef` 是 Transit 密钥定位，不是 KV 文档 Ref。CipherResult 和 HMACResult 没有完整 KeyRef，应用需要自行保存密钥名称及客户端作用域。RequestID 只帮助关联请求，不是“此业务操作只执行一次”的保证。

## 9. 可编译的完整调用示例

以下是辅助函数，不会自行启动客户端或准备密钥，也不会输出敏感材料。调用前必须满足：

- 根 Client 已 Start；传入有效的 Context 和 Transit 门面。
- `SigningRoundTrip` 使用已存在的非 derived `ecdsa-p256` 密钥及明确版本。
- `CipherRoundTrip` 使用已存在的非收敛 AES-GCM 密钥，源、目标版本均可用；非 derived 时传空派生 Context。
- `HMACRoundTrip` 使用可做 HMAC 的非 derived 密钥；无版本表时必须传服务端报告的 LatestVersion。
- 三个 round-trip 示例分别演示三组能力。真实业务不一定需要同一个身份同时拥有签名/验签或加密/解密全部权限；不要照示例扩大授权。
- 示例不会保存数据库记录，不能代替持久化、防重放、幂等或故障恢复设计。这里只做调用和结果判断。

```go
package transitguide

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"

	bao "github.com/RockInMars/openbao-sdk-go"
	"github.com/RockInMars/openbao-sdk-go/sensitive"
	"github.com/RockInMars/openbao-sdk-go/transit"
)

func Bind(client *bao.Client, mount string) (*bao.TransitClient, error) {
	return client.Transit(mount)
}

func SigningRoundTrip(ctx context.Context, transitClient *bao.TransitClient, name string, version int, payload []byte) (transit.Signature, error) {
	if ctx == nil || transitClient == nil {
		return transit.Signature{}, errors.New("context and transit client are required")
	}
	metadata, err := transitClient.ReadKeyMetadata(ctx, name)
	if err != nil {
		return transit.Signature{}, err
	}
	if !metadata.SupportsSigning {
		return transit.Signature{}, errors.New("key does not support signing")
	}
	publicKey, err := transitClient.ReadPublicKey(ctx, name, version)
	if err != nil {
		return transit.Signature{}, err
	}
	if len(publicKey.SPKIDER) == 0 {
		return transit.Signature{}, errors.New("public key is missing")
	}
	message := sensitive.NewBytes(payload)
	defer message.Zero()
	profile := transit.ECDSAP256SHA256ASN1
	signed, err := transitClient.Sign(ctx, transit.SignRequest{
		KeyName: name, KeyVersion: version, Profile: profile, Message: message,
	})
	if err != nil {
		return transit.Signature{}, err
	}
	verified, err := transitClient.Verify(ctx, transit.VerifyRequest{
		KeyName: name, ExpectedVersion: version, Profile: profile,
		Message: message, Signature: signed.Signature,
	})
	if err != nil {
		return transit.Signature{}, err
	}
	if !verified.Valid {
		return transit.Signature{}, errors.New("signature verification failed")
	}
	digestArray := sha256.Sum256(payload)
	digest := sensitive.NewBytes(digestArray[:])
	defer digest.Zero()
	defer clear(digestArray[:])
	digestSigned, err := transitClient.SignDigest(ctx, transit.SignDigestRequest{
		KeyName: name, KeyVersion: version, Profile: profile, Digest: digest,
	})
	if err != nil {
		return transit.Signature{}, err
	}
	digestVerified, err := transitClient.VerifyDigest(ctx, transit.VerifyDigestRequest{
		KeyName: name, ExpectedVersion: version, Profile: profile,
		Digest: digest, Signature: digestSigned.Signature,
	})
	if err != nil {
		return transit.Signature{}, err
	}
	if !digestVerified.Valid {
		return transit.Signature{}, errors.New("digest signature verification failed")
	}
	return signed.Signature, nil
}

func CipherRoundTrip(ctx context.Context, transitClient *bao.TransitClient, name string, version, targetVersion int, payload, derivationContext []byte) (transit.Ciphertext, error) {
	if ctx == nil || transitClient == nil {
		return transit.Ciphertext{}, errors.New("context and transit client are required")
	}
	plaintext := sensitive.NewBytes(payload)
	defer plaintext.Zero()
	derivation := sensitive.NewBytes(derivationContext)
	defer derivation.Zero()
	encrypted, err := transitClient.Encrypt(ctx, transit.EncryptRequest{
		KeyName: name, KeyVersion: version, Plaintext: plaintext, Context: derivation,
	})
	if err != nil {
		return transit.Ciphertext{}, err
	}
	decrypted, err := transitClient.Decrypt(ctx, transit.DecryptRequest{
		KeyName: name, Ciphertext: encrypted.Ciphertext, Context: derivation,
	})
	if err != nil {
		return transit.Ciphertext{}, err
	}
	defer decrypted.Plaintext.Zero()
	revealed := decrypted.Plaintext.RevealCopy()
	defer clear(revealed)
	if !bytes.Equal(revealed, payload) {
		return transit.Ciphertext{}, errors.New("decrypted plaintext differs")
	}
	rewrapped, err := transitClient.Rewrap(ctx, transit.RewrapRequest{
		KeyName: name, TargetVersion: targetVersion,
		Ciphertext: encrypted.Ciphertext, Context: derivation,
	})
	if err != nil {
		return transit.Ciphertext{}, err
	}
	return rewrapped.Ciphertext, nil
}

func HMACRoundTrip(ctx context.Context, transitClient *bao.TransitClient, name string, version int, payload []byte) (string, error) {
	if ctx == nil || transitClient == nil {
		return "", errors.New("context and transit client are required")
	}
	message := sensitive.NewBytes(payload)
	defer message.Zero()
	generated, err := transitClient.HMAC(ctx, transit.HMACRequest{
		KeyName: name, KeyVersion: version, Message: message,
	})
	if err != nil {
		return "", err
	}
	verified, err := transitClient.HMACVerify(ctx, transit.HMACVerifyRequest{
		KeyName: name, ExpectedVersion: version, Message: message,
		WrappedHMAC: generated.Wrapped,
	})
	if err != nil {
		return "", err
	}
	if !verified.Valid {
		return "", errors.New("HMAC verification failed")
	}
	return generated.Wrapped, nil
}
```

示例中的 `Sign` 与 `SignDigest` 分别生成并验证，不通过比较两次签名字节来证明等价。解密副本在函数退出时清理，调用者原始 `payload` 的生命周期仍由调用者管理。

## 10. OpenBao 有，但当前 SDK 没有的方法

不要因为官方 Transit 文档列出了某项能力，就认为本 SDK 可以直接调用。OpenBao 的管理端还包含密钥创建、轮换、配置、删除、导入/导出及备份恢复等能力；原生 API 也有更多算法、数据密钥、随机数、哈希和批量选项。[OpenBao 2.6.x Transit API](https://openbao.org/docs/2.6.x/api/secret/transit/)

**当前运行时 SDK 没有** `CreateKey`、`RotateKey`、`DeleteKey`、`ExportPrivateKey`、密钥配置/备份/恢复/导入接口，也没有公开的 `Hash`、`Random`、`GenerateDataKey` 或 BatchTransit 方法。这里列的是能力差别，不是提供可调用的 Go 方法签名。

如果需要创建 Transit 引擎或密钥、改变 minimum version、开启导出，应该走经授权的独立运维流程；本指南不会代为执行这些服务端操作，也不建议为了绕过运行时边界增加一个全能 RawClient。

## 11. 源码与继续阅读

- [公开请求/结果类型](../transit/types.go)：字段和 Profile 常量的准确定义。
- [签名/验证](../transit_client.go)、[密钥读取](../transit_keys.go)、[加解密](../transit_cipher.go)、[HMAC](../transit_hmac.go)：当前运行时行为。
- [公共接口契约](spec/02-接口契约.md#6-transit-契约)：接口设计与版本校验约定。
- [功能手册](sdk-feature-manual.md#6-transit签名加密和-hmac)：其他模块的入口与组合。
- [接入说明](sdk-integration-guide.md)、[认证](authentication.md)、[错误处理](error-handling.md)：生命周期、凭据管理与恢复边界。

本文新增的是说明文档，不增加任何密码学接口，不修改既有验收矩阵，也不授予下载、生产服务访问、提交、推送或再次发布权限。
