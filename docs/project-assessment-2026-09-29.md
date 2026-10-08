# 项目评估与完善建议

评估日期：2026-09-29。工作区：`E:\xen\code\claude\project\jiupiao\openbao-sdk-go`；分支：`master`；提交：`6d8251658e76f88aec5935126f303b2c59497691`。

项目已具备边界清晰的 OpenBao 运行时 SDK、自有协议与安全处理、测试和接入文档。本轮原源码在正式官方依赖下的 Go 测试、race、vet、build、模块校验均通过；但本轮 Windows 覆盖率未达到仓库门槛，Python 工具测试仍有失败，真实服务、独立消费者及安全扫描未在本轮验证。建议保持现有分层，先完善验证与证据管理，再按实际业务接入需求扩展能力。

## 优先完善的事项

这里的 P1 表示正式交付前优先处理，P2 表示后续工程完善；它们是工作排序，不是漏洞严重度。

### 1. P1：补齐关键路径覆盖，并完成真实环境验证

本轮 race 测试退出码为 0，但使用仓库现有 `coverage_result` 评估得到 15 组中 5 组未达到 85%：

| 覆盖组 | 本轮语句覆盖率 | 仓库门槛 |
| --- | ---: | ---: |
| 根包 | 76.33% | 85% |
| `internal/engine` | 75.34% | 85% |
| KV 功能域 | 72.65% | 85% |
| PKI 功能域 | 78.66% | 85% |
| Transit 功能域 | 81.69% | 85% |

这是 Windows/Go 1.26.3 的实际采样结果，不能据此推断 Linux CI 的覆盖率。指标是 Go **语句覆盖率**；项目设计文档中的“行覆盖率”措辞也应统一，或另行定义可执行的行覆盖标准。门槛实现见 [tooling.py](../scripts/tooling.py#L94)，设计依据见 [总体设计](spec/01-总体设计.md#L443)。

优先从未覆盖语句较集中的 `pki_client.go`、`kv_metadata.go`、`internal/engine/errors.go`、`internal/engine/client.go` 入手，结合既有验收矩阵补正常、错误和边界场景。具体漏测分支应再根据 profile 定位，不能仅凭覆盖率认定实现有缺陷，也不应为达标降低门槛。

`deploy/test/server-lock.json` 仍为 `NOT_VERIFIED`，版本和摘要为空。应在明确的隔离测试环境中固定服务版本和可信摘要，执行现有真实集成、独立消费者及固定版本扫描流程，分别保存证据。客户端编译成功不能替代 OpenBao 服务端兼容性证明；已有契约测试和真实集成用例应继续复用。

### 2. P1：加强发布门禁对验收记录版本的检查

确认存在潜在误放行条件：[release_problems](../scripts/tooling.py#L62) 检查任务是否 `VERIFIED`、AC 是否 `PASS`，并检查四类报告的源码哈希，但没有核对任务台账和验收记录本身的版本归属。

只读合成检查中，全部标绿的旧哈希台账配上四份当前哈希、正确类别的通过报告，函数返回 `[]`。当前真实台账仍有阻塞项，**本轮没有发现实际发布误放行**。

建议校验台账及验收表的当前基线，并规定历史逐项证据如何经影响分析后重新确认有效。历史记录可以保留；复核记录需要明确适用版本、依据和未覆盖部分。补充“旧台账搭配新报告”“错误证据类别”“缺少逐项复核”等拒绝用例。现有测试只覆盖报告过期等情况，见 [test_tooling.py](../scripts/tests/test_tooling.py#L35)。

### 3. P1：同步当前状态，保留历史证据的时间边界

当前入口文档存在已确认的事实漂移：

| 当前文件说明 | 本轮核对事实 |
| --- | --- |
| README 第 7、26 行仍以 Go 1.23.2 描述当前环境 | 项目 `go.mod` 声明 Go 1.25.0；本机实际运行 Go 1.26.3 |
| README 第 7 行、兼容性文档第 8 行称 `go.sum` 为空 | `go.sum` 有 100 条记录，包含官方 api/v2 v2.7.0 的模块与 go.mod 校验和 |
| 兼容性页称未取得完整官方模块/go.mod | 已读取本机官方缓存中的 go.mod，其 Go 指令为 1.25.0；本轮正式依赖测试和构建通过 |
| 两份台账表达同一实施进度 | `task-status.json` 顶层源码哈希以 `9836c0` 开头；`acceptance-results.json` 顶层哈希以 `459e42` 开头，并不一致 |

建议更新 [README](../README.md#L7)、[兼容性页](compatibility.md#L3) 和当前交接入口，以机器验证报告提供当前事实，将 R1/R2/R3 的环境限制明确标为历史。不要删除旧失败记录，也不要因本轮 Go 检查通过就把所有验收项改为 PASS。本轮覆盖率、工具测试和未运行项仍须如实保留。

### 4. P2：修复 Windows 工具链兼容性，并稳定源码指纹

工具测试首次执行 35 项，出现 1 项 FAIL 和 1 项 ERROR。ERROR 来自 [test_tooling.py:52](../scripts/tests/test_tooling.py#L52) 按 Windows 默认 GBK 解码 UTF-8 Go 源码。启用 `PYTHONUTF8=1` 后，重跑结果为 34 项通过、1 项失败；剩余失败是 [test_dependency_bundle.py:64](../scripts/tests/test_dependency_bundle.py#L64) 把路径硬编码为 `/isolated/modcache`，而 Windows 实际输出为反斜杠路径。

建议在 Python 文本读写处显式指定 UTF-8，并用 `pathlib` 构造或比较平台路径，保留缓存隔离和身份清除断言的原有语义。

另外，[source_hash](../scripts/tooling.py#L17) 使用 `str(relative_path)`，同一批文件在 Windows/Linux 上会因路径分隔符而产生不同哈希；`source_files` 还纳入 `.codex/config.toml`。本轮对同一批 125 个文件计算原生路径和统一正斜杠路径哈希，结果不同。建议使用稳定的 POSIX 相对路径，并明确哪些运行、构建、验证配置属于版本化证据输入，排除纯本机代理配置；用固定 fixture 验证跨平台一致性。`.serena/project.yml` 不在现有后缀白名单内，本轮未计入该哈希。

### 5. P2：让 CI 的失败证据完整、可汇总

当前 [CI](../.github/workflows/ci.yaml#L23) 在同一 runtime job 顺序执行 normal、fuzz、consumer、security；前面的命令失败会使后续独立检查未运行。真实集成已有独立 job，这是良好基础。工作流尚未上传验证报告或汇总执行 `release-check`。

建议保留各检查的原始退出码，拆开可以独立执行的检查，在成功或失败时均上传脱敏报告、覆盖率及必要日志，并为预发布阶段增加汇总检查。汇总需绑定同一源码基线和已复核台账；当前 CI 失败仍然会报失败，此项是证据完整性改进，并非已发现“失败变绿”。

同时，[normal-test.py](../scripts/normal-test.py#L8) 的多条 subprocess 调用没有单项超时，且普通测试又为收集 JSON 结果重复执行一次。可统一用一次 JSON 运行收集普通测试结果，保留独立 race 检查，并为子进程设置合理超时及中断记录，确保异常时也能落下报告。

### 6. P2：完成可分发基线，再补接入便利性

模块路径仍为 `git.example.com/infra/openbao-sdk-go`，项目也明确披露它是占位地址。正式共享前，应确定实际托管路径，统一修改 module、导入和消费方引用，核对包在干净消费者中的可获取性。源码授权由项目权利人明确；本轮不代选许可证，也没有发布版本。

后续可补一个最小 `WithObserver` 示例，展示低基数指标、脱敏字段和快速返回的回调；现有 [观察接口文档](observability.md#L3) 已说明边界，示例可直接复用这些规则。全库未找到 `func Benchmark` 声明，可为 JSON 文档、证书校验、Transit 编码及执行器并发建立基准，记录环境和分配量，作为未来优化的依据。

## 对项目的理解

这是官方 API 客户端之上的有限运行时封装，主要面向需要访问秘密、签发证书和执行密码学操作的 Go 服务。

| 层次 | 主要职责 | 关键位置 |
| --- | --- | --- |
| 公开门面 | `New` / `Start` / `Close` / `State`；KV v2、PKI、Transit、Diagnostics | `client.go`、各 `*_client.go`、`config_types.go` |
| 认证 | 外部 Token、Managed AppRole、凭据提供器、登录/续期协调 | `auth/`、`internal/authn/` |
| 请求执行 | 总预算、并发限制、按操作分类重试、错误及副作用语义 | `internal/engine/` |
| 官方适配 | 使用 api/v2，显式关闭官方重试、重定向和隐式环境配置 | `official_sender.go` |
| 数据与校验 | 敏感字节、无损 JSON、PKI/Transit/PEM 校验 | `sensitive/`、`kv/`、`pki/`、相关 `internal/*util/` |
| 工程保障 | 单元、契约、fuzz、真实集成、独立消费者、扫描与发布检查 | `tests/`、`scripts/`、`deploy/test/`、`.github/` |

本轮统计为 98 个 Go 文件，其中 55 个源码文件、43 个测试文件，合计约 1.05 万行；数量不代表测试覆盖充分。现有设计值得保持的部分包括：安全读取才进入重试循环、写入响应丢失保留 UNKNOWN、敏感值显式揭示、集群健康与受限身份就绪分开，以及管理能力与运行时能力分界。

官方适配器的显式配置与 api/v2 v2.7.0 的构造约定相符：上游 `NewConfig` 默认禁用隐式环境读取，项目还显式设置 `MaxRetries=0` 与 `DisableRedirects=true`。依据：[官方 client.go](https://raw.githubusercontent.com/openbao/openbao/api/v2.7.0/api/client.go)、[本地适配器](../official_sender.go#L14)。此静态核对不替代真实服务兼容测试。

## 本轮验证记录

运行环境：Windows，Go 1.26.3，Python 3.13.13，GCC 16.1.0，`CGO_ENABLED=1`。Go 检查使用 `GOWORK=off`、空 `GOFLAGS`、`GOTOOLCHAIN=local`、`GOPROXY=off`，使用已有缓存，没有安装依赖。适用的 Go 命令显式使用 `-mod=readonly`。

| 实际检查 | 退出码／结果 | 证明范围 |
| --- | --- | --- |
| `go mod verify` | 0 / PASS | 现有缓存模块校验 |
| `go test -mod=readonly -count=1 -timeout=120s ./...` | 0 / PASS | 24 个包，其中 16 个包实际有测试；不含 integration 构建标签 |
| `go vet -mod=readonly ./...` | 0 / PASS | 当前平台的静态检查 |
| `go build -mod=readonly ./...` | 0 / PASS | 当前模块内的正常构建，含示例包 |
| `go test -mod=readonly -race -count=1 -timeout=120s -coverprofile=E:\Temp\openbao-sdk-go-race-e4303529d8504fa9a4225fb4f85c1910.out ./...` | 0 / PASS | 当前平台测试中的竞争检测与覆盖率采样 |
| 对上述 profile 调用项目 `tooling.coverage_result` | FAIL | 15 组中 5 组未达到 85%；该函数返回结果对象，不是独立 shell 检查退出码 |
| `python -B -m unittest discover -s scripts/tests -v` | 1 / FAIL | 首次：35 项，1 FAIL、1 ERROR |
| 设置 `PYTHONUTF8=1` 后运行同一 Python 命令 | 1 / FAIL | 35 项，34 通过、1 FAIL；保留首次失败，不将两轮数量相加 |
| 同文件集原生路径/POSIX 路径哈希比较 | 不同 | 指纹的跨平台不稳定性 |
| 旧台账＋当前通过报告的合成门禁调用 | 返回 `[]` | 门禁未拒绝旧台账的潜在条件；未改真实台账 |

本轮没有运行 `make normal-test` 包装器、5 项计时 fuzz、真实 OpenBao 集成、独立 go.mod 消费者下载、扫描器或完整发布门禁；没有 Linux CI 执行记录。普通测试运行不能替代计时 fuzz。Go 单项检查通过也不等于包含覆盖率在内的完整 normal 门禁通过。

覆盖率原始产物保存在上述系统临时路径，可能被系统清理。本报告记录了命令、基线及主要结果；正式交付应由 CI 保存同一基线下可追溯的脱敏原始证据。

## 建议推进顺序与范围说明

1. 同步当前基线说明，修复 UTF-8、平台路径和源码指纹问题，补门禁版本校验。
2. 针对覆盖率薄弱路径补充有意义的测试，在计划支持的平台验证；复用现有 85% 门槛和验收矩阵。
3. 固定真实 OpenBao 测试基线，完成集成、独立消费者、扫描和计时 fuzz；将证据接入 CI 汇总。
4. 逐项复核台账，确定实际模块地址、授权与试用版本，再接入一个只读消费者推进试用。

本轮仅新增这份评估文档，未修改业务源码、测试、公开契约或历史验收台账，未提交、推送或发布。用户原有 `.codex/config.toml` 修改保留；Serena 激活后观察到的 `.serena/` 本地元数据仍为未跟踪状态。

主代理核对了核心执行、认证、官方适配器和验证脚本；只读定位代理梳理了公开 API 与文档；验证代理执行了上述检查；独立审查代理复核了门禁、哈希和 CI 建议。独立审查范围不覆盖全部运行时代码，不能当作完整安全审计。code-review-graph 当前没有索引数据，本轮使用 Serena 的 Go 符号定位与有界搜索完成分析。
