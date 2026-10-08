# 兼容性与依赖基线

| 项目 | 本次观察 | 结论 |
|---|---|---|
| 工程主工具链 | 固定 Go 1.26.8，Windows/Linux amd64 与各自 CGO 编译器 | 当前指纹的正式报告分别校验；范围见实施交接 |
| 声明最低 Go | go.mod 为 1.25.0；已准备两个平台的官方核验工具链 | normal/consumer 四份最低版本证据独立于主版本；兼容下限不构成安全维护承诺 |
| Linux / Windows / 远端 CI | 本地矩阵使用同字节源码的 Windows 工作树与隔离 Linux 容器；远端 CI 未运行 | 本地矩阵不冒充 GitHub Actions 运行，macOS 未验证 |
| 官方客户端 | api/v2 v2.7.0，已有模块缓存和 go.sum | 固定不升级；完整普通门禁单独记录 |
| OpenBao 服务端 | 正式 fixture 输入固定为经官方签名与 SHA256 核验的 2.6.3 Linux amd64 二进制 | 输入出处见本轮 tool-provenance，运行结果见当前 REAL_OPENBAO；不外推所有 2.x 版本 |
| go.sum | 已有真实依赖校验记录 | 本轮保持原记录，不手填上游校验和 |
| SDK module | github.com/RockInMars/openbao-sdk-go | 维护者确认的公开 GitHub 地址；未推送本轮变更、未验证本轮远端分发 |
| 源码许可 | 上游现有 Apache-2.0 LICENSE | Git blob 261eeb9e9f8b2b4b0d119366dda99c6fd7d35c64，复用原文 |
| 实际 SaaS/游艇调用仓库 | 未提供 | OB-019/AC-072外部阻塞 |

不要以 api/v2 模块版本推导兼容所有 OpenBao 2.x 服务。[历史远端报告](openbao-remote-test-report-2026-09-30.md)记录自报 2.6.2 的 sdk-test 专用夹具 Core/Transit 全场景通过，也保留此前 KV List/HMAC 失败及修复过程；它不属于当前指纹，也不是跨版本兼容认证。正式独立 fixture 使用另一份可信 2.6.3 输入；最低 Go、平台和扫描结果均必须匹配当前源码。复现步骤见[发布检查](release.md#2026-10-01-续作验证与复现)。

最低版本使用 `python -B scripts/normal-test.py --profile minimum-go` 和 `python -B scripts/consumer-test.py --profile minimum-go --offline-proxy <已准备目录>`。它们分别生成 `MINIMUM_GO_COMPATIBILITY` / `MINIMUM_GO_CONSUMERS`，包含实际 Go 版本日志及平台；要求恰为 1.25.0，缺失时在工作命令前 BLOCKED。普通、race、覆盖率、vet、模块校验、构建与独立消费者的标准保持不变。主证据要求恰为 1.26.8；两类证据不能互换，Linux 与 Windows 也不能互相代替。发布门禁另要求四份最低版本报告，CI summary 独立校验对应四个作业。

固定漏洞扫描使用主工具链 Go 1.26.8。Go 1.25.0 的运行仅证明声明下限兼容，不代表该旧工具链获得安全扫描通过或适合作为当前生产构建版本。

`deploy/test/server-lock.json` 已为 `PINNED`，版本为 2.6.3、模式为 binary，二进制摘要与[可信来源记录](evidence/OB-018/release-validation-2026-10-01/tool-provenance.json)一致；该状态只表示输入固定，不表示集成通过。锁文件属于 v2 指纹，测试通过后不能修改锁文件再复用旧报告。

## 数字与签名协议边界

`kv.Document` 的本地解析、`Decode` 和 `RevealJSON` 使用精确 JSON 数字；AC-006 的 `9007199254740993` 本地往返测试保留。服务端存储和返回的精度另受后端实现约束：本轮对固定 OpenBao 2.6.3 的独立原生 HTTP 探针确认，同一数字写入返回 200，读回却为 `9007199254740992`。[该版本 KV 实现](https://github.com/openbao/openbao/blob/v2.6.3/builtin/logical/kv/path_data.go)在读取版本数据时使用普通 `json.Unmarshal` 解码到 `map[string]interface{}`；SDK 无法恢复服务端已经丢失的位数。要求端到端精确的大整数标识应由调用方按业务契约编码成 JSON 字符串，SDK 不自动改变字段类型。真实 KV 生命周期夹具使用该后端可精确表示的整数，仍验证原值、版本、CAS、删除与恢复。首次失败与独立对照日志见本轮[集成诊断](evidence/OB-018/release-validation-2026-10-01/integration-diagnosis.md)。

Ed25519 的公开 Profile 始终签署原始消息，`prehashed=false`。固定 2.6.3 的 [Transit 参数校验](https://github.com/openbao/openbao/blob/v2.6.3/builtin/logical/transit/path_sign_verify.go)会拒绝显式 `hash_algorithm=none`；SDK 对 Ed25519 省略该可选字段。ECDSA/RSA 的 SHA-256、ASN.1/PSS 与版本约束保持原语义，所有签名仍在客户端本地核验。

以下 R2/R3 记录保留原版本事实；它们不是本轮验证。

## R2 实际复核

2026-09-28，仍未下载到 `api/v2@v2.7.0` 完整模块。Go1.23.2只作为本轮实际运行工具链记录，不作为已确认的上游最低版本。官方go.mod、Sum、GoModSum继续未核实；没有手工填充go.sum。98个Go源码、go.mod、go.sum与R1逐字节一致。

新增`make dependency-check`：真实下载后才读取最低Go指令、记录源码/模块包哈希、tidy、校验依赖图及go.sum。该入口本轮在下载阶段返回BLOCKED，不是依赖已验证。具体恢复条件见`dependency-bootstrap.md`。


## R3 公共依赖转移入口

新增 `make dependency-export` 与 `scripts/dependency-bundle.py verify`；在新缓存中由真实 Go 下载/认证后才能生成转移包，接收端不预装 go.sum 再进行独立重放。本轮只有工具及拒绝测试通过，实际导出仍在 DNS 阶段失败。没有真实依赖包，Go 最低版本、官方适配器编译、服务端兼容和发布状态均未改变。使用说明见 `dependency-transfer.md`。98 个 Go 文件及 go.mod/go.sum 与 R2 相同。
