# 指定外部测试服务的补充验证

入口：`rtk proxy python -B scripts/remote-test.py --execute --mode readonly`。默认模式为只读；不带 `--execute` 或缺少必填输入时生成 BLOCKED 收据，不构建、不发送请求。每次使用新的 run_id；可通过 `--run-id <新标识>` 指定。

这些结果属于 `REMOTE_SUPPLEMENTAL` / `COMMAND_RECEIPT`，不是独立 fixture 集成验收。常规只读入口不修改 namespace、mount、策略、Token 或业务密钥；下方隔离和全场景入口只在各自另行获准的范围内写入。所有入口都不修改服务锁或 acceptance-rules。仅支持已明确确认为非生产的 `https://kms.jiup9.com[:443]`。不跟随重定向，不使用环境代理，不关闭 TLS 校验。

## 可复用全场景清单（本轮仅本地验证）

`scripts/remote-fixture.py` 负责专用夹具的准备、两个 SDK 子进程、报告与反向回收；`scripts/remote-test.py` 保留常规只读/隔离入口。全场景不再接受硬编码的历史标识，必须提供 `BAO_REMOTE_MANIFEST` 和原始文件的 `BAO_REMOTE_MANIFEST_SHA256`，Python 与 Go 分别校验并将摘要写入报告。当前真实执行状态见[自动摘要](current-status.md)；旧 120 请求/20 分钟授权不能用于新清单。

从 [remote-manifest.example.json](../deploy/test/remote-manifest.example.json) 创建新计划。样例刻意已过期，不能直接运行。`run_id` 格式是 `remote-sdk-YYYYMMDD-<16位小写十六进制随机值>`，资源前缀必须是去掉 `remote-` 的同一标识；UTC `issued_at`/`expires_at` 使用秒精度 `Z` 时间，窗口最多 20 分钟。该清单不含凭据，**文件校验成功不是远端执行授权**。维护者仍须明确批准目标、namespace、时间、额度、独占范围和回收。

固定 v1 profile 限定 `https://kms.jiup9.com:443`、非生产 `sdk-test`、bootstrap `auth/approle`，禁止额外字段、重复 JSON 字段、路径越界、未来或过期运行。请求额度固定为总计 120，其中 Core 43、Transit 32、清理预留 15，准备阶段最多 30；不能提高上限或用不足以覆盖该 profile 的额度启动。9 项顶层资源为 bootstrap Token、三个 mount、一个 auth mount、两个 policy 和两个运行 Token；CA、角色、Transit key、KV 路径和 AppRole SecretID 只能位于本轮独占 mount 子树，跟随所属 mount 回收。允许操作与清理规则是闭合枚举，不接受任意 URL 或脚本。

AppRole 场景另会派生登录 Token，它随本轮专用 auth mount 成功禁用而撤销；`Client.Close` 本身不撤销它。依据为 [OpenBao auth disable 契约](https://openbao.org/docs/commands/auth/disable/)与已核对的 2.6.2/2.6.3 服务端 `RevokePrefix` 路径。auth mount 禁用失败、未执行或结果不明时，该派生 Token 也不能记作已回收。

```text
rtk proxy python -B scripts/remote-fixture.py --manifest <新清单.json> --validate-only
# 仅在另行获得本次完整授权并注入凭据后：
rtk proxy python -B scripts/remote-fixture.py --manifest <同一清单.json> --execute
```

bootstrap 凭据只从当前进程的 `BAO_REMOTE_ROLE_ID`、`BAO_REMOTE_SECRET_ID`、`BAO_REMOTE_APPROLE_MOUNT` 读取；不放入清单、命令或报告。父进程为新 run 建立独占目录，冻结原始清单，拒绝已用父/子 run ID。先在请求预算内读取健康版本，只有已核对创建语义的 OpenBao `2.6.2`/`2.6.3` 才继续，其他版本在创建任何资源前停止；这项限制仅针对此补充夹具，不改变 SDK 的一般版本承诺。

创建 mount/auth 时使用服务端原子冲突拒绝，policy 使用 `cas=-1` 与 `cas_required=true`，KV 种子使用 CAS=0。远端是否已存在只有请求服务后才能知道，不能把本地计划校验称为远端不存在证明。[mount 冲突检查](https://github.com/openbao/openbao/blob/v2.6.3/vault/mount.go#L195-L226)、[auth 冲突检查](https://github.com/openbao/openbao/blob/v2.6.3/vault/auth.go#L62-L103)及 [policy CAS 检查](https://github.com/openbao/openbao/blob/v2.6.3/vault/policy/policy_store.go#L258-L303)是版本核对依据。只有明确的 mount/auth 409 或精确 policy CAS 冲突响应才记为 `not_created`；其他创建失败或响应不明保留待核归属。

清理前重新核对 mount/auth accessor 摘要或 policy 内容、版本、修改时间摘要，并按实际创建顺序反向回收。资源前缀必须由维护者保证独占：身份查询与删除之间没有服务端 CAS，其他管理员并发替换资源会破坏这一前置条件。归属改变、子进程超时/收据缺失、请求计数不明、检查点写入失败均阻止自动清理。到期后停止所有请求，包括清理；待处理项保留，须另行获得精确恢复授权。吊销记录、审计日志和服务端历史不因此消失。

本地覆盖包括非法清单、摘要篡改、重复运行、预算耗尽、越界/未拥有子树、已存在资源、部分创建、身份改变、清理失败及不确定子进程。模拟通过仅证明防护分支，不是新一轮真实服务 PASS。

## 2026-09-30 历史指纹复测

当时用户明确授权仅在 `https://kms.jiup9.com:443` 的非生产 `sdk-test`，以最多 120 次串行 API 请求、20 分钟、最多 9 项专用资源完整复测。夹具前缀为 `sdk-codex-20260930-68edb3eb`，Core/Transit run ID 为 `remote-sdk-full-20260930-07-core` / `remote-sdk-full-20260930-07-transit`。当时入口限定该组标识；当前全场景入口已改用上述新清单，不能重用该历史授权。

开头“不修改 mount/策略/Token”只适用于常规 `scripts/remote-test.py` 入口；本次另行授权的一次性夹具创建并删除其专用 KV/PKI/Transit mount、策略和测试身份，未操作既有业务资源。

[第八轮不可变报告](../.artifacts/runs/remote-full-fixture-20260930-08/report.json)为 PASS：源码指纹 v2 `e9d371d3a35385803a651db667a05cbd8fb3bb6cd36b03b23bf24ffd6139f14e` 前后一致，112/120 次请求精确计数；[Core](../.artifacts/runs/remote-sdk-full-20260930-07-core/remote/result.json) 39/39 场景、40 次请求，[Transit](../.artifacts/runs/remote-sdk-full-20260930-07-transit/remote/result.json) 22/22 场景、27 次请求。9 项本轮专用资源均标记 `removed`，清理状态 PASS；两个 SDK 子进程与内部 Go 命令自然退出 0。服务自报版本 `2.6.2` 不构成可信发布物 digest。此次结果只证明本次固定身份和资源在指定服务上的补充场景，**不替代正式独立 fixture 集成**；本次预算不结转。证据归属和门禁状态见[完整远端报告](openbao-remote-test-report-2026-09-30.md)。

## 先前 sdk-test 全场景续跑（历史）

上述“不修改 mount/策略/Token”仅描述常规 `scripts/remote-test.py` 入口。用户另行明确授权在非生产 `sdk-test` 创建并最终回收本轮专用 KV/PKI/Transit 夹具，以及专用策略和受限测试身份；一次性夹具限定目标 `https://kms.jiup9.com:443`、AppRole auth mount `auth/approle`，RoleID/SecretID 从受控环境读取，不写入命令、仓库和报告。该授权不扩展到其他 namespace、主机、端口或业务资源。已使用的 run_id 不可重跑，先核对报告和回收清单。

[第七轮全场景夹具报告](../.artifacts/runs/remote-full-fixture-20260930-07/report.json)为 PASS：112/120 次串行请求，Core 39/39、Transit 22/22 场景通过，专用资源全部回收；源标识 v2 `dee54e4f2f49c24f71e8cd412064eac0286b2ba3f415c1850de05e82ef1d093b` 前后一致。该轮专用 HMAC key 的元数据实际省略 `keys`，SDK HMAC/验证成功。此前[第二次全场景夹具](../.artifacts/runs/remote-full-fixture-20260930-03/report.json)在 KV List 停止、[方法诊断](../.artifacts/runs/remote-list-method-diagnostic-20260930-01/report.json)观察到 `GET ?list=true` 成功，以及[第六轮 HMAC 失败](../.artifacts/runs/remote-full-fixture-20260930-06/report.json)均保留为历史证据。KV List 已按用户明确批准固定为 `GET ?list=true` 并同步[接口契约](spec/02-接口契约.md)。此一次性夹具只证明指定服务与本轮资源上的补充场景，不替代正式独立 fixture 集成；本轮 20 分钟预算不可结转，后续远端运行需要新的明确范围与额度。详见[完整远端报告](openbao-remote-test-report-2026-09-30.md)。

## 执行前输入

通过当前进程的受控环境注入以下变量。凭据不得作为 CLI 参数、写入配置文件或粘贴到聊天；普通 Go 构建及版本探测使用单独的环境白名单，不继承测试凭据。运行子进程的 stdout/stderr 在持久化前直接丢弃；允许的结果由固定结构的 JSON 单独记录。曾在聊天出现的凭据建议由维护者轮换，本工具不会自行撤销或替换。

| 变量（均有 `BAO_REMOTE_` 前缀） | 内容 |
| --- | --- |
| `ADDRESS`、`CONFIRMED`、`ENVIRONMENT` | 已确认的上述 HTTPS 地址、`yes`、`nonproduction`；测试账号不自动代表服务器为非生产 |
| `NAMESPACE_MODE`、`NAMESPACE` | `named` 与明确名称，或 `root` 与空值。本轮已收到的 namespace 为 `sdk-test` |
| `TOKEN` | 受控注入的测试身份凭据；不打印其值 |
| `KV_MOUNT`、`KV_PATH`、`KV_VERSION`、`KV_MARKER` | 已存在的 KV v2 精确测试路径、正整数版本及 `sdk_test_marker` 字段的预期非敏感值；不枚举目录寻找替代 |
| `CA_FILE` | 可选：PEM 受信 CA 文件；未设置时使用系统根 |
| `CLIENT_CERT_FILE`、`CLIENT_KEY_FILE` | 可选：必须成对提供的 mTLS 文件；不复制到证据目录 |

只读执行顺序为 SDK 无认证健康检查、Start、自查身份、精确路径能力、指定版本读取并核对标记、CheckReady、Close。只解析身份响应中的 TTL/剩余次数用于预算校验，不持久化响应；有限预算必须覆盖本轮剩余请求和时间。身份自查被禁止会单独标记，不能直接推断 Token 无效，也不能算整轮 PASS；隔离模式立即停止后续写入。任何被查询路径具有 root 能力都会拒绝。只读最多 10 次 API 请求、90 秒，串行且读重试为 1；健康成功不代表认证成功。

## 隔离场景

显式选择 `--mode isolated`，仍需上述已知只读资源。同时提供 `WRITE_PREFIX`、`ALLOW_WRITE=yes`、`ALLOW_CLEANUP=yes`。前缀必须是维护者确认独占、可永久回收的测试区域。工具仅使用 `<WRITE_PREFIX>/<run_id>/kv`，CAS=0 创建、CAS 更新、验证旧 CAS 冲突；回收包括删除该自有路径的全部版本及 metadata。不得指向业务前缀。并发修改该独占前缀违反测试前置条件：KV metadata 删除本身没有 CAS 保护。

可选 `ALLOW_SOFT_DELETE=yes` 授权本轮版本的软删除与恢复。创建前预留至少 20 个请求及 60 秒；删除前逐版本核对随机归属标记和最新版本，删除成功后检查确切 metadata 路径已不存在。既有数据、权限失败、归属不明、响应丢失或检查点持久化失败会停止受影响动作。UNKNOWN 不重放；未确认回收的路径保留在资源清单，由维护者核对后处理。

可选场景缺输入可以不启用，不阻止明确获准的 KV 场景：

| 场景 | 所需输入及限制 |
| --- | --- |
| Transit | `TRANSIT_MOUNT`、`TRANSIT_KEY`、`TRANSIT_TYPE`。必须是已存在的专用 key；类型支持 `aes256-gcm96`、`ecdsa-p256`、`rsa-2048/3072/4096`、`ed25519`。验证加解密或签验，以及篡改负例；不建 key、轮换、导出或删除。encrypt 路径需要 update-only 权限，具有 create/root 时拒绝，避免已有 key 并发消失时隐式创建 |
| PKI | `PKI_MOUNT`、`PKI_ROLE`、`PKI_DNS`、`PKI_ROOT_FILE`、`ALLOW_SIGN_CSR=yes`。本地生成 P-256 私钥及 CSR，TTL 固定 5 分钟，核对受信链、服务端用途、SAN、公钥、有效期；不启用 Issue。若还获准吊销，设置 `ALLOW_REVOKE=yes`，仅吊销本轮序列号。签发/吊销记录仍在服务端保留，不称为已删除 |
| 权限负例 | 独立受限 `NEGATIVE_TOKEN`、`NEGATIVE_MOUNT`、`NEGATIVE_PATH`；先验证该身份有效及预算，再只读明确禁止访问的测试路径。必须明确返回 403，404 或无效身份返回的 403 都不作为隔离成功 |

隔离场景最多 60 次 API 请求、10 分钟，当前实现串行；预检、能力查询、SDK 内部元数据读取和清理都计入。普通请求 5 秒，连接/TLS 3 秒，PKI 使用 SDK 既有预算。不能通过扩容或自动重试越过上限。远端不运行 fuzz、benchmark、故障注入、封印或切换。

## 证据与恢复

证据位于 `.artifacts/runs/<run_id>/remote/`：不可覆盖的 `report-NNN.json`、命令收据/日志 hash、`checkpoint-NNN.json` 和最终 `result.json`。`.artifacts/remote-report.json` 只是 latest 指针。源码标识前后不一致时无法 PASS。结构化数据不含 Token/accessor、请求/响应正文、KV 内容或私钥。

每次变更前先写资源意向检查点。KV 检查点保存合成随机归属标记的 `ownership_sha256`，不保存该标记或 KV 内容；恢复时只能读取确切自有路径，对 `sdk_test_owner` 字符串的 UTF-8 原始字节计算 SHA256 并比较，同时核对全部版本及独占范围，才能确认本轮归属。进程超时或中断后先核对收据中的 owned_pid 和最后检查点，不直接重跑同一 run_id，更不能盲目重发写入/删除。`creation_pending`、`cleanup_pending`、`issuance_pending`、`revocation_pending` 均需人工依据精确路径或序列号核对；无法确认归属时不删除。仅可回收本轮创建的进程和资源。

普通 `go test ./...` 不包含 `TestRemoteService`；该目标仅在 `remote` build tag 下编译，且仍要求显式执行变量。`tests/remote` 的普通测试仅使用本地 TLS fixture，其成功不代表指定真实服务通过。源码或脚本变化后，normal、fuzz、consumer 等需要重新生成当前指纹的正式证据；benchmark 不作为发布硬门槛。
