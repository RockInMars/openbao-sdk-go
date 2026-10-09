# 文档导航

当前源码与证据结论统一见[当前验证状态](current-status.md)；指定测试服务的入口、范围与资源恢复见[远端测试说明](remote-testing.md)，旧运行事实见[历史远端报告](openbao-remote-test-report-2026-09-30.md)。

本目录面向当前 `openbao-sdk-go` 工程快照。R1/R2/R3 和 R3-D1 原记录保留；2026-10-01 改进的执行计划与检查点见[实施交接](implementation-handoff.md)，SDK 发布门禁仍与本地实现/验证分开。

## 接入者先读

| 文档 | 解决的问题 |
|---|---|
| [SDK 接入说明](sdk-integration-guide.md) | 准备条件、模块引用、认证、配置、生命周期、只读接入、权限与错误恢复 |
| [功能手册与调用示例](sdk-feature-manual.md) | 每个公开功能做什么、参数、返回值、支持范围、已知限制 |
| [首次使用指南](first-use-guide.md) | 按五个现有示例配置和执行，明确每步副作用及首次结果记录 |

## 专项说明

[认证](authentication.md) · [KV v2 方法详解](kv-methods.md) · [PKI 方法详解](pki-methods.md) · [Transit 方法详解](transit-methods.md) · [错误处理](error-handling.md) · [观察接口](observability.md) · [兼容性](compatibility.md) · [测试](testing.md) · [发布](release.md) · [业务接入与回滚](integration-and-rollback.md)

## 依赖恢复与实施

[依赖预检](dependency-bootstrap.md) · [公共依赖离线转移](dependency-transfer.md) · [实施交接](implementation-handoff.md) · [原总体设计](spec/01-总体设计.md) · [原接口契约](spec/02-接口契约.md) · [原实施计划](spec/03-实现任务计划.md) · [原验收矩阵](spec/04-验收矩阵.md)

当前执行状态以 [任务台账](../task-status.json)、[验收结果](../acceptance-results.json) 和 [实施交接](implementation-handoff.md) 为准。[改进方案](implementation-plan-2026-09-29.md) 是输入设计，[评估](project-assessment-2026-09-29.md) 和 [R3实施报告](../IMPLEMENTATION_REPORT_R3.md) 是历史材料，均不能代替当前运行证据。

本轮交付的命令、结果、覆盖率、性能数据与阻塞见 [2026-09-29实施结果](implementation-execution-report-2026-09-29.md)；[证据索引](evidence/OB-018/implementation-2026-09-29/index.json) 对照原始收据和归档副本。

文档变更说明：[R3-D1 文档增补记录](documentation-change-R3-D1.md)。文档检查与运行时测试是不同证据：[文档检查结果](documentation-validation-R3-D1.json)。
