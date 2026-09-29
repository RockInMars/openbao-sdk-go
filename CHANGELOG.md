# Changelog

## Unreleased — 2026-09-29 R3-D1 documentation

新增中文 SDK 接入说明、功能手册与调用示例、首次使用指南和文档导航；README 与示例说明加入入口。按源码记录 Transit 预检权限、CheckReady 否定结果及 HMAC/PKI 示例限制。仅文档变更，运行时源码、依赖、原方案、任务与验收状态均不变；没有新软件发布或真实服务验证。

## Unreleased — 2026-09-28 implementation snapshot

新增运行时 Core/Auth/KV v2/PKI/Transit/Diagnostics/Observe 代码、四类示例、故障恢复业务示例、独立消费模块、真实集成环境和安全门禁脚本。

保持 **unreleased / 未验收**：官方 api/v2 模块获取、服务端精确基线、真实集成、独立消费构建、固定工具扫描和完整覆盖率仍需实证。没有发布 tag、远端 push 或现有业务迁移。

测试中发现并修复的行为、对应红绿证据见 `docs/review.md`。完整记录不是一个已验证产品版本号。
