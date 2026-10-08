# 测试证据等级和门禁

当前源码标识、各类报告是否有效以及门禁结论统一见[当前验证状态](current-status.md)。`scripts/current-status.py --check` 只检查摘要是否与现有台账和证据一致，不运行测试、不改台账，也不能替代发布检查。

## Transit 编码请求体边界（2026-10-04）

`transit_request_boundary_test.go` 通过现有本地 TLS 协议夹具覆盖九个入口、72 个叶子场景，不访问真实 OpenBao。小上限用例核对最终请求体恰好等限时的正确 JSON/版本/算法/派生 context；一字节超限时元数据、业务请求和操作认证快照均为零。额外场景确认失败的元数据不会遮蔽本地错误、超限优先于取消/关闭、合法输入保留取消/关闭/元数据错误，以及调用方持有的敏感输入保持不变。

针对性入口为 `rtk proxy go test -count=1 -timeout=30s -json -run '^TestTransitEncodedPayloadBoundary$' .`；本地离线环境使用 `GOTOOLCHAIN=local`、`GOPROXY=off`、`GOSUMDB=off`、`GOFLAGS=-mod=readonly`、`GOWORK=off`。RED 与 GREEN、实际命令和后续检查记录见[实施交接](implementation-handoff.md)。父测试和子测试通过事件不作为独立验收数量相加；本地协议夹具和 Go 1.26.3 检查不替代固定工具链、真实 fixture、消费者及扫描报告。

## 2026-09-30 指定远端服务补充入口

`scripts/remote-test.py --execute --mode readonly|isolated` 独立于 fixture 门禁，使用 `BAO_REMOTE_*`；全场景由 `scripts/remote-fixture.py --manifest <新清单> --execute` 准备并运行，须另行授权。`full` 是清单绑定的子进程配置，不是 `--mode` 的取值。详情见 [远端测试配置、限制与恢复](remote-testing.md)。普通 Go 测试只运行本地 TLS 模拟，实际远端目标需要 `remote` build tag 和显式启用。远端 stdout/stderr 在持久化前丢弃，结果只记录经过过滤的结构化字段和命令收据。[历史 sdk-test 全场景报告](../.artifacts/runs/remote-full-fixture-20260930-08/report.json)绑定 `e9d371d3a35385803a651db667a05cbd8fb3bb6cd36b03b23bf24ffd6139f14e`：Core 39/39、Transit 22/22、112/120 次请求、9 项资源回收 PASS。本轮改进后的源码没有新增远端执行；旧授权已过期。此前 KV List/HMAC 失败同样保留为历史。外部服务补充测试不能替代 REAL_OPENBAO、固定扫描器或独立消费者。

远端入口历史见[远端测试报告](openbao-remote-test-report-2026-09-30.md)；本轮本地检查与历史远端结果在[实施交接](implementation-handoff.md)分开记录。首次失败、审查修复及未运行远端场景均保留。

## 当前入口和来源（2026-10-01）

本轮使用正式 `Client → engine → officialSender` 工厂和固定 api/v2 v2.7.0，不使用 overlay、replace、skip、关闭 vet 或降低覆盖率。正式入口要求主工具链 Go 1.26.8，最低兼容检查要求 Go 1.25.0；Linux、Windows 和远端 CI 的实际运行分别列在 [兼容矩阵](compatibility.md)。结果以 [交接](implementation-handoff.md) 和两个原台账为准。

最低版本分别运行 `scripts/normal-test.py --profile minimum-go` 与 `scripts/consumer-test.py --profile minimum-go --offline-proxy <已准备目录>`。它们复用完整 normal/独立消费者检查，证据类别和输出文件单列；Go 1.25.0 与主 Go 1.26.8 严格分开。CI 为 Linux/Windows 各执行 normal 和 consumer，summary 与 release gate 均要求四份对应证据。错误工具链先 BLOCKED；没有实际执行的作业不会因本地脚本单测通过而变成 PASS。

离线准备应在隔离临时目录复制根 `go.mod`/`go.sum` 后执行 `go mod download all`，补齐模块图元数据；正式 normal 仍使用 `GOPROXY=off`。消费者由脚本创建全新缓存与签名代理，不预填消费者 `go.sum`。模块打包在遍历前剪枝生成目录、符号链接和嵌套 module，避免把工作区工具缓存纳入递归扫描；归档成员排序及源码选择规则保持不变。

固定扫描仍使用真实官方漏洞数据库。`deploy/test/gitleaks.toml` 保留内置规则；运行器固定在仓库根扫描 `.`，只排除根目录生成的 `.artifacts`，嵌套 `examples/.artifacts` 仍须扫描。仅对两份证据文件中的已核验公开摘要/公钥指纹设置“精确路径且精确值”例外，不整体忽略测试、示例或证据目录。反例验证要求同值出现在源码、原证据出现新值、嵌套目录的合成秘密以及私钥标记仍被检出。初次漏洞、网络、缓存和无效回归夹具结果均保留，后续 PASS 不覆盖首次失败。

在根目录运行以下 Python 入口，本地按全局规则加 `rtk proxy`。依赖下载和工具准备必须先于冻结，不能混在只读正式检查中。

| 命令 | 收据及结束条件 |
|---|---|
| `python -B -m unittest discover -s scripts/tests -v` | Windows 用 `PYTHONUTF8=0` 验证显式 UTF-8；测试数大于零，无 skip |
| `python -B scripts/normal-test.py` | unit JSON、fresh race profile、vet、mod verify、build、module graph 六项自然成功 |
| `python -B scripts/coverage-check.py --profile <本轮 profile>` | 只读取指定文件；不存在或本轮 race 未成功时不复用旧 coverage.out |
| `python -B scripts/fuzz-test.py` / Linux `make fuzz-test` | 五个目标各 30 秒、独立退出码和实际非零 execs；种子单测不能替代 |
| `python -B scripts/integration-test.py` | 固定版本与 digest，新建自有临时实例；Runtime 和五个子用例全部运行 |
| `python -B scripts/consumer-test.py --offline-proxy <目录>` | 冻结前准备本地模块与签名 sumdb 代理；全新 GOMODCACHE/GOPATH、无预置 go.sum、无网络回退 |
| `python -B scripts/consumer-test.py --allow-network` | 需下载授权，两个独立模块、fresh cache、临时代理、无 replace；与根包测试分开 |
| `python -B scripts/security-test.py --allow-network` | 需工具/数据库下载授权；固定 govulncheck v1.1.4、gitleaks v8.24.3，显式官方漏洞库并核对版本/数据库更新时间 |
| `python -B scripts/benchmark-test.py` | 六个子基准各三轮，非零迭代；记录分配、吞吐和独立延迟样本，不设置发布阈值 |
| `python -B scripts/verify-release.py` | 只检查当前 OB/AC 与真实证据，不修改台账 |

消费者未提供离线代理或下载授权、扫描器未取得下载授权时，生成 `BLOCKED` 报告。离线代理须在冻结前准备 `.mod/.info/.zip` 和公开 sum.golang.org 的签名 lookup、tile、latest、supported；只复制材料不表示认证通过，Go 必须在空消费者缓存内自行验签，缺材料或签名错误都会失败。此模式不使用根 go.sum 预置消费者校验值、不复制 ziphash 到消费者缓存，显式本地校验数据库 URL 禁止网络回退。脚本不会安装 Go 或启动共享服务。命令运行器使用有限超时；自然退出、超时、缺工具、中断分别记载。回收只针对本次 PID/进程组；树回收不能确认时记 `cleanup_incomplete` 并停止后续检查，须先核查该 PID 后恢复。

每次运行保存 `.artifacts/runs/<不可复用 run_id>/<类别>/report-NNN.json`、日志和 profile；根 `.artifacts/*-report.json` 只是最新入口。命令记录 argv、cwd、真实退出码、时间及日志摘要。源码前后变化导致 FAIL，即使中间改动后来恢复，`baseline_changed` 也保持失败。首次失败和重跑记录均保留。

## 工具内部职责与局部回归

2026-09-29 结构调整保留全部 CLI、报告类别和验收规则。`tooling.py` 只提供基础能力，`evidence.py` 单向使用基础工具并独立校验七类证据，`verify-release.py` 显式传入仓库 root。生产 runner 的命令列表不作为验证器的预期命令来源。

`reporting.RunRecord` 负责不依赖 Go 的收据、前后标识和不可变持久化；`Run` 保留正式 runner 的目录与 latest 入口约定。六个 Go runner 显式调用 `probe_go()`；Python tooling 和 `record.py` 不探测 Go。已记录的致命状态通过 `RunStopped` 交给 CLI 的 `run_cli()` 转为退出码，未预期异常继续暴露；清理不完整时锁存 FAIL 并禁止后续命令。latest 是可更新入口，`report-NNN.json` 和 run_id 不得复用覆盖。

证据测试使用 `scripts/tests/evidence_fixture.py` 中显式接收临时 root 的普通 builder。每个 TestCase 管理自己的 TemporaryDirectory 和 patch；合成报告仅位于测试临时目录，不能用于台账验收。局部检查可分别运行 `python -B -m unittest discover -s scripts/tests -p test_release_provenance.py -v`、`... -p test_ci_summary.py -v`、`... -p test_reporting.py -v`；完整门禁仍使用上表的 discover 入口。

## v2 source_identity

`scripts/tooling.py` 统一产生 `source_hash_version: 2` 与 `source_sha256`。SHA256 输入从原始字节 `openbao-sdk-go/source-hash/v2\0` 开始；文件按 POSIX 相对路径排序，每项依次追加 UTF-8 路径、NUL、原始文件字节、NUL。不归一化换行；同字节跨平台相同，CRLF/LF 差异会被检测。

清单包括根 `.go`、go.mod/go.sum/Makefile，SDK/内部/示例/测试目录中的 Go 文件和嵌套模块及 testdata，scripts 的 Python、acceptance-rules.json、测试 fixtures，CI YAML，以及 deploy/test 的 HCL/YAML/TOML 和 server-lock.json。拒绝选中文件的符号链接；排除本机 `.codex/.serena`、Git/缓存/产物、台账及普通文档。源清单以 `source_files()` 为执行定义；黄金字节/中文路径/配置影响测试防止算法漂移。此标识与 dependency-bundle 的输入指纹及上游 checksum 是三种不同数据。

旧证据保留旧 hash 版本，不重写成 v2。`scripts/acceptance-rules.json` 给 AC-001～071 定义证据类别和必需测试/文档；检查具体目标、自然退出和摘要，路径存在或出现 PASS 均不充分。运行证据必须重跑；静态影响复核仅能使用明确 allowlist，本轮列表为空。

## CI 导出边界

失败时仍打包允许的证据；原始日志保留在隔离 job 内，上传只含 Go Action/Package/Test、fuzz 执行计数、unittest 汇总、命令状态和无秘密的 profile。原始摘要与导出事件摘要分别记录，不把净化文本冒充原始日志。扫描发现只导出条数占位，不上传 Secret/Match/Line；本地校验与打包先核对 `findings_sha256`，CI 再核对 `count-only-v1` 类型、原摘要关联和导出文件摘要。缺摘要或附件字节改变都会失败。CI summary 仅在显式 structured-v1 模式读取这些导出事件；本地 release 校验仍要求原始收据日志。

Test/Package 只保留源码声明和验收规则中的固定选择器；动态子测试名收敛到固定父测试，未知包/测试转成不含原值的失败事件，必需集成子目标保留。job 内导出先核验原始摘要，summary 只能独立核验导出事件摘要，不能从导出文件重新计算原日志；不得将其描述成原日志的二次完整验证。服务报告还须匹配导出锁和当前 checkout 的锁字节。

Go 1.26 对无测试文件的包输出包级 `skip`，本轮已从真实日志固定该格式。只有缺少 Test 字段、具有精确 `[no test files]` 输出、整包从未出现 Test 字段时才归为“不含测试”，不计入已运行测试数；真实测试 skip、空/畸形 Test、无法解释的包 skip 和总体零测试仍失败。CI 仅导出经 job 核实的 NoTestFiles 布尔，原始日志模式不会信任该字段。

summary 使用 `always()`，按固定十二个 job 目录分别下载，其中四个为最低 Go 的 Linux/Windows normal/consumer；缺报告、缺平台、缺目标、零测试或重复报告均失败。当前 CI YAML 已改不等于远端已运行。

## 性能基线

Document 对 64/65536 字符 JSON 做 Parse/Decode/Zero；证书使用内存 P-256 两证书链解析及独立根验证；Transit 对 32/4096 字节做 base64/vault 包装往返；执行器访问自身 TLS httptest，8 个并发槽、GOMAXPROCS 个 worker。后者每轮计时区外另采 256 个端到端延迟，用 nearest-rank 计算 P50/P95，包含排队与 TLS/HTTP；不能从 ns/op 均值推导。结果只描述该主机/fixture，不表示生产 SLA 或性能提升。

## 历史证据类别与限制

1. `DIRECT_OWN_CODE`：标准库依赖的自有包直接 go test/race；不经过工厂替换。
2. `CONTRACT_WITH_TEST_SENDER`：只替换自有 official_sender.go 工厂，真实执行其余代码并访问临时 TLS httptest。这是 HTTP 契约/故障测试，不是官方 api/v2 编译或真实 OpenBao。
3. `NORMAL_OFFICIAL_CLIENT`：稳定源码、完整官方依赖的正常 test/race/vet/build/mod verify；历史 R1/R2/R3 失败在依赖获取。
4. `REAL_OPENBAO`：正式工厂 + 经 SHA256 校验的新建真实临时集群 + Namespace/受限身份。R1/R2/R3 未执行业务测试；启动前置检查真实失败。
5. `INDEPENDENT_CONSUMERS`：两个独立 go.mod 通过临时模块代理加载 SDK，禁止 replace。R1/R2/R3 下载 SDK 成功、官方依赖下载失败，消费者未构建；本轮结果见当前交接。

固定版本安全扫描独立归为 `PINNED_SECURITY_SCANNERS`，下载失败不代表发现零漏洞。

## 可复跑命令

`make normal-test` 连续记录各项原始退出码，不以一项通过遮蔽其它失败；`make tooling-test` 验证脚本在缺环境/skip/错误证据类别/旧 source hash 下拒绝发布；`make integration-test` 缺配置 exit2（make 包装后同样非零）；`make consumer-test` 无本地 replace；`make release-check` 只检查，不把台账改为 PASS。

`make fuzz-test` 依次对 Path、Document、PKI CSR、Transit 编码和严格 PEM 解码运行30秒 fuzz（续作R1新增第5项，原4项不删改）。历史中被工具硬超时中断的进程保留 exit_code=null，不转成PASS；后续完整运行独立记证。

## 覆盖率

Go 原生 coverprofile 表示语句覆盖率，不是严格的物理代码行覆盖率。原方案用了“行覆盖率”措辞。本实现明确报告原生 Go 语句覆盖率，**未把它偷换成原方案的行覆盖率已通过**；若发布验收要求独立行指标，须追加定义与采集，原门禁保持未通过。

物理包要求根包、internal/engine、internal/authn、auth、kv、pki、baoerr、sensitive、internal/jsondoc、internal/pkiutil、internal/transitutil、internal/pemutil各≥85%；另外检查 kv/pki/transit 功能门面+类型/校验包组合，避免 root 平均掩盖功能域缺口。types-only的transit/diagnostics/observe不冒充有执行覆盖率。

历史 Go1.23 对 overlay 根包的覆盖率/自动vet会读取原始第三方导入。当时的补充运行只覆盖指定自有叶子包，根包/official_sender.go 缺正常覆盖率证据。本轮正式工厂的 fresh race profile 单独记录；历史补充叶子报告绝不能用于 make release-check 中的正常报告。

## 台账

根 task-status.json 保留OB001–019及原依赖。写了代码、仅补充测试通过、前置依赖未验证，不标VERIFIED。acceptance-results.json逐AC记录状态、证据等级和缺口；PARTIAL 表示观察到部分行为但没有完成要求的证据链。原始 docs/spec/04不改写。

每次最终验证用 source_identity 绑定实际源代码。中间红绿记录保留当时原始退出码，旧记录不自动视作新源码通过。R1/R2/R3 为作者自审；本轮独立 risk_reviewer 的实际范围、发现和修复单独记录于实施交接。

`python3 scripts/consumer-contract-test.py` 仅验证两份消费者源码在SDK同模块下的类型/本地协议。临时目录退出后删除；没有replace指令，也不是独立依赖解析证据，AC067继续等待真正consumer-test通过。

## 续作R1证据范围

本轮原始证据使用 `resume01-` 前缀，历史红绿日志保持不变。`resume01-final-direct-tests`是直接包测试；`resume01-final-contract-tests`是仅替换官方工厂的完整自有代码回归；`resume01-leaf-instrumented-contracts`通过显式-coverpkg只采集关键叶子包，其余未被采集的根包和完整功能域保持未验证。普通覆盖命令失败的原始输出也保留，不用补充命令覆盖它。

真实集成新增 `TestIntegrationKVScheduledDeletion`：临时管理员预配置自动删除元数据，有限权限业务身份写入并按准确版本读取，再显式软删除验证。源码编译检查的 `-run=^$`不执行任何集成用例，不能用于AC065。
