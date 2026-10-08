# 发布检查

维护者已确认仓库 `github.com/RockInMars/openbao-sdk-go` 和其现有 Apache-2.0 LICENSE。最新验证状态以[当前状态页](current-status.md)为准；本地提交不代表发布批准，也不代表远端分发已验证。不要用 tag/push 掩盖未通过门禁。

1. OB-001核对当前 Go 1.25.0 声明下限、固定 api/v2 v2.7.0 和已有 go.sum；主 Go 1.26.8 的通过不替代最低版本兼容验证。
2. 保持已确认 module 大小写与原 LICENSE；后续远端分发需要另行明确授权和可获取性验证。
3. 配置经过可信来源核验的服务端版本与镜像digest/二进制SHA256，运行正常、真实集成、独立消费者和固定扫描。
4. 冻结纳入 v2 指纹的全部输入后，全部 AC001–071 按 acceptance-rules.json 满足对应检查才更新台账并运行 make release-check。逐项校验当前标识、证据类别、具体测试名、命令/cwd、自然退出和日志摘要；运行证据不允许静态复用。OB 的关联 AC 或依赖未满足时不得 VERIFIED。
5. 经明确授权才发布第一个v0.x版本。v1.0仅在兼容契约稳定后使用；破坏性变更需要独立主版本和模块路径管理。

发布门禁不要求把AC072伪造成已完成；真实业务接入单独报告。源代码ZIP是当前实施快照，不是服务器部署包，包含失败与未执行证据也是合理交付，但不能作为release artifact对外标稳定。

私密安全报告渠道已由维护者确认为 [git@whereisit.cc](mailto:git@whereisit.cc)，使用方式见 [SECURITY.md](../SECURITY.md)。支持版本策略和响应时限仍待维护者确认，不能由报告邮箱的确定推导这些承诺。

CI 使用固定 commit 的 checkout/setup-go/upload/download，仅 contents:read、关闭凭据持久化，无发布步骤。新 upload-artifact v4.6.2 和 download-artifact v4.3.0 引用经官方 GitHub tag API 核验；失败仍上传结构化脱敏证据，summary 即使依赖失败也运行。它尚未在远端运行，缺真实集成输入时 job 失败。Linux/Windows 必须使用同字节源码（checkout 禁止 autocrlf 转换），跨平台逻辑单测不能代替两个平台正式运行。

主工具链四类正式报告、最低版本两类报告（分别要求 Linux/Windows）、逐目标 fuzz 和 CI 补充结果分开。benchmark 保留可复跑记录但不新增发布硬门槛。`PINNED` 只表示服务输入已固定；测试后不得改锁文件延用旧报告。最终结论必须区分实现完成、本地通过、门禁通过和已发布。

## 标签发布脚本

`scripts/release.py` 只使用 Python 标准库和 Git，默认预检查，不暂存、提交、创建标签或推送。先冻结并提交候选，再按现有验证入口为相同 v2 指纹补齐真实证据；新增脚本或测试也会改变指纹，旧报告不得改绑为新快照的 PASS。

预检查示例（版本号由维护者选择，不会自动递增）：

```text
rtk proxy python -B scripts/release.py --version v0.1.0
```

脚本复用 `verify-release.py` 的 `release_problems` 门禁，读取当前台账及 `.artifacts/*-report.json`。缺失、过期、失败或无法读取的证据均拒绝发布，没有跳过门禁选项。预检查通过仅表示候选满足检查，不授予发布批准；AC-072 业务迁移仍是独立门禁。

同时必须满足以下条件：

- 在 SDK 仓库根目录对应的 checkout 中运行，暂存区为空，SDK 源码、测试、工具与文档没有未提交或未跟踪改动。工具定义的机器配置、缓存和产物排除目录可保留本地文件，但候选提交树中不得包含任何排除目录的路径；脚本在创建标签前拒绝这些已提交文件。原先已跟踪的机器文件须经明确范围核对后仅取消跟踪，保留本地原文件和改动，不能只依赖忽略规则或跳过暂存。
- 候选提交等于 `HEAD`，使用完整 checkout，不允许 SDK 路径上的 `skip-worktree` / `assume-unchanged` 隐藏改动。提交与文件系统的 v2 输入集合必须一致，所有输入的原始字节与提交中的 blob 一致；脚本所有 Git 操作禁用 replacement objects，避免用替代树验证原提交。Git 把 CRLF 规范化为 LF 后显示 clean，并不能替代同字节验证；发现不一致时使用同字节 checkout 重建证据，不自动转换源码或复用旧报告。
- module 仍为 `github.com/RockInMars/openbao-sdk-go`。版本必须是规范的 `v0.x.y` 或 `v1.x.y`，可带 `-rc.1` 等预发布标识，不接受前导零、build metadata 或需要另行管理模块路径的 v2 标签。v1 的稳定兼容承诺仍由维护者决定。
- `--remote` 默认为 `origin`，只接受已配置的 remote 名称；其 push destination 必须唯一，且不能被 Git 再解析为任一已配置 remote 名称。即使 fetch URL 不同，冲突检查和发布后核验也使用实际 push destination。有任何 `url.*.pushInsteadOf` 定向重写配置时拒绝发布，以免读写落到不同仓库；脚本不自动关闭或修改这些配置。
- 本地与目标远端均不存在同名标签。远端不可访问时也拒绝；不会覆盖、删除或自动恢复任何已有标签。

经明确授权后，使用已核对的完整提交 SHA 发布。PowerShell 示例：

```powershell
$releaseSha = (rtk proxy git rev-parse HEAD).Trim()
rtk proxy python -B scripts/release.py --version v0.1.0 --commit $releaseSha --publish
```

`--publish` 必须显式提供 `--commit`，不接受缩写或分支名。脚本在网络检查后和写操作前再次核对候选与证据，创建绑定该 SHA 的 annotated tag，然后仅推送固定 tag object SHA 到单一 `refs/tags/<version>`。它不推送分支或其他标签，不递归推送 submodule，不使用 force、不跳过 hooks，也不改 Git 身份、remote 或 hooks 配置。默认每次 Git 操作超时 300 秒，可用 `--timeout 600` 调整（范围 1–3600）。认证应提前配置；脚本不交互索取凭据，不输出 remote URL 或可能含凭据的 Git 原始错误。

发布窗口内应冻结 checkout、证据及 Git/传输配置，不与其他配置或标签操作并行。上述分离检查不是跨进程原子锁；外部并发改写配置仍可能改变传输语义，不能把脚本当作对不可信 Git 配置的隔离器。

成功条件是从同一 push destination 重新读取并确认 tag object 和 peeled commit 均匹配，而非仅依据 `git push` 退出码。退出 0 的 `DRY-RUN: PASS` 没有任何 tag/push；退出 0 的 `RELEASE: PUBLISHED` 表示远端标签已核验，**不表示 Go 公共代理已可获取、GitHub Release 已创建或业务迁移已完成**。

写操作开始后的失败、超时、中断或本地/远端标签核验不一致返回退出 1 和 `RELEASE: PARTIAL`，保留本地/远端已有状态，不自动回滚或重试。此时先检查本地标签、真实 push destination 和运行进程；不要重复发布、删改可能已分发的版本或强推。写操作前的拒绝返回 `RELEASE: BLOCKED`；参数解析错误返回退出 2。

公共分发还需确认仓库公开可读，并在独立消费者环境核验下载（不使用 `replace`）：

```text
rtk proxy go list -m github.com/RockInMars/openbao-sdk-go@v0.1.0
rtk proxy go get github.com/RockInMars/openbao-sdk-go@v0.1.0
```

私有模块应使用消费方既有的私有模块代理与认证配置。脚本不会替你修改 `GOPROXY`、`GOPRIVATE` 或全局 Go 环境；发布后修复使用新版本，不重写已发布标签。

## 2026-10-01 续作验证与复现

维护者后续回复“继续”，授权了本轮工作区隔离工具、官方公开发布物/模块代理/校验数据库及漏洞库访问和非生产验证。工具不安装到全局目录，共享服务和生产环境不作修改；远端 GitHub CI、分发、发布和旧远端服务预算不因此获得授权。实际结果、首次失败与清理记录见[实施交接](implementation-handoff.md)和[本轮证据](evidence/OB-018/release-validation-2026-10-01/tool-provenance.json)。

主工具链因 Go 1.26.3 的真实标准库漏洞结果更新为 **Go 1.26.8**；正式入口、收据校验与 CI 同步固定该版本，Go 1.25.0 兼容下限和 api/v2 v2.7.0 不变。Windows/Linux 两个版本的官方归档均按 Go 发布清单核验 SHA256。OpenBao **2.6.3** 归档经官方公开密钥身份、签名和校验和核验，再单独计算解包二进制摘要，`server-lock.json` 已固定为 binary 模式。锁定输入不自动代表集成通过。

复跑时在自己的隔离目录准备并核验同版本工具；将所需 Go 的 `bin` 放到本进程 PATH，保留 `GOTOOLCHAIN=local`、`GOENV=off`、`GOWORK=off` 和现有 CGO/race 条件。根模块离线缓存需先通过已认证代理执行 `go mod download all`；然后分别运行 normal、minimum-go normal/consumer、完整 tooling、fuzz、独立消费者和固定扫描入口。真实 fixture 设置 `BAO_TEST_VERSION=2.6.3`、`BAO_TEST_BINARY=<已核验绝对路径>`、`BAO_TEST_BINARY_SHA256=<server-lock.json 中的值>`，运行 `python -B scripts/integration-test.py`；不传外部地址或 Token。

每个 job 完成后立即用 `ci-summary.py --package-report .artifacts/<name>-report.json --output <独立 job 目录>` 导出，避免另一平台覆盖同名 alias。12 个 job 的本地汇总与 `verify-release.py` 分别执行；本地矩阵 PASS 不等于远端 GitHub Actions 已运行。CI 集成准备步骤已同步 binary 锁：从官方固定 URL 下载 2.6.3 归档，核对本轮验签确认的归档 SHA256，只解出 `bao`，再核对锁文件中的二进制 SHA256，最后导出绝对二进制路径与版本。任一步失败都不能开始真实测试；本轮不触发托管 CI。若改为镜像锁，必须先核验 digest、变更新源码基线并重建相关证据。安全支持版本和响应时限仍待维护者决定。

## 2026-10-01 验证环境准备方案（尚未执行）

以下是续作授权之前的历史方案，“尚未执行”和环境阻塞仅描述当时检查点。当前授权、输入和执行结果以上节及实施交接为准。

本机已核对 Go 1.26.3；没有 Go 1.25.0。Docker 命令存在，当前 context 为本地 socket，但服务端版本探测退出 1，没有可用 daemon。未启动服务。`govulncheck` 和 `gitleaks` 不在 PATH，只有前者的固定模块缓存；这不能证明扫描环境和数据库已备齐。下列准备需要本次单独授权，不能沿用旧远端运行额度。

维护者已选择保留最低 Go 和扫描环境的阻塞状态，不扩大本轮下载、安装或联网扫描授权；下列方案保留供后续另行授权时使用。

1. **Windows 最低工具链**：从 [Go 官方下载清单](https://go.dev/dl/#go1.25.0) 获取 `go1.25.0.windows-amd64.zip`（约 64 MB），先核对官方 SHA256 `89efb4f9b30812eee083cc1770fdd2913c14d301064f6454851428f9707d190b`，再解压到本工作区 `.artifacts/tools/go1.25.0/`。只在测试子进程调整 PATH/GOROOT，继续 `GOTOOLCHAIN=local`，不更改全局安装。用已准备的本地签名 sumdb 代理运行 normal/consumer 的 `--profile minimum-go`；Linux 由独立 CI 作业或另行授权的 Linux 环境提供证据。
2. **正式 OpenBao fixture**：准备候选固定版本 **2.6.3**。这是待验证输入选择，不改变 SDK 的 `api/v2 v2.7.0` 依赖或声明跨版本认证。候选发布物为 [官方 Linux amd64 tar.gz](https://github.com/openbao/openbao/releases/download/v2.6.3/openbao_2.6.3_linux_amd64.tar.gz)；[官方发布资产](https://github.com/openbao/openbao/releases/expanded_assets/v2.6.3)列出的归档 SHA256 为 `c6463ddd4fdc4214b62a7ffdeaa0fc6df170f7e6b75c6dea2c5e525bdc932ed3`。本轮仅查询元数据，**未下载或验证签名**。按照[官方安装验证说明](https://openbao.org/docs/install/#signature-verification)，核验同一发布的 `checksums.txt` 及签名与受信任身份，再核对归档；解包后另算可执行文件的 SHA256。归档摘要不能填进 `binary_sha256`。确认可信的 Linux 执行环境后，才可固定 `server-lock.json` 并运行 `scripts/integration-test.py`。若改用容器，另核对实际镜像 manifest digest 并以 `@sha256:` 固定，不能把上述归档摘要当镜像 digest；服务启停和拉取须有授权。
3. **固定扫描**：现有入口 `scripts/security-test.py --allow-network` 会下载/执行 `golang.org/x/vuln/cmd/govulncheck@v1.1.4` 和 `github.com/zricethezav/gitleaks/v8@v8.24.3`。govulncheck 显式使用 `-db=https://vuln.go.dev -show=version ./...`；收据核对工具版本、官方数据库地址和更新时间，CI 只导出这些受限字段。固定模块代理/公开校验数据库，清除禁止校验的环境覆盖。[官方 v1.1.4 参数定义](https://github.com/golang/vuln/blob/v1.1.4/internal/scan/flags.go#L34-L45)及[版本信息输出](https://github.com/golang/vuln/blob/v1.1.4/internal/scan/text.go#L85-L113)是核对依据。需要明确允许公共依赖与数据库下载以及缓存写入；在本地分析源码，漏洞库请求包含模块信息，见[官方隐私说明](https://vuln.go.dev/privacy.html)。当前未建立等价的离线扫描证据，不能用空数据库或任意同名可执行文件代替。

准备后重新冻结 v2 源码摘要，保留版本、来源核验、日志和退出码，运行相应检查与 `scripts/verify-release.py`。未获环境准备授权时，交付上述方案和 BLOCKED 收据，并继续其他已授权的本地工作。
