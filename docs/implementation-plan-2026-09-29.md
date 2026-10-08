# OpenBao SDK 完善实施方案

> **For agentic workers:** 实施时使用 `superpowers:executing-plans`，按下列检查项推进。主代理负责源码、测试、契约及文档写入；定位、验证和独立审查沿用仓库既有只读角色。本文是实施方案，尚未执行，不授予提交、发布或生产操作权限。

**Goal：** 修复评估中确认的工程问题，使现行源码、验证报告、验收台账与接入说明一致，并完成具备可追溯证据的试用交付准备。

**Architecture：** 保持现有 `Client → engine → officialSender` 分层和公开 SDK 行为。先稳定验证工具及证据标识，再补覆盖与执行流程；在源码冻结后运行正式验证，最后更新台账。复用原 OB 任务和 AC 验收编号，本方案只细化执行顺序，不建立另一套任务状态系统。

**Tech Stack：** Go、Python 标准库、现有 Makefile、GitHub Actions、现有临时 OpenBao fixture；默认不增加生产依赖。

**Spec：** [项目评估](project-assessment-2026-09-29.md)、[原总体设计](spec/01-总体设计.md)、[接口契约](spec/02-接口契约.md)、[原任务计划](spec/03-实现任务计划.md)、[验收矩阵](spec/04-验收矩阵.md)。新工具字段和执行方式均为 proposed；不将本方案当作已通过的验收证据。

## 范围、基线与约束

- 编制基线：`master`，提交 `6d8251658e76f88aec5935126f303b2c59497691`。工作区已有 `.codex/config.toml` 修改、`.serena/` 和评估文档未跟踪，实施时先重新核对并保留。
- 评估记录：Windows/Go 1.26.3 下，普通测试、race、vet、build、模块校验通过；15 个覆盖组有 5 个低于 85%；Python 工具测试存在编码和路径问题。这些是评估时的结果，本次编制方案没有重跑运行测试。
- Scope：Windows 工具兼容、源码指纹、报告及台账版本校验、关键路径测试、命令超时、CI 证据、当前文档、分发准备、最小 Observer 示例与性能基线。
- Out of scope：新增管理 API、认证模式、业务缓存或重试语义；更换框架、全仓格式化、生产部署、真实业务迁移、自动提交或发布。OB-019/AC-072 的真实业务接入独立保留。
- 保持官方 api/v2 v2.7.0、Go 声明下限 1.25.0；本轮工程验证主基线拟固定为 Go 1.26.3，Linux 与 Windows 分别记录结果。声明下限不等于已验证的最低兼容版本。
- 不降低现有 85% 覆盖门槛，不使用 overlay、replace、skip 或关闭 vet 代替正式验证；补充契约证据保留原有类别。
- `task-status.json` 仍是唯一任务台账，`acceptance-results.json` 仍是验收结果入口；本文件的复选框只是执行检查点。原 `docs/spec/` 和历史失败记录保留。
- 主要风险：指纹升级使旧报告失效；错误迁移台账可能产生假通过；进程超时可能遗留子进程；CI 上传可能带入敏感材料。下列阶段分别给出控制与验证。
- Stopping condition：本地改进完成且适用检查通过后交付；未取得外部前置条件时，仅将依赖项标为 BLOCKED 并继续独立工作。只有全部正式门禁和逐项验收满足要求，才可称“具备试用交付条件”；实际发布另需授权。

## Review Focus

| 输入或失败条件 | 必须保持的行为 | 所属阶段 |
| --- | --- | --- |
| 相同字节、不同路径分隔符；本机代理配置变化 | 指纹一致；真正的源码、测试或门禁配置变化使指纹变化 | 一 |
| 当前报告搭配旧台账、缺失版本字段或旧 AC 证据 | 拒绝放行，并指出具体记录；历史证据有明确复核来源 | 二 |
| 命令超时、中断、缺工具或执行了零个目标测试 | 保存真实状态并返回失败，回收自己创建的进程，不伪造 PASS | 四 |
| 写入响应丢失、畸形响应、取消和并发排队 | 保持 UNKNOWN、不重放写入、遵守总预算和资源释放规则 | 三 |
| CI 缺报告、报告类别混淆、错误服务摘要、混入秘密 | 汇总失败；缺环境不全 skip；只留存允许的脱敏证据 | 四、六 |

## 与原任务、验收矩阵的映射

| 本方案工作 | 原任务与验收 | 说明 |
| --- | --- | --- |
| 文本、路径、指纹、台账与当前文档 | OB-001 / AC-001；OB-018 / AC-070 | Windows 工具问题属于工程子项，没有虚构新的既有 AC |
| 执行器、生命周期与认证回归 | OB-004…007 / AC-013…034；OB-015 / AC-063…064 | 按真实覆盖缺口补测，保留既有安全断言 |
| KV / PKI / Transit 回归 | OB-008…013 / AC-035…058 | 覆盖率是辅助验收，不替代各业务行为 |
| Observer 示例 | OB-014 / AC-061；示例交付关联 OB-017 | benchmark 没有现有独立 AC，作为可选改进记录，不新增发布硬门槛 |
| 真实集成、消费者、扫描和发布 | OB-016 / AC-065…066；OB-017 / AC-067；OB-018 / AC-070…071 | 工具测试不能替代这些实际运行证据 |
| 真实业务迁移 | OB-019 / AC-072 | 本轮范围外，保留外部依赖与状态 |

原任务依赖仍有效：OB-014 依赖 OB-009/011/013，之后依次为 OB-015、016、017、018、019。允许提前完善工具、文档和独立回归，但任务 VERIFIED 不能越过原依赖和关联验收要求。

## 阶段一：修复文本、路径与源码指纹

**输入：** 当前源码、现有 35 项 Python 工具测试及评估中的两个失败。**产出：** 跨平台可复现的工具行为、版本化项目指纹；不改变公开 SDK 类型。

**文件：** 修改 `scripts/tooling.py`、`scripts/tests/test_tooling.py`、`scripts/tests/test_dependency_bundle.py`；定点修正 `scripts/*.py` 和对应测试中处理仓库 UTF-8 文本的读写。新增 `scripts/tests/test_source_identity.py`。同步调用 `source_hash` 的 `normal-test.py`、`integration-test.py`、`consumer-test.py`、`security-test.py`、`dependency-check.py`、`record.py`。

**Interfaces / 拟定规则：**

- 保留 `source_files(root: Path)` 和 `source_hash(root=ROOT) -> str` 的调用方式；新增 `source_identity(root=ROOT) -> dict`，返回 `source_hash_version: 2` 与 `source_sha256: str`。各生产者通过同一入口取得标识。
- v2 计算：SHA-256 先写入固定前缀 `openbao-sdk-go/source-hash/v2\0`，再按 POSIX 相对路径排序，依次写入 UTF-8 路径、零字节、原始文件字节、零字节。文件内容不做隐式换行转换；不同字节仍表示不同快照。
- 建立明确输入清单：根目录 Go 文件、`go.mod/go.sum/Makefile`；现有源码、示例和测试目录下的 Go 文件及嵌套模块文件；`scripts/**/*.py`、`scripts/acceptance-rules.json`、`scripts/tests/fixtures` 下固定 fixture；`.github/workflows` 下 YAML；`deploy/test` 下 HCL/YAML/TOML 及 `server-lock.json`；各测试包实际使用的 `testdata` 固定材料。修改输入规则本身也改变指纹。
- 排除 `.git/.artifacts/.tools/.codex/.serena`、缓存、覆盖率、任务台账、验收 JSON、历史证据及纯 Markdown 文档。选中范围内的符号链接拒绝处理，不沿链接读取；新增构建输入须同步清单和测试。
- `dependency-bundle.py:input_fingerprint` 已使用 POSIX 路径，且只绑定依赖转移的 SDK 输入，继续保留其独立用途；不将它和项目验收指纹合并，不顺手改转移包格式。`dependency-check.py` 内上游文件校验字典也不得被批量替换成项目标识。
- 报告生产者在运行开始和结束分别记录源码标识；二者不同则记 FAIL，并说明 `baseline_changed`，即使命令退出 0 也不得 PASS。`record.py` 同样执行此检查，不能仅在命令结束后给变化过的源码贴上新哈希。

- [ ] 固定现有失败：在 Windows 默认编码下运行工具测试，保存编码错误和路径断言的原始结果；后续 Linux 测试另记，不覆盖首次记录。
- [ ] 为 UTF-8 文件读写显式指定 `encoding='utf-8'`；对子进程文本解码显式选 UTF-8。路径断言使用 `str(Path('/isolated') / 'modcache')` 等平台表达，保留隔离缓存、禁止凭证及校验绕过的断言。
- [ ] 增加 `test_identity_is_independent_of_root_and_path_separator`、`test_local_agent_config_is_excluded`、`test_code_tests_workflows_and_server_lock_change_identity`、`test_source_symlink_is_rejected`：分别验证相同 fixture 的 `hash(a) == hash(b)`、本机配置变更哈希不变、有效输入变更哈希不同和链接拒绝。
- [ ] 实现 v2 和生产者字段同步，为 UTF-8/POSIX 固定 fixture 固化一个预计算期望值，Windows/Linux 都必须匹配。旧报告缺少版本字段时按历史 v1 处理，不给旧报告补一个 v2 标签就继续使用。
- [ ] 运行 `rtk proxy python -B -m unittest discover -s scripts/tests -v`；必须执行原有用例和新增用例且全部通过。Windows 验证须覆盖 `PYTHONUTF8=0`，不能仅依赖全局 UTF-8 模式掩盖问题。

**验收：** 两项已知 Windows 问题解除；跨平台固定 fixture 一致；真实源码/门禁配置变更可检测；所有当前报告生产者输出 v2。历史报告可以查阅，但尚不满足新发布门禁。

## 阶段二：校验台账与逐项验收的版本归属

**依赖：** 阶段一的 `source_identity`。**文件：** 修改 `scripts/tooling.py`、`scripts/verify-release.py`、`scripts/tests/test_tooling.py`、`docs/release.md`；新增 `scripts/acceptance-rules.json`、`scripts/tests/test_release_provenance.py`。最终更新两份台账安排在阶段七，不在此处预先标绿。

**Interfaces / 拟定规则：** 保留 `release_problems(ledger, acceptance, reports, current_hash) -> list[str]`；当前版本由统一常量确定。问题列表必须能区分缺字段、过期基线、缺任务、缺 AC、类别错误和无效证据。

- 两份台账顶层、四类正式报告，以及当前 `VERIFIED/PASS` 的 OB-001…018、AC-001…071，均需携带 v2 和当前源码哈希；现有状态、必需项、无 skip、覆盖率等检查继续生效。
- 每项标绿记录保留非空证据引用。旧运行证据只进历史字段；需要运行才能成立的 AC 必须取得当前基线的运行证据，不能以影响评审替代。
- 纯静态检查可复用历史依据，但必须追加 `revalidation`：`method='impact_review'`、`previous_source_sha256`、`source_sha256`、`source_hash_version`、`reviewed_at`、`rationale`、`evidence`。该记录说明本次基线如何重新核对；不是给旧 PASS 自动刷新时间戳。
- 将允许静态复核的 AC 明确列为白名单，并依据原验收矩阵逐项审查；默认空白名单，即默认要求重验。OB 任务的 VERIFIED 还需其关联 AC 满足要求。OB-019/AC-072 保持独立业务迁移边界。
- `acceptance-rules.json` 逐项绑定 AC-001…071 的证据要求：`kind` 为 `runtime/static`，`required_evidence` 是必须全部满足的组；每组明确 `allowed_classes` 及目标测试、消费者或扫描命令选择器。测试标识使用包的仓库相对路径与完整测试名，避免绑定占位 module。规则从原矩阵和现有测试逐条核对，缺规则即拒绝；运行记录不得自行声明更宽松的通过条件。
- 运行类证据必须解析所引报告和目标结果：核对当前 v2、开始/结束基线一致、正确类别、成功状态、自然完成、真实退出码和必要目标实际通过。Go JSON 事件、消费者子报告、扫描命令结果、fuzz 逐目标结果分别校验；错误、skip、零测试或缺目标都不能放行。日志/文件仅存在或报告只写一个 PASS 均不足；普通命令收据只可用于规则明确允许的项目，不冒充四类正式报告。

- [ ] 用合成 fixture 先复现“旧台账＋四类当前通过报告”被接受；新增 `test_rejects_stale_ledger`、`test_rejects_stale_acceptance_item`、`test_rejects_missing_hash_version`，期望问题列表非空。
- [ ] 加入“完整当前记录可通过”“旧证据文件存在仍拒绝”“报告 PASS 但目标缺失仍拒绝”“错误类别/缺规则/缺报告/缺 AC 被拒绝”“运行验收不能用 impact_review 代替”“静态复核缺依据被拒绝”等用例；合成证据写入测试临时目录，不修改真实台账。
- [ ] 实现版本和逐项来源校验，限定证据引用为仓库内可审查的相对路径，拒绝不存在或越界的本地文件引用；不把任意路径字符串非空当成有效证据。
- [ ] 运行 `rtk proxy python -B -m unittest discover -s scripts/tests -p 'test_release_provenance.py' -v`，再运行全部工具测试。完整当前 fixture 必须 PASS；真实台账的未完成项仍必须使 `rtk proxy python -B scripts/verify-release.py` 非零退出。

**验收：** 已确认的潜在假阳性被回归用例覆盖；历史证据、当前静态复核和当前运行证据可区分；失败诊断指向具体 OB/AC/报告。

## 阶段三：补关键路径回归和覆盖率

**输入：** 当前平台的新 coverage profile 与原 AC 要求。**文件：** 优先扩展 `kv_client_test.go`、`pki_client_test.go`、`transit_client_test.go`、`internal/engine/executor_test.go`、`internal/engine/semantics_test.go`；新增 `internal/engine/transport_test.go` 承接独立传输边界用例。`scripts/coverage-check.py` 增加 `--profile <path>` 参数，默认 `coverage.out` 保持开发用法；新增 `scripts/tests/test_coverage_cli.py` 验证选中 profile、缺失及畸形输入，其余已存在测试按实际未覆盖块定点补充。不得仅为覆盖率改写正常实现。

| 优先对象 | 应补或核对的场景 | 核心断言 |
| --- | --- | --- |
| `internal/engine/client.go`、`errors.go` | 并发槽位等待时取消；畸形 200/204、代理形状的错误、503 写入 | 未准入不发送；预算包含排队；写入最多一次；无拒绝证明时保持 UNKNOWN |
| `internal/engine/transport.go` | 非法压缩、截断 body、解压后超限、取消与关闭 | 受控错误、响应体关闭、不泄漏正文；不因失败重放写操作 |
| `kv_metadata.go` | 非法版本数/时间字段、列表键类型、删除/恢复版本输入边界 | 拒绝不符合契约的响应；不回退版本；保留准确版本与权限语义 |
| `pki_client.go` | 证书或私钥缺失/损坏、CSR 公钥不匹配、CN/SAN 不符 | 返回受控错误并清理自有敏感材料；错误材料不作为成功结果 |
| `transit_keys.go`、`transit_cipher.go` | 版本缺失、密钥类型/编码不符、密文或返回版本不匹配 | 无算法降级、无自动创建密钥；固定版本操作不可静默换版 |

- [ ] 先读取 profile 的未覆盖块，并对照已有测试；重复场景复用原测试，不为达到数量重复造用例。
- [ ] 用现有 TLS httptest/fixture 和正式适配器编写行为测试；执行器内部用其已有 Sender 接口验证发送次数。普通根包测试不替换 officialSender，也不引入生产可见的测试开关。
- [ ] 对暴露的实际缺陷先保存失败证据，按已确认契约最小修复并重跑相关用例；需要新增业务语义时只阻塞对应项，交回主代理处理契约。
- [ ] 定向执行 `rtk proxy go test -mod=readonly -count=1 -timeout=120s ./internal/engine` 及根包相关 `-run` 选择器，确认目标用例实际运行；稳定后再执行全量 race/coverage。
- [ ] 在 Linux/Windows 各分配新的、不复用的 `.artifacts/runs/<run_id>/` 目录，运行 `rtk proxy go test -mod=readonly -race -count=1 -timeout=180s -coverprofile=.artifacts/runs/<run_id>/coverage.out ./...`。只有 race 自然完成且退出 0、profile 确认由该次运行生成，才运行 `rtk proxy python -B scripts/coverage-check.py --profile .artifacts/runs/<run_id>/coverage.out`；否则该次覆盖验收 NOT_RUN/FAIL，不读旧的根目录 profile。检查器单独读取一个 profile 只能证明其数值，发布来源证明由正式 normal 报告绑定。

**验收：** 现有 `coverage_result` 的全部 15 组在各声明验证平台上均达到 85% Go 语句覆盖率；race 和相关行为断言通过。覆盖率不能代替 AC 逐项证据。物理行指标与语句指标的文档口径须在阶段七发布复核前明确。

## 阶段四：改进执行器脚本、超时和 CI 证据

**依赖：** 阶段一、二的证据标识和校验规则；CI 框架可在补覆盖期间准备，但不得把失败标绿。

**文件：** 新增 `scripts/command_runner.py`、`scripts/tests/test_command_runner.py`、`scripts/fuzz-test.py`、`scripts/tests/test_fuzz_runner.py`、`scripts/ci-summary.py`、`scripts/tests/test_ci_summary.py`，以及 `scripts/tests/fixtures` 下必要的真实输出 fixture；修改 `scripts/normal-test.py`、`scripts/consumer-test.py`、`scripts/security-test.py`、`scripts/integration-test.py`、`scripts/record.py`、`scripts/tests/test_tooling.py`、`.github/workflows/ci.yaml`、`Makefile`、`docs/testing.md`。已有依赖预检/转移脚本的 180 秒限制保留，只有确认需要统一行为时才迁移它们。

**Interfaces / 拟定规则：**

- `run_command(command: list[str], *, cwd: Path, env: dict[str, str], log: Path, timeout_s: int) -> dict`，返回原命令、日志相对路径、起止时间、`completion` 和 `exit_code`。`completion` 为 `exited/timeout/unavailable/interrupted`；未自然完成时不能记为成功，终止进程的返回码另记。
- 只启动并终止自己拥有的进程树：POSIX 使用独立进程组，Windows 使用受控子进程树终止；禁止按进程名称批量杀进程。先用短时 Python 子进程及其子进程验证回收，日志只保存到允许目录。
- normal 普通测试改为一次 `go test -mod=readonly -json -count=1 -timeout=120s ./...`，从同一 JSON 日志统计测试和 `TestDependencyBaseline`；删除原 `test-list` 的重复执行。独立 race/coverage、vet、build、mod verify、module graph 检查继续保留。race 使用本轮独有的 profile 路径；报告绑定其路径、摘要、run_id 和成功命令收据，失败或没有新 profile 时不回退到旧文件。
- 初始预算：unit 300 秒；race 与真实集成各 600 秒；vet/build/mod verify/module graph 各 180 秒；消费者 tidy 180 秒、消费者测试 600 秒；每项扫描 600 秒。Go 测试自身同时保留包级超时。预算是拟定值，首次运行若需调整必须记录实际耗时与原因。
- 每条命令结束或异常后立即写 UTF-8 报告；顶层通过条件同时检查自然完成、退出码、实际目标用例、无 skip 和覆盖门槛。缺工具/环境记 BLOCKED，执行失败记 FAIL；不能用最后一项通过覆盖前面的失败。
- 每轮生成唯一 `run_id`，不可变日志和报告写入 `.artifacts/runs/<run_id>/<check>/`；`.artifacts/normal-report.json` 等既有路径只保留指向该轮的最新副本，并携带真实日志路径，不能覆盖历史运行目录。台账归档证据复制到所属 OB 的本轮目录，保留失败和重跑记录。
- 工具测试复用 `record.py` 收据：增加可选 `--timeout-s`（默认 180 秒）、v2、`completion`、`run_id`，保留 task/name/command 位置参数。`name` 必须包含本轮标识；目标证据文件已存在时拒绝覆盖。收据和原始日志一起上传，summary 按 job 类型验证，不把普通命令收据冒充四类正式报告。
- fuzz 新增独立 runner：固定映射 `FuzzPath→./internal/engine`、`FuzzDocument→./kv`、`FuzzPKICSR→./pki`、`FuzzEncoding→./internal/transitutil`、`FuzzDecode→./internal/pemutil`。各目标沿用 `-run='^$' -fuzz=<目标> -fuzztime=30s -parallel=1 -timeout=90s`，进程预算 120 秒；每目标一份命令收据和原始日志，汇总为 `FUZZ_RUNTIME` 类别的 `.artifacts/fuzz-report.json`。此类别只供 fuzz 验收，不能替代四类正式报告。

- [ ] 为退出 0、退出非零、超时、命令缺失、中断、子进程回收和部分日志保存建立短时 fixture；测试不联网，不启动真实 OpenBao。
- [ ] 实现 runner，并将需要超时恢复的脚本逐个接入。用模拟 Go JSON 结果验证零测试、缺 `TestDependencyBaseline`、skip、失败或不完整事件不能生成正常 PASS；增加“旧 profile 存在但本轮 race 失败/未生成 profile”用例，确保仍拒绝覆盖验收。
- [ ] 将 `make fuzz-test` 改为调用 `python3 scripts/fuzz-test.py`，更新现有入口契约测试以核对同样的五个目标及实际函数声明。用固定 Go 工具链的真实 fuzz 输出建立解析 fixture，确认每个目标进入 fuzz 且执行样本；无匹配、零执行、格式不可识别或超时均非 PASS，不仅判断总 make 退出码。
- [ ] 将 CI 拆为 tooling-Linux、tooling-Windows、normal-Linux、normal-Windows、fuzz、consumers、security、real-integration 和 summary；互不依赖的检查独立执行，summary 使用 always 条件保留失败信息，不使用 `continue-on-error` 隐藏必验失败。
- [ ] 每个 job 的 upload 步骤显式设置 `if: always()`，上传独立名称的脱敏 report、必要日志及 coverage。summary 的 `needs` 列出全部检查 job，job 本身使用 `if: always()`；按固定 artifact 名称分别下载到 `.artifacts/downloads/<job>/`，不扁平合并同名文件。缺 artifact 或平台硬超时按缺证据失败处理。现有 checkout/setup-go 固定引用保留，新 upload/download action 在实施时从官方来源核验并固定完整提交 SHA。
- [ ] 上传使用显式白名单：当前轮 report、其引用的脱敏日志和 coverage；不得上传整个工作区、模块缓存、临时集群目录、Token、CA 私钥或解封材料。
- [ ] `ci-summary.py` 的 CLI 固定为 `python -B scripts/ci-summary.py --artifacts .artifacts/downloads --output .artifacts/ci-summary.json`。固定映射为：tooling 两个平台的命令收据、normal 两个平台各自的 normal report、fuzz report、consumer report、security report、integration report。分别校验类别、PASS/自然完成、退出码、v2、当前 hash 及原始日志；tooling 须非零测试且无失败/skip，fuzz 须五个目标实际运行通过。缺失、重复、错误来源或不同 hash 均失败；Linux normal 作为既有四类正式报告中的 normal，Windows 独立保留，不互相覆盖。
- [ ] 新增合成 artifact 集测试：一项失败/缺失、旧 hash、类型混淆、额外重复报告均拒绝；全部当前报告通过才汇总 PASS。预发布流程在此基础上运行阶段二的 `verify-release.py`，仍需已复核台账；普通 PR 的技术检查不自动更新台账或批准发布。

**验收：** 中断可留下可信证据且无自建进程遗留；独立检查不会因无关检查失败而全部消失；CI 技术通过和发布门禁通过分开呈现。GitHub 远端实际运行需要对应工作流已获授权进入远端，仅静态 YAML 检查不能称远端 CI 通过。

## 阶段五：补接入示例与性能基线

**定位：** P2 改进，安排在最终源码冻结前；若维护者暂缓，则在原 OB 任务中记录理由，不能事后加入 Go 文件却沿用此前哈希的报告。

**文件：** 新增 `examples/observer/main.go`、`examples/observer/main_test.go`；修改 `examples/internal/bootstrap/config.go`、其测试、`examples/README.md`、`docs/observability.md`；新增 `kv/document_bench_test.go`、`internal/pkiutil/verify_bench_test.go`、`internal/transitutil/encoding_bench_test.go`、`internal/engine/executor_bench_test.go`。

- [ ] 将示例内部 helper 扩为 `Run(action func(context.Context, *bao.Client) error, opts ...bao.Option) error` 并向 `bao.New` 传递选项；现有调用无需修改，不改变公开 SDK API。
- [ ] Observer 示例通过 `WithObserver` 注入使用原子计数器的快速回调，演示显式配置下的受限只读操作。计数标签仅来自固定操作/错误类别；回调中不做网络 I/O、不揭示秘密、不把路径、RequestID、租户或序列号作为标签。
- [ ] 增加示例测试，确认可选参数透传、旧调用兼容、事件只影响计数、输出无凭据及秘密正文；用本地 fixture 验证，不运行生产示例。执行 `rtk proxy go test -mod=readonly ./examples/...` 和 `rtk proxy go build -mod=readonly ./examples/...`。
- [ ] 在上述四个包分别实现 `BenchmarkDocument`、`BenchmarkCertificateValidation`、`BenchmarkTransitEncoding`、`BenchmarkExecutorConcurrent`；覆盖 JSON 文档小/大输入、证书校验、Transit 编码和执行器并发。数据由公开或内存生成的 fixture 提供，只访问自身 httptest；使用 `ReportAllocs`，不将性能阈值混入普通功能测试。
- [ ] 执行 `rtk proxy go test -mod=readonly -run='^$' -bench=. -benchmem -benchtime=1s -count=3 ./kv ./internal/pkiutil ./internal/transitutil ./internal/engine`；核对四个预期名称及其启用的子基准均实际输出三轮结果、迭代数大于 0，记录 ns/op、B/op、allocs/op 和 Go/OS/CPU、算法、输入大小、并发度。零匹配或缺包结果不能算通过，即使进程退出 0。并发场景另采集 P50/P95 和样本数，不能从 benchmark 平均值推导分位数；不设无依据的吞吐 SLA。

**验收：** 示例可编译、测试通过且遵守观察边界；有可复跑的性能记录，尚不据此宣称生产吞吐或性能提升。

## 阶段六：固定分发与真实验证输入，执行正式验证

**依赖：** 阶段一至五中纳入本轮的源码和工具改动稳定。**文件：** 按已确认输入修改 `go.mod`、模块导入、两个 `tests/consumers/*/go.mod`、示例/文档引用、`deploy/test/server-lock.json`；`scripts/tooling.py` 和 `scripts/tests/test_tooling.py` 补充锁定输入的一致性校验。必要时由权利人提供源码授权文件。既有 `internal/testenv` 和 `tests/integration/runtime_test.go` 继续作为真实验证入口。

| 前置输入 | 决定者／依据 | 缺失时的处理 |
| --- | --- | --- |
| 实际 module 托管地址、仓库可访问方式 | 项目维护者 | 本地工具/覆盖改进继续；正式可分发声明阻塞，不猜地址或创建远端 |
| 源码授权条款 | 项目权利人 | 不代选许可；不对外分发 |
| OpenBao 精确版本、镜像 digest 或二进制 SHA256 | 维护者指定测试基线，并核对官方发布物 | 真实集成保持 BLOCKED；不使用 latest、外部服务器或现有业务 Token |
| 非生产网络下载、扫描数据库、隔离 Docker/二进制环境 | 现有执行授权与环境 | 缺项仅阻塞相关验证，不擅自安装或启停共享服务 |
| Go 1.25.0 下限的兼容声明、覆盖指标口径 | 验收维护者 | 未实际验证前仅标为声明下限；85% 语句结果不冒充物理行覆盖结果 |

- [ ] 在地址确定后一次性同步 module/import/消费者 require 和文档；检查无遗留占位引用。涉及模块解析的改动必须发生在最终验证之前，不能在报告通过后再改 module。
- [ ] 固定测试服务输入：`BAO_TEST_VERSION` 加 `BAO_TEST_IMAGE=<image>@sha256:<digest>`，或 `BAO_TEST_BINARY` 加 `BAO_TEST_BINARY_SHA256`，二选一。冻结前完成发布物来源与摘要核验，锁文件用 `status='PINNED'` 表示输入已固定，不表示集成通过；冻结后不得因测试通过再改锁文件。`integration_lock(env)` 核对锁文件、环境输入和实际摘要的一致性；新增缺锁、版本/摘要不匹配的拒绝用例。启动后再核对实际运行版本，运行结果只写报告，保留仅新建本地临时实例、动态端口和最小权限 fixture 的限制。
- [ ] 冻结源码，生成 v2 标识；Linux/Windows 验证须使用同字节快照并记录换行设置。所有正式报告从该快照生成，不能只改报告里的 hash。最低 Go 兼容检查另记结果，不与主验证环境合并。
- [ ] 按下表执行并留存各类结果。命令缺环境或失败时保留原记录；修复后作为新一次运行记录，不覆盖成“从未失败”。

| 验证入口（仓库根目录） | 要求与产物 |
| --- | --- |
| `rtk proxy python -B -m unittest discover -s scripts/tests -v` | Windows 默认编码及 Linux 工具测试全部通过，包含原有拒绝测试 |
| `rtk proxy python -B scripts/normal-test.py` | 正式工厂，测试/race/vet/build/mod verify/module graph、无 skip、15 组覆盖率；`.artifacts/normal-report.json` |
| `rtk proxy make fuzz-test` | 在具备现有 Bash/Make 的 Linux 环境，5 个既有目标各运行 30 秒；保留真实退出与样本信息 |
| `rtk proxy python -B scripts/integration-test.py` | `REAL_OPENBAO`；Runtime 及 KV、计划删除、PKI、Transit、AuthNamespace 必需子用例真实通过 |
| `rtk proxy python -B scripts/consumer-test.py` | `INDEPENDENT_CONSUMERS`；reader/signer 两个独立模块，`GOWORK=off`，无 replace，不能用根包测试代替 |
| `rtk proxy python -B scripts/security-test.py` | `PINNED_SECURITY_SCANNERS`；固定工具和真实数据库扫描，保留发现及处置，不把下载失败当无漏洞 |

本地遵循 RTK 包装；CI 可直接调用同一原生命令，不要求为 CI 新增 RTK 依赖。`dependency-check.py --prepare` 会 tidy 并写模块文件，只在基线准备阶段确有需要时执行；不放进冻结后的只读验证流程。

**验收：** 正式证据具备同一当前 v2 标识、准确环境/版本/命令/退出码及脱敏日志；失败、BLOCKED、NOT_RUN 明确区分。临时消费者证明模块打包与独立解析，实际托管地址的可获取性仍需在授权后的目标渠道验证。

## 阶段七：文档、台账复核与试用交付

**文件：** `README.md`、`docs/README.md`、`docs/compatibility.md`、`docs/testing.md`、`docs/release.md`、`docs/implementation-handoff.md`、必要的接入指南、`task-status.json`、`acceptance-results.json`；新增本轮脱敏证据到原 `docs/evidence/OB-xxx/` 下，保留历史记录。

- [ ] 执行初期即可修正文档中“go.sum 为空”等确定过时说明，并引用评估日期；本阶段再用实际新报告补齐工具链、依赖、服务兼容矩阵、已通过/未验证项。历史 R1/R2/R3 明确标历史，不改写原结论。
- [ ] 保持一处当前接入状态入口，其余文档引用它；兼容矩阵分开记录“声明”“已构建”“真实集成通过”，不从客户端版本推导所有服务端兼容。
- [ ] 按原 OB/AC 逐项复核：完成运行证据才能标相应 PASS；静态证据按阶段二规则复核；关联验收未满足的任务不得 VERIFIED。两份台账顶层及当前记录基线一致，业务迁移继续单独报告。
- [ ] 运行全部工具回归和 `rtk proxy python -B scripts/verify-release.py`。证据齐备时必须退出 0；否则报告确切未满足项，不手改脚本或台账为绿。
- [ ] 对稳定差异进行独立审查，重点检查门禁不能被旧证据绕过、公开错误/重试/权限语义无回归、秘密未进入产物。成立的问题由主代理修复，变更源码后重新确定基线并更新受影响证据；四类正式报告不能沿用旧 hash 冒充当前运行。
- [ ] 核对最终 diff、暂存区与未跟踪文件，交付改动、命令、原始证据位置、仍缺的外部输入和下一项动作。仅在另有明确授权时执行提交、推送、版本 tag 或发布。

**完成标准：** 本方案纳入的改进有实际产出；适用测试和审查完成；覆盖指标口径明确；任务、验收和证据基线一致；真实发布前置条件未解决时如实报告“本地改进完成，交付门禁仍阻塞”。不将 SDK 试用准备与 OB-019/AC-072 的真实业务迁移混为一项。

## 执行依赖与中断恢复

主顺序：阶段一 → 阶段二 → 阶段三/四 → 阶段五中纳入本轮的事项 → 阶段六 → 阶段七。外部输入的收集可提前进行；当前文档事实修正可与早期工作同步，但只有稳定源码上的验证能支撑最终验收。

每个执行单元在原 OB 台账中追加本轮进度和证据位置；未达到原任务完成标准时保留 IN_PROGRESS/BLOCKED。中断后先核对进程、工作树、提交、v2 标识和最后一个已验证步骤，从首个未完成项继续。超时后换成有界命令或聚焦用例诊断，不重复等待同一失败路径。

本次编制只新增这份方案，不修改实现、测试、门禁或验收结果，不宣称以上计划已经执行。
