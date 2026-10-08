# 当前验证状态（自动生成）

由 `scripts/current-status.py` 读取现有台账、证据索引和报告生成；此页不修改验收状态，也不授予发布批准。

源码标识：v2 `fd39a6a6ff089e5185d4a52ef72ec43dc5b94e4a746d53665e91ca45baa7b183`。

- 任务台账：STALE；记录状态为 BLOCKED 1 / VERIFIED 18。
- 验收台账：STALE；记录状态为 NOT_RUN 1 / PASS 71。
- 台账指向的证据索引：STALE；文件摘要匹配 249/249。
- 按当前源码即时计算的发布门禁：**FAIL**，99 项拒绝。

| 报告 | 证据类别 | 报告原状态 | 当前校验 |
| --- | --- | --- | --- |
| [normal](../.artifacts/normal-report.json) | `NORMAL_OFFICIAL_CLIENT` | PASS | STALE |
| [integration](../.artifacts/integration-report.json) | `REAL_OPENBAO` | PASS | STALE |
| [consumers](../.artifacts/consumer-report.json) | `INDEPENDENT_CONSUMERS` | PASS | STALE |
| [scans](../.artifacts/security-report.json) | `PINNED_SECURITY_SCANNERS` | PASS | STALE |
| [minimum-normal-linux](../.artifacts/minimum-normal-linux-report.json) | `MINIMUM_GO_COMPATIBILITY` | PASS | STALE |
| [minimum-normal-windows](../.artifacts/minimum-normal-windows-report.json) | `MINIMUM_GO_COMPATIBILITY` | PASS | STALE |
| [minimum-consumer-linux](../.artifacts/minimum-consumer-linux-report.json) | `MINIMUM_GO_CONSUMERS` | PASS | STALE |
| [minimum-consumer-windows](../.artifacts/minimum-consumer-windows-report.json) | `MINIMUM_GO_CONSUMERS` | PASS | STALE |

PASS 要求当前源码、前后标识、实际目标、命令、工具链、平台及日志摘要通过现有证据校验。STALE 表示历史源码；INVALID 表示当前报告不能支持其声明；NOT_RUN 包括缺失报告。BLOCKED/FAIL 保留报告原有结论，不代表检查成功。

补充远端：台账记录 PASS，归属 STALE；它始终独立于 `REAL_OPENBAO`，不替代正式 fixture、扫描或最低版本验证。此处不重新执行远端检查。

重生成：`python -B scripts/current-status.py --write`；一致性检查：`python -B scripts/current-status.py --check`。本地命令遵循全局 RTK 规则。

具体范围、首次失败、审查和续作见[实施交接](implementation-handoff.md)；原始事实见[任务台账](../task-status.json)、[验收台账](../acceptance-results.json)。
