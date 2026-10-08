# OpenBao Go SDK 代码结构调整执行报告

日期：2026-09-29。结论：**本地结构改进完成，适用的 Windows 本地验证通过；正式交付门禁仍阻塞，未发布。** 本轮完成四项主体调整，另外两项按用户定义的按需范围暂缓。没有重新执行或重构已经完成的七阶段工程设计。

## 1. 当前基线与范围

工作目录为 `E:\xen\code\claude\project\jiupiao\openbao-sdk-go`，分支 `master`，HEAD 保持 `6d8251658e76f88aec5935126f303b2c59497691`。本轮比较基线是开始时的实际未提交工作树快照，不是单独的 HEAD：

- [开始快照及清单](E:/xen/code/claude/project/jiupiao/openbao-sdk-go/.artifacts/runs/structure-refactor-20260929-start/baseline.json)：v2 `cd46dc83078962d602886183bba7aa4a77f60702df5a4a167a4340f529fe3af2`，159 个输入。
- [最终输入逐文件摘要](E:/xen/code/claude/project/jiupiao/openbao-sdk-go/docs/evidence/OB-018/structure-refactor-2026-09-29/source-manifest.json)：v2 **`771ac8fe1964aaebc7dd258d1060a34bbb8781e823ea86b182f8a465d9f8be0c`**，163 个输入。增加的是四个 Python 测试/fixture 文件。所有冻结后的正式报告前后标识一致。
- [本轮证据总索引](E:/xen/code/claude/project/jiupiao/openbao-sdk-go/docs/evidence/OB-018/structure-refactor-2026-09-29/index.json) 包含不可变报告、日志/profile 摘要、补充收据、首轮失败和重跑记录。原始 `.artifacts` 文件须连同索引保留；仅复制索引不构成完整证据包。
- 使用现有 Windows/amd64、Go 1.26.3、Python 3.13.13、模块缓存和已准备的本地签名代理。未安装、升级或下载工具；Go 验证保持约定的离线环境。工具测试以 `PYTHONUTF8=0` 检查显式 UTF-8 读写。

## 2. 六项建议逐项结果

| 建议 | 状态 | 实际变化与维护收益 |
|---|---|---|
| 1. 基础工具与验收单向依赖 | 已完成 | 删除 `tooling.release_problems` 内部转发；CLI 和所有实际测试调用方直接导入 evidence 并显式传入 root。基础工具不再反向导入验收模块。保留原 source_identity/coverage 基础实现，没有按函数新增文件。 |
| 2. 证据校验职责拆分 | 已完成 | 在 evidence 同模块提取公共 envelope、Go 版本要求及 normal、integration、consumer、security、fuzz、tooling、static 七类私有校验；统一 `report_results` 返回结构与 `release_problems` 独立 OB/AC 汇总保持兼容。类别要求和固定预期 argv 仍由验证器独立判断。 |
| 3. 独立证据测试夹具 | 已完成 | 新增普通 `EvidenceFixture(root)` builder。验收来源和 CI summary 测试分别拥有临时目录与 patch 生命周期，不再实例化另一个 TestCase 或调用其 setUp/doCleanups，不依赖共享 stopall。合成报告只用于临时测试。 |
| 4. 通用运行记录与 Go 探测分离 | 已完成 | `RunRecord` 复用初始化、收据、标识和不可变写入；构造对象不启动 Go。六个 Go runner 显式 `probe_go()`；`RunStopped` 返回已记录的致命状态，CLI `run_cli()` 负责退出码转换。保留 cleanup_incomplete 锁存、后续命令禁止、自然退出分类、latest 原子更新和不可覆盖报告。Python record/tooling 可在无 Go PATH 下运行。 |
| 5. 配置校验整理 | 按需暂缓 | 本轮没有触及 config.go。增加私有配置函数没有本轮必要性，避免扩大行为风险；OB-003 仍以本轮实际配置测试证据逐项核对，未虚报结构整理完成。 |
| 6. 历史 Go 测试归属整理 | 按需暂缓 | 没有修改 review_fixes_test.go、hardening_test.go 或相关 Go 用例，无需移动测试。原测试名称、断言、选择器及 R1/R2/R3 历史保持。 |

关键实现：[tooling.py](E:/xen/code/claude/project/jiupiao/openbao-sdk-go/scripts/tooling.py)、[evidence.py](E:/xen/code/claude/project/jiupiao/openbao-sdk-go/scripts/evidence.py)、[reporting.py](E:/xen/code/claude/project/jiupiao/openbao-sdk-go/scripts/reporting.py)、[record.py](E:/xen/code/claude/project/jiupiao/openbao-sdk-go/scripts/record.py)、[verify-release.py](E:/xen/code/claude/project/jiupiao/openbao-sdk-go/scripts/verify-release.py)。六个 runner 仅接入显式 Go 探测和 CLI 状态转换。

关键测试：[evidence_fixture.py](E:/xen/code/claude/project/jiupiao/openbao-sdk-go/scripts/tests/evidence_fixture.py)、[test_evidence_fixture.py](E:/xen/code/claude/project/jiupiao/openbao-sdk-go/scripts/tests/test_evidence_fixture.py)、[test_module_boundaries.py](E:/xen/code/claude/project/jiupiao/openbao-sdk-go/scripts/tests/test_module_boundaries.py)、[test_reporting.py](E:/xen/code/claude/project/jiupiao/openbao-sdk-go/scripts/tests/test_reporting.py)。补充缺工具、探测失败、超时、中断、清理失败、重复 run_id、基线变化锁存和无 Go 的真实 Python 执行回归；原类别负向测试没有削弱。

## 3. 实际验证与范围

以下列出实际检查进程命令。外层 shell 使用 `rtk proxy`；工具/补充命令通过 `record.py` 保存实际 argv、cwd、自然退出码，正式 Go runner 使用自身报告机制。完整参数、run_id、工具版本及日志 SHA 见链接，不把包装器成功当成子进程成功。

| 检查进程命令 | 结果 | 当前证据与范围 |
|---|---|---|
| `python -B -m unittest discover -s scripts/tests -v` | PASS，自然退出 0 | [正式 tooling 收据](E:/xen/code/claude/project/jiupiao/openbao-sdk-go/.artifacts/runs/structure-refactor-20260929-final/tooling-structure-tools-final-01.json)：106 项，无失败或 skip；重构前为 90 项。 |
| `python -B -m unittest discover -s scripts/tests -p test_release_provenance.py -v` | PASS，0 | [独立来源测试](E:/xen/code/claude/project/jiupiao/openbao-sdk-go/.artifacts/runs/structure-refactor-20260929-final/provenance-structure-provenance-final-01.json)：14 项。 |
| `python -B -m unittest discover -s scripts/tests -p test_ci_summary.py -v` | PASS，0 | [独立 CI 测试](E:/xen/code/claude/project/jiupiao/openbao-sdk-go/.artifacts/runs/structure-refactor-20260929-final/ci-tests-structure-ci-tests-final-01.json)：12 项。两组也在全集组合运行中通过。 |
| `python -B scripts/normal-test.py` | PASS，0 | [normal 报告](E:/xen/code/claude/project/jiupiao/openbao-sdk-go/.artifacts/runs/2026-09-29T135041.840129Z-6eddb617defa40c0be49641fa4e34be7/normal/report-007.json)：unit、race/coverage、vet、mod verify、build、module graph 六项均自然成功；291 个测试/子测试结果，不把它说成 291 个独立顶层测试。 |
| `python -B scripts/fuzz-test.py` | PASS，0 | [fuzz 报告](E:/xen/code/claude/project/jiupiao/openbao-sdk-go/.artifacts/runs/2026-09-29T135317.950426Z-f86b6e7019904de4b004c4ad28d9ecfa/fuzz/report-006.json)：五目标各约 30 秒，FuzzPath / Document / PKICSR / Encoding / Decode 分别有 111289 / 49335 / 301880 / 388849 / 765840 次执行，无崩溃语料。 |
| `python -B scripts/benchmark-test.py` | PASS，0 | [benchmark 报告](E:/xen/code/claude/project/jiupiao/openbao-sdk-go/.artifacts/runs/2026-09-29T135720.358360Z-690c6652ffbf4d1580e2ef6b8e329507/benchmark/report-002.json)：四个预期 benchmark 的六个启用子项均三轮、非零迭代。Executor 每轮 256 个独立延迟样本；不从均值推导分位数，不宣称性能提升或生产 SLA，不新增发布门槛。 |
| `python -B scripts/consumer-test.py --offline-proxy .artifacts/offline-consumer-proxy` | PASS，0 | [独立消费者报告](E:/xen/code/claude/project/jiupiao/openbao-sdk-go/.artifacts/runs/2026-09-29T135840.986917Z-ba1b5b65ca95455182a404c67ade609a/consumer/report-007.json)：reader/signer 各 tidy、test 自然成功，fresh cache 下指定 Test 有 run/pass、零 skip。仅本地签名代理，无外部下载回退。 |
| `go test -mod=readonly -count=1 -json -timeout=120s ./examples/...` | PASS，0 | [示例测试](E:/xen/code/claude/project/jiupiao/openbao-sdk-go/.artifacts/runs/structure-refactor-20260929-final/examples-test-examples-test-structure-01.json)：3 个有测试包、9 项 run/pass，无 skip。 |
| `go build -mod=readonly ./examples/...` | PASS，0 | [示例构建](E:/xen/code/claude/project/jiupiao/openbao-sdk-go/.artifacts/runs/structure-refactor-20260929-final/examples-build-examples-build-structure-01.json)。 |
| `python -B scripts/integration-test.py` | BLOCKED，runner 退出 2 | [integration 报告](E:/xen/code/claude/project/jiupiao/openbao-sdk-go/.artifacts/runs/2026-09-29T135127.370221Z-11e221b716534d87ae576389726648a9/integration/report-001.json)：缺精确版本与可信服务器发布物；真实集成 NOT_RUN，没有启动实例。 |
| `python -B scripts/security-test.py` | BLOCKED，runner 退出 2 | [security 报告](E:/xen/code/claude/project/jiupiao/openbao-sdk-go/.artifacts/runs/2026-09-29T135152.834752Z-d33c6a5c42c24faebf0dcea24b6de3b5/security/report-001.json)：缺工具/数据库准备及下载授权；实际扫描 NOT_RUN，不能解释为零漏洞。 |
| `python -B scripts/ci-summary.py --artifacts .artifacts/runs/structure-refactor-ci-final-v2 --output .artifacts/runs/structure-refactor-20260929-final/local-ci-summary-v2.json` | FAIL，1，预期拒绝 | [本地汇总](E:/xen/code/claude/project/jiupiao/openbao-sdk-go/.artifacts/runs/structure-refactor-20260929-final/local-ci-summary-v2.json)：Windows tooling、normal PASS；其余六个 Linux job 缺报告。不是远端 CI 运行。 |
| `python -B scripts/verify-release.py` | FAIL，1 | [发布门禁收据](E:/xen/code/claude/project/jiupiao/openbao-sdk-go/.artifacts/runs/structure-refactor-20260929-final/verify-release-structure-release-final-01.json)：25 条拒绝原因，见下节。 |
| `git diff --check` | PASS，0 | [差异检查](E:/xen/code/claude/project/jiupiao/openbao-sdk-go/.artifacts/runs/structure-refactor-20260929-final/diff-check-structure-diff-check-final-01.json)。 |

[覆盖率分析](E:/xen/code/claude/project/jiupiao/openbao-sdk-go/docs/evidence/OB-018/structure-refactor-2026-09-29/coverage-analysis.json) 只读取本轮自然成功 race 生成的 profile，15/15 组达到 85%，最低根包为 85.498%。这是 Go 语句覆盖率，不是物理行覆盖率；没有读取旧 coverage.out 补通过。

benchmark 子项是 `Document/small_64`、`Document/large_65536`、`CertificateValidation/p256_chain2`、`TransitEncoding/payload_32`、`TransitEncoding/payload_4096`、`ExecutorConcurrent/slots8`。采样存在波动，原始三轮数值均保留，不作不受证据支持的性能比较。

## 4. 首次失败、重跑与独立审查

- 重构前 90 项工具测试及四个聚焦 Go 测试均实际通过；补充 Go JSON 收据确认四个目标各有 run/pass。基线结果仅属于旧指纹。
- 三个预期 RED 均保留：反向动态导入、CI 测试导入另一个 TestCase、Run 构造隐式启动 Go。[RED/GREEN 收据目录](E:/xen/code/claude/project/jiupiao/openbao-sdk-go/.artifacts/runs/structure-refactor-20260929-start) 和总索引同时保留后续通过结果。
- 首次本地 CI 导出错误使用了不可变报告路径，而 summary 要求固定 `.artifacts/*-report.json` 入口，导致八个 job 均拒绝。[首次失败](E:/xen/code/claude/project/jiupiao/openbao-sdk-go/.artifacts/runs/structure-refactor-20260929-final/local-ci-summary.json) 保留；换成与 CI 一致的固定入口，在新目录重导出后 Windows 两项通过、六个缺失 Linux job 仍失败。没有修改校验规则或手工拼出报告。
- Shell/编辑恢复：PowerShell 引号和原生命令编码问题改用 UTF-8/编码参数；context-mode 路径校验错误指向另一项目后，改用本仓库明确绝对路径的有界原生读取，未修改工具权限。台账大补丁先遇 Windows 长度限制，随后单 hunk 写入在中途失败；先核对仍为有效 JSON 和已完成边界，再按小批补丁补齐，最终重新解析比对成功。没有回滚用户工作树、重复重启验证或把编辑失败算作测试成功。
- [独立 risk_reviewer 审查](E:/xen/code/claude/project/jiupiao/openbao-sdk-go/docs/evidence/OB-018/structure-refactor-2026-09-29/independent-review.md) 对本轮快照差异未发现可证实的新缺陷。审查只读、未运行测试；不存在需要修复却搁置的已成立发现。独立 verifier 执行 Go 正式检查并核对退出、目标和摘要。审查范围不覆盖全部历史未提交代码，也不替代缺失的真实集成、扫描或跨平台运行。

## 5. 台账、拒绝原因与剩余外部条件

[task-status.json](E:/xen/code/claude/project/jiupiao/openbao-sdk-go/task-status.json) 与 [acceptance-results.json](E:/xen/code/claude/project/jiupiao/openbao-sdk-go/acceptance-results.json) 是唯一状态入口。每项重新关联当前报告并按 acceptance-rules 核对目标，而不是只替换 hash。结构调整归属 OB-018；OB-003 的配置行为继续引用本轮对应测试，未新增配置整理完成声明。原七阶段、R1/R2/R3 和完整重构前台账快照均保留。

当前 AC 为 **66 PASS / 3 PARTIAL / 3 NOT_RUN**。AC-001、066、070 为 PARTIAL，AC-065、071、072 为 NOT_RUN；AC-072 是明确排除的真实业务迁移。全部 19 个 OB 保持 BLOCKED；门禁检查的 OB-001～018 没有因局部实现完成而改为 VERIFIED。

`verify-release` 的 25 条拒绝为：integration 和 scans 两类正式报告非成功；AC-001/065/066/070/071 未 PASS；OB-001～018 未 VERIFIED。报告的共用错误文字可能同时提及版本或陈旧，但本轮已核对前后指纹一致，实际原因是 integration/security 的 BLOCKED 和原任务依赖，不是旧证据失配。缺失运行证据与外部输入阻塞已分别记录。

| 剩余条件 | 影响 | 已查来源与下一步最小动作 |
|---|---|---|
| OpenBao 精确版本及可信镜像 digest，或本地二进制路径和 SHA256 | OB-001/016、AC-001/065/066/070，并阻塞后续原依赖 | 现有 server-lock 与本轮 integration 前置报告仍缺条件。提供并核验发布物；冻结前固定锁，再用仅本次临时实例运行真实集成。改锁会改变指纹，必须重建受影响正式证据。不要提供业务 Token。 |
| 固定 govulncheck/gitleaks 和真实漏洞数据库的可用环境或明确准备授权 | OB-018、AC-070/071 | 本轮 security 入口无下载授权而 BLOCKED。提供现成隔离扫描环境或明确授权准备；随后运行真实扫描。不能把未扫描解释为安全通过。 |
| 隔离 Linux、最低 Go 1.25.0 环境与另行授权的远端 CI/分发 | OB-001/018、AC-001/070 的完整交付范围 | 当前只验证 Windows Go1.26.3；CI summary 缺 Linux job。准备平台后以同字节快照运行，远端操作另行授权。没有擅自启动共享 WSL/服务或安装工具。 |

模块地址和 Apache-2.0 许可已经确定，不再作为缺失输入询问。没有其他可执行本地工程任务被无故留到下一轮。

## 6. 兼容性与工作区收尾

与开始快照逐字节比较，Go SDK 源码和 Go 测试、go.mod/go.sum、官方依赖声明、服务锁、Makefile/CI 输入均未改变。本轮未编辑 LICENSE；开始快照没有收录这个原有未跟踪文件，因此不把 LICENSE 的历史字节比较标成 PASS。收尾时误将它纳入快照保护断言而触发一次审计断言失败，核实后记录这一证据边界；当前文件仍为 Apache-2.0，摘要见工作区审计。公开 SDK API、`Client → engine → officialSender`、认证/权限、数据/错误、取消/预算/重试以及 UNKNOWN 和不得重放不确定写入的语义保持。module 仍为 `github.com/RockInMars/openbao-sdk-go`，api/v2 为 v2.7.0，Go 声明下限为 1.25.0。

[最终工作区审计](E:/xen/code/claude/project/jiupiao/openbao-sdk-go/docs/evidence/OB-018/structure-refactor-2026-09-29/workspace-audit.json) 记录 HEAD、暂存区、实际差异、相关未跟踪文件、输入摘要和本次 owned PID 核对。`.codex/config.toml` 的原有字节保持；发现 `.serena/project.yml` 的 `language_backend` 从空值变成 LSP，这不是本轮主动源码补丁，未擅自回滚该本机工具状态，不纳入源码指纹。其余开始快照文件没有被删除。

没有未关闭验证线程、遗留本次测试进程或临时实例；缓存和证据按授权保留。**未擅自暂存、提交、推送、切换分支、创建工作树、发布或操作生产数据。** 最后检查点和恢复动作见 [现有交接入口](E:/xen/code/claude/project/jiupiao/openbao-sdk-go/docs/implementation-handoff.md)。
