# 发布检查

维护者已确认仓库 `github.com/RockInMars/openbao-sdk-go` 和其现有 Apache-2.0 LICENSE；本轮已同步 module/import，未提交、推送、发布或部署，尚未验证远端分发本轮快照。不要用 tag/push 掩盖未通过门禁。

1. OB-001核对当前 Go 1.25.0 声明下限、固定 api/v2 v2.7.0 和已有 go.sum；主 Go 1.26.8 的通过不替代最低版本兼容验证。
2. 保持已确认 module 大小写与原 LICENSE；后续远端分发需要另行明确授权和可获取性验证。
3. 配置经过可信来源核验的服务端版本与镜像digest/二进制SHA256，运行正常、真实集成、独立消费者和固定扫描。
4. 冻结纳入 v2 指纹的全部输入后，全部 AC001–071 按 acceptance-rules.json 满足对应检查才更新台账并运行 make release-check。逐项校验当前标识、证据类别、具体测试名、命令/cwd、自然退出和日志摘要；运行证据不允许静态复用。OB 的关联 AC 或依赖未满足时不得 VERIFIED。
5. 经明确授权才发布第一个v0.x版本。v1.0仅在兼容契约稳定后使用；破坏性变更需要独立主版本和模块路径管理。

发布门禁不要求把AC072伪造成已完成；真实业务接入单独报告。源代码ZIP是当前实施快照，不是服务器部署包，包含失败与未执行证据也是合理交付，但不能作为release artifact对外标稳定。

私密安全报告渠道已由维护者确认为 [git@whereisit.cc](mailto:git@whereisit.cc)，使用方式见 [SECURITY.md](../SECURITY.md)。支持版本策略和响应时限仍待维护者确认，不能由报告邮箱的确定推导这些承诺。

CI 使用固定 commit 的 checkout/setup-go/upload/download，仅 contents:read、关闭凭据持久化，无发布步骤。新 upload-artifact v4.6.2 和 download-artifact v4.3.0 引用经官方 GitHub tag API 核验；失败仍上传结构化脱敏证据，summary 即使依赖失败也运行。它尚未在远端运行，缺真实集成输入时 job 失败。Linux/Windows 必须使用同字节源码（checkout 禁止 autocrlf 转换），跨平台逻辑单测不能代替两个平台正式运行。

主工具链四类正式报告、最低版本两类报告（分别要求 Linux/Windows）、逐目标 fuzz 和 CI 补充结果分开。benchmark 保留可复跑记录但不新增发布硬门槛。`PINNED` 只表示服务输入已固定；测试后不得改锁文件延用旧报告。最终结论必须区分实现完成、本地通过、门禁通过和已发布。

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
