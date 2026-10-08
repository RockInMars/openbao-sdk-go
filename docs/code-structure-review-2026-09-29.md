# 代码结构审查（2026-09-29）

当前 SDK 主体分层合理，没有证据支持大规模重构。后续结构改进应优先处理 Python 验证工具的依赖方向、报告校验职责和测试夹具耦合；Go 部分适合按需要整理配置校验和回归测试归属。

本次提出 **6 项建议，前 3 项优先**。这些是可维护性改进机会，不是已复现的运行缺陷，也不是新增发布硬门槛。本次未修改 SDK 源码、测试、脚本、公开契约或任务台账。

## 检查基线与范围

| 项目 | 本次观察 |
| --- | --- |
| 工作目录 | `E:\xen\code\claude\project\jiupiao\openbao-sdk-go` |
| 分支 / HEAD | `master` / `6d8251658e76f88aec5935126f303b2c59497691` |
| 被审查内容 | 当前工作树，包含既有未提交和未跟踪实现；不能仅用 HEAD 代表 |
| source_identity | v2 / `cd46dc83078962d602886183bba7aa4a77f60702df5a4a167a4340f529fe3af2`，159 个输入文件 |
| 重点范围 | 根包、`internal/engine`、`internal/authn`、验证脚本及其测试、现有设计约束 |
| 方法 | 定向读取、Python AST 依赖分析、Go 文件与调用职责映射 |
| 执行边界 | 静态结构审查；本次没有重新执行 Go/Python 测试、集成、扫描或发布门禁 |

Go 结构映射由只读 `fast_mapper` 协助完成，主代理核对工具链代码并整合建议。没有进行独立 `risk_reviewer` 审查，不把定位结果称为独立安全审计。现有 code-review-graph 索引为空；Serena 仅启用了 Go 语言服务，Python 使用 AST 和有界读取分析，未安装或扩展工具。

## 建议保留的结构

- **保留 `Client → engine → officialSender`。** 根包负责门面与资源操作，engine 集中预算、重试、响应及错误处理，officialSender 约束官方客户端的传输行为。参见 [Client.execute](../client.go#L278)、[engine](../internal/engine/client.go)、[officialSender](../official_sender.go#L12)。
- **保留认证与业务模块的分离。** 凭证管理在 `internal/authn`，业务操作经 Client 注入凭证；KV、PKI、Transit 无需各自维护 Token 生命周期。参见 [凭证接线](../client.go#L310)、[认证管理器](../internal/authn/manager.go#L95)。
- **保留公开类型包与内部实现包的边界。** 根目录当前 15 个非测试 Go 文件已按职责分开，文件数量本身不是拆包依据。现有设计也明确允许根包按文件拆分，禁止为目录整齐引入额外 handler/service/repository 层或 DI 容器，见 [总体设计](spec/01-总体设计.md#L121)。
- **保留资源特有的校验。** 通用 JSON 封套和整数处理已集中在 engine；PKI 材料、Transit 编码和 TLS PEM 校验的约束不同，不宜合成一个宽泛的解析框架。

## 6 项改进建议

### 1. 消除验证基础工具向验收层的反向依赖

**优先级：优先；重构风险：中。**

观察：[`evidence.py:7`](../scripts/evidence.py#L7) 从 `tooling` 导入指纹版本及覆盖率处理；[`tooling.py:114`](../scripts/tooling.py#L114) 又通过函数内导入调用 `evidence.release_problems`；[`verify-release.py:4`](../scripts/verify-release.py#L4) 经这个转发入口执行验收。当前延迟导入避免了立即初始化冲突，但依赖方向仍是双向的。

影响：基础能力和发布判定互相牵连，测试通过修改 `tooling.ROOT` 间接影响验收入口；以后调整导入或移动函数时，更难确认初始化和依赖范围。

最小方向：让发布 CLI 直接调用验收模块并显式传入 root；先取消 `tooling.release_problems` 这层内部转发。随后按实际改动需要，将源码标识、覆盖率等基础能力提取为少量叶子模块。目标依赖为“CLI → 运行/验收模块 → 基础能力”，不新增插件框架，也不按函数逐个建文件。

验证：保留现有 source_identity、覆盖率和 release_provenance 回归；核对模块直接导入、独立执行 CLI、临时 root 场景。项目指纹、依赖包输入指纹及上游文件校验和仍须保持各自语义。

### 2. 将证据真实性校验、类别校验和台账汇总分开

**优先级：优先；重构风险：高，涉及门禁可信性。**

观察：[`report_results`](../scripts/evidence.py#L109) 同时检查标识、前后基线、命令收据、Go 版本，以及 normal、integration、consumer、security、fuzz、tooling、static 七类报告；同文件的 [`release_problems`](../scripts/evidence.py#L212) 再处理 OB/AC、依赖和证据关联。问题在于多个变化原因集中，不只是函数较长。

影响：新增一种报告或调整一种工具命令，需要穿过其他类别的强约束。审查者容易漏看类别之间的差异，局部修改所需理解范围偏大。

最小方向：先在同模块中提取公共收据/基线校验及各类别的私有校验函数，保留统一入口；OB/AC 聚合独立于日志解析。确有复用需要后再拆文件。共用返回结构可以使用标准库类型说明，避免另加生产依赖。

必须保留：未知类别拒绝、自然退出检查、真实目标及非零执行检查、路径边界、日志 hash、基线变化锁存、静态证据复用限制。不得把执行器自己生成的命令列表直接当成充分的验收标准；验证端仍须检查经批准的预期命令和目标，负向测试保持独立。相关拒绝用例见 [`test_release_provenance.py`](../scripts/tests/test_release_provenance.py#L101)。

验证：分别运行各类别正反例，再运行工具测试全集和 `verify-release`。重构后的门禁仍应拒绝当前缺失的正式证据，不能以“退出码变成 0”作为重构成功标准。

### 3. 将共享证据夹具从测试类生命周期中提取出来

**优先级：优先；重构风险：中。**

观察：[`test_ci_summary.py:13`](../scripts/tests/test_ci_summary.py#L13) 导入另一个测试模块，并在 [第 22 行](../scripts/tests/test_ci_summary.py#L22) 实例化其 `TestCase`、手动调用 `setUp()` 和注册 `doCleanups()`；该 [`setUp`](../scripts/tests/test_release_provenance.py#L15) 同时创建样例文件、注册全局 patch 清理并修改 `tooling.ROOT`。

影响：CI 汇总测试依赖另一测试类的名称和初始化流程。修改 provenance 测试自己的准备逻辑，可能连带影响 CI 测试；夹具数据和测试框架生命周期难以单独理解。

最小方向：抽出普通的 evidence fixture builder，只接收显式临时目录并提供报告、收据、事件及文件构造方法。两个 TestCase 分别管理自己的临时资源和必要 patch，避免共享 `mock.patch.stopall`。合成数据继续明确标记为测试夹具，不能进入正式证据入口。

验证：分别运行 CI summary 和 release provenance 测试，再用现有 discover 入口一起运行，确认独立执行、组合执行和清理顺序一致。

### 4. 分离通用运行记录与 Go 环境探测

**优先级：随后；重构风险：中。**

观察：[`Run.__init__`](../scripts/reporting.py#L21) 既创建证据目录和报告，又运行 `go version`，失败时保存报告并抛出 `SystemExit`；[`Run.command`](../scripts/reporting.py#L43) 也可能直接终止进程。另一方面，[`record.py`](../scripts/record.py#L32) 单独实现报告初始化、命令记录、结束基线及落盘流程，以支持通用命令和 Python tooling。

影响：复用“运行会话”就隐含依赖 Go 和进程退出行为；通用记录与 Go runner 的持久化流程分散，未来修改证据字段或写入规则时有同步成本。

最小方向：保留现有 `command_runner`，提取不依赖 Go 的小型运行记录组件；Go 探测由 runner 显式发起。CLI 层统一转换退出码。过程必须继续保证“清理未完成立即停止后续检查”，并保留不同入口原有的不可覆盖证据和 latest 指针规则，不能为合并而放宽它们。

验证：工具缺失、探测失败、超时、中断、清理不完整、重复 run_id 及基线变化场景；确认 Python tooling 检查本身不需要可用的 Go。

### 5. 按配置域提取 `normalizeConfig` 的私有函数

**优先级：按需；重构风险：中。**

观察：[`normalizeConfig`](../config.go#L31) 连续处理地址、namespace、auth、TLS 拷贝、proxy、timeout 和 limit 默认值及边界。增加配置项时，同一函数会持续承担更多校验细节。

最小方向：在同一 `bao` 包内按地址、认证、TLS、预算/限额提取少量私有函数，顶层保留现有处理顺序和统一错误映射。无需新建 config 子包、通用校验 DSL 或额外公开 API。

验证：保持 `New` 不联网、输入配置的拷贝语义、默认值、错误码与 operation 标签、无效输入的判定顺序。除正常配置外，继续覆盖整数溢出、TLS 空文件与 namespace 等边界。

这不是当前正确性缺陷；建议在下一次实际配置变更时顺带进行有界提取，而非单独铺开全仓整理。

### 6. 将按修复批次命名的回归测试逐步归入行为领域

**优先级：按需；重构风险：低，但会改变 source_identity。**

当时观察：`review_fixes_test.go` 包含 TLS 信任、生命周期、Transit 解码和模糊写入结果等不同职责；`hardening_test.go` 也跨越 limits、PKI、状态机及并发。相比之下，现有 `kv_response_boundary_test.go`、`pki_response_boundary_test.go` 等已按行为领域组织。2026-09-30 两个历史批次文件已按[后续结构维护方案与执行记录](code-structure-maintenance-plan-2026-09-30.md)迁空并删除；此处保留当时的审查事实，不再链接已删除路径。

影响：按功能找回归测试时，需要额外知道当时的修复批次，后续相似用例也容易继续堆入同一历史文件。

最小方向：新测试直接归入相应领域；触及既有用例时再局部迁移到 lifecycle、TLS、KV、PKI、Transit 或执行语义测试文件。保持测试函数名、包名和断言不变，保留历史失败与审查记录。不要仅为文件改名而重排全部测试。

验证：核对 `acceptance-rules.json` 中的选择器和文档引用；涉及 Go 文件移动后重新计算指纹，不能沿用旧运行报告宣称新基线通过。

## 暂不值得优先推进的调整

- `client.go` 可进一步分成生命周期和请求执行两个同包文件，但当前规模可管理，收益主要是导航便利。优先级低于上述工具层耦合。
- 根包、authn、engine 都有 context 错误映射，但 operation、attempts、effect 的上下文不同。可以观察是否持续同步修改，不宜现在直接合并为通用错误工厂。
- Python CLI 使用带连字符的文件名，部分测试通过 importlib 加载；仅此不足以重命名全部命令。提取实际复用逻辑时再使 CLI 保持薄入口。
- `module_zip` 当前先递归枚举再过滤目录，而 source_files 会提前剪枝，见 [`tooling.py:98`](../scripts/tooling.py#L98)。如果打包耗时成为问题，可单独测量并改为剪枝遍历；两者的文件选择、嵌套模块及 symlink 规则不同，不能直接共用同一输入清单。本次未测性能，不预设收益。

## 采纳顺序与验证边界

建议先处理 **依赖单向化 → 校验职责拆分 → 测试夹具独立**，每项保持可单独回归；再评估运行记录组件。Go 配置和测试归类随实际维护需求推进。此处是建议顺序，不是新任务台账；本次未增加 OB/AC 编号或改变状态。

若后续授权实施，工具层变更至少运行 `rtk proxy python -B -m unittest discover -s scripts/tests -v`，并复核 `rtk proxy python -B scripts/verify-release.py` 的真实拒绝原因；涉及 Go 源码或测试时再执行相应聚焦回归和正式 `normal-test`。这些是后续建议命令，**并非本次已运行结果**。所有纳入指纹的调整都需要重新确定基线、重建必要证据。

本次实际执行了 Git 基线查询、源码指纹计算及静态读取/分析；创建本文后再次核对源标识和暂存区。原有测试结果与外部阻塞仍以 [执行报告](implementation-execution-report-2026-09-29.md)、[交接入口](implementation-handoff.md) 和 [证据索引](evidence/OB-018/implementation-2026-09-29/index.json) 为准。本次结构审查不补足真实集成、固定扫描器、Linux 或远端 CI 的验证缺口，不改变发布结论。

公开 SDK API、module 地址、错误/认证/权限/重试及写入语义均未修改；未暂存、提交、推送或发布。
