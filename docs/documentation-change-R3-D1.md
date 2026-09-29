# R3-D1 文档增补记录

修订日期：2026-09-29。此次请求按“SDK 接入说明、功能手册，并包含首次使用流程”执行。

## 输入与范围

依据 R3 源码快照、docs/spec 原方案、公开接口契约测试、task-status.json、acceptance-results.json 与既有 examples。未使用外部资料改写接口，未下载依赖、启动 OpenBao、操作生产或接入真实业务项目。

新增 docs/sdk-integration-guide.md、docs/sdk-feature-manual.md、docs/first-use-guide.md、docs/README.md；README、CHANGELOG、examples/README 增加导航。原方案、历史证据、Go 源码、go.mod、go.sum、任务与验收状态保持不变。根 SHA256SUMS 按新归档更新，旧清单单独保存。

## 文档发现并保留的问题

1. HMACVerify 请求使用 transit/verify/<name>/sha2-256；测试 control fixture 只配置 transit/verify/mac。本文记录为真实集成前必须核对的差异，不修改代码、不扩大权限、不宣称已验收。
2. PKI 一次性 main 的 KV 文档保存 ca_chain_pem 为 [][]byte 编码，并未单独写入 IssuingCAPEM；回读仅检查操作号与代次，不能当作完整材料及跨进程恢复方案。功能手册明确标注，不擅自改变存储 schema。
3. service-bootstrap 的 started 不证明远端 ACL/引擎可用；首次使用指南明确后续要执行真实探测或调用。

## 文档验证和未执行项

执行了 Markdown 链接、围栏、公开方法名称、现有示例环境变量与命令目标检查；独立 Go 调用片段通过 gofmt 语法解析。源文件接口名称/字段按 R3 源码人工复核。没有做 import 解析、Go 类型检查、正式 SDK 编译或真实服务调用，不能称为“示例运行通过”。

验收仍保留 R3 的 8 PASS / 59 PARTIAL / 5 NOT_RUN，任务仍为 0 VERIFIED / 14 IN_PROGRESS / 5 BLOCKED；没有将文档检查换算为 AC 状态。具体检查结果见 documentation-validation-R3-D1.json。

下一实施入口仍为原 docs/implementation-handoff.md。本次不修改交接中的软件状态；读者从 docs/README.md 进入新增接入文档。
