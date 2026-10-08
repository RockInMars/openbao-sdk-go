# 发布验证续作独立审查

仓库为 `E:\xen\code\claude\project\jiupiao\openbao-sdk-go`，`master` / HEAD `6d8251658e76f88aec5935126f303b2c59497691`，原有暂存区为空、工作树修改保留。最终源码候选为 v2 `d3183d2e79ec39057ae5f4d437362d4e7d87b61aa16c1055431ca5db11d37233`，191 输入。主代理负责落盘本报告；子代理只读核对，没有修改源码或执行测试。

## risk_reviewer

`remote_retest_guard_review` 首轮核对工具链版本、可信服务锁、模块打包剪枝与扫描边界；e60e 阶段复核实际 SDK 与 fixture 差异；d318 阶段补审依赖工具遥测准备。后两阶段之间，已审的 SDK、扫描与 CI 核心字节未变。结论仅覆盖列明范围，不表示全仓无缺陷或获得发布批准。

| 审查项 | 结论与闭环 |
| --- | --- |
| Go 主线、最低版本和服务锁 | 主线正式入口/校验/CI 同步固定 1.26.8；Go 1.25.0 声明下限及 api/v2 v2.7.0 不变。OpenBao binary 摘要与可信出处一致。 |
| 模块打包 | 在遍历前剪枝生成目录、符号链接和嵌套模块，未发现确定的归档边界回归。 |
| gitleaks 排除范围 | 首轮发现任意层级 `.artifacts` 会漏扫嵌套目录；现仅锚定仓库根，实际命令扫描 `.`，证据校验拒绝局部目录。固定扫描器 RED/GREEN 保留嵌套目录的检出。 |
| CI fixture 接线 | 首轮指出镜像注入与 binary 锁不一致；现固定官方归档摘要后解包，再校验锁文件 binary 摘要。只读未发现确定缺陷；托管 CI 未运行。 |
| Ed25519 | Sign/Verify 共用 wire 映射，仅省略不兼容的 hash 字段，保留原消息、`prehashed=false` 及本地验签；HTTP 回归和真实 Transit 场景覆盖。 |
| KV 与认证 fixture | 后端生命周期使用可精确表示整数，本地大整数标准不变。认证只允许旧快照到期后的 `authentication_failed`，仍要求最终恢复和重新登录；未发现放宽生产权限或认证语义。 |
| HMAC | 保留 v1 拒绝、v2 成功及篡改消息负例；动态与参考 ACL 只授权精确 SHA-256 verify 路径。 |
| 临时工具进程生命周期 | 要求修复覆盖 export、verify、replay 三个独立 scratch；d318 确认配置仅在临时目录，关闭遥测的记录名不冲突，准备非零仍阻止发布，原签名/不回网/严格清理断言保留。 |

上述已审稳定范围未发现尚未解决的确定缺陷。reviewer 只读核对过 e60e 真实集成报告的同指纹、6 条通过与服务端基线；它没有替代 verifier 执行，也不将 e60e 运行继承为 d318 运行。

## contract_guard

`provider_contract` 在修改前分别核对 EXPECTED、IMPLEMENTED、CONSUMED：AC-006 的已批准范围为本地 Document 精度；认证最大 TTL 允许失败关闭后的重登；Ed25519 契约要求原消息且不预散列；HMAC 在无历史元数据时仅预检 latest，并使用 `/verify/{name}/sha2-256`。前置核对明确了 fixture 与契约的两处 HMAC 差异，未编造新业务规则。详细诊断与原始失败见 [integration-diagnosis.md](integration-diagnosis.md)。

最终 post-change 核对为 **PASS**，针对 d318 源码和已修订手册。来源包括批准的接口契约/AC-006、本轮观察到的 SDK 与 fixture、实际消费方文档，以及当前不可变 REAL_OPENBAO 报告；源码未提交，因此 HEAD 不能单独代表本次基线。

| 核对项 | EXPECTED / IMPLEMENTED / CONSUMED / VERIFIED | 状态 |
| --- | --- | --- |
| HMAC 版本和 ACL | 批准的 latest-only 规则不变；v1 负例、v2 成功及篡改消息负例保留；两份 policy、SDK 和手册均使用精确 SHA-256 verify 路径。 | PASS |
| KV 精度 | 批准范围为本地 Document 精度。真实 fixture 使用后端可精确表示数，兼容说明记录后端限制；发现手册真实写入示例仍用大整数数字后，已改为字符串并独立复核。 | PASS |
| 认证过期和 Ed25519 | 过期后的失败关闭、最终重登、namespace 隔离要求保留；Ed25519 原消息及 `prehashed=false` 保留，只省略不兼容的可选 hash 字段。 | PASS |
| 服务锁与当前真实运行 | CI 校验归档和 binary 两层摘要；当前 Linux Go 1.26.8 / OpenBao 2.6.3 报告前后均为 d318，命令自然退出 0、日志摘要匹配，根与五个子场景通过，无 fail/skip。 | PASS |
| dependency-bundle 遥测准备 | 不属本次 SDK/远端业务契约核对范围，另由 risk_reviewer 与实际工具测试覆盖。 | NOT_APPLICABLE |

本范围无未解决的 FAIL。contract_guard 没有执行测试；它亲自核对了 [d318 REAL_OPENBAO 报告](../../../../.artifacts/runs/2026-10-01T164718.704427Z-9e10d0a7f0f140bea8c7545adee9e7a2/integration/report-002.json)及其日志事件、摘要和服务基线。其它平台运行报告、12-job 汇总及最终台账门禁由 verifier/主代理核对，不冒称全部由 contract_guard 独立审计。

## 运行证据与限制

主代理另有绑定当前文档字节的 [静态核对](static-review.json)。最终实际检查、源码前后摘要、日志摘要、首次失败和资源回收以 [证据索引](index.json) 为准，静态审查不替代运行验证。当前远端 manifest、真实业务迁移、托管 GitHub Actions、提交/推送/发布均不在本轮执行范围；维护者支持版本和响应时限仍待决定。
