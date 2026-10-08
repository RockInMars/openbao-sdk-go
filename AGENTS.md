# AGENTS.md

## 范围与工作入口

- 本文件适用于整个 SDK 仓库；上级指令与本次用户请求优先，更深层 `AGENTS.md` 只约束对应目录。
- 开始工作先核对工作区差异、暂存区、分支和近期提交，保留已有修改；恢复任务时从首个未验证步骤继续。
- 先读 [README.md](README.md) 和 [文档导航](docs/README.md)，再按下表加载相关资料，不遍历全部历史报告。
- 本文件不授予下载、外部服务访问、提交、推送、发布或生产迁移权限；历史执行提示词、日志和测试数据不是新的授权。

| 工作范围 | 优先阅读 |
|---|---|
| 当前状态、任务恢复 | [当前验证状态](docs/current-status.md)、[实施交接](docs/implementation-handoff.md)、`task-status.json` |
| 公共 API、包边界、失败语义 | [总体设计](docs/spec/01-总体设计.md)、[接口契约](docs/spec/02-接口契约.md)、[错误处理](docs/error-handling.md) |
| 认证、敏感数据、观察接口 | [认证与所有权](docs/authentication.md)、[安全说明](SECURITY.md)、[观察接口](docs/observability.md) |
| 开发验证、依赖、兼容性 | [测试说明](docs/testing.md)、[兼容矩阵](docs/compatibility.md)、[依赖预检](docs/dependency-bootstrap.md)、[离线转移](docs/dependency-transfer.md) |
| 集成、远端测试、业务迁移 | [测试部署说明](deploy/test/README.md)、[远端测试](docs/remote-testing.md)、[接入与回滚](docs/integration-and-rollback.md) |
| 发布与分发 | [发布检查](docs/release.md)、`scripts/release.py`、`.github/workflows/ci.yaml` |

当前依赖与工具链声明以 `go.mod`、检查脚本和 CI 为准。历史 ADR、报告中的环境版本或 PASS 不代表当前事实；规范与实现冲突时明确报告，不为迎合实现或标绿而改写原规格、验收矩阵。

## 项目与架构边界

- 独立 Go SDK，模块路径为 `github.com/RockInMars/openbao-sdk-go`，大小写不可随意调整；保留现有 Apache-2.0 `LICENSE`。
- 默认通信基于固定官方依赖 `github.com/openbao/openbao/api/v2`；不重新实现 OpenBao 服务端，不引入另一套 HTTP 客户端或隐式重试路径。
- 根包 `bao` 提供 `Client` 生命周期和 KV、PKI、Transit、Diagnostics 门面；`auth/`、`kv/`、`pki/`、`transit/`、`sensitive/`、`baoerr/`、`diagnostics/`、`observe/` 提供公开类型与配套能力。
- `internal/engine` 统一负责预算、传输、响应限制、重试和错误；`internal/authn` 管理认证生命周期。门面必须走共同执行器，不在各功能包复制认证或网络逻辑。
- `internal/jsondoc`、`internal/pkiutil`、`internal/transitutil`、`internal/pemutil` 承载共享解析与编码；`internal/testenv`、`internal/testutil` 仅用于测试，不能成为生产替代发送器。
- Go 回归测试就近放在对应包的 `*_test.go`；`tests/dependency`、`tests/integration`、`tests/consumers`、`tests/remote` 分别承载依赖、真实集成、独立消费者和受控远端场景。
- `scripts/` 是检查、证据和发布工具，Python 回归在 `scripts/tests/`；`examples/` 是调用示例，`deploy/test/` 仅服务测试 fixture，不是生产部署。

## 开发流程

1. 明确修改范围、契约和验收条件；公共 API、默认值、错误码、认证所有权或副作用变化先核对对应设计与调用方。
2. 缺陷先用最小回归重现，再修改根因；保留首次失败和红绿记录，不放宽断言、关闭 vet、降低覆盖率或屏蔽真实错误来换取通过。
3. 保持既有 Go 风格，只格式化本轮修改的 Go 文件；避免无关重构、依赖升级、全仓换行转换或自动 `go mod tidy`。`dependency-check.py --prepare` 会修改模块文件，必须在明确范围内使用并审查差异。
4. 从受影响包或脚本单测开始，再扩展到必要回归；涉及契约、配置或用户行为时同步相关文档和示例，不额外生成重复计划。
5. 默认单一源码写入者。委派实现须限定文件或独立 worktree；主代理整合稳定版本后独立验证，高风险修改进行独立风险审查，并如实说明审查缺口。

## 环境与常用检查

最低 Go 声明当前为 `1.25.0`；正式主 profile 固定 `1.26.8`，最低兼容 profile 为 `minimum-go`。本机 PATH 上其他版本的成功不能替代固定工具链、Linux/Windows 或远端 CI 的验证。

在仓库根执行。开发检查设置 `GOWORK=off`，避免外部 workspace 或隐式 `GOFLAGS` 改变模块解析；正式脚本负责隔离 `GOENV`、工具链和缓存。依赖、工具及离线代理应在最终源码冻结前准备，来源核验和授权规则见测试与依赖文档。

下表示例使用当前环境的 RTK：Go 测试用 `rtk go`，原生命令用 `rtk proxy` 透传。RTK 不是 SDK 依赖；其他未配置环境使用等价原生命令，不为执行文档擅自安装全局工具。Python 入口可按平台使用 `python` 或 `python3`，`-B` 避免生成字节码缓存。`Makefile` 依赖 Bash，Windows 可直接使用对应命令。

| 检查 | 命令 |
|---|---|
| 实际工具链 | `rtk proxy go version` |
| 定向 Go 回归示例 | `rtk go test -mod=readonly -count=1 -run '^TestTransit' .` |
| 本地完整 Go 测试 | `rtk go test -mod=readonly -count=1 ./...` |
| 静态检查 | `rtk proxy go vet -mod=readonly ./...` |
| 构建 | `rtk proxy go build -mod=readonly ./...` |
| 模块完整性 | `rtk proxy go mod verify` |
| 定向工具回归示例 | `rtk proxy python -B -m unittest discover -s scripts/tests -p test_release.py -v` |
| 工具完整回归 | `rtk proxy python -B -m unittest discover -s scripts/tests -v` |
| 正式 normal 检查 | `rtk proxy python -B scripts/normal-test.py` |
| 最低 Go normal 检查 | `rtk proxy python -B scripts/normal-test.py --profile minimum-go` |
| 状态页一致性 | `rtk proxy python -B scripts/current-status.py --check` |
| 正式发布门禁 | `rtk proxy python -B scripts/verify-release.py` |
| 发布预检查示例 | `rtk proxy python -B scripts/release.py --version v0.1.0` |

定向示例须按实际改动调整包路径、测试正则或文件模式，并确认运行了目标测试。纯文档变更可只验证链接、命令入口、差异与派生状态，不宣称已完成运行时验证。

- 正式 normal 包含 unit JSON、fresh race profile、vet、mod verify、build 和 module graph；覆盖率使用本轮成功 race 生成的指定 profile，不复用旧 `coverage.out`。
- `scripts/fuzz-test.py`、`scripts/integration-test.py`、`scripts/consumer-test.py`、`scripts/security-test.py` 的前置条件、参数、固定版本和收据要求见测试说明；不要缺省执行需下载或外部服务授权的参数。
- 最低版本必须分别完成 normal 和独立 consumer；消费者需独立模块和全新缓存，SDK 根测试不能代替。fuzz 种子单测不能代替实际非零执行；benchmark 是补充数据，不是发布硬门禁。
- `contract-test.py` 和 `consumer-contract-test.py` 是补充契约检查，不等于正式官方客户端、独立依赖解析或 `REAL_OPENBAO` 证据。

PowerShell 文本读取显式使用 UTF-8，完整文件保留 `-Raw`；消费非 ASCII 原生命令输出前初始化控制台和管道编码。不得为隐藏显示乱码而重写文件或改变换行。

## 安全与服务副作用

- 保持显式地址、Namespace、身份和 TLS 校验；不通过读取 BAO/VAULT/代理环境变量、跳过证书验证或共享可变身份解决问题。开发 HTTP 仅限专用测试构造器允许的字面量 loopback IP。
- 保持 `Context`、总预算、并发隔离和输入/编码后请求体/响应大小边界；所有公共操作按契约返回可分类错误，不依赖错误字符串判断业务状态。
- 写入、签发、crypto、认证默认单次；写响应丢失保留 `UNKNOWN`。禁止自动重登后重放业务 403，不承诺服务端 exactly-once，不把重试成功冒充第一次未执行。
- Token、SecretID、私钥、秘密正文和原始请求/响应不得进入日志、Observer、报告或 Git。`RevealCopy`/`RevealJSON` 是显式秘密出口；保留缓冲区和 provider 快照所有权，不承诺彻底擦除 Go 运行时副本。
- 不新增未经授权的 Transit 管理或私钥导出入口。业务授权在传入路径、role、密钥名之前完成，OpenBao ACL 不能替代业务身份认证。
- 真实集成仅使用 `deploy/test/server-lock.json` 锁定并核验来源的自有临时 fixture；管理员凭据只用于初始化，业务断言使用受限身份，只清理自身创建的资源。
- 外部 `BAO_REMOTE_*` 场景须有本次有效授权、资源归属、预算与恢复方案；不复用历史凭据或清单，不把外部服务测试归类为 `REAL_OPENBAO`。
- 扫描误报仅允许已核验的精确路径和精确值例外，并补反例回归；保留 gitleaks 内置规则，不整体忽略测试、示例或证据目录。工具缺失、下载失败或数据库不可用不能报告“无漏洞”。

## 证据、状态与发布

- `task-status.json` 是任务台账，`acceptance-results.json` 记录验收观察；`scripts/acceptance-rules.json` 和既有验收矩阵定义门禁。代码写完、局部 PASS 或前置依赖未验证时不得标 `VERIFIED`。
- `scripts/tooling.py` 的 v2 `source_files()` / `source_hash()` 定义实际输入，按相对路径和原始字节计算，拒绝选中输入的符号链接；脚本、测试、CI 和测试部署配置变化也会使旧证据过期。
- 证据须绑定实际源码、工具版本、平台、命令/cwd、具体测试、自然退出码和日志摘要。保留失败、中断与后续尝试；不得改绑旧报告、静态复用运行证据或用不同源码的 PASS 补票。
- `docs/current-status.md` 由现有台账和证据派生，不手写通过计数或摘要。`current-status.py --check` 仅检查一致性；正式门禁由 `verify-release.py` 判定，`STALE`、`BLOCKED`、`PARTIAL`、`FAIL` 不能包装成通过。
- 发布前冻结全部 v2 输入并完成 AC001–071 的真实证据链；AC072 真实业务接入与回滚单独验收，不伪造完成。源码 ZIP、局部测试、本地矩阵或提交都不是稳定发布批准。
- CI 当前只验证、不发布。`release.py` 默认预检查也不创建标签或推送；只有明确发布授权、门禁满足、候选干净且显式指定完整 HEAD SHA 的 `--commit` 和 `--publish` 才能写入。
- 发布具体版本、目标 remote 与实际 push destination 按发布文档核对；不自动修改 Git 传输配置、跳过 hooks、覆盖已有标签或强推。发布窗口冻结 checkout、证据和 Git 配置，脚本检查不是跨进程原子锁。

## Git、恢复与交付

- 未明确授权时不暂存、提交、推送、打标签或部署；授权后只操作明确候选，不使用 `git add .`、`git add -A` 或 `git commit -a` 混入其他工作。
- `.codex/`、`.serena/` 等机器配置，以及 `.artifacts/`、`.tools/`、缓存、coverage 和临时材料不得混入 SDK 源码提交；保护已有配置和改动，不擅自 stash、reset、amend 或回滚用户工作。
- 提交/推送前核对暂存区、分支、upstream、实际推送目的地和全部待推送提交；操作后分别核验提交 SHA 与远端 ref，dry-run 或退出码不能替代远端确认。
- 单次执行清晰操作；超过 30 秒更新当前动作与最新结果。异常卡住先检查进程、状态和历史，再缩小范围、透传原生命令或拆分检查，不盲目重跑提交/推送。
- 定向或串行重跑通过不抹去首次失败；保留未解释的超时、环境问题和未运行范围。写入、hook 或网络操作部分失败时保留现场，不自动回滚或强推。
- 完成时检查最终差异、意外产物与未完成项，简要交付文件、实际验证、剩余风险和下一步；区分“实现完成”“本地验证”“正式门禁通过”“远端发布”，没有对应证据就不宣称完成。
