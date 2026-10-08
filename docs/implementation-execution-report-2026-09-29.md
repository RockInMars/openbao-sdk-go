> 历史范围说明：本文件保留七阶段实施结束时的结果与原指纹；后续结构调整的当前基线、回归及门禁状态见 [代码结构调整执行报告](code-structure-refactor-report-2026-09-29.md) 和 [现有交接入口](implementation-handoff.md)。请勿将下列历史 PASS 直接当作当前工作树证据。

# OpenBao Go SDK 完善实施结果（2026-09-29）

**本地改进完成，交付门禁仍阻塞；未发布。** 七阶段中可在现有授权及环境内执行的改进、测试、审查和文档已完成。真实 OpenBao、固定安全扫描、Linux/最低 Go 和远端渠道验证仍未完成。唯一任务台账仍是 [task-status.json](../task-status.json)，逐项验收仍是 [acceptance-results.json](../acceptance-results.json)。

## 基线和授权

- 工作目录：`E:\xen\code\claude\project\jiupiao\openbao-sdk-go`；分支 `master`；HEAD `6d8251658e76f88aec5935126f303b2c59497691`，包含本次未提交修改，不能只用 HEAD 表示测试版本。
- 最终 v2：`cd46dc83078962d602886183bba7aa4a77f60702df5a4a167a4340f529fe3af2`，159 个输入。[逐文件字节清单](evidence/OB-018/implementation-2026-09-29/source-manifest.json)。最终正式报告执行前后标识一致。
- 实测 Windows/amd64、Go 1.26.3、Python 3.13.13；使用既有 GCC 和模块缓存。Windows 工具回归显式 `PYTHONUTF8=0`，验证默认 cp936 环境中的 UTF-8 读写。
- 已按维护者确认将 module/import/消费者 require 同步到 `github.com/RockInMars/openbao-sdk-go`。复用[上游已有 Apache-2.0 LICENSE](https://github.com/RockInMars/openbao-sdk-go/blob/main/LICENSE)，本地文件 Git blob 为 `261eeb9e9f8b2b4b0d119366dda99c6fd7d35c64`，11357 字节。
- `Client → engine → officialSender`、公开函数签名及错误、认证、权限、重试、UNKNOWN 和不重放写入语义保持。只扩展示例内部 helper 的可选参数；正式 module 地址变化已获确认。官方 api/v2 v2.7.0、go.mod 下限 1.25.0、go.sum 未升级或重写；无新增生产依赖。
- 未 stage、commit、push、pull、merge、rebase、stash、创建工作树/分支/tag、发布或生产迁移。用户原有 `.codex/config.toml`、`.serena/` 及方案材料受到保护；暂存区为空。

## 七阶段结果

| 阶段及原任务 | 实际完成 | 明确边界 |
|---|---|---|
| 一：OB-001/018 | `tooling.py` v2 前缀、POSIX 排序、原始字节清单、UTF-8/Windows 路径；跨平台逻辑与黄金数据测试 | 旧报告保留原标识；项目指纹、依赖包输入指纹、上游 checksum 分开 |
| 二：OB-018，AC-001～071 | `acceptance-rules.json`、`evidence.py` 和 release 校验：当前版本、具体目标、argv/cwd、自然退出、日志摘要、依赖链 | 旧证据、缺字段、零测试、skip、越界路径、缺目标等拒绝测试实际通过；没有提前标绿 |
| 三：OB-004/009/011/013/015 | 六个回归文件覆盖预算、排队、取消、响应丢失/畸形、传输释放、重定向和 KV/PKI/Transit 边界；fresh race profile 全15组达标 | 保留正式工厂；无 overlay/replace/关闭 vet/降低门槛；语句覆盖不冒充物理行覆盖 |
| 四：OB-015/018 | 有限命令执行器、进程树回收、不可变 run_id、五目标 fuzz、CI 分 job 与 always 汇总、脱敏导出 | 缺报告/目标时失败；远端 CI 未运行；树回收不能确认会停止后续检查 |
| 五：OB-014/015 | 可编译可测试 Observer 示例、四个 benchmark 六子项三轮、独立256延迟样本 | 无公开 SDK API 扩展；无新增发布性能阈值，不宣称生产 SLA 或性能提升 |
| 六：OB-001/016/017/018 | 地址与许可落实；Windows normal/fuzz/consumer/benchmark 同字节冻结验证；消费者现支持有界本地离线代理 | 真实 OpenBao 与扫描受外部条件阻塞；PINNED 输入未提供，锁仍 NOT_VERIFIED；最低 Go/Linux/远端未验证 |
| 七：OB-018 | README、兼容/测试/发布/接入文档、原台账、证据、独立审查、verify-release 和最终工作区核对 | R1/R2/R3、首次失败与重跑保留；OB-019/AC-072 真实业务迁移不在范围内 |

## 实际验证与证据

下列命令均在仓库根目录运行，本地实际带 `rtk proxy`。原始命令 argv、cwd、超时、自然退出码、开始/结束时间、工具版本及日志摘要保存在报告内。普通 Go 环境为 `GOWORK=off`、`GOFLAGS=`、`GOTOOLCHAIN=local`、`GOENV=off`、`GOPROXY=off`；消费者显式改用准备好的 file proxy 和本地签名校验库。

| 实际入口 | 当前结果及范围 | 可复查报告 |
|---|---|---|
| `python -B -m unittest discover -s scripts/tests -v`，由 record.py 记录 | PASS，90项，无失败/跳过；自然退出0 | [工具回归](evidence/OB-018/implementation-2026-09-29/tooling-report.json) |
| `python -B scripts/normal-test.py` | PASS，六命令自然退出0；unit 291个测试/子测试结果，无跳过；其中142个顶层 Test、6个 fuzz 种子入口，不把重复运行相加 | [正式工厂](evidence/OB-018/implementation-2026-09-29/normal-report.json) |
| `python -B scripts/fuzz-test.py` | PASS，五目标各30～31秒，真实非零执行次数 | [逐目标 fuzz](evidence/OB-015/implementation-2026-09-29/fuzz-report.json) |
| `python -B scripts/benchmark-test.py` | PASS，六子项各三轮，非零迭代 | [性能原始结果](evidence/OB-015/implementation-2026-09-29/benchmark-report.json) |
| `python -B scripts/consumer-test.py --offline-proxy .artifacts/offline-consumer-proxy` | PASS，reader/signer 各自 tidy 与 race 测试退出0，两个指定 Test 各运行一次 | [独立消费者](evidence/OB-017/implementation-2026-09-29/consumer-report.json) |
| `go test -mod=readonly ./examples/...` | PASS；3个测试包、4个无测试包，专项命令部分命中缓存；normal 的 `-count=1` 已实跑 Observer | [示例测试](evidence/OB-014/implementation-2026-09-29/examples-test.json) |
| `go build -mod=readonly ./examples/...` | PASS，自然退出0 | [示例构建](evidence/OB-014/implementation-2026-09-29/examples-build.json) |
| `python -B scripts/integration-test.py` | BLOCKED，脚本自然退出2；仅前置检查，未运行真实业务测试 | [真实集成前置报告](evidence/OB-016/implementation-2026-09-29/integration-report.json) |
| `python -B scripts/security-test.py` | BLOCKED，脚本自然退出2；工具/数据库未准备，不能称零漏洞 | [扫描前置报告](evidence/OB-018/implementation-2026-09-29/security-report.json) |
| YAML 静态解析与固定 jobs/needs/always/action SHA 检查 | PASS；只证明本地结构，不是远端 CI | [静态 CI 收据](evidence/OB-018/implementation-2026-09-29/ci-yaml.json) |
| `python -B scripts/ci-summary.py --artifacts .artifacts/runs/final-ci-export-v2 --output .artifacts/runs/final-supplemental/local-ci-summary-v2.json` | FAIL，退出1；真实 Windows tooling/normal 导出通过，其余6个Linux job目录缺失时正确拒绝 | [本地 CI 汇总](evidence/OB-018/implementation-2026-09-29/local-ci-summary.json) |
| `python -B scripts/verify-release.py` | FAIL，退出1；2类正式报告、5项AC及18项OB状态未满足，共25个拒绝原因 | [发布门禁收据](evidence/OB-018/implementation-2026-09-29/verify-release.json) |

`record.py` 包装“退出2的前置脚本”时，命令收据状态为 FAIL、真实内层退出码为2；对应正式报告状态为 BLOCKED。RTK 外层可能统一返回1，不能据此覆盖内层退出码。原始日志/profile 保存在不可变 `.artifacts/runs/`；[证据索引](evidence/OB-018/implementation-2026-09-29/index.json) 列出原始位置、原始摘要与归档副本。归档不重写源码 hash；移动交付包时须同时保留引用的原始产物。

normal 的六条实际 Go 命令如下，race 的绝对 profile 路径由 run_id 唯一确定：

```text
go test -mod=readonly -json -count=1 -timeout=120s ./...
go test -mod=readonly -race -count=1 -timeout=180s -coverprofile=E:\xen\code\claude\project\jiupiao\openbao-sdk-go\.artifacts\runs\2026-09-29T044908.236076Z-f0280f637e9342a0b07ac5a6dd583ff0\normal\coverage.out ./...
go vet -mod=readonly ./...
go mod verify
go build -mod=readonly ./...
go list -mod=readonly -m -json all
```

覆盖 profile：`.artifacts/runs/2026-09-29T044908.236076Z-f0280f637e9342a0b07ac5a6dd583ff0/normal/coverage.out`；SHA256 `db34a47ec6d08cbffef27faf51ef4ffb3d7f4ac6fb565e310c5fe02fec4d62b8`。`coverage-check.py --profile <该路径>` 已实际运行并通过，没有读取旧根目录 coverage.out。[覆盖率分析](evidence/OB-015/implementation-2026-09-29/coverage-analysis.json)。

| 组 | Go语句覆盖率 |
|---|---:|
| package:. | 85.50% |
| package:internal/engine | 95.62% |
| package:internal/authn | 90.60% |
| package:auth | 86.49% |
| package:kv | 88.00% |
| package:pki | 98.57% |
| package:baoerr | 100.00% |
| package:sensitive | 90.91% |
| package:internal/jsondoc | 90.11% |
| package:internal/pkiutil | 90.80% |
| package:internal/transitutil | 95.40% |
| package:internal/pemutil | 100.00% |
| domain:kv | 91.88% |
| domain:pki | 90.49% |
| domain:transit | 87.32% |

| fuzz目标 | 最后进度时间 | 实际执行数 |
|---|---:|---:|
| FuzzPath | 30s | 135601 |
| FuzzDocument | 31s | 10284 |
| FuzzPKICSR | 31s | 261605 |
| FuzzEncoding | 30s | 298861 |
| FuzzDecode | 30s | 550219 |

## 性能记录

Go 1.26.3 / Windows AMD64，CPU记录为 `Intel64 Family 6 Model 165 Stepping 2, GenuineIntel`。实际执行参数为 `go test -mod=readonly -run=^$ -bench=. -benchmem -benchtime=1s -count=3 ./kv ./internal/pkiutil ./internal/transitutil ./internal/engine`，由 Python argv 传递以避免 PowerShell 错分 `-bench=.`。输入、P-256算法、并发8槽和采样定义见 [测试说明](testing.md)。这是一台现有开发机的观测基线，不是优化前后比较。

| 子基准 | 三轮 ns/op | 三轮 B/op | 三轮 allocs/op |
|---|---|---|---|
| BenchmarkDocument/small_64 | 12719.0 / 10669.0 / 11252.0 | 3064.0 / 3064.0 / 3064.0 | 45.0 / 45.0 / 45.0 |
| BenchmarkDocument/large_65536 | 2895988.0 / 3057672.0 / 3486375.0 | 800443.0 / 800442.0 / 800446.0 | 59.0 / 59.0 / 59.0 |
| BenchmarkCertificateValidation/p256_chain2 | 297211.0 / 315183.0 / 337488.0 | 13073.0 / 13072.0 / 13072.0 | 121.0 / 121.0 / 121.0 |
| BenchmarkTransitEncoding/payload_32 | 1694.0 / 2708.0 / 3312.0 | 352.0 / 352.0 / 352.0 | 7.0 / 7.0 / 7.0 |
| BenchmarkTransitEncoding/payload_4096 | 117824.0 / 50522.0 / 57487.0 | 35632.0 / 35632.0 / 35632.0 | 7.0 / 7.0 / 7.0 |
| BenchmarkExecutorConcurrent/slots8 | 1177492.0 / 831753.0 / 984858.0 | 63783.0 / 65950.0 / 67023.0 | 374.0 / 374.0 / 385.0 |

执行器每轮计时区外独立采样256个端到端延迟，包含排队及TLS/HTTP；使用 nearest-rank，未从 ns/op 均值推导分位数：

| 轮次 | worker | 样本 | P50 ms | P95 ms |
|---|---:|---:|---:|---:|
| 1 | 12 | 256 | 14.2643 | 51.0379 |
| 2 | 12 | 256 | 8.0037 | 24.6298 |
| 3 | 12 | 256 | 8.0831 | 32.6925 |

## 失败、修复和独立审查

[独立审查记录](evidence/OB-018/implementation-2026-09-29/review.md) 列出9项成立问题及只读复核：命令替换造成假通过、CI原始日志及动态名称泄漏、A→B→A指纹失效、进程树回收、集成锁一致性、Go版本证据、畸形Test字段、离线路径解析到UNC。主代理增加失败回归后最小修复；最终90项工具回归通过，限定复核无剩余成立问题。Go/module/Observer/benchmark/边界测试差异也经独立只读审查；运行验证由 verifier 单独执行。静态审查不代替真实服务、安全扫描或未执行平台。

- 初始35项Python回归出现Windows编码/路径失败；保留 `.artifacts/runs/baseline-python-1803d892ccca46d58d85019a295c8612.log`。
- 最初覆盖命令相对路径被错误解析，改为唯一绝对profile后自然成功；不读失败留下的旧coverage。
- 首次Observer/PKI测试期望与既有错误码/序列号路由不符，依据契约修正测试，未改正常实现迎合数字。
- 首次benchmark PowerShell参数拆分使进程退出0但没有benchmark；保留 `bench-checkpoint/benchmark-bench-01.json`，它不是性能通过证据。正式runner明确拒绝零目标/缺轮次。
- 首次正式normal六命令均退出0，但Go1.26无测试文件的包级skip触发严格解析失败；用真实JSON固定格式，仍拒绝真实测试skip、空/null Test及零测试。失败报告保留在 `.artifacts/runs/2026-09-29T040650.171064Z-7907a7584de64082856d1d7cff17d5fd/normal/report-007.json`。
- 离线消费者最初因代理尚未准备完整而BLOCKED，随后真实通过；再因UNC修复更新源码，所有当前报告重新运行。代理仅从已有公开缓存复制模块及签名材料，Go在fresh缓存自行认证，没有预置go.sum/ziphash或关闭SUMDB。未下载工具或依赖。
- R1/R2/R3、红绿日志、超时/中断记录和被后续指纹替代的报告均保留，未改hash复用。历史记录不属于当前PASS。

## 原台账及剩余输入

72项验收为 **66 PASS / 3 PARTIAL / 3 NOT_RUN**。PARTIAL为AC-001、066、070；NOT_RUN为AC-065、071，以及本次排除的AC-072。全部PASS均匹配当前规则的实际目标。OB-001因真实服务基线缺失未VERIFIED，原依赖链随之保留BLOCKED；OB-019另标OUT_OF_SCOPE，不把19个BLOCKED解释为还有19项本地编码工作。

| 仍缺条件 | 影响 | 已查来源与最小后续动作 |
|---|---|---|
| 精确OpenBao版本 + 可信镜像digest，或可用本地二进制及SHA256 | OB-001/016，AC-001/065/066/070及依赖 | 当前锁、环境输入、PATH均无合格发布物。维护者提供其一；先核验、在冻结前固定PINNED锁，再重建所有受新指纹影响的证据并运行真实集成。不要提供业务Token |
| govulncheck v1.1.4、gitleaks v8.24.3及真实漏洞数据库 | OB-018，AC-070/071 | PATH、go/bin和常规数据库缓存未发现可用输入。提供隔离扫描环境或明确授权非生产工具/数据库准备，再实际扫描；不能把下载失败记为无漏洞 |
| Linux、Go1.25.0工具链、远端CI/分发的执行条件 | OB-001/018及正式交付 | WSL只有两个Stopped Rancher发行版，未启动共享服务；最低工具链不可用。提供现有隔离平台和工具链，同字节源码检查；远端运行/分发另行授权 |
| 真实业务迁移材料 | OB-019/AC-072 | 本次明确排除；后续按业务仓库、契约和授权单独执行，不影响本次本地改进已完成的结论 |

维护者无需再确认模块地址或代选许可。只需补充上述仍缺条件；没有授权时不自动安装、启动共享服务、推送或触发远端CI。`PINNED`仅表示输入固定，不能用测试后改锁的方式继续沿用旧报告。

最终恢复入口仍为 [实施交接](implementation-handoff.md)。源代码或门禁输入再改动时，应重新计算指纹和必要证据；仅调整不参与指纹的交接说明不能把外部阻塞变成通过。

[最终工作区审计收据](../.artifacts/runs/final-audit/final-workspace-workspace-01.json) 自然退出0：分支/HEAD未变、暂存区为空、`git diff --check`无错误、用户配置差异仍为原14增5删、go.sum与服务锁未改、全部OB/AC标识一致、当前文档本地链接有效；所检查正式命令的记录PID均已结束。Git提示既有autocrlf设置未来可能转换换行，本轮未修改该设置，也未据此重写文件。
