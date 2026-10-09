# KV v2 全部公开方法详解

本文解释当前 SDK 的 **1 个挂载入口、9 个网络方法、2 个 Document 构造函数和 8 个 Document 方法**，并说明全部 KV 公开结果类型。没有隐藏的通用 Put、Patch 或 Destroy 方法。

范围按当前源码 HEAD `80f758eb3625cf0af1de2cb1d410ae5f847cb3db` 核对；方法定义以 [KV 门面](../kv_client.go)、[元数据与版本操作](../kv_metadata.go)、[Document](../kv/document.go)、[公开类型](../kv/types.go) 为准。本文是接口说明，不是新的运行时验证、兼容认证或发布批准。

## 1. 先理解 KV v2

### 1.1 它存什么，与 Transit 有何不同

KV 存储可读回的秘密文档，例如应用凭据、证书材料和配置；读取成功会把明文交给调用方。Transit 则提供服务端密码运算，通常不让业务拿到其管理的私钥。不要因为 Document 默认脱敏，就误认为业务无法揭示 KV 的正文。

这里的 KV **只支持 v2**。v2 会给同一路径的写入编号；`path` 表示键，`version` 表示该键的一次版本。版本是正整数，不是应用的 `schema_version`。

例如：

| 动作 | 版本关系 |
|---|---|
| 首次创建 `apps/demo/credential` | 得到一个具体版本，例如 v1 |
| 以当前 v1 为条件 CAS 写入 | 生成 v2；不会就地修改 v1 |
| `ReadVersion(..., 1)` | 只读取 v1 |
| `ReadLatest(...)` | 读取服务端当时的最新版本 |
| 软删除 v1 | 改变 v1 的可读状态，不等于删除整个键 |
| 恢复 v1 | 恢复这个版本，不生成新的正文版本 |

写入的 Document 是**完整替换的新快照**，不是字段合并。旧文档有 `username` 和 `password`，新文档只有 `password`，则新版本不会自动保留 `username`。需要部分更新时，业务应读出、按自己的 schema 合并，再 CAS 写入完整文档；SDK 不自动实施这个流程。

### 1.2 方法总览

| 入口 / 方法 | 目的 | 结果 | 网络副作用 |
|---|---|---|---|
| `Client.KVv2(mount)` | 绑定已有 KV v2 挂载 | `*bao.KVClient` | 无网络 |
| `Create(ctx, path, data)` | 只创建原先没有版本的键 | `*kv.WriteResult` | 写入 |
| `CompareAndSwap(ctx, path, expected, data)` | 当前版本匹配时写入新版本 | `*kv.WriteResult` | 写入 |
| `ReadVersion(ctx, path, version)` | 读取指定版本 | `*kv.ReadResult` | 只读 |
| `ReadLatest(ctx, path)` | 明确读取最新版本 | `*kv.ReadResult` | 只读 |
| `ReadRef(ctx, ref)` | 按绑定的完整引用读取指定版本 | `*kv.ReadResult` | 只读 |
| `ReadMetadata(ctx, path)` | 查看键配置及版本状态 | `*kv.Metadata` | 只读 |
| `List(ctx, prefix)` | 列出一个目录的直接子项 | `*kv.ListResult` | 只读 |
| `DeleteVersions(ctx, path, versions)` | 软删除明确的版本 | `error` | 写入 |
| `UndeleteVersions(ctx, path, versions)` | 恢复明确的软删除版本 | `error` | 写入 |

有结果的网络方法还返回 `error`；`DeleteVersions` 和 `UndeleteVersions` 只返回 `error`。方法签名固定见 [公共契约测试](../public_contract_test.go)。

## 2. 入口、路径与共同规则

### 2.1 `Client.KVv2`

签名：`func (c *Client) KVv2(mount string) (*KVClient, error)`。

- `mount` 是挂载名称或路径，例如 `secret`、`team/secret`，不是完整 URL，也不是某个数据键。
- 返回的 KVClient 固定使用该 Client 的地址、集群别名、Namespace、身份和挂载。
- 只做本地绑定与路径校验；不创建挂载、不检测 v1/v2、不证明权限或服务就绪。
- 网络调用前，外层 Client 需要成功 `Start(ctx)`；不能使用自行构造的零值 KVClient。
- nil Client 或不合法挂载返回参数错误。KVClient 没有独立 `Close`；外层 Client 负责生命周期，关闭也不会删除 KV 数据。

初始化、认证及 TLS 配置见 [接入说明](sdk-integration-guide.md) 和 [认证说明](authentication.md)。

### 2.2 参数传业务路径，ACL 写服务端路径

以挂载 `secret`、键 `apps/demo/credential` 为例：

| 场景 | 路径 |
|---|---|
| `Client.KVv2` 参数 | `secret` |
| `Create / ReadVersion` 等参数 | `apps/demo/credential` |
| SDK 生成的数据 API | `/v1/secret/data/apps/demo/credential` |
| SDK 生成的元数据 API | `/v1/secret/metadata/apps/demo/credential` |
| ACL policy 路径 | `secret/data/apps/demo/credential` 等，不含 `/v1/` |

**不要自行给 SDK 参数加 `data/` 或 `metadata/`。** 它会被当成业务键的一部分，导致访问另一个路径。它也不会去掉你意外重复的挂载前缀。

当前 [路径校验](../internal/engine/path.go) 比服务端允许的命名范围更严格：

- 每段只允许 ASCII 字母、数字、`_`、`-`、`.`，段之间用单个 `/`。
- 不允许空段、`.`、`..`、末尾为点的段、开头/结尾斜杠、重复斜杠。
- 不允许空格、中文、`%`、`?`、`#`、反斜杠或完整 URL。
- `List(ctx, "")` 是唯一允许空资源路径的例外，表示挂载根目录。
- 传给 List 的非空前缀也不能带尾部斜杠，例如应传 `apps/demo`，不是 `apps/demo/`。

SDK 不会把不合法路径“修好再发”。业务仍必须先做租户和资源授权；通过路径校验不等于获得业务授权。

### 2.3 Context、预算、限制与重试

全部网络方法的 `ctx` 必须非 nil。SDK 将调用方 deadline 与操作预算结合；等待凭据、并发、传输和只读重试共用预算，不给每次重试重新增加完整时限。

当前默认普通请求预算为 10 秒，请求体上限 1 MiB、响应体上限 2 MiB；配置可改变，见 [配置实现](../config.go)。KV 写入不仅检查正文大小，还受 `options`、`data` 等外层包装后的请求体大小限制，因此正文恰好达到上限也未必能发送。

- 只读方法默认最多 **3 次总尝试**，不是“先读 1 次再重试 3 次”；只对执行器认可的暂时性失败重试。
- Create、CAS、删除、恢复默认只发 **1 次 SDK 业务请求**。
- 403 不会被解释成“自动重登并重放这次写入”。
- SDK 无法证明外部代理没有隐藏重放。`Attempts` 是 SDK 的计数，不是所有链路线级请求的总数。

这不是每个方法各自实现的网络逻辑，而是统一执行器的约束；详见 [错误处理](error-handling.md)。

## 3. 写入方法

### 3.1 `Create`：仅创建，不覆盖

签名：`func (k *KVClient) Create(ctx context.Context, path string, data kv.Document) (*kv.WriteResult, error)`。

**参数**

- `path`：相对挂载的键路径。
- `data`：有效、尚未 Zero 的 Document，根必须是 JSON 对象；`{}` 可用，零值 Document 不可用。

**实际行为**

向 `/v1/{mount}/data/{path}` 发送 POST，SDK 强制带 `options.cas=0`：

```json
{"options":{"cas":0},"data":{"schema_version":1,"label":"example"}}
```

这不是普通覆盖写。软删除通常仍保留键的版本信息，因此“读不到正文”不等于可以再次 Create。确认的 CAS 不匹配错误会映射为 `baoerr.CodeCASConflict`；其他 400 不会被一律归类为冲突。

**成功结果与边界**

- 返回准确的 `WriteResult.Ref`、创建时间、请求 ID 和尝试次数。
- SDK 要求响应版本为正整数、创建时间有效；不要自己假设结果一定是 v1，使用返回的版本。
- 传入的 Document 仍归调用方所有，SDK 清理自己的临时副本，不替你 Zero 原始 Document。
- 已发送后超时或响应损坏，可能返回 `EffectUnknown`：服务端可能已经创建成功，不能直接重跑 Create 或换路径逃避核对。

适合新资源首次写入，不适合作为“存在就更新”的 upsert。

### 3.2 `CompareAndSwap`：带当前版本条件的完整更新

签名：`func (k *KVClient) CompareAndSwap(ctx context.Context, path string, expected int, data kv.Document) (*kv.WriteResult, error)`。

`expected` 必须是正整数且不能为 `math.MaxInt`；**0 不能用来表示无条件覆盖或首次创建**，首次创建应调用 Create。

例如，当前版本是 7：

1. 业务基于 v7 的完整正文，生成更新后的 Document。
2. 调用 `CompareAndSwap(ctx, path, 7, document)`。
3. 服务端只在当前版本仍为 7 时接受；成功生成 v8。
4. SDK 要求响应版本恰好等于 `expected+1`，否则报告响应错误，而不是把任意版本算成功。

两位写入者同时基于 v7 更新，CAS 能阻止二者都覆盖同一代数据。失败者应重新读取并按业务规则决定是否合并、拒绝或重新尝试；SDK 不会默默读取新版本再覆盖。

**CAS 不提供的保证**

- 它不是字段级合并，也不把 KV 与数据库提交组成事务。
- 它不是跨请求、跨代理的 exactly-once 承诺。
- 响应丢失后，版本增加了也不证明是你的写入。应结合受控业务 operation ID、任务所有权及正文核对；只看 latest 版本号不足以归因。
- CAS 可以为已有键写入新版本，但不会“恢复”已经删除的旧版本；旧版本恢复是 UndeleteVersions 的事情。

## 4. 读取方法

### 4.1 `ReadVersion`：准确版本，不回退

签名：`func (k *KVClient) ReadVersion(ctx context.Context, path string, version int) (*kv.ReadResult, error)`。

`version` 必须 `>0`。发送 `GET /v1/{mount}/data/{path}?version=N`；**不能传 0 来读 latest**。

成功必须同时满足：

- 返回的版本与请求完全相同。
- 版本创建时间、销毁标记和删除时间形态有效。
- 没有明确销毁，也没有已经到达的删除时间。
- 正文是可校验的 JSON 对象。

返回其他版本属于 `CodeInvalidResponse`，SDK 不会悄悄接受。指定版本已不可用时，也不会回退到较旧或较新的版本。

使用场景：按业务持久化的准确版本读取历史凭据或证书材料；其中的秘密通过 `result.Data.Decode(...)` 或显式 `RevealJSON()` 使用，用完应 `result.Data.Zero()`。

### 4.2 `ReadLatest`：明确追踪最新版本

签名：`func (k *KVClient) ReadLatest(ctx context.Context, path string) (*kv.ReadResult, error)`。

访问同一个 data 路径，但不发送 `version` 参数。成功时返回服务端选出的版本，并把这个准确版本写入 `ReadResult.Ref`。

- 适合主动追踪最新配置或发现当前版本。
- 不适合替代历史业务引用；只保存 path 可能在轮换后读取另一份秘密。
- 最新版本已软删除时，不承诺自动寻找“最近一个未删除版本”。
- 两次 ReadLatest 之间可能发生更新；Metadata 与读取之间也不是原子快照。
- SDK 不因此承诺任意复制拓扑的强一致性，见 [总体设计](spec/01-总体设计.md)。

即便只想拿 Ref，ReadLatest 也会读取秘密正文，仍应清理返回的 Data。只检查版本状态应优先考虑 ReadMetadata。

### 4.3 `ReadRef`：绑定身份边界的准确读取

签名：`func (k *KVClient) ReadRef(ctx context.Context, ref kv.Ref) (*kv.ReadResult, error)`。

先在本地核对：

| Ref 字段 | 必须匹配 |
|---|---|
| `ClusterAlias` | 当前 Client 的集群别名 |
| `Namespace` | 当前 Client 的 Namespace 路径 |
| `Mount` | 当前 KVClient 的固定挂载 |

然后用 `ref.Path` 和 `ref.Version` 调用准确版本读取。绑定不匹配在发送前拒绝；路径与版本仍要通过普通校验。

示例引用形态：

```json
{
  "cluster_alias": "sdk-demo",
  "namespace": "",
  "mount": "secret",
  "path": "apps/demo/credential",
  "version": 7
}
```

这里空 Namespace 表示示例的根 Namespace，不是让 SDK 自动选择根空间。业务应保存 SDK 返回的 Ref，不应把外部输入随意改写成“匹配的引用”。

Ref 不包含秘密正文，但不是访问凭证，也不是服务端签名、数据摘要或永远有效的地址。集群别名是本地配置标识，不会从 Ref 中解析远端 URL。调用方仍需授权资源，并保证别名映射正确、版本保留策略可满足历史引用。

## 5. 元数据与列表

### 5.1 `ReadMetadata`：状态，不是秘密正文

签名：`func (k *KVClient) ReadMetadata(ctx context.Context, path string) (*kv.Metadata, error)`。

发送 `GET /v1/{mount}/metadata/{path}`，不获取 Document。返回字段见第 8 节；核心用途是查看：

- 当前版本与最老保留版本。
- 该键配置的版本保留、CAS 与自动删除参数。
- 每个可见版本的创建时间、删除时间和销毁标记。
- 键级自定义元数据。

**不要误读这两个字段**

1. `DeletedAt` 保存原始 `deletion_time`。nil 表示没有这个时间；未来时间可能是计划自动删除的截止时间，**非 nil 不等于已经删除**。当前读取实现仅在销毁或删除时间已到时判定版本不可用；临界时间还受本地时钟判断影响，不能把一次检查视为长期有效。
2. `CustomMetadata` 是当前键级信息，**不是各版本的不可变历史事实**。属于正文版本的业务 generation、schema 或 operation ID，应按业务契约保存在版本化正文及业务记录中。

`Versions` 按版本号升序排列；不要用 slice 下标充当版本号，保留策略可能产生缺口。它不是“从 v1 到当前的完整永恒历史”。

Metadata 读取也不是挂载配置查询。键级 `MaxVersions=0` 或 `DeleteVersionAfter` 的值，不能单独推导所有挂载级有效策略；需要运维核对继承配置。SDK 不提供修改这些策略的管理接口。

SDK 拒绝非法字段类型、负数版本配置、最老版本大于当前版本、非法时间/时长及非规范版本键等响应，不会用零值静默掩盖形态错误。

### 5.2 `List`：目录的直接子项

签名：`func (k *KVClient) List(ctx context.Context, prefix string) (*kv.ListResult, error)`。

当前 SDK 实际使用 **GET + `list=true`**，不是原始 LIST 动词：

| 调用 | 实际请求 |
|---|---|
| `List(ctx, "")` | `GET /v1/{mount}/metadata/?list=true` |
| `List(ctx, "apps/demo")` | `GET /v1/{mount}/metadata/apps/demo/?list=true` |

例如服务端 keys 为 `["credential","old/"]`，SDK 返回：

| Name | IsFolder |
|---|---|
| `credential` | `false` |
| `old` | `true` |

目录 Name 已去掉尾部斜杠；要继续列目录，应组合成 `apps/demo/old`，而不是把 `old` 当成挂载根下的路径。

- 只列一层，不自动递归、读取正文或过滤成“可读取的有效版本”。
- 结果按 Name 排序；同名键与目录可同时存在，键排在目录前。
- 服务端相同原始条目重复、非法名称或 keys 形态错误会被拒绝。
- `404` 不会转换为成功的空列表。成功的空 entries 与失败是不同结果。
- 知道一个名称不表示有读正文权限；同样，能读一个键也不表示能列整个目录。

OpenBao 默认列举不会按各键的访问策略筛掉名称，路径命名也应避免秘密。运维可在策略中显式配置 `list_scan_response_keys_filter_path` 过滤返回项；SDK 本身不额外执行这种过滤，也不能假设每个部署的策略相同。参见 [KV v2 API](https://openbao.org/docs/2.6.x/api/secret/kv/kv-v2/#list-secrets) 和 [策略过滤说明](https://openbao.org/docs/2.6.x/concepts/policies/#filtering-list-or-scan-results)。这些服务端能力不意味着本 SDK 提供递归 SCAN 或详细目录元数据接口。

## 6. 删除与恢复

### 6.1 `DeleteVersions`：软删除指定版本

签名：`func (k *KVClient) DeleteVersions(ctx context.Context, path string, versions []int) error`。

发送 `POST /v1/{mount}/delete/{path}`，正文为明确的版本集合：

```json
{"versions":[2,5]}
```

`versions` 必须非空、每项为正整数且不重复。`nil`、`[]`、`[0]`、`[-1]`、`[2,2]` 都是本地参数错误。SDK 拷贝版本列表后编码，不替业务自动选择当前版本。

软删除用于暂时停止指定版本的正常读取，不是永久销毁，也不是：

- 删除最新版本专用的 `DELETE /data/{path}`。
- 删除整个键的全部正文和元数据。
- 自动取消数据库引用、撤销外部凭据或吊销证书。

成功只返回 nil，**没有每个版本的状态收据**。不存在的版本、已删除版本或并发操作的最终状态，不能仅由“返回 nil”推导出来。需要确认时，按业务权限读取 Metadata 或准确版本；有未知写结果时先核对，不盲目重放。

### 6.2 `UndeleteVersions`：恢复指定软删除版本

签名：`func (k *KVClient) UndeleteVersions(ctx context.Context, path string, versions []int) error`。

发送 `POST /v1/{mount}/undelete/{path}`，输入校验与 DeleteVersions 相同。

- 目标是原有版本，不产生新的正文版本。
- 不把恢复 v2 变成“令当前版本倒退到 v2”。
- 不能恢复已永久销毁、被版本保留策略清理或已被整个键删除的数据。
- 返回 nil 仍不代替目标版本读取确认；`UndeleteVersions` 成功而后续 `ReadVersion` 失败，是两个不同操作的结果。
- 若原引用就是 v2，应恢复后准确读取 v2，不改用 ReadLatest 来掩盖恢复问题。

软删除与永久销毁的区别，以及保留策略可能永久清理旧版本的行为，参见 [OpenBao KV v2 删除说明](https://openbao.org/docs/2.6.x/secrets/kv/kv-v2/#deleting-and-destroying-data)。业务不能只凭“有 Ref”就承诺历史版本一定能恢复。

## 7. Document：2 个构造函数与 8 个方法

Document 是保存经过校验的 JSON 对象字节的秘密容器，不是普通 `map[string]any`。它提供本地校验、受控揭示和默认输出脱敏，**不执行服务端写入，也不替业务校验 schema**。

### 7.1 `kv.ParseDocument`：已经有 JSON 字节

签名：`func ParseDocument(raw []byte) (Document, error)`。

校验成功后独立复制输入并保留原始 JSON 对象字节，包括数字表示；不会清理调用方的 `raw`。

拒绝：

- 空输入、非法 UTF-8、不合法或有损的 Unicode 转义。
- 根为数组、字符串、数字或 null；对象内可以有数组等普通 JSON 值。
- 任意层级的重复对象键。
- 额外尾随 JSON 值及超出当前 64 层边界的嵌套结构。

ParseDocument 本身不附加固定的正文字节上限；网络方法按配置检查大小。因此业务在处理任意大输入前仍应设置自己的接入限制。

适用于来自文件、消息或协议的**原始 JSON 对象**。调用方用完输入 buffer 应自己清理；ParseDocument 不取得原始 buffer 的所有权。

### 7.2 `kv.NewDocument`：已经有 Go 值

签名：`func NewDocument(value any) (Document, error)`。

先用 `encoding/json` 编码 Go 值，再走同一对象校验。适合带 JSON 标签的业务 struct 或对象 map；不是任意 Go 值都能成为 Document。

- 直接传普通字符串或 `[]byte` 不等于传入原始 JSON 对象；已有 JSON 请用 ParseDocument。
- 支持业务字段内的 `json.Number`，但无法恢复 value 中已有 float64 的精度损失。
- 文档生成与校验失败返回普通安全错误，不公开原始正文。
- 编码的临时 buffer 会尽力清理；调用方的 struct、string、map 和 slice 仍归调用方管理。

**数字重要边界**：本地 Document 的精确数字能力，不等于服务端端到端无损。固定 2.6.3 后端已记录大整数往返精度限制；例如超过安全表示范围的业务标识应按契约用 JSON **字符串**保存，见 [兼容性说明](compatibility.md#数字与签名协议边界)。

### 7.3 `Document.Decode`：转换为业务值

签名：`func (d Document) Decode(dst any) error`。

`dst` 应为有效的可写指针，例如 `&credential`、`&values`。

- 解码器启用 `UseNumber`；解码到 `map[string]any` 时，JSON 数字使用 `json.Number`，不是默认 float64。
- 解码到有类型的 struct 时，字段类型仍决定范围与精度：int 的溢出会失败，float64 仍有其精度限制。
- 不启用 `DisallowUnknownFields`；不认识的 struct 字段不会自动被当成 schema 错误。
- 空的或已 Zero 的 Document 不能 Decode。
- 解码失败返回普通安全错误，调用方应丢弃可能已部分赋值的目标，不把它当成成功数据。
- 方法清理自身揭示的临时字节，但**不会清理 dst 中的秘密字符串、slice 或后续复制**。

因此 Decode 成功后，业务仍应检查 schema 版本、必填字段、格式和资源归属。Zero Document 不会替你清理解码后的对象。

### 7.4 `Document.RevealJSON`：显式明文出口

签名：`func (d Document) RevealJSON() []byte`。

返回独立的明文 JSON 副本。调用方负责在使用后 `clear(raw)`；不能把这份副本直接打印、写日志或放入 Observer。空的或清理后的 Document 返回空内容，而不是一个有效的 `{}`。

副本可以修改而不改变 Document；相反，Document.Zero 不会追踪并清理已经返回的副本。把副本转成 string 又会产生新的明文表示，不能再把它视为受 Document 管理。

正常 KV 发送已由 SDK 内部处理，业务不需要为了 Create/CAS 手工 RevealJSON。

### 7.5 `Document.Zero`：清理共享句柄

签名：`func (d *Document) Zero()`。

- 尽力清理 Document 持有的 buffer；nil 指针调用为无操作。
- **值复制共享清理句柄**：`copy := document` 不是独立秘密副本，一个 Zero 后，另一个不能再依赖正文。
- 不会清理 ParseDocument 的原始输入、NewDocument 的原始 Go 值、RevealJSON 的副本或 Decode 的目标。
- 无法承诺 Go 运行时、GC、栈复制或历史 string 副本被彻底擦除。
- 所有仍依赖文档的操作结束后再 Zero；不要一个 goroutine 提前清理而另一个仍指望使用同一文档。

需要独立 Document 时，应显式取得字节副本、重新 Parse，并清理中间副本，而不是简单赋值。

### 7.6 输出保护方法：`String`、`GoString`、`Format`、`LogValue`、`MarshalJSON`

这 5 个公开方法主要由 Go 的格式化和序列化设施调用；它们不发网络请求。

| 方法与签名 | 行为 |
|---|---|
| `String() string` | 返回 `[REDACTED]`，普通字符串格式化不输出正文 |
| `GoString() string` | 返回 `[REDACTED]`，Go 风格表示不输出正文 |
| `Format(state fmt.State, verb rune)` | 对 fmt 格式化输出脱敏，不因更换格式 verb 揭示正文 |
| `LogValue() slog.Value` | 向结构化 slog 提供脱敏字符串 |
| `MarshalJSON() ([]byte, error)` | 返回错误，要求显式揭示；不是返回正文或成功输出 null |

普通 `json.Marshal(document)` 会失败；直接序列化包含 Data 的 ReadResult 也不能借此输出秘密。需要持久化定位信息时，单独保存 Ref，而不是完整 ReadResult。

这些保护不延伸到已经 Decode 的业务对象或 RevealJSON 的明文字节，也不替业务证明整个应用日志不会泄漏。

来源：[Document 实现](../kv/document.go)、[严格 JSON 校验](../internal/jsondoc/json.go)、[Unicode 校验](../internal/jsondoc/unicode.go)、[共享秘密句柄](../sensitive/bytes.go)。

## 8. 所有 KV 公开数据类型

### 8.1 `kv.Ref`

| 字段 | 类型 | 含义 |
|---|---|---|
| `ClusterAlias` | string | 本地配置中的集群标识 |
| `Namespace` | string | 固定 Namespace 路径；根空间为 "" |
| `Mount` | string | KV 挂载路径 |
| `Path` | string | 相对挂载的键路径 |
| `Version` | int | 准确版本 |

JSON 字段分别为 `cluster_alias`、`namespace`、`mount`、`path`、`version`。Ref 没有独立的公开 Validate、Read 或持久化方法；校验由 ReadRef 等调用执行，序列化与数据库设计由业务负责。

### 8.2 `kv.WriteResult` 与 `kv.ReadResult`

| 字段 | WriteResult | ReadResult | 含义 |
|---|---|---|---|
| `Ref` | 有 | 有 | 本次成功结果的完整准确引用 |
| `Data` | 无 | `kv.Document` | 读取出的秘密正文，归调用方清理 |
| `CreatedAt` | 有 | 有 | 这个正文版本的创建时间，不是本次读取时间 |
| `RequestID` | 有 | 有 | 受控响应中的请求标识，不能代替业务 operation ID |
| `Attempts` | 有 | 有 | 本次 SDK 操作尝试次数 |

结果没有公开的事务提交状态或额外 Effect 字段；错误的副作用语义通过 `baoerr.Error` 表达。写入成功也不会替业务保存 Ref 到自己的数据库。

### 8.3 `kv.Metadata` 与 `kv.VersionMetadata`

| Metadata 字段 | 类型 | 含义 |
|---|---|---|
| `CurrentVersion` | int | 当前版本号，不代表它一定可读 |
| `OldestVersion` | int | 服务端报告的最老保留版本 |
| `MaxVersions` | int | 该键报告的版本保留配置 |
| `CASRequired` | bool | 该键报告的 CAS 配置；本 SDK 写入总带 CAS |
| `DeleteVersionAfter` | time.Duration | 该键报告的自动删除时长 |
| `CustomMetadata` | map[string]string | 键级可变自定义元数据，可为 nil |
| `Versions` | []kv.VersionMetadata | 版本状态，按版本号升序 |
| `RequestID` | string | 请求标识 |

每项 VersionMetadata 有 `Version int`、`CreatedAt time.Time`、`DeletedAt *time.Time`、`Destroyed bool`。读取时没有额外提供 `IsDeleted` 布尔值；须正确解释 DeletedAt，不把未来计划直接当作已删除。

Metadata、ListResult **没有 Attempts 字段**；不要把 WriteResult 的所有字段套用到它们。

### 8.4 `kv.ListEntry` 与 `kv.ListResult`

`ListEntry` 只有 `Name string`、`IsFolder bool`。`ListResult` 只有 `Entries []ListEntry`、`RequestID string`。条目没有版本、正文、权限证明或完整绝对路径。

Document 加上上述 7 个结构体就是当前 kv 包的公开数据类型；它们没有额外的网络方法。

## 9. 最小 ACL 路径对照

以下是挂载 `secret`、准确键 `apps/demo/credential`、目录 `apps/demo` 的权限对照；它说明服务端 ACL，不授权业务自己扩展范围。

| SDK 操作 | policy 路径 | 能力 |
|---|---|---|
| Create 新键 | `secret/data/apps/demo/credential` | `create` |
| CompareAndSwap 已有键 | `secret/data/apps/demo/credential` | `update` |
| ReadVersion / ReadLatest / ReadRef | `secret/data/apps/demo/credential` | `read` |
| ReadMetadata | `secret/metadata/apps/demo/credential` | `read` |
| List apps/demo | `secret/metadata/apps/demo/` | `list` |
| DeleteVersions | `secret/delete/apps/demo/credential` | `update` |
| UndeleteVersions | `secret/undelete/apps/demo/credential` | `update` |

注意删除指定版本的能力是 `update`，不是 data 路径上的 `delete`；后者属于另一种服务端操作。当前 SDK 用 GET 搭配 `list=true`，仍按列表能力授权，不因此变成普通 metadata read。

List 的请求会规范化成目录前缀，表中保留 policy 路径的尾部斜杠；这与 SDK 参数 `apps/demo` 不能带尾部斜杠是两件事。根目录列表对应 `secret/metadata/`。若策略用 `secret/metadata/apps/demo/*` 覆盖更多目录，应明确审查扩大的列表范围，而不是给整个挂载通配权限来解决 403。依据见 [官方策略路径规则](https://openbao.org/docs/2.6.x/concepts/policies/#policy-syntax)。

同一身份承担多项操作时，按实际需要组合能力。只读消费者不应获得写入、恢复或管理能力；业务不能把“拥有 KV ACL”当成自己的用户/租户身份校验。

ACL 路径依据 [官方 KV v2 权限说明](https://openbao.org/docs/2.6.x/secrets/kv/kv-v2/#acl-rules)。若还需要挂载级配置或管理权限，应由运维单独设计，不能在业务 SDK 中补全 root 权限。

## 10. 错误处理：冲突、不可用与未知结果

### 10.1 网络错误用稳定分类，不匹配文本

| 稳定错误码 / 判断 | 应怎样理解 |
|---|---|
| `CodeInvalidArgument` | 参数、文档/请求大小或确认的 API 参数拒绝；检查输入与配置 |
| `CodeCASConflict` | 已确认 CAS 条件不匹配；Create 也可能返回，不假设 HTTP 409 |
| `CodeVersionUnavailable` | 有明确版本已删除/销毁证据；不寻找另一版冒充原引用 |
| `CodeNotFoundOrHidden` | 404 类结果：不存在、不可见或其他未确认情形，不能断言资源不存在 |
| `CodePermissionDenied` | 确认的 403 类拒绝；不自动扩大权限或重登重放 |
| `CodeInvalidResponse` | 响应形态、版本或时间等不符合契约；不能静默吃掉错误 |
| `CodeNotReady / CodeClosed` | 外层 Client 生命周期不允许本次操作 |
| `CodeCanceled / CodeDeadlineExceeded` | 被取消或耗尽预算；写请求还须看 Effect |
| `baoerr.HasUnknownOutcome(err)` | 写入可能已经发生，应先核对再决定动作 |

只在 API 400 的明确单条 CAS 不匹配形态成立时映射冲突，**不是所有 400 都是 CASConflict**。版本不可用也要求明确证据；普通 404 不会被当成“确定已经删除”。

网络错误可用 `baoerr.IsCode`、`baoerr.HasUnknownOutcome` 或 `errors.As` 检查。Document 构造/Decode 属于本地操作，返回普通错误，不承诺统一为 `*baoerr.Error`。

### 10.2 `Code` 与 `Effect` 不是一回事

- `EffectNone`：本次操作无副作用，或有确认的拒绝证据。
- `EffectUnknown`：已发送的写可能成功，但无法确认；即使错误码是 deadline 或 invalid_response，也不能理解成“没写进去”。
- `EffectConfirmed` 是公共错误模型的一部分，不是 KV 结果结构体里一个额外可读取的提交字段。

业务把 Create/CAS 与数据库保存引用组合时，应准备持久化任务身份、generation、恢复步骤和回滚职责。读取 Metadata 后发现版本改变，只能帮助核对，不能证明作者是谁。详情见 [错误处理](error-handling.md) 与 [接入及回滚](integration-and-rollback.md)。

## 11. 可编译的组合示例

下面只有函数定义，没有 main、认证初始化或自动执行写入。调用者提供已 Start 的 Client、受授权的路径与有效 context；外部身份和初始化见接入文档。

- CreateCredential 与 ReplaceCredential 各自只有一次写入，不把失败转成盲目重试。
- ReadCredential 用业务保存的 Ref；ReadExactCredential 用明确版本。
- FindCurrentReference 会读到正文，即便只返回 Ref 也清理 Data。
- InspectKey 是两次独立只读调用，不是原子快照。
- RestoreAndReadCredential 是“恢复 + 准确读取”两步：后一步失败不表示前一步没执行。
- Credential 中的 Secret、解码后的业务对象及原始 JSON 仍由调用方管理。ExternalID 使用 string 避免大整数往返问题。
- OperationID 只是示例业务字段，SDK 不赋予它服务端幂等键语义。
- DecodeLocalJSON 展示显式揭示与清理；这里只为说明所有权，正常 Decode 无需先 RevealJSON。
- ClassifyFailure 仅分类，不自动执行恢复、刷新或重放。

```go
package kvguide

import (
	"context"
	"errors"

	bao "github.com/RockInMars/openbao-sdk-go"
	"github.com/RockInMars/openbao-sdk-go/baoerr"
	"github.com/RockInMars/openbao-sdk-go/kv"
)

type Credential struct {
	SchemaVersion int    `json:"schema_version"`
	OperationID   string `json:"operation_id"`
	Secret        string `json:"secret"`
	ExternalID    string `json:"external_id"`
}

func BindStore(client *bao.Client, mount string) (*bao.KVClient, error) {
	if client == nil {
		return nil, errors.New("client is required")
	}
	return client.KVv2(mount)
}

func requireInputs(ctx context.Context, store *bao.KVClient) error {
	if ctx == nil || store == nil {
		return errors.New("context and store are required")
	}
	return nil
}

func CreateCredential(ctx context.Context, store *bao.KVClient, path string, value Credential) (kv.Ref, error) {
	if err := requireInputs(ctx, store); err != nil {
		return kv.Ref{}, err
	}
	document, err := kv.NewDocument(value)
	if err != nil {
		return kv.Ref{}, err
	}
	defer document.Zero()
	result, err := store.Create(ctx, path, document)
	if err != nil {
		return kv.Ref{}, err
	}
	return result.Ref, nil
}

func ReplaceCredential(ctx context.Context, store *bao.KVClient, path string, expectedVersion int, value Credential) (kv.Ref, error) {
	if err := requireInputs(ctx, store); err != nil {
		return kv.Ref{}, err
	}
	document, err := kv.NewDocument(value)
	if err != nil {
		return kv.Ref{}, err
	}
	defer document.Zero()
	result, err := store.CompareAndSwap(ctx, path, expectedVersion, document)
	if err != nil {
		return kv.Ref{}, err
	}
	return result.Ref, nil
}

func decodeCredential(result *kv.ReadResult) (Credential, error) {
	defer result.Data.Zero()
	var value Credential
	if err := result.Data.Decode(&value); err != nil {
		return Credential{}, err
	}
	return value, nil
}

func ReadCredential(ctx context.Context, store *bao.KVClient, ref kv.Ref) (Credential, error) {
	if err := requireInputs(ctx, store); err != nil {
		return Credential{}, err
	}
	result, err := store.ReadRef(ctx, ref)
	if err != nil {
		return Credential{}, err
	}
	return decodeCredential(result)
}

func ReadExactCredential(ctx context.Context, store *bao.KVClient, path string, version int) (Credential, error) {
	if err := requireInputs(ctx, store); err != nil {
		return Credential{}, err
	}
	result, err := store.ReadVersion(ctx, path, version)
	if err != nil {
		return Credential{}, err
	}
	return decodeCredential(result)
}

func FindCurrentReference(ctx context.Context, store *bao.KVClient, path string) (kv.Ref, error) {
	if err := requireInputs(ctx, store); err != nil {
		return kv.Ref{}, err
	}
	result, err := store.ReadLatest(ctx, path)
	if err != nil {
		return kv.Ref{}, err
	}
	defer result.Data.Zero()
	return result.Ref, nil
}

func InspectKey(ctx context.Context, store *bao.KVClient, path string, prefix string) (*kv.Metadata, *kv.ListResult, error) {
	if err := requireInputs(ctx, store); err != nil {
		return nil, nil, err
	}
	metadata, err := store.ReadMetadata(ctx, path)
	if err != nil {
		return nil, nil, err
	}
	children, err := store.List(ctx, prefix)
	if err != nil {
		return metadata, nil, err
	}
	return metadata, children, nil
}

func SoftDeleteKnownVersions(ctx context.Context, store *bao.KVClient, path string, versions []int) error {
	if err := requireInputs(ctx, store); err != nil {
		return err
	}
	return store.DeleteVersions(ctx, path, versions)
}

func RestoreAndReadCredential(ctx context.Context, store *bao.KVClient, path string, version int) (Credential, error) {
	if err := requireInputs(ctx, store); err != nil {
		return Credential{}, err
	}
	if err := store.UndeleteVersions(ctx, path, []int{version}); err != nil {
		return Credential{}, err
	}
	return ReadExactCredential(ctx, store, path, version)
}

func DecodeLocalJSON(raw []byte) (Credential, error) {
	document, err := kv.ParseDocument(raw)
	if err != nil {
		return Credential{}, err
	}
	defer document.Zero()
	revealed := document.RevealJSON()
	defer clear(revealed)
	var value Credential
	if err := document.Decode(&value); err != nil {
		return Credential{}, err
	}
	return value, nil
}

func ClassifyFailure(err error) string {
	switch {
	case err == nil:
		return "success"
	case baoerr.HasUnknownOutcome(err):
		return "reconcile-before-any-retry"
	case baoerr.IsCode(err, baoerr.CodeCASConflict):
		return "refresh-and-decide"
	case baoerr.IsCode(err, baoerr.CodeVersionUnavailable):
		return "handle-the-exact-version"
	case baoerr.IsCode(err, baoerr.CodeNotFoundOrHidden):
		return "investigate-path-identity-and-permissions"
	default:
		return "fail-without-blind-replay"
	}
}
```

这些示例不包括数据库持久化、租户授权、schema 校验或业务幂等协议；调用方需要补充它们，不能把示例直接当成生产事务。本文示例编译不等于执行真实 OpenBao 生命周期。

## 12. 当前 SDK 不提供什么

不要把官方 KV API 的所有端点误当成本 SDK 的所有方法。以下不在当前运行时公开范围：

- KV v1 客户端或自动检测并迁移挂载。
- 无条件 Put/覆盖写、JSON Merge Patch、自动 read-modify-write 回退。
- 自动删除最新版本的快捷接口。
- 永久 Destroy、删除全部 metadata、修改版本保留或自动删除配置。
- 挂载创建/配置、修改 custom_metadata。
- subkeys、递归 SCAN、detailed-metadata 或任意 RawRequest。
- 自动恢复删除版本、准确版本失败后回退 latest。
- 与业务数据库的跨系统事务或 exactly-once 保证。

这是运行时边界，不是遗漏方法。相关维护操作应使用单独授权、单独审计的管理流程；本 SDK 不因文档示例而扩大权限。

## 13. 最重要的选择规则

1. 新资源用 **Create**，已有资源条件更新用 **CompareAndSwap**，两者都不无条件覆盖。
2. 历史业务关系保存 **Ref** 并用 **ReadRef**；只有确实追踪最新时才用 ReadLatest。
3. 版本状态用 **ReadMetadata**，目录子项用 **List**；它们不返回秘密正文，也不证明业务访问权限。
4. 暂时停用用 **DeleteVersions**，恢复明确版本用 **UndeleteVersions**；软删除不等于永久销毁。
5. Go 对象用 **NewDocument**，原始 JSON 用 **ParseDocument**；Decode 与 RevealJSON 都是秘密出口。
6. 正文用完清理；日志脱敏不保护已揭示的副本。
7. CAS 冲突需重新决策，未知写结果需先核对；都不应该自动改为无条件写或重跑整个流程。

## 参考入口

- [功能手册](sdk-feature-manual.md)、[接口契约](spec/02-接口契约.md)、[总体设计](spec/01-总体设计.md)。
- [认证及所有权](authentication.md)、[错误处理](error-handling.md)、[兼容性边界](compatibility.md)。
- [当前 KV 门面源码](../kv_client.go)、[元数据与删除恢复源码](../kv_metadata.go)、[Document 与输出保护](../kv/document.go)、[全部结果结构体](../kv/types.go)。
- [版本响应边界回归](../kv_response_boundary_test.go)、[计划删除回归](../kv_scheduled_deletion_test.go)。

官方链接特意使用与当前固定 2.6.3 fixture 同系列的 2.6.x 文档，不代表最新服务端版本或所有部署都通过兼容认证。当前工程与正式证据结论仍以 [当前状态](current-status.md) 和 [发布检查](release.md) 为准。
