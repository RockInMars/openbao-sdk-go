# 固定 OpenBao 2.6.3 集成诊断

诊断候选为 v2 `20508f793a1b992b546d1309065000882144399edb0fc9d021ffb50d952ea231`。服务端为本轮官方签名与 SHA256 核验的 Linux amd64 二进制，仅由 `internal/testenv` 启动新的 loopback TLS fixture。未连接历史远端目标、未使用旧额度。

[首次正式报告](../../../../.artifacts/runs/2026-10-01T150613.577024Z-dd2ae29274734b39a08ba8f0eb5c12a8/integration/report-002.json)真实退出 1：KV、Transit、AuthNamespace 失败；PKI、KV 定时删除通过。下面的探针属于定位证据，不代替正式验收，首次失败没有覆盖或改写。

| 现象 | 独立对照与依据 | 处理决定 |
| --- | --- | --- |
| 大整数真实读回不同 | 原生 HTTP 写 `9007199254740993` 返回 200，原生 HTTP 读回 `json.Number("9007199254740992")`；字符串对照保持原值。2.6.3 KV 读取使用普通 `json.Unmarshal`。 | SDK 本地精度测试不变。AC-006 在实施计划中限定 `ParseDocument/Decode/RevealJSON`，其验收规则要求 `TestDocumentExactNumber`。真实 KV 生命周期夹具改用后端可精确表示的整数，并明确兼容限制；不宣称后端大整数无损。 |
| Ed25519 签名 invalid_argument | ECDSA/RSA 原生请求均 200；Ed25519 显式 `hash_algorithm=none` 返回 400，省略字段返回 200。 | 修复 SDK 的 Ed25519 wire 参数；保留原消息、`prehashed=false`、签名版本及本地验签，增加 sign/verify HTTP 回归。 |
| 最大租期交界认证失败 | 第二探针第 37 秒 `Ready=false`、剩余租期 -736ms，读取 `authentication_failed`；第 38 秒 provider 调用数 2、`Ready=true`，恢复读取。 | 现有契约明确到期失败关闭、退避及重新登录，未承诺无瞬断。测试仅允许旧快照确已过期时的此类错误，仍要求最终恢复、重新登录计数和 namespace 隔离；不修改 SDK 认证语义。 |

来源：[本地精度任务范围](../../../spec/03-实现任务计划.md)、[验收规则](../../../../scripts/acceptance-rules.json)、[认证约定](../../../authentication.md)、[2.6.3 KV](https://github.com/openbao/openbao/blob/v2.6.3/builtin/logical/kv/path_data.go)、[2.6.3 Transit](https://github.com/openbao/openbao/blob/v2.6.3/builtin/logical/transit/path_sign_verify.go)。独立 `contract_guard` 已按这些来源核对上述决定；最终定向及正式运行结果见本页末尾。

两次探针均以已核验 Go 1.26.8 执行 `timeout 180s go run -mod=readonly .artifacts/release-validation-20261001/diagnose-integration.go`，自然退出 0。首轮源在增加 TTL 日志后按该定点修改逆向保存为 [first-source](../../../../.artifacts/release-validation-20261001/diagnose-integration-first-source.go)，第二轮源为 [probe](../../../../.artifacts/release-validation-20261001/diagnose-integration.go)。[首轮日志](../../../../.artifacts/release-validation-20261001/diagnose-integration-final02.log)与 [TTL 日志](../../../../.artifacts/release-validation-20261001/diagnose-integration-ttl-01.log)保留原字节；Windows Tee-Object 写入的 UTF-16LE BOM 按 BOM 解码，没有为显示问题重写。

verifier 确认两轮前后 `/tmp/bao-sdk-isolated-*` 目录均为 0，结束无 `bao` 进程，诊断前后生产源码摘要相同。容器内另有本轮较早遗留的 `python3` 进程，未按名称终止，将在最终回收本轮自有容器时一并处理。

## HMAC 夹具闭环

后续候选 v2 `169b03d2c6c75080611990c3c9234adc6edc87c8464ace94eff2e4764486b077` 的[正式复测](../../../../.artifacts/runs/2026-10-01T154558.563301Z-8474947846304ac6a817852885ead50b/integration/report-002.json)仍真实退出 1：KV、定时删除、PKI、认证与 namespace 四个子场景通过，Transit 运行至 HMAC 时返回 `version_unavailable`。该失败保留，未覆盖前一轮结果。

夹具创建 `mac` 后已轮换至 v2，而成功场景仍请求 v1。批准契约规定：HMAC 元数据不含历史版本表时，只能预检明确的 latest。修复将成功和篡改消息验证均指向 v2，并保留 v1 必须返回 `version_unavailable` 的负例；生产版本预检规则不变。另将动态和参考 control policy 同步为准确的 `transit/verify/mac/sha2-256`，不授予通配权限。新增 bootstrap 回归核对实际发送的精确 ACL，并拒绝旧的无后缀路径及宽泛路径。

独立 `contract_guard` 已确认以上修复对应既有 [HMAC 契约](../../../spec/02-接口契约.md)，不是新行为决定。最终 PASS 必须来自修复后新指纹的完整 REAL_OPENBAO 报告；本页诊断日志不能替代该报告。

修复后的 v2 `e60e3d4b8dad675867e6105a5b81a110bc36dd429c1a226c471656ad03079540` 已取得[完整 REAL_OPENBAO PASS](../../../../.artifacts/runs/2026-10-01T160544.345749Z-b2769877600748238a90b0e0f1be1523/integration/report-002.json)：实际命令退出 0，根测试及五个子场景共 6 条通过、无跳过，运行前后指纹相同。verifier 确认结束无 `bao` 进程，`bao-sdk-isolated-*` 临时目录数为 0。HMAC ACL 的 [RED](../../../../.artifacts/release-validation-20261001/hmac-policy-red-01.json) 真实退出 1，[GREEN](../../../../.artifacts/release-validation-20261001/hmac-policy-green-01.json) 真实退出 0；Ed25519 也保留独立 [RED](../../../../.artifacts/release-validation-20261001/ed25519-wire-red-01.json) 和 [GREEN](../../../../.artifacts/release-validation-20261001/ed25519-wire-green-01.json)，早期 GREEN 的旧指纹仅作历史，最终运行以上述当前正式报告为准。

## 最终候选与变更后核对

上面的 e60e 运行已保留为中间候选历史。最终 v2 `d3183d2e79ec39057ae5f4d437362d4e7d87b61aa16c1055431ca5db11d37233` 重新执行了[完整 REAL_OPENBAO](../../../../.artifacts/runs/2026-10-01T164718.704427Z-9e10d0a7f0f140bea8c7545adee9e7a2/integration/report-002.json)，实际命令及脚本均退出 0；根测试与 KV、定时删除、PKI、Transit、AuthNamespace 共 6 条通过，无 fail/skip。运行前后摘要、Go 1.26.8、OpenBao 2.6.3 binary 摘要及日志摘要经独立核对一致。结束无 `bao` 进程，fixture 临时目录为 0。

变更后契约核对同时发现手册 Create→ReadRef→CAS 示例仍使用超出后端精确范围的 JSON 数字，现将该字段改为字符串并链接兼容性边界。本地 Document 精度契约和源码候选保持不变；文档静态核对单独更新为 `release-validation-static-d3183d-02`，上一份静态记录保留在同目录。最终审查与运行范围见[独立审查](independent-review.md)及[证据索引](index.json)。
