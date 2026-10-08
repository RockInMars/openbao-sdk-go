# openbao-sdk-go 实施交接

## 2026-10-08 v0.1.0 发布候选

用户明确授权发布 `v0.1.0`，并允许按现有流程在隔离非生产环境重建 Linux/Windows 门禁，访问官方模块代理、工具源和漏洞库；没有生产或历史远端 OpenBao 授权。候选收录此前 SDK 源码、测试、工具和文档，机器配置、缓存与临时产物不入 SDK 提交。

最终冻结源码 v2 为 `0bca95690d54ed20de1703f4cb1523984cdc122398957665b85502b9738de74b`，194 个选中输入。起点为 `master` / HEAD `2a842ab1012b097d1097735cc21c7dc4c64229db`；远端为 `https://github.com/RockInMars/openbao-sdk-go.git`。默认远端分支与本地分支不同，本轮不自动切换、合并、改写历史或推送其他分支。

当前候选的实际命令 `scripts/verify-release.py` 自然退出 0，`RELEASE GATE: PASS`。台账逐项依据当前报告及前置依赖重建：18 VERIFIED / 1 BLOCKED，AC001–071 共 71 PASS；AC072 真实业务迁移仍为 NOT_RUN。完整索引与具体不可变路径见[本轮证据](evidence/OB-018/release-v0.1.0-2026-10-08-final/index.json)，不将旧源码 PASS 改绑。

- 本地十二作业 CI 汇总自然通过：Linux/Windows tooling、main normal、最低 Go normal/consumer，以及 Linux fuzz、main consumer、固定扫描和真实 fixture；不是托管 GitHub Actions 执行。
- 主工具链 Go 1.26.8、最低兼容 Go 1.25.0；四份 normal 的 unit、fresh race coverage、vet、mod verify、build、module graph 均自然成功。三个独立消费者 profile 各两个全新缓存模块通过签名离线代理，未用工作树 replace 掩盖模块消费。
- 当前 Linux 主 normal 的 fresh race profile：15/15 既有 85% 门槛通过。五个 fuzz 目标各实际执行 30 秒并有非零执行数；固定 govulncheck 1.1.4、gitleaks 8.24.3 及官方数据库扫描通过。
- 自有、来源和摘要核验的 OpenBao 2.6.3 fixture 自然通过，解析到 runtime 与五项子场景共六个记录。管理员只初始化，业务断言使用受限身份；未访问历史远端或生产服务。
- Windows 独立 verifier 完整 190 tests 自然通过，706.619 秒，Python 3.13.13、显式 PYTHONUTF8=0、实测 utf8_mode=0；四个正式 Windows 作业没有失败或超时，原始收据、结构化导出和最终审计均复验。

### 首次问题及替代路径

最早 `1b31b5677db1bbcefc0f03ba2790aa3b9a9862c8fbadeff4c8cd75231b2f9414` 的完整矩阵完成后，独立审查发现 P1：原 HEAD 跟踪 `.codex/config.toml`，忽略本地改动不等于从整个标签树排除机器配置。两个真实临时 Git 回归先复现错误发布，再通过最小全树 EXCLUDED 检查变绿。新源码 `0bca…` 的全部正式证据已重建；脚本在创建标签前拒绝任意排除路径，正常发布回归确认远端树没有机器目录且本地文件保留。独立风险复核无新增高风险或发布阻塞，详见[审查](evidence/OB-018/release-v0.1.0-2026-10-08-final/independent-review.md)。

Linux 在 Windows 挂载路径的完整 190 项 tooling 两次自然 FAIL（312.479 秒两项、574.302 秒三项），均是既有 20 秒子 CLI 启动预算超时；降并发的两项定向诊断曾通过，未掩盖完整失败。随后改用任务自有容器的原生 Linux 文件系统实际副本，复制前后 194 输入及 v2 完全相同，全套 190 项在不修改断言、预算或门禁的情况下自然通过（28.034 秒）。原报告与日志逐字节传回并由原仓库 parser 复验，旧失败完整保留。

Windows 验证资源最初委派范围过窄，故曾停在只读预检；补齐本次自有 E:\Temp 临时资源及 Go 自然缓存范围后执行正式四作业。辅助监控/CRLF 统计与一次审计参数引号错误已保留，不能混计为正式作业失败。物理 index 统计缓存摘要变化的早期 BLOCKED 判断也由原 verifier 撤回；没有逻辑 stage、HEAD 或 v2 污染证据。

### 当前动作及交付边界

工程候选已验证，标签尚未在本检查点创建。下一步仅按既有流程定向取消机器文件跟踪并保留本地字节，提交明确 SDK 清单；再用新 HEAD 完整 SHA 运行只读预检、单次推送 `v0.1.0` 注解标签，并从实际目的地核对标签对象及 peeled commit。不能以 dry-run、提交或推送退出码代替远端核验，也不强推或自动回滚局部发布。

`.codex/config.toml` 旧提交 blob 已存在于 upstream；本轮不上传本地修改，也不改写已有公共历史。`.serena/` 及所有本地配置保持原文件，运行材料留在 `.artifacts/`。任务自有 Linux 容器、独立 daemon 和 stdio 官方代理隧道在验证完成后按归属严格清理；不动共享 Rancher daemon、现有机器代理或用户配置。

托管 CI、公共模块代理可获取性、GitHub Release 页面对象、新性能 benchmark、维护者支持承诺及 AC072 真实业务接入/回滚不是本地工程 PASS 的推论；Git 标签发布与后续分发结果以实际操作收据为准。以下 2026-10-04 及更早章节均保留为历史，不覆盖本节的冻结源码与下一步。

## 2026-10-04 Transit 预检优化与交付准备

本轮以 [项目评估](project-assessment-2026-10-04.md) 为输入，只处理 Transit 编码后请求体检查、回归、当前源码证据及接入交接准备。未安装/升级依赖，未访问真实 OpenBao、运行安全扫描或修改业务仓库；未暂存、提交、推送、切换分支、发布或部署。原有工作树修改全部保留。唯一状态入口仍为 `task-status.json`，当前检查点为 `transit_preflight_2026_10_04`；旧 OB/AC 记录及报告保留其原源码归属，不改写 hash 或复制 PASS。

起点：`master` / HEAD `6d8251658e76f88aec5935126f303b2c59497691`，v2 `d3183d2e79ec39057ae5f4d437362d4e7d87b61aa16c1055431ca5db11d37233`，113 项 tracked 修改、85 个 untracked 状态项，暂存区为空。三份 Transit 原始文件的精确字节、SHA256 和 Git 状态保存于 `.artifacts/transit-preflight-20261004/baseline.json`。当前候选 v2 为 `46985fc2ffc7b508cc60d32df8092ad20da79754dc66b18a6a853eb89d3f5aa9`；HEAD 不能代表这个带有原有未提交修改的完整候选。

| 顺序 | 当前动作与下一步 | 状态 |
| --- | --- | --- |
| 1 | 核对基线/契约，用本地协议夹具建立编码后超限的失败回归。 | PASS：九入口均复现多余元数据/auth snapshot；最终业务请求被原 engine 门禁拒绝，不是越限写入。 |
| 2 | 将实际 JSON/Base64 body 的大小门禁前移，保持版本/类型/派生 context、所有权和不重放边界。 | PASS：72 个叶子场景通过；三份实现定点修改，新增一份回归测试。 |
| 3 | 在固定源码上执行已有资源允许的本地/正式检查，独立审查稳定增量。 | PASS（授权本地范围）：Windows 主/最低 Go normal、race/vet/build、离线消费者及覆盖率通过；静态契约与独立代码审查无成立差异。 |
| 4 | 更新候选清单、门禁摘要与 OB-019/AC-072 交接；列明剩余最小依赖。 | PASS（交接准备）：当前摘要与证据一致；发布门禁 FAIL、95 项拒绝，未发布；剩余授权/环境/业务输入明确。 |

本轮只改变本地错误优先级：原始输入/签名/密文/profile 首校验之后，编码最终 body 大于 MaxRequestBytes 时返回 `invalid_argument + none`，优先于取消、关闭及 metadata/auth 错误，不获取操作认证快照或发送请求。等限或普通合法输入仍走原有 Context、生命周期、公钥/版本/算法/派生 context 预检。新增内部 `requestPayload` 在拒绝时清理 payload，成功时由各入口 defer 清理；原 raw/decoded 拷贝清理和 engine 最终大小门禁保留。公开 API、金额/数字字段、依赖、最低 Go、写操作重试及 UNKNOWN 语义不变。

RED：`rtk proxy go test -count=1 -timeout=30s -json -run '^TestTransitEncodedPayloadBoundary$' .`，退出 1，36 个叶子场景失败、36 个通过；失败证实 metadata/auth 获取早于最终大小拒绝，失败 metadata、取消和关闭能遮蔽本地错误。GREEN：同一命令退出 0，72 个叶子场景通过（82 个含父/子测试的通过事件，不能混计为 82 项验收）。日志分别为 `.artifacts/transit-preflight-20261004/red.jsonl`、`green.jsonl`；使用 Windows/Go 1.26.3，离线环境为 `GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOFLAGS=-mod=readonly GOWORK=off`。这些结果只证明本地协议边界，不是正式固定工具链或真实 OpenBao 证据。

### 当前候选的实际验证与审查

只使用已经存在的本地工具、编译器和已签名离线代理，未安装、下载或升级。verifier 的完整 argv/cwd、自然退出码、工具链、源码前后标识与日志位置见 `.artifacts/transit-preflight-20261004/summary.json` 和四份不可复用运行目录；主代理重新用现有 `report_results` 校验四份报告，并核对 coverage profile SHA256 与本次 normal 收据一致。

| 实际入口（外层 `rtk proxy`） | 结果与范围 |
| --- | --- |
| 固定 Go 1.26.8 的 `go test -count=1 -timeout=90s -json -run '^TestTransitEncodedPayloadBoundary$' .` | PASS，退出 0，72 叶子场景；日志 `focused-go1268.jsonl`。 |
| 固定 Go 1.26.8 的 `go test -count=1 -timeout=90s -json . ./internal/engine ./internal/authn ./auth` | PASS，退出 0，四包 301 叶子测试、335 个通过事件；日志 `affected-go1268.jsonl`。 |
| `python -B scripts/normal-test.py`，Windows/Go 1.26.8 | PASS，退出 0；unit、race/coverage、vet、mod verify、build、module graph 六命令自然退出 0。 |
| `python -B scripts/normal-test.py --profile minimum-go`，Windows/Go 1.25.0 | PASS，退出 0；同样六命令通过，最低兼容报告与主线分开。 |
| `python -B scripts/consumer-test.py --offline-proxy .artifacts/offline-consumer-proxy`；另运行 `--profile minimum-go` | 两版均 PASS、退出 0；reader/signer fresh cache 的 tidy/race 各四命令通过。只证明本地离线代理消费，不证明远端仓库可获取。 |
| `python -B scripts/coverage-check.py --profile .artifacts/runs/2026-10-04T053341.351166Z-bec8ef1311e24c72bf060dbe8d01747f/normal/coverage.out --output .artifacts/transit-preflight-20261004/coverage-current.json` | PASS，退出 0，15/15 分组达到原有 85% 门槛，最低 87.50%。 |
| `python -B scripts/current-status.py --write`、`python -B scripts/current-status.py --check` | 均退出 0；只生成/校验状态摘要，不升级 OB/AC 或替代门禁。 |
| `python -B scripts/verify-release.py` | FAIL，退出 1，95 项拒绝；日志 `release-gate.log`。这是过期证据/绑定拒绝，不是 95 个代码缺陷。 |

两版 normal 的根模块默认 build tags 单测各 482 个 Test 通过事件、432 个叶子测试、18 包、0 skip；不能将主/最低、定向/全量或 unit/race 的重复测试相加。工具脚本没有改动，因此本轮未重跑全量 Python 工具测试。verification 阶段两次只读计数命令曾解析失败，修正后完成统计，没有重跑或隐藏测试失败；实现前预期 RED 仍完整保留。

contract_guard 的 preflight/post-change 静态核对 PASS；risk_reviewer 以保存的三份原始文件字节独立审查增量，未发现可执行的代码缺陷。二者均未执行测试，运行证据来自 verifier 和主代理实际检查；静态 PASS 不表示发布批准。源码完整 192 项输入与旧 191 项清单比较，仅三份 Transit 变化及新增测试，其余输入摘要一致。HEAD/分支不变、暂存区为空；消费者临时目录已回收，没有遗留 Go/编译器/vet 验证进程，全部子线程已关闭。

### 候选交付边界与目标模块获取（未执行）

本轮源码增量仅 `transit_client.go`（签名/验签编码前移与小型 helper）、`transit_cipher.go`（加解密/重包裹）、`transit_hmac.go`（HMAC/验 HMAC）及新增 `transit_request_boundary_test.go`。必要文档增量为请求体契约、测试说明、本文、业务接入指南及自动状态摘要；台账仅记录本轮检查点和当前阻塞，不升级历史 OB/AC。完整源码输入清单为 `.artifacts/transit-preflight-20261004/candidate-source-manifest.json`；只含本轮源码增量、以保存的工作树基线为参照的片段为 `candidate-source-increment.patch`，不是对 HEAD 的交付组合。

这个完整候选还包含起点的用户已有修改；本轮清单不是“只暂存这几份文件即可发布”的授权。增量应以保存的 d318 工作树基线比较，不把全部 `git diff HEAD` 当成本轮修改。任何整文件/部分暂存、应用到不同提交或剪裁原有改动都会产生新组合，必须对实际交付组合重新验证。当前源码尚无发布的目标提交或版本，本轮不执行远端获取。

以后取得明确 Git 交付/获取授权并确定实际仓库、完整目标 SHA 或版本后，按下列步骤验真：

1. 核对真实远端目标引用和完整 SHA、候选内容及本轮保留的源码输入清单；不把本地 HEAD、分支名或本地代理当成远端证据。
2. 使用固定工具链在全新消费方目录与空 GOPATH/GOMODCACHE/GOCACHE 中初始化独立 module，设置 `GOWORK=off`；按已授权的仓库认证与 Go 校验数据库访问规则取包，不能使用 replace、旧缓存或关闭认证来“跑绿”。
3. 执行 `go get github.com/RockInMars/openbao-sdk-go@<已发布目标>`，再用 `go list -m -json github.com/RockInMars/openbao-sdk-go`、`go mod download -json github.com/RockInMars/openbao-sdk-go@<已发布目标>` 记录实际版本、Sum/GoModSum、下载来源及可取得的完整 Origin.Hash。缺少完整来源 SHA 时独立核对目标源码清单，不能只凭伪版本短 SHA 断言完全一致。
4. 编译和运行 reader/signer 的公开导入与关键调用；记录实际 import/module 版本、无 Replace、空缓存依赖认证、argv/cwd/退出码及脱敏日志。真实服务行为仍需专用 fixture 和另外授权。

### 业务接入准备与剩余边界

[业务接入与回滚](integration-and-rollback.md) 已列明 Issuer/Store/Journal 的持久化和原子领取责任、发证后中断、Unknown 核对、重启、轮转及历史引用回滚场景，并给出缺失仓库/契约/隔离资源/授权的最小输入。精确业务 ID 建议经消费方批准为 JSON 字符串，并进入真实链路验收；没有授权 SDK 改写既有数值字段。本轮未提供实际业务项目，OB-019 仍为 BLOCKED/OUT_OF_SCOPE，AC-072 仍 NOT_RUN。

源码变化使旧正式报告、台账及证据索引失效。Windows normal/consumer 的主/最低 Go 四份报告已用本轮真实执行重建；REAL_OPENBAO、固定扫描、最低 Linux normal/consumer 四份仍 STALE，原 OB/AC 及索引仍绑定 d318，没有换 hash 或复制 PASS。原索引 249/249 文件摘要一致，但不能因此认定它验证新源码。当前 [验证状态](current-status.md) 为发布门禁 FAIL、95 项拒绝；OB/AC 的 18 VERIFIED/71 PASS 只是历史记录，不是本轮升级。

| 剩余项 | 最小解除条件与下一动作 |
| --- | --- |
| REAL_OPENBAO | 本轮明确授权固定 OpenBao 2.6.3 的隔离 fixture 生命周期及自有非生产资源/操作，才运行既有 integration 入口。没有继承旧服务或远端写入授权。 |
| 固定安全扫描 | 明确授权已有固定扫描器及官方数据库的执行/网络范围，再运行既有 security 入口；不能以缓存或旧 PASS 推断当前扫描成功。 |
| Linux 当前源码矩阵 | 提供已经可用、已授权的独立 Linux 环境，或另行批准环境准备；在固定 Go 1.26.8/1.25.0 与已签名离线材料上运行原入口。不启动/改变共享 WSL/容器服务。 |
| OB/AC 和证据索引绑定 | 必需当前源码证据及适用静态核对完整后，按实际不可变报告更新原台账/索引；不能仅替换 d318 hash。 |
| 真实目标模块获取 | 先获得实际发布目标 SHA/版本与 Git 交付/获取授权，再执行前述全新消费者验真；本轮没有提交或远端分发。 |
| OB-019 / AC-072 | 提供业务仓库、已批准持久化/代次契约、隔离资源与明确迁移范围，按接入指南恢复；不能把 SDK gate 当成业务验收。 |

已达到本轮停止条件：所有允许且前置满足的本地实现、检查和交接准备完成，剩余必需验证依赖明确的外部输入/授权。总体仍为“实现完成，验证未完成”，不是完整工程验收或发布成功。恢复从首个依赖已解除的正式证据步骤开始，不重新实现 Transit，不在无源码变化时重复已通过的 Windows 检查。

## 2026-10-01 发布验证续作（历史基线）

用户在获知验证环境准备需要新增授权后回复“继续”，本轮据此准备工作区隔离工具、访问官方公开发布物/模块代理/校验数据库及漏洞库，并执行可用的非生产验证。此前“保留环境阻塞”是上一检查点的决定，本轮以上述续作授权为准。提交、推送、发布、共享服务启停、生产操作和旧 `kms.jiup9.com` 额度不在本轮范围。

目标是补齐当前候选的发布验证证据；范围为最低 Go、固定扫描、可信 OpenBao fixture、可用 Linux/Windows 检查及原台账/证据更新。复用现有入口和验收规则，仅在实际失败证明需要时最小修复。起点为 `master` / HEAD `6d8251658e76f88aec5935126f303b2c59497691`、v2 `8a1b5313ccda18142539b6cc167ee987d1f07fed1368662970fd09a5897d628f`，暂存区为空，保留原有修改。

| 顺序 | 当前动作与产出 | 状态 |
| --- | --- | --- |
| 1 | 核对执行环境、正式命令及副作用；验证公开发布物，工具仅放工作区隔离目录；先确定服务端锁定信息。 | PASS：Go 1.25.0/1.26.8 官方归档、OpenBao 2.6.3 签名与二进制摘要已核对；隔离 Linux 环境可用。 |
| 2 | 冻结指纹，运行当前环境适用的最低版本、扫描、真实集成和受影响回归，保留首次失败及实际退出码。 | PASS：最终 `d3183d2e…` 的 Linux/Windows 正式矩阵 12/12 通过；首次失败和中间候选仍保留为历史。 |
| 3 | 独立审查验证范围与证据，更新原台账和摘要，重跑发布门禁。 | PASS：risk_reviewer 与 contract_guard 复核闭环；原台账更新，实际发布门禁退出 0；专属运行环境回收完成。 |

风险与停止条件：服务锁属于指纹输入，修改后旧运行报告仅作历史；下载物经可信来源核验后才执行。缺少独立 Linux/远端 CI 条件时仅阻塞依赖项；全部可执行工作完成或剩余工作均依赖明确外部条件时结束。安全支持版本和响应时限仍属维护者待决定项，不自行承诺。本轮原始产物集中于 `.artifacts/release-validation-20261001/`。

当前冻结候选为 v2 `d3183d2e79ec39057ae5f4d437362d4e7d87b61aa16c1055431ca5db11d37233`（191 输入），[输入清单](evidence/OB-018/release-validation-2026-10-01/source-manifest.json)包含保留的原有修改。本轮实际失败推动的最小修复包括：主工具链与 CI 改为 Go 1.26.8；模块打包在遍历前剪枝；Windows 专属清理单测通过平台模拟在 Linux 实际执行；gitleaks 仅排除根 `.artifacts` 并为具体公开元数据设置精确例外；Ed25519 省略不兼容的可选 hash 参数；HMAC 夹具使用既有 latest-only 规则和准确 ACL；依赖转移工具在各个临时 HOME 内先记录 `go telemetry off`，隔离配置并保留严格清理。API 签名、模块依赖、Go 声明下限和生产认证语义未改变。

真实 KV 后端对大整数的舍入和认证最大 TTL 交界的短暂不可用已按批准契约说明；本地精度和过期失败关闭要求保留。首次失败、原生 HTTP 对照、RED/GREEN 及当前真实集成报告见[集成诊断](evidence/OB-018/release-validation-2026-10-01/integration-diagnosis.md)。归档签名、工具链与镜像来源见[工具出处](evidence/OB-018/release-validation-2026-10-01/tool-provenance.json)。本地执行过 CI 二进制准备逻辑（以已验证缓存代替下载）；该结果不代表托管 GitHub Actions 已执行。

授权范围内可执行工作已完成。以下正式入口均绑定最终 d318 源码，覆盖主线 Go 1.26.8 和独立的最低 Go 1.25.0 兼容性检查。Linux 工作目录为隔离容器 `/workspace`，Windows 为本仓库；外层使用 `rtk proxy`，完整命令、环境、源码前后摘要和日志摘要见[证据索引](evidence/OB-018/release-validation-2026-10-01/index.json)。表中的 Python 在 Linux 使用 `python3`，在 Windows 使用 `python`。

| 实际入口与范围 | 最终结果 |
| --- | --- |
| `python -B -m unittest discover -s scripts/tests -v`，由 `record.py --evidence-class COMMAND_TOOLING` 记录；Linux/Windows | 两个平台各退出 0，156 项通过、无 skip。 |
| `python -B scripts/normal-test.py`；Linux/Windows，Go 1.26.8 | 两个平台 unit、race/coverage、vet、模块校验、构建与模块图六步全部退出 0；Linux 401、Windows 400 条测试/子测试通过。 |
| `python -B scripts/normal-test.py --profile minimum-go`；Linux/Windows，Go 1.25.0 | 两个平台六步全部退出 0；Linux 401、Windows 400 条测试/子测试通过。最低版本兼容性不等于该旧补丁版本的安全认证。 |
| `python -B scripts/consumer-test.py --profile minimum-go --offline-proxy .artifacts/offline-consumer-proxy`；Linux/Windows | 两个平台全新缓存、已签名离线依赖；reader/signer 的 tidy 与 race 测试各四条命令全部退出 0。 |
| `python -B scripts/consumer-test.py --offline-proxy .artifacts/offline-consumer-proxy`；主线 Linux | 四条命令全部退出 0，reader/signer 全新缓存离线消费通过。 |
| `python -B scripts/fuzz-test.py`；Linux | 五个目标各执行完整 30 秒预算，均有非零执行，全部退出 0。 |
| `python -B scripts/security-test.py --allow-network`；Linux | 固定 govulncheck 与 gitleaks 均退出 0，使用真实漏洞库及完整仓库扫描。公开元数据精确例外的正/负控制另有收据。 |
| `python -B scripts/integration-test.py`；Linux/OpenBao 2.6.3 | REAL_OPENBAO 命令和脚本退出 0；根测试与 KV、定时删除、PKI、Transit、AuthNamespace 共 6 条通过，无 fail/skip；fixture 已释放。 |
| `python -B scripts/coverage-check.py --profile <最终 normal/coverage.out> --output .artifacts/release-validation-20261001/coverage-final.json` | 退出 0，15/15 组满足原有 85% 门槛，最低 87.50%。仅使用本轮 normal profile。 |
| `python -B scripts/ci-summary.py --artifacts .artifacts/release-validation-20261001/ci-jobs --output .artifacts/release-validation-20261001/ci-summary-final.json` | 退出 0，12/12 job PASS；是本地收集的双平台结果，托管 GitHub Actions 未运行。 |
| `python -B scripts/verify-release.py`，由 `record.py` 记录 | 退出 0，[最终门禁收据](../.artifacts/release-validation-20261001/release-final-01.json)为 PASS。 |

`task-status.json` 为 **18 VERIFIED / 1 BLOCKED**，`acceptance-results.json` 为 **71 PASS / 1 NOT_RUN**；历史字段完整保留旧状态和证据引用。工程门禁 PASS 的范围为 OB-001～018、AC-001～071。OB-019 / AC-072 需要实际消费方项目及单独迁移授权；托管 CI、远端分发和发布未执行；维护者仍需决定支持版本与响应时限。新远端 manifest、旧 KMS 额度、真实业务迁移和新性能 benchmark 本轮均未执行，未暂存、提交、推送、切分支或发布。

[独立审查](evidence/OB-018/release-validation-2026-10-01/independent-review.md)记录并关闭扫描根目录边界、CI binary 接线、依赖工具遥测生命周期与手册大整数示例等发现；范围内无未解决的确定缺陷。最终手册真实写入示例以字符串保存大整数，静态记录 `release-validation-static-d3183d-02` 绑定修订后文档，上一份静态记录保留。审查不替代运行检查，不构成发布批准。

资源回收记录：[Windows 代理停止](../.artifacts/release-validation-20261001/cleanup-proxy-final-01.json)退出 0；[Linux 首次清理](../.artifacts/release-validation-20261001/cleanup-final-01.json)因专属 `exec/netns/default` 的 nsfs 挂载残留退出 1，失败未覆盖。核对无专属进程、只剩该准确挂载与目录后，执行非强制 `umount` 并清理，生成[恢复收据](../.artifacts/release-validation-20261001/cleanup-final-02.json)，退出 0。随后只读确认三个本轮运行目录、专属进程及挂载均不存在；未停共享 WSL/Rancher 服务。公开工具归档、构建缓存和脱敏运行产物保留在工作区 `.artifacts` 以便复查。

最终工作树与历史完整性以[工作区核对](../.artifacts/release-validation-20261001/final-worktree-audit.json)为准：HEAD/分支不变、暂存区为空，191 项当前源码输入及 14 项静态文档核对通过，55 份上一检查点证据摘要保持一致；237 个起点文件均存在，授权路径之外的原有修改未被改写。当前可读摘要由 `scripts/current-status.py` 生成，见[当前状态](current-status.md)。

## 2026-10-01 历史检查点：项目改进与环境阻塞

本轮依据[项目评估](project-assessment-2026-10-01.md)和用户确认的[改进范围](project-improvement-prompt-2026-10-01.md)实施。起点为 `master` / HEAD `6d8251658e76f88aec5935126f303b2c59497691` 加保留的原有修改，v2 源码摘要 `e9d371d3a35385803a651db667a05cbd8fb3bb6cd36b03b23bf24ffd6139f14e`；17/17 历史证据哈希匹配、暂存区为空，无遗留本轮进程。修改后的候选需要重新验证，不能继承下方历史 PASS。

| 顺序 | 范围与原任务关联 | 状态 / 下一步 |
| --- | --- | --- |
| 1 | 构造失败敏感副本清理；OB-002/003/005 | PASS（本轮范围）：回归 RED→GREEN，18 个相关顶层 race 测试通过，独立静态审查无已确认发现。 |
| 2 | Provider 快照所有权；OB-005/006/007 | 本轮定向/race 验证与变更后契约核对 PASS；26 个顶层测试、13 个子测试通过，独立静态审查无已确认发现。 |
| 3 | 当前证据摘要和可复用远端运行清单；OB-015/018 | PASS（本地范围）：摘要一致性、manifest 模拟与 Go 回归通过；独立审查和静态契约核对完成。新清单真实执行 NOT_RUN，无新授权。 |
| 4 | 最低 Go、正式集成与固定扫描；OB-001/016/018 | 作业、证据类型、扫描来源及附件摘要校验完成并通过本地检查；Go 1.25.0 实测、可信 server lock、完整固定扫描环境仍 BLOCKED，准备方案见 release。 |
| 5 | GoDoc、确定性 Example、发布准备材料；OB-017/018 | PASS（本轮范围）：高频 GoDoc、四个无网络 Example、未发布变更与恢复限制完成；当前候选正式本地验证与证据归档完成。 |

授权范围内独立工作已完成，下一项仅依赖外部验证条件。最终候选为 v2 **`8a1b5313ccda18142539b6cc167ee987d1f07fed1368662970fd09a5897d628f`（191 输入）**，仍在原分支、原 HEAD 和保留的未提交工作树上。公开导入路径、既有接口签名、鉴权/租户边界、错误与重试语义保持稳定。原 `task-status.json` 与 `acceptance-results.json` 已逐项重绑当前报告，原记录保留在历史字段；没有另建任务台账。当前状态见[自动摘要](current-status.md)、[证据索引](evidence/OB-018/project-improvement-2026-10-01/index-security-contact-01.json)和[独立审查](evidence/OB-018/project-improvement-2026-10-01/independent-review.md)。远端预算已过期，本轮没有远端执行、安装、共享服务启停、暂存、提交、推送或发布。

本机 Windows/amd64、Go 1.26.3 的最终验证如下。本地按全局规则以 `rtk proxy` 包装命令；原始子命令退出码、源码前后标识及报告路径在[验证收据](../.artifacts/improvement-20261001/retry2-verification-receipt.json)和索引中。

| 实际入口 | 结果 |
| --- | --- |
| `python -B -m unittest discover -s scripts/tests -v`（由 `record.py --evidence-class COMMAND_TOOLING` 记录） | 退出 0，152 项通过。 |
| `python -B scripts/normal-test.py` | 退出 0，unit、race/coverage、vet、模块校验、构建、模块图六步通过，399 个测试/子测试结果。 |
| `python -B scripts/coverage-check.py --profile <本轮 normal/coverage.out> --output .artifacts/improvement-20261001/retry2-coverage.json` | 退出 0，15/15 组通过，最低 87.50%，既有 85% 门槛不变。 |
| `python -B scripts/consumer-test.py --offline-proxy .artifacts/offline-consumer-proxy` | 退出 0；reader/signer 全新缓存的 tidy 和 race 测试四条命令通过。 |
| `python -B scripts/fuzz-test.py` | 退出 0；五目标各 30 秒、非零执行。 |
| integration、security、minimum-go normal/consumer 的预检入口 | 四条子命令均退出 2并生成 BLOCKED；没有把准备失败当作场景通过。 |
| `python -B scripts/verify-release.py` | 退出 1，[真实门禁收据](../.artifacts/improvement-20261001/release-security-contact-01.json)记录 29 项拒绝。 |
| 本地 `ci-summary.py`，四份 Windows 导出输入 | 退出 1；normal/tooling 两项 PASS，其余十项因最低版本阻塞或 Linux 报告缺失而 FAIL。没有执行远端 CI。 |
| `python -B scripts/current-status.py --check` | 退出 0，仅证明[摘要一致性](../.artifacts/improvement-20261001/summary-final-status-01.json)，不代表发布通过。 |

最终独立审查发现并关闭一处证据完整性问题：gitleaks 附件摘要原先未参与校验。现已在原始报告、打包和 CI 导出三处核验，新增篡改用例 RED→GREEN、完整工具检查通过。首次失败的 GREEN 属于新测试未真正改变夹具字节，修正输入后通过；记录没有覆盖。此前远端 Go 回归的缺少 manifest 夹具、补充 import 前的构建失败，以及旧 `846b34…` 候选的本地通过报告同样保留。它们不替代最终 `8a1b53…` 的证据。

原验收为 **66 PASS / 3 PARTIAL / 3 NOT_RUN**；19 个 OB 因原依赖或范围保持 BLOCKED。29 项发布拒绝对应正式集成、固定扫描、四份最低版本证据、五个 AC 和 OB-001～018；当前 PASS 项没有旧指纹或目标失配。新远端 manifest、Linux、最低 Go 1.25.0 与远端 CI 未运行；已有 benchmark 本轮未复跑，保留历史结果且不提出新的性能结论。私密安全报告邮箱已由维护者确认为 [git@whereisit.cc](mailto:git@whereisit.cc)，支持版本策略和响应时限仍待确认；最低 Go 和扫描环境按维护者选择继续保持阻塞。

解除条件及具体下载/校验/隔离方案见[发布准备](release.md#2026-10-01-验证环境准备方案尚未执行)：可信精确 OpenBao 发布物、获准的固定扫描器与真实数据库、Go 1.25.0/Linux 环境。新远端执行还需具体 manifest 的目标、有效期、请求与资源额度、独占前缀和清理授权。任何指纹输入改变后须重新冻结并重建相应证据；旧远端额度不能结转，历史补充 sdk-test PASS 不替代 `REAL_OPENBAO`。

最终工作区检查：`git diff --check` 退出 0，暂存区为空，HEAD/分支未变；191 项源码清单逐文件匹配、55/55 证据摘要匹配。`go.mod`、`go.sum` 和服务锁与本轮开始前的字节摘要相同，原有工作树状态条目未被移除。相关验证进程已结束，无新增 fuzz corpus。全仓 diff/stat 含用户原有修改，不作为本轮新增规模；Git 的 CRLF 提示没有被当作理由全仓改写文件。

首轮回归先提取生产构造的内部所有权边界，再验证空 Option、无效 observer、缺失 CA 和错误 CA 四条可达路径。候选 `4e26a8c3e8948e48470df647cd3c3e453bef803e0419419ee2d52bf6fda876c7` 的[RED 收据](../.artifacts/improvement-20261001/ownership-red-receipt.json)实际退出 1，四条失败都来自未清理 SDK 副本。配置校验后才克隆秘密、失败时统一清理后，候选 `093a7fbd6532a4988d16fab55c0f114f6963d59555834eae8195c4de4554f832` 的[GREEN 收据](../.artifacts/improvement-20261001/ownership-green-receipt.json)实际退出 0，原输入和成功关闭路径均覆盖。官方客户端初始化没有可安全构造的额外失败入口，不为覆盖该分支增加故障注入接口。

Provider 的兼容技术选择：既有 `TokenProvider` / `SecretIDProvider` 接口与快照字段不变；没有明确协议的自定义 provider 默认借用，SDK 只清理自己的明文副本。内建未导出具体类型实现可选 `SnapshotOwnedByConsumer(actual any) bool`，只有传入的实际 provider 为同一具体实例时才交付快照所有权。包装者即使提升该方法，传入包装者自身也返回 false，避免误擦共享句柄。明确自行实现此协议的 provider 才能主动交付；协议查询留在已有 provider panic 防护边界内，在取得快照前完成，成功、错误、取消后均按实际归属处理。这是主代理在本轮兼容改进授权内的实现决定。候选 `2df6cb683b66aa403e17d10a819e1f96e7720d53f8bc313435f4b9dde63c28b7` 的 [GREEN](../.artifacts/improvement-20261001/provider-ownership-green-receipt.json) 和 [race](../.artifacts/improvement-20261001/provider-ownership-race-receipt.json) 收据实际退出均为 0；后续源码变更仍需最终验证。

## 2026-09-30 历史交接：sdk-test 全场景复测

当前 `master` / HEAD `6d8251658e76f88aec5935126f303b2c59497691` 加保留的未提交修改，source_identity v2 为 **`e9d371d3a35385803a651db667a05cbd8fb3bb6cd36b03b23bf24ffd6139f14e`（175 输入）**。用户授权仅在 `https://kms.jiup9.com:443` 的非生产 `sdk-test` 重测，最多 120 次串行 API 请求、20 分钟、9 项本轮专用资源。一次性[第八轮夹具报告](../.artifacts/runs/remote-full-fixture-20260930-08/report.json)为 PASS：112/120 次请求，Core 39/39、Transit 22/22，9 项资源均记为已回收、清理 PASS，前后源码指纹相同。默认 TLS 校验和本轮专用 PKI 证书签发、读取、链验证、撤销场景通过；不代表服务器所有证书或业务资源都已核验。完整范围、首次失败、本地检查与限制见[远端测试报告](openbao-remote-test-report-2026-09-30.md)、[当前证据索引](evidence/OB-018/remote-retest-2026-09-30/index.json)和[独立只读审查](evidence/OB-018/remote-retest-2026-09-30/independent-review.md)。

同一指纹的 Windows/Go 1.26.3 本地 `go test -mod=readonly -count=1 ./...` 通过；Python 工具测试 118 项通过，正式 normal 349 个测试结果及 15/15 组覆盖率、五目标 fuzz、两个离线消费者和六个 benchmark 子项各三轮均通过。消费者第一次因隔离 `TEMP` 位于工作区内被证据校验拒绝，改用工作区外既有 `E:\Temp` 后重新运行通过；两次报告均保留在索引。[当前发布门禁收据](../.artifacts/runs/remote-retest-20260930-final/release-remote-retest-01.json)为 FAIL（实际退出 1、25 项拒绝），因正式独立集成缺可信 OpenBao 发布版本及 digest/二进制 SHA256、固定扫描环境未备妥，另有五个 AC 和 OB-001～018 依赖未满足。Linux、最低 Go 1.25.0 与远端 CI 未运行；补充 sdk-test 不能替代 `REAL_OPENBAO` 或扫描。原验收保持 66 PASS / 3 PARTIAL / 3 NOT_RUN，19 个 OB 保持 BLOCKED。无待回收的本轮资源记录，无遗留本轮测试进程；未暂存、提交、推送或发布。

后续须取得可信 OpenBao 发布物标识、已获准的固定扫描环境，以及 Linux/最低 Go 与远端 CI 条件，分别完成正式门禁。若再次修改任何指纹输入，先重新冻结源码并重建受影响证据；不能沿用本轮远端结论。下方结构整理及此前远端运行是历史检查点，其中“当前”仅指当时的指纹。

## 2026-09-30 代码结构整理历史交接

当前工作区为 `master` / HEAD `6d8251658e76f88aec5935126f303b2c59497691` 加保留的未提交修改；source_identity v2 为 **`96a62a5230ff7e114831e39ab62b7c280e0708ac806b85fa19a053bc36be91f5`（175 输入）**。本轮按[结构维护方案与执行记录](code-structure-maintenance-plan-2026-09-30.md)完成 README 导航、`tests/remote` 同包三类辅助拆分，以及 12 个历史回归测试归位。未修改公开 SDK API、运行时包边界、module、Go 声明下限或官方 api/v2 版本。完整当前证据及原始文件 SHA256 见[索引](evidence/OB-018/structure-maintenance-2026-09-30/index.json)和[逐文件源码清单](evidence/OB-018/structure-maintenance-2026-09-30/source-manifest.json)。

Windows/amd64、Go 1.26.3、Python 3.13.13 的本地验证已完成：`go test -mod=readonly -count=1 ./...` 退出 0；[工具测试收据](../.artifacts/runs/structure-maintenance-20260930-final/tooling-tooling-01.json)为 118 项、无 skip；[normal 报告](../.artifacts/runs/2026-09-30T105643.005724Z-32c835a94e3b42babc7621ecc92f9fe9/normal/report-007.json)六步骤自然成功，记录 349 个测试/子测试结果；[本轮 race profile 覆盖率分析](../.artifacts/runs/structure-maintenance-20260930-final/coverage-analysis.json)的 15 组 Go 语句覆盖率均 ≥85%，最低 `auth` 为 86.49%。[fuzz 报告](../.artifacts/runs/2026-09-30T110029.905207Z-3ac740dad32d4acd9c8613318679e220/fuzz/report-006.json)五目标各非零执行，[消费者报告](../.artifacts/runs/2026-09-30T110435.874052Z-47db4e32da064837aeda7beef24cdaee/consumer/report-007.json)为 reader/signer 全新缓存离线通过，[benchmark 报告](../.artifacts/runs/2026-09-30T110648.016068Z-7eba31c81f7f4a2882efd922375f114c/benchmark/report-002.json)为六个子项各三轮；带 `remote` tag 的测试程序仅编译，没有执行真实连接。

迁移前远端包 24 个顶层测试通过；根包 12 个目标测试均有 Run/Pass 事件。首次根包名称比对发现已跟踪目标文件的原有测试被 `Add File` 覆盖，69→68；精确恢复后该测试通过，最终 69→69，根包全集通过。[名称审计](evidence/OB-018/structure-maintenance-2026-09-30/name-audit.json)保留首次失败与修复证据。[独立只读审查](evidence/OB-018/structure-maintenance-2026-09-30/independent-review.md)在 Task 2/3 范围未发现可证实迁移缺陷；它不替代运行验证，原未提交远端 runner 字节缺失使逐字节搬迁无法独立证明。

本轮[正式集成](../.artifacts/runs/2026-09-30T141027.493234Z-679c8128476c42269f3001b84feb1527/integration/report-001.json)因缺可信精确版本及 digest/二进制 SHA256 为 BLOCKED；[固定扫描](../.artifacts/runs/2026-09-30T141044.523769Z-f2caf1b36bb04adfb0152bbb98a635af/security/report-001.json)因未授权下载工具/数据库为 BLOCKED。Linux、最低 Go 1.25.0 和远端 CI 未运行。旧指纹 `dee54e4f2f49c24f71e8cd412064eac0286b2ba3f415c1850de05e82ef1d093b` 的 [sdk-test 全场景报告](../.artifacts/runs/remote-full-fixture-20260930-07/report.json)仅是历史补充；本轮未获新请求/资源预算，**当前指纹远端场景 NOT_RUN**，本轮未创建远端资源。[发布门禁收据](../.artifacts/runs/structure-maintenance-20260930-final/release-release-02.json)记录 `verify-release` 实际退出 1：两类正式报告、五个 AC 与 OB-001～018 共 25 条拒绝；当前台账与 PASS 报告没有指纹失配。AC 保持 66 PASS / 3 PARTIAL / 3 NOT_RUN，19 个 OB 保持 BLOCKED，未发布。

下一步仅依赖外部输入：可信 OpenBao 发布物版本及 digest/本地 binary SHA256、隔离固定扫描环境或明确准备授权、Linux/最低 Go 环境与另行授权的远端 CI；如需当前指纹的 sdk-test 结论，须另给明确请求及资源预算再运行。任何纳入指纹的改动都需重新冻结并重建受影响证据。没有遗留本轮测试进程或远端待回收资源；暂存区为空，未创建工作树/分支、未暂存、提交、推送或发布。下文为旧指纹历史检查点，不再代表当前。

## 2026-09-30 全场景续跑历史交接

先读[远端测试报告的最新检查点](openbao-remote-test-report-2026-09-30.md)。当前 `master` / HEAD `6d8251658e76f88aec5935126f303b2c59497691` 加未提交修改，source_identity v2 `dee54e4f2f49c24f71e8cd412064eac0286b2ba3f415c1850de05e82ef1d093b`（173 输入）。[第七轮 sdk-test 全场景夹具](../.artifacts/runs/remote-full-fixture-20260930-07/report.json)使用 112/120 次串行请求，Core 39/39、Transit 22/22，HMAC 无 `keys` 元数据场景通过；9 项本轮专用资源全部回收，前后指纹一致。该结果为目标服务补充验证，不替代缺失精确版本/digest 的正式独立 fixture、固定扫描器、Linux/最低 Go 或远端 CI。当前本地正式 normal、fuzz、离线消费者、benchmark 与工具检查通过；真实集成和扫描维持 BLOCKED。源码和台账以本节与当前 JSON 为准；下文旧指纹及“当前/最终”均为历史检查点。

当时原台账 `task-status.json` 与逐项 `acceptance-results.json` 关联该轮指纹的正式证据，并在补充字段关联[第七轮报告](../.artifacts/runs/remote-full-fixture-20260930-07/report.json)。该轮 `rtk proxy python -B scripts/verify-release.py` 自然退出 1，[门禁收据](../.artifacts/runs/remote-20260930-current/verify-release-release-20260930-03.json)记录 25 项真实拒绝；该轮通过的目标服务夹具没有填进仅接受 `REAL_OPENBAO` 的证据字段。此段后续动作已由上方当前交接更新。

### 历史远端续跑记录（不代表当前）

旧指纹 `376c3dbba0d9340762216bd161170f435eb3834daeb76342aa6eb99b6496a85d` 的两次全场景夹具续跑均在 KV List 停止，各自 53 次请求、9 项专用资源全部回收；两次定向诊断分别使用 17 和 18 次请求，各自 4 项资源全部回收，原预算累计 88/120 次。详见[第二次续跑](../.artifacts/runs/remote-full-fixture-20260930-03/report.json)、[斜杠诊断](../.artifacts/runs/remote-list-diagnostic-20260930-01/report.json)和[方法诊断](../.artifacts/runs/remote-list-method-diagnostic-20260930-01/report.json)。

两轮分别创建的同名专用目录中，带斜杠 `LIST` 得 404、带斜杠 `GET ?list=true` 得 200，且后者列出自有种子键；管理员和受限测试身份均如此。这强烈提示方法入口不兼容，但不是同一资源/会话的严格 A/B，不能唯一归因。官方 api/v2 v2.7.0 的 `Logical.ListWithContext` 使用后者，但本项目[既有接口契约](spec/02-接口契约.md)明确禁止静默换 GET。当前等待用户明确契约决定；此前试探性加末尾斜杠已撤销。可恢复的下一步是在契约决定后完成最小实现/测试或保持原协议并标记该服务不兼容，再取得新远端预算才重跑完整夹具。当前 Go 聚焦测试三个包 PASS、Python 工具测试 118 项 PASS；正式报告/台账仍属旧指纹，`verify-release` 返回 1，须在最终源码冻结后重建适用证据并逐项更新原 OB/AC。没有依据将真实独立集成、扫描、Linux/最低 Go 或远端 CI 标绿；未暂存、提交、推送或发布。

## 2026-09-30 远端测试入口历史交接

当前入口是 [远端测试报告](openbao-remote-test-report-2026-09-30.md) 和 [配置/恢复说明](remote-testing.md)。主代码已冻结为 v2 `32dfe9e8dbfa041033bb2ae2ce10488af173b9fceb4fda8388f7af450764471e`（172 输入），HEAD 和 master 未变。原台账的 `remote_execution_2026_09_30` 保存检查点，结构重构及 R1/R2/R3 保留为历史。

**本地远端入口与适用验证完成；指定服务 TLS/健康连通性及新凭据在 `sdk-test` 的身份自查通过，SDK 业务测试及正式交付门禁仍阻塞，未发布。** [首次只读连接报告](../.artifacts/runs/remote-connect-20260930-01/report.json)记录健康 HTTP 200、匿名与带首次凭据的自查 HTTP 403；[新凭据报告](../.artifacts/runs/remote-connect-20260930-02/report.json)记录健康 200、匿名自查 403、带凭据自查 200，有限身份字段形状有效。两次探针共六次 GET；首次 403 的原因未定。独立审查发现已修复；当前正式证据为工具 114 项、normal 329 个测试结果/15 组覆盖率达标、五目标 fuzz、四 benchmark 六子项各三轮、两个离线独立消费者、示例测试/构建及 remote tag 仅编译通过。AC 为 66 PASS / 3 PARTIAL / 3 NOT_RUN，19 个 OB 因验收/依赖或范围保持 BLOCKED。verify-release 保留 25 项真实拒绝，缺 Linux job 的本地 CI 汇总也保持 FAIL；详见 [当前证据索引](evidence/OB-018/remote-2026-09-30/index.json)。

服务目标为 `https://kms.jiup9.com:443`，已收到 namespace `sdk-test` 及测试账号全场景授权。两次凭据分别通过交互遮蔽输入供窄范围探针使用，未注入正式 runner；环境性质与 KV/PKI/Transit 的具体测试资源尚未提供。已发送六次只读请求，未创建远端资源、无待回收远端资源；探针均已退出。缺项只阻塞相应远端动作；既有真实 fixture、扫描、Linux/最低 Go/远端 CI 阻塞仍保留。

恢复时先核对本次进程、Git、当前指纹及 `.artifacts/runs/remote-20260930-final/` 和最新正式报告。下一步取得精确获准的 KV v2 测试 mount/path/version/标记及环境性质；不要重复连接探针，也不要在聊天中再次传送凭据。禁止复用旧报告的 hash 或把连接探针改记正式集成通过。

## 2026-09-29 结构调整历史交接

**本地结构改进完成，交付门禁仍阻塞，未发布。** [本轮完整报告](code-structure-refactor-report-2026-09-29.md) 和 [证据索引](evidence/OB-018/structure-refactor-2026-09-29/index.json) 是当前结果入口。唯一进度仍在原 `task-status.json` 的 `structure_refactor_2026_09_29`；没有新增平行任务台账。

当前 master / HEAD `6d8251658e76f88aec5935126f303b2c59497691`，v2 `771ac8fe1964aaebc7dd258d1060a34bbb8781e823ea86b182f8a465d9f8be0c`，163 个输入。比较依据是 `.artifacts/runs/structure-refactor-20260929-start/baseline.json` 和 `source/` 的原未提交工作树（v2 `cd46dc...3af2`、159 个输入），不是只比较 HEAD。

完成依赖单向化、七类证据校验拆分、独立 fixture、RunRecord 与显式 Go 探测/CLI 退出分离；未触及的配置整理和 Go 历史测试移动按需暂缓。公开 SDK API、Go 代码/测试、module、LICENSE、依赖和锁文件未改。独立 risk_reviewer 对本轮稳定差异无发现，verifier 完成正式 Go 检查。

冻结后 Windows/Go1.26.3：106 项工具测试、14/12 项夹具独立测试、normal 六命令/291 个 Go 测试结果、15/15 覆盖组≥85%、五目标 fuzz、六个 benchmark 子项各三轮、两个 fresh-cache 离线消费者、示例测试/构建全部通过。真实集成和扫描为 BLOCKED；Linux/最低 Go/远端 CI 未运行。首次 CI 导出路径错误保留，按固定入口重导出后 Windows 两个 job PASS、六个 Linux job 缺失仍 FAIL。

原 AC 逐项重新关联当前真实证据：66 PASS / 3 PARTIAL / 3 NOT_RUN；19 个 OB 保持 BLOCKED。verify-release 实际退出 1，25 条拒绝对应两类正式报告、五个 AC 和 OB001～018，当前无旧指纹失配。AC072/OB019 真实业务迁移明确排除。

剩余最小动作：取得精确 OpenBao 版本和可信发布物、固定扫描工具/数据库的准备环境或授权、隔离 Linux/最低 Go 环境及另行远端授权。改动服务锁或其他指纹输入后重新冻结，再重建必要正式证据。不要用本轮 Windows 结果补写 Linux 或扫描 PASS，不发送业务 Token。

没有遗留本次验证进程或测试实例，暂存区为空；本机 `.serena/project.yml` 的 language_backend 状态变化已单独披露，未回滚。未 stage/commit/push/pull、创建分支/工作树、下载安装或发布。恢复时先核对 owned PID、Git、当前指纹和本检查点；可执行本地工作已完成，不从头重做七阶段工程。

执行裁定：使用用户指定的现有工作区、唯一台账和连续执行方式，替代技能默认的工作树、额外 ledger、提交或阶段确认流程。下方七阶段及 R1/R2/R3 作为历史记录保留。

## 2026-09-29 七阶段历史交接（结构重构前）

执行依据为 `docs/implementation-plan-2026-09-29.md`；当前工作区为 `E:\xen\code\claude\project\jiupiao\openbao-sdk-go`，HEAD 为 `6d8251658e76f88aec5935126f303b2c59497691`。进度、裁定和验证位置统一记录在 `task-status.json` 的 `execution_2026_09_29`，本节不复制另一份任务状态。

**本地改进完成，交付门禁仍阻塞，未发布。** 最终 source_identity v2 为 `cd46dc83078962d602886183bba7aa4a77f60702df5a4a167a4340f529fe3af2`（159个输入）。[完整执行结果](implementation-execution-report-2026-09-29.md) 说明七阶段产出、实际命令、性能数据、失败历史、独立审查和剩余条件；[证据索引](evidence/OB-018/implementation-2026-09-29/index.json) 保留不可变原始报告/日志位置及归档副本摘要。

Windows Go1.26.3：90项工具回归、正式normal六命令、15组语句覆盖率、五目标fuzz、六个benchmark子项三轮、两个离线独立消费者、示例测试与构建均已实际结束并通过。原验收为66 PASS / 3 PARTIAL / 3 NOT_RUN；原任务依赖未满足时保持BLOCKED，没有把实现完成标成VERIFIED。

`verify-release.py` 实际退出1：真实集成、固定扫描、AC-001/065/066/070/071和原OB依赖链未满足。Linux、最低Go1.25.0和远端CI未执行；本地CI汇总实际拒绝缺失的Linux job，静态YAML通过不等于远端CI通过。

模块地址和Apache-2.0许可已落实。下一步只依赖外部条件：提供OpenBao精确版本及可信镜像digest/本地binary SHA256；提供或授权准备固定扫描器/数据库；提供隔离Linux/最低Go环境并另行授权远端操作。服务锁改变会改变源码指纹，必须在冻结前更新，再重建对应正式证据。OB-019/AC-072真实业务迁移本次排除。不要提供密码或业务Token。

没有遗留验证进程或临时测试实例；本地缓存与证据产物保留供复跑。暂存区为空，原用户改动受到保护；未提交、推送、发布或操作生产数据。恢复时先核对Git、当前指纹、实际进程和原台账，不重启已经完成的本地改进。

## R3 历史交接（保留原记录）

以下R3及其“下一条命令/下一阶段”均为历史记录，不是2026-09-29当前待办；不要用历史依赖下载阻塞覆盖本轮已完成的离线消费者验证。

当前工作目录：`/mnt/data/openbao-r3/openbao-sdk-go`。当前源码指纹：`459e4262e9d1665e9ff6018fda0797046daf361b55a2052c230903c0c80e3399`。

最早阻塞仍为 OB-001。本轮在 R2 原源码上增加公共依赖转移工具，未修改 98 个 Go 文件、go.mod、go.sum、公开签名、原始方案或任务依赖。

## 下一条可执行命令

**在能够取得公共 Go 依赖的隔离开发机、使用本交付包源码执行：**

```bash
cd openbao-sdk-go
make dependency-export
```

成功才产生 `.artifacts/openbao-public-dependencies.zip` 和对应 `.sha256`。需要将它们通过可信通道回传；不是上传整个个人缓存，也不需要生产 Token 或私钥。当前本会话未产生这两个真实文件。

接收后按 `docs/dependency-transfer.md` 运行 `scripts/dependency-bundle.py verify`，真实 Go 从新的空缓存和本地 file proxy 独立认证，不预装传入 go.sum。再以验证后的代理执行 `make dependency-check` 和 `make normal-test`。固定工具链不足时只依据真实上游材料切换，禁止猜版本、auto 下载或替换官方模块。

## 本轮验证

- 35 项 Python 工具测试通过，其中 18 项新增；只证明转移/拒绝逻辑。
- 64 个自有 Go 顶层 Test 函数通过，race 与对应 vet 通过；另外运行到 6 个 fuzz 种子入口，不与 Test 数量混加。
- 原五个指定 fuzz 目标分别运行 30 秒并取得退出码 0。初次 path 包装调用和初次 direct/consumer 包装调用的中断日志保留，不计作正常通过。
- 正式 normal-test 返回 2（其内部七条 Go 命令各返回 1）；官方依赖未取得。
- integration/consumer/security 入口均返回 2，真实服务、消费与扫描主体未通过。
- 原 72 项验收：8 PASS / 59 PARTIAL / 5 NOT_RUN；任务：0 VERIFIED / 14 IN_PROGRESS / 5 BLOCKED。工具测试不升级 SDK 验收。

## 下一阶段待解决

获得真实依赖后优先运行官方适配器路径，并用失败测试修复实际问题。OpenBao 二进制/镜像、扫描工具以及固定 Go 工具链不包含在本转移包中，另需核验。`consumer-test.py` 当前固定追加网络代理，独立消费者的纯离线运行还需后续修正并验证，不承诺此包已解除 OB-017。OB-019继续缺真实项目源码。

原始方案在 `docs/spec/`；唯一台账 `task-status.json`；验收 `acceptance-results.json`；本轮证据在 `docs/evidence/OB-001/resume03/`。全新隔离目录没有 .git，未重建 Git 历史、提交、push 或发布；审查为作者自审。无会话结束后的后台任务。
