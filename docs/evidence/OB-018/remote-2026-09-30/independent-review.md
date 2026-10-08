# 远端测试入口独立审查

## 当前指纹及第七轮证据复核

`/root/remote_risk_review` 在未提交 source_identity v2 `dee54e4f2f49c24f71e8cd412064eac0286b2ba3f415c1850de05e82ef1d093b` 上只读审查了 [fixture07](../../../../.artifacts/runs/remote-full-fixture-20260930-07/fixture.py)、[本轮报告](../../../../.artifacts/runs/remote-full-fixture-20260930-07/report.json)、两份 SDK 子结果及相关 Go/Python 限制。首次发现新 run_id、marker 与 Core 三层额度未同步，判为远端执行阻断；主代理先以 Python/Go 聚焦用例取得失败，再同步 `scripts/remote-test.py`、`tests/remote/config.go` 及负向测试，聚焦用例通过。复核确认 run_id/marker 同为 `remote-sdk-full-20260930-06`，Core 三层均 43、Transit 均 32；新增只读 HMAC 形状探测后，静态上界为 30 次管理预备 + 43 Core + 32 Transit + 15 清理 = 120。前序 fixture06 与两个 HMAC 诊断的固定报告 hash/回收状态均在新夹具前置检查中核对；探测只持久化预定字段类型、数量和匹配布尔。审查者没有修改文件、运行测试、访问远端或读取凭据。

执行后同一只读审查角色核对：本轮 [外层报告](../../../../.artifacts/runs/remote-full-fixture-20260930-07/report.json)为 `REMOTE_SUPPLEMENTAL_FIXTURE` / PASS，112/120 次 EXACT，Core 39/39、Transit 22/22，子进程自然退出 0；外层与内层报告/日志 hash、前后指纹匹配，9 项资源记为 `removed`、cleanup PASS，HMAC 专用 key 的 `keys` 形状为缺失。当前[远端报告](../../../openbao-remote-test-report-2026-09-30.md)明确它不属于 `REAL_OPENBAO`。没有发现新的证据阻断。局限：这是文件、收据与代码的独立只读核对，不是独立观测服务端流量或删除后状态；主代理对本轮 257 个 JSON/日志文件的凭据扫描没有由审查者重复。正式验收仍因可信发布物、固定扫描器及平台/CI 条件受阻。

独立 `contract_guard` 在同一指纹上再次只读核对：已批准的 KV List 固定 `GET ?list=true`、目录尾斜杠、404 分类与 SafeRead 归属一致，无失败后换方法重发；HMAC 缺失/空 `keys` 的 latest-only 预检、畸形元数据拒绝、公开 Go API 和已发送写请求的 UNKNOWN/不重放规则与[当前契约](../../../spec/02-接口契约.md)一致。fixture07 的 `keys_kind=absent` 与 HMAC 三项 PASS 仅证实指定服务场景；正式 `REAL_OPENBAO` 仍 BLOCKED。该审查未运行测试，也未核验固定镜像 digest。

以下为先前指纹下的历史审查。

## 2026-09-30 全场景续跑后的只读复核

同一 `/root/remote_risk_review` 风险审查角色在当前未提交指纹 v2 `376c3dbba0d9340762216bd161170f435eb3834daeb76342aa6eb99b6496a85d` 上复核了[第二次完整夹具](../../../../.artifacts/runs/remote-full-fixture-20260930-03/report.json)、[带斜杠 LIST 诊断](../../../../.artifacts/runs/remote-list-diagnostic-20260930-01/report.json)、[LIST/GET 方法诊断](../../../../.artifacts/runs/remote-list-method-diagnostic-20260930-01/report.json)、当前 SDK 代码、既有契约及 api/v2 v2.7.0 缓存。审查只读，未运行测试、发送远端请求、接收凭据或修改文件；运行和文档修正由主代理完成。

审查发现报告措辞曾将两轮诊断描述为“同一路径、同一身份”严格 A/B，证据不足：资源在两轮间已删除并重建，后轮内部又比较无斜杠 LIST 与带斜杠 GET。主代理已将远端报告、交接和配置说明修正为跨轮观察。前轮**带斜杠** LIST 对管理员/受限身份都为 404，后轮带斜杠 GET `?list=true` 对两者都为 200 且包含本轮 seed；两轮资源各自 cleanup PASS、请求计数 EXACT。完整夹具的普通 KV 读取通过而 List 失败，强烈支持目标入口链路对 LIST 不兼容，但不能确定拒绝发生在代理还是服务，也不是同一资源/会话的单变量实验。

既有[接口契约](../../../spec/02-接口契约.md)第 548 行明定 LIST 且禁止静默换 GET。若用户明确批准改动，审查建议仅将 KV List 固定为 GET `?list=true`，非空 prefix 带目录尾 `/`，保持 SafeRead、权限、解析和 404 错误语义，测试根/嵌套 method/path/query，不做失败后自动重发另一方法。当前未取得该契约决定，因此尚未实施。该复核不能替代真实全场景重跑；下方为原入口实现阶段的历史审查记录。

范围：`scripts/remote-test.py`、新 Python 回归、`tests/remote/*`、公共执行器新增的输出抑制参数，与本轮初始工作区快照比对。基线提交为 `6d8251658e76f88aec5935126f303b2c59497691`，实际审查的是未提交工作区，不能用 HEAD 替代。冻结后 source_identity v2 为 `32dfe9e8dbfa041033bb2ae2ce10488af173b9fceb4fda8388f7af450764471e`，172 个输入。

独立角色：`/root/remote_risk_review`（risk_reviewer），只读，无测试、远端请求或文件修改。主代理负责全部修复及下述运行。最终静态复核未发现新的已证实缺陷；这不是运行验收或发布批准。

| 发现 | 修复及验证 |
| --- | --- |
| 未核验 Token TTL/剩余使用次数，可能写入后无额度清理 | 自查只解析必要字段；有限预算不足时不进入写入。RED-01 的低 TTL/次数用例失败，GREEN-01/04 通过 |
| lookup-self 403 后 isolated 仍可写入，BLOCKED 可能汇总 PASS | isolated 直接停止；BLOCKED 不能汇总 PASS；精确 403 回归通过 |
| 无效身份的 403 被当成权限隔离成功 | 受限身份先认证，按两次请求的独立预算检查，再访问明确禁止路径。RED-02 复现，GREEN-02/04 通过 |
| root 能力、Transit encrypt create 或 key 管理能力被接受 | 全路径拒绝 root；encrypt 拒绝 create；keys 路径拒绝 create/update/delete/sudo。RED-01/02/03 复现，GREEN-03/04 通过 |
| 旧 CAS 请求响应丢失时，重新构造错误抹掉 UNKNOWN | 保留原 SDK 错误；任何不确定写入后停止自动清理，资源保留 cleanup_pending。RED-03 复现，GREEN-03/04 通过 |
| 原始 metadata DELETE 部分异常没有 UNKNOWN | 非 204 及传输失败保守记 UNKNOWN，禁止重放；204 后独立读取确认不存在；丢响应回归通过 |
| 创建已生效但响应丢失后，内存归属 nonce 无法恢复 | 创建前持久化合成 nonce 的 SHA256 承诺值，不保存 KV 内容。RED-04 复现，GREEN-04 通过 |
| 报告缺精确 namespace/资源与请求分类 | 加入经过校验的 target_context 及有限 request_categories；Python 独立比对输入、预期目标和请求总数 |

原始 RED/GREEN 收据均在 `.artifacts/runs/remote-20260930-start/`，分别为 `remote-review-red-remote-review-red-01..04.json`、`remote-review-green-remote-review-green-01..04.json`（`..` 仅作编号说明，不是文件名）。最后全包命令为 `go test -mod=readonly -race -count=1 -timeout=120s ./tests/remote`，由 `record.py` 记录自然退出 0；审查者不以该主代理运行冒充自己的执行结果。

保留限制：KV metadata 删除没有 CAS。即使删除前核验了全部自有版本，也存在核验与删除之间的并发窗口；独占前缀必须由维护者保证没有其他写入者。当前本地测试不能证明远端 ACL、环境性质或真实服务满足此条件。未开展真实远端、Linux、最低 Go 或远端 CI 审查。凭据未传给审查线程。
