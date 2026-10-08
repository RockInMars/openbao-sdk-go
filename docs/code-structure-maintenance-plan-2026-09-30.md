# OpenBao SDK 代码结构渐进整理实施方案（2026-09-30）

> **执行者须知：** 实施时使用 `superpowers:executing-plans` 逐项核对。本文件是实施方案，不授权自动创建工作树、切换分支、暂存、提交、推送或访问远端服务；源码、测试和项目文档由主代理统一修改。

**Goal：** 降低远端测试入口和历史回归测试的导航成本，维持现有 Go 包边界、公开 SDK API 与运行语义。

**Architecture：** 根包 `bao` 继续拥有 `Client` 及其 KV、PKI、Transit、Diagnostics 方法；公开类型包与 `internal` 实现包不迁移。仅在同一 Go 包内按职责拆分过长测试文件，保留脚本 CLI 路径和当前单向依赖。

**Tech Stack：** Go 1.26.3 主验证环境、`go.mod` 声明下限 Go 1.25.0、官方 `github.com/openbao/openbao/api/v2` v2.7.0、Python 3.13、现有 RTK 与证据运行器。

**Spec：** [现有结构审查](code-structure-review-2026-09-29.md)、[结构调整报告](code-structure-refactor-report-2026-09-29.md)、[总体设计](spec/01-总体设计.md)、[接口契约](spec/02-接口契约.md)、[验收矩阵](spec/04-验收矩阵.md)。本方案只处理上次审查后仍有明确维护收益的局部结构问题，不重做已完成的工具层四项改进。

## 全局约束与执行触发

- 保持 module `github.com/RockInMars/openbao-sdk-go`、Apache-2.0 LICENSE、公开 API、`Client → engine → officialSender`、认证、权限、错误、取消、预算、重试、UNKNOWN 和不重放不确定写入语义。
- 不新增依赖、通用 `models`/`utils`/`service` 层、DI 容器或第二套任务台账；不全仓格式化。只格式化实际编辑的 Go 文件。
- 当前参考快照：`master` / `6d8251658e76f88aec5935126f303b2c59497691` 加未提交差异；source_identity v2 为 `dee54e4f2f49c24f71e8cd412064eac0286b2ba3f415c1850de05e82ef1d093b`，173 个输入。此值仅用于辨认历史，不作为执行时冻结基线。2026-09-30 检查到 101 个已修改、59 个未跟踪条目，暂存区为空；必须在实施前重新核对并保护全部已有修改。
- 方案初稿仅授权制定计划；2026-09-30 用户已明确授权在现有工作区执行任务 1—4。每个任务完成后继续下一个依赖已满足的任务，不在步骤边界反复询问。
- 远端 `sdk-test` 全场景报告属于上述参考旧指纹的补充证据。移动纳入 v2 指纹的 Go/Python 文件后，它只能保留为历史；若要宣称新指纹的远端场景 PASS，须另有明确的请求预算/时间窗和实际重跑，不复用或手改旧报告。

## 结构决策与文件职责

| 区域 | 决定 | 理由 |
| --- | --- | --- |
| 根包 15 个运行时 Go 文件 | 保留原目录；`client.go`、`kv_*`、`pki_*`、`transit_*`、`diagnostics_client.go` 继续按功能分文件 | `Client` 定义在根包；把其方法直接搬到子包会改变或破坏公开 API。文件数不是拆包依据。 |
| `auth/`、`kv/`、`pki/`、`transit/` 等公开类型包和 `internal/*` | 保留现有边界 | 根包引用叶子类型，`internal/engine` 和 `internal/authn` 的职责已清楚。 |
| `tests/remote/runner.go`（约 671 行） | 在同一 `remote` 包提取预算、预检和 KV 夹具辅助；`runner.go` 保留流程与结果编排 | 缩小阅读范围，不改变测试入口、配置、请求顺序或资源归属。 |
| `tests/remote/full.go`（约 308 行） | 本轮保留 | 四类全场景函数已分段；只有继续增长或同一功能连续修改时再按领域拆文件。 |
| `review_fixes_test.go`、`hardening_test.go` | 保留测试名和断言，将用例归入既有领域测试文件；必要时仅新建一个写入不确定性测试文件 | 历史批次名不利于按行为找回归；迁移不得改变测试语义或验收选择器。 |
| `scripts/` 顶层 CLI 与基础模块 | 本轮保留路径及导入关系 | 已完成依赖单向化、证据夹具和运行记录拆分；移动 CLI 会影响 Makefile/CI/报告消费者，当前没有对应收益。 |

## 审查重点

1. **远端预算耗尽：** `TestTLSFailureAndBudgetExhaustion` 与 `TestFullRequestBudgetsLeaveScenarioReserve` 仍须证明超额前停止，不额外发请求。
2. **不确定写入与回收：** `TestUnknownWriteIsNeverReplayed`、`TestLostCleanupResponseIsUnknownAndNotReplayed` 与 `TestCleanupFailureAndOwnershipMismatchRemainPending` 仍须保持 UNKNOWN、不得重放及待回收记录。
3. **权限和 namespace：** `TestRootCapabilityAndUnavailableIdentityPreventWrites`、`TestTransitKeyAdministrationPrivilegeIsRejected` 及 `TestCrossNamespaceConcurrency` 仍须证明越界/高权限输入不会变为允许写入。
4. **CLI 和普通测试隔离：** `tests/remote` 默认测试不得访问外部服务；带 `remote` tag 只编译，真实运行仍须显式配置。`scripts/remote-test.py`、Makefile 和 CI 调用保持原路径。
5. **证据归属：** `scripts/tooling.py` 的 v2 文件选择、POSIX 排序与原始字节算法不改；移动文件后 `verify-release` 必须拒绝旧指纹报告，且不得把零测试或远端补充报告认作正式集成。

以下清单保留原计划的操作与条件；实际执行状态、验证结果和阻塞以文末“执行记录”及原 `task-status.json` 为准。

## Task 1：执行前基线和目录导航

**输入：** 本方案与上列 Spec、当前 Git 差异、`README.md` 的“项目结构”段落。

**文件：** 仅在执行时定点修改 `README.md` 的现有“项目结构”段落；不另建目录说明文件。检查 `task-status.json`、`acceptance-results.json`、`scripts/acceptance-rules.json`，此任务不提前修改它们。

- [ ] 核对绝对仓库根、分支、HEAD、暂存/未暂存/未跟踪内容及本次相关进程；运行只读 source_identity，记录实际执行前清单。确认当前代码与本方案参考快照的差异，不回滚用户修改。
- [ ] 只补充根包门面、`tests/remote`、`scripts` CLI/基础模块的导航说明，链接现有总体设计和测试说明；不宣称结构迁移已经完成。
- [ ] 复查 README 链接及 `git diff -- README.md`。结束条件：读者可从现有结构段落定位运行时、类型包、内部实现、测试和工具入口，没有新公开包或新任务台账。

## Task 2：远端测试运行器同包拆分

**输入：** Task 1 的实际基线；`tests/remote/runner.go`、`tests/remote/full.go`、`tests/remote/runner_test.go`。先确认入口命令和测试副作用，不运行带凭据的真实服务测试。

**文件：** 新建 `tests/remote/budget.go`、`tests/remote/preflight.go`、`tests/remote/kv_fixture.go`；定点缩减 `tests/remote/runner.go`。保留 `tests/remote/full.go`、`config.go` 和 `scripts/remote-test.py` 的行为及 CLI 参数。

**接口：** 所有现有私有函数、方法和类型的名称、参数与返回类型原样保留；仅变更所在文件，不新增包边界。

- [ ] 先运行 `rtk proxy go test -mod=readonly -count=1 ./tests/remote`，核对目标测试真实执行并保存基线结果；若已有失败，先归因而不把迁移当作修复。
- [ ] 将 `budget` 及 `Observe`、`count`、`reserve`、`counts` 移至 `budget.go`；运行 `rtk proxy go test -mod=readonly -count=1 -run '^Test(TLSFailureAndBudgetExhaustion|FullRequestBudgetsLeaveScenarioReserve)$' ./tests/remote`，确认目标执行、请求上限、Observer 标签和计数语义未变。
- [ ] 将 `raw`、`checkTokenBudget`、`checkRemainingTokenBudget`、`requiredPermissions`、`contains` 移至 `preflight.go`；运行 `rtk proxy go test -mod=readonly -count=1 -run '^Test(InsufficientTokenLifetimeOrUsesPreventsMutation|RootCapabilityAndUnavailableIdentityPreventWrites|TransitKeyAdministrationPrivilegeIsRejected)$' ./tests/remote`，确认目标执行且没有新增网络请求或放宽权限。
- [ ] 将 `readOwner`、`writeKV`、`createKV` 移至 `kv_fixture.go`；运行 `rtk proxy go test -mod=readonly -count=1 -run '^Test(IsolatedKVAndCleanup|UnknownWriteIsNeverReplayed|CleanupFailureAndOwnershipMismatchRemainPending|LostCleanupResponseIsUnknownAndNotReplayed)$' ./tests/remote`，确认目标执行且日志与资源清单格式未变。
- [ ] 仅格式化上述四个被编辑的 Go 文件；重新运行 `rtk proxy go test -mod=readonly -count=1 ./tests/remote`。再用现有 Go 缓存将 `./tests/remote` 带 `remote` tag **编译**到本次不可覆盖的 `.artifacts/runs/<run_id>/`，不执行真实连接。结束条件：默认测试无远端副作用、聚焦和全包测试通过，现有 CLI/结果 JSON 字段兼容。

## Task 3：历史回归测试按行为归位

**输入：** Task 2 的稳定差异、两个历史测试文件、目标测试文件的现有 helper/import，以及 `scripts/acceptance-rules.json` 中按名称选择的用例。

**文件映射：**

| 来源用例 | 目标文件 |
| --- | --- |
| `TestExplicitEmptyCAFileIsNotSystemTrust` | `tls_pem_regression_test.go` |
| `TestStartAfterLifetimeCancellationDoesNotReportReady`、`TestStartResultBelongsToAttempt`（连同 `sequenceTokenProvider`）、`TestCloseDuringWrite` | `client_shutdown_regression_test.go` |
| `TestPublicKeyDecodeAttemptsNotHTTPStatus` | `transit_response_boundary_test.go` |
| `TestLimitsRejectIntegerOverflow` | `config_test.go` |
| `TestPKIZeroTTLRejected` | `pki_client_test.go` |
| `TestCrossNamespaceConcurrency`、`TestLoopbackTestConstructor` | `client_test.go` |
| `TestAmbiguousWrite4xxRetainsUnknown`、`TestVoidErrorEnvelope`、`TestAcceptedThenDisconnected` | 新建 `write_uncertainty_test.go` |

- [ ] 移动前运行 `rtk proxy go test -mod=readonly -count=1 -json .`，从真实 Run/Pass 事件记录上表 12 个测试的结果，并用 `rtk proxy go test -mod=readonly -list '^Test' .` 记录名称集合；保留已有失败，不改断言以迎合实现。
- [ ] 按上表一次迁移一组，保留包名、测试名、断言、fixture 行为及 R1/R2/R3 历史记录；删空的旧批次文件仅在核对没有其他内容后进行，不触及生产源码。
- [ ] 每组执行对应的 `rtk proxy go test -mod=readonly -count=1 -run '<对应 Test 名正则>' .`，确认至少一个目标实际运行；最后比较移动前后的测试名集合，并运行 `rtk proxy go test -mod=readonly -count=1 .`。
- [ ] 核对 `scripts/acceptance-rules.json` 的六处按名称选择器和文档引用；名称不变时不改规则。结束条件：原测试集合与语义不变，批次文件不再承载跨域用例，相关聚焦回归通过。

## Task 4：冻结、正式回归、证据和交接

**输入：** Tasks 1—3 的稳定差异。执行本任务前确定所有纳入 source_identity 的文件和测试输入；一旦后续再改这些文件，重新确定基线并重建受影响证据。

**文件：** 按真实结果定点更新 `README.md`、`docs/implementation-handoff.md`、本方案执行状态、`task-status.json` 与 `acceptance-results.json` 的原 OB/AC 关联；保持历史失败和旧指纹报告原样。新报告进入现有不可变 `.artifacts/runs/<run_id>/`，不另建状态系统。

- [ ] 运行 `rtk proxy python -B -m unittest discover -s scripts/tests -v` 和 `rtk proxy go test -mod=readonly -count=1 ./...`；核对真实测试数、退出码、无非预期 skip。
- [ ] 运行 `rtk proxy python -B scripts/normal-test.py`，仅使用本轮自然成功 race 产生的新 profile 验证 15 组 Go **语句**覆盖率均 ≥85%；再运行 `rtk proxy python -B scripts/fuzz-test.py`（五目标非零执行）、`rtk proxy python -B scripts/consumer-test.py --offline-proxy .artifacts/offline-consumer-proxy`（两个消费者）及 `rtk proxy python -B scripts/benchmark-test.py`（四个预期 benchmark 的启用子项三轮有效样本）。执行前从当前脚本核对命令和副作用，不下载工具或访问生产。
- [ ] 对稳定差异做独立只读审查，重点核对 remote 预算、权限、资源归属、清理与进程回收、公开 API、验收选择器和日志脱敏；主代理修复成立问题并重跑受影响检查。
- [ ] 运行 `rtk proxy python -B scripts/verify-release.py`。若独立集成、固定扫描器、Linux/最低 Go 或远端 CI 前置条件仍缺失，保持 BLOCKED/NOT_RUN；不得用旧 `sdk-test` 补充报告替代正式 `REAL_OPENBAO`。
- [ ] 仅在获得新一轮明确的 `sdk-test` 资源/请求预算后才重跑真实全场景。否则旧远端报告标为历史，不宣布当前指纹的远端功能 PASS。
- [ ] 复算最终 source_identity、报告前后标识和文件哈希；核对工作树、暂存区、本次 owned 进程/资源以及 README、台账、验收入口与证据索引。结束条件：实现、当前可执行本地验证和证据归属一致；外部阻塞逐项列明，不自动提交或发布。

## 执行记录（2026-09-30）

执行工作区为 `master` / HEAD `6d8251658e76f88aec5935126f303b2c59497691` 加既有未提交修改。开始时 source_identity v2 为 `dee54e4f2f49c24f71e8cd412064eac0286b2ba3f415c1850de05e82ef1d093b`（173 输入）；最终源码冻结为 `96a62a5230ff7e114831e39ab62b7c280e0708ac806b85fa19a053bc36be91f5`（175 输入），逐文件摘要见[源码清单](evidence/OB-018/structure-maintenance-2026-09-30/source-manifest.json)。

- **Task 1 完成：** 定点完善 README 原“项目结构”段落，链接总体设计与测试说明；链接存在，差异检查通过。
- **Task 2 完成：** `tests/remote` 同包拆出预算、预检、KV 夹具；迁移前 24 个顶层测试全部 Run/Pass，三组聚焦测试与全包测试通过，`remote` tag 仅编译成功，没有连接外部服务。
- **Task 3 完成：** 12 个历史回归测试与专属 helper 按行为归位，六个验收名称选择器保持不变。首次名称比对 69→68 发现目标文件原有 `TestCloseCancelsHealthBeforeStart` 被覆盖；从 HEAD 精确恢复后该测试及根包全集通过，最终名称集合 69→69 完全一致。首次失败和修复见[名称审计](evidence/OB-018/structure-maintenance-2026-09-30/name-audit.json)。
- **Task 4 本地工作完成，正式交付仍阻塞：** 当前指纹的工具测试 118 项、normal 六步骤/349 个 Go 测试与子测试结果、15/15 组 Go 语句覆盖率（最低 `auth` 86.49%）、五目标 fuzz、两个离线消费者、六个 benchmark 子项各三轮均通过。独立[只读审查](evidence/OB-018/structure-maintenance-2026-09-30/independent-review.md)在指定迁移范围无可证实发现。正式集成与固定扫描入口生成当前指纹的 BLOCKED 报告；Linux、最低 Go、远端 CI 和新指纹 `sdk-test` 场景未运行。[证据索引](evidence/OB-018/structure-maintenance-2026-09-30/index.json)保存报告路径、退出及哈希；`verify-release` 退出 1，保留 25 项实质拒绝。

Ruling：用户明确要求在现有工作区连续执行并禁止工作树、分支及 Git 发布操作，因此不采用技能默认的隔离工作树、独立 ledger 和提交步骤；进度写回原 `task-status.json` 与 `acceptance-results.json`。Ruling：本轮是已覆盖行为的代码与测试搬移，按用户明确要求执行迁移前后现有测试和名称集合核对，不制造新语义或人为失败用例。旧 `sdk-test` 全场景报告仅归属迁移前指纹，不转记当前远端 PASS；没有新请求/资源预算，未重跑。未暂存、提交、推送或发布。

## 明确暂缓与退出条件

- `config.go` 的 `normalizeConfig` 仅在实际增加配置校验时，按地址、认证、TLS、预算/限额在同一 `bao` 包内局部提取；本轮不为它新建 `config/`。
- `scripts/` 的 CLI 和基础模块只在出现新的重复逻辑或反向依赖时再考虑私有包；不能先搬文件再修改 Makefile/CI 来制造整理成果。
- `tests/remote/full.go` 仅在后续全场景持续增长时按领域拆为同包文件；当前四类函数保持原样。
- 任何包移动若要求修改消费者导入路径、公开方法接收者、SDK 语义或验收规则，即超出本方案；停止该移动并先做契约评审。
- 任务 1—4 的实施范围完成，或仅余明确外部条件阻塞时结束。最终报告区分结构调整完成、本地验证通过、远端指定场景通过、正式交付门禁通过与发布。
