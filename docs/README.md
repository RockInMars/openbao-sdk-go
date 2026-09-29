# 文档导航

本目录面向 `openbao-sdk-go` R3 实施快照。新增接入文档的版本 **R3-D1** 只表示文档修订；SDK 仍未通过发布门禁。

## 接入者先读

| 文档 | 解决的问题 |
|---|---|
| [SDK 接入说明](sdk-integration-guide.md) | 准备条件、模块引用、认证、配置、生命周期、只读接入、权限与错误恢复 |
| [功能手册与调用示例](sdk-feature-manual.md) | 每个公开功能做什么、参数、返回值、支持范围、已知限制 |
| [首次使用指南](first-use-guide.md) | 按四个现有示例配置和执行，明确每步副作用及首次结果记录 |

## 专项说明

[认证](authentication.md) · [错误处理](error-handling.md) · [观察接口](observability.md) · [兼容性](compatibility.md) · [测试](testing.md) · [发布](release.md) · [业务接入与回滚](integration-and-rollback.md)

## 依赖恢复与实施

[依赖预检](dependency-bootstrap.md) · [公共依赖离线转移](dependency-transfer.md) · [实施交接](implementation-handoff.md) · [原总体设计](spec/01-总体设计.md) · [原接口契约](spec/02-接口契约.md) · [原实施计划](spec/03-实现任务计划.md) · [原验收矩阵](spec/04-验收矩阵.md)

当前执行状态以 [任务台账](../task-status.json)、[验收结果](../acceptance-results.json) 和 [R3实施报告](../IMPLEMENTATION_REPORT_R3.md) 为准。新增文档没有生成 SDK 测试通过记录。

文档变更说明：[R3-D1 文档增补记录](documentation-change-R3-D1.md)。文档检查与运行时测试是不同证据：[文档检查结果](documentation-validation-R3-D1.json)。
