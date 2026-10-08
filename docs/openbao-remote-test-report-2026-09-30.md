# 远端测试入口实施与验证结果（2026-09-30）

## 最新检查点：当前源码指纹的 sdk-test 全场景复测通过

本轮在 `master` / HEAD `6d8251658e76f88aec5935126f303b2c59497691` 的保留修改工作树上执行；source_identity v2 为 **`e9d371d3a35385803a651db667a05cbd8fb3bb6cd36b03b23bf24ffd6139f14e`**。用户确认仅针对 `https://kms.jiup9.com:443` 的非生产 `sdk-test`，最多 120 次串行 API 请求、20 分钟、创建并回收最多 9 项本轮专用资源。AppRole mount 为已确认的 `auth/approle`；RoleID/SecretID 从受控进程环境读取，未放入 argv、源码或报告。为一次性新前缀 `sdk-codex-20260930-68edb3eb` 与 run ID `remote-sdk-full-20260930-07` 定点更新 Python/Go 全场景范围白名单；原 `remote-sdk-full-20260930-06` 不再被当前白名单接受。执行入口为 `rtk proxy python -B .artifacts/runs/remote-full-fixture-20260930-08/fixture.py --execute --max-total-requests 120`。

[第八轮不可变夹具报告](../.artifacts/runs/remote-full-fixture-20260930-08/report.json)为 **PASS**：112/120 次精确计数，Core [39/39 场景、40 次请求](../.artifacts/runs/remote-sdk-full-20260930-07-core/remote/result.json)，Transit [22/22 场景、27 次请求](../.artifacts/runs/remote-sdk-full-20260930-07-transit/remote/result.json)。两个 SDK 子进程及其内部 Go 命令自然退出 0，报告、结果和日志 SHA256 复核匹配，源码指纹前后一致。9 项专用资源全部记为 `removed`，清理状态 PASS，无待回收项。本轮持久化的 260 个非源码 JSON/日志文件经定向扫描，未发现注入的 RoleID/SecretID 原文。默认 TLS 信任链与主机名校验下健康探测通过；专用测试 CA 的 CSR 签发、校验、证书签发/读取、颁发链读取与撤销场景均通过。服务自报版本为 2.6.2，不能代替可信发布物 digest。

这仅证明指定服务、身份及本轮专用资源上的补充 SDK 场景；没有对生产、其他 namespace、既有业务资源或服务器所有证书做全量检查。[独立只读审查](evidence/OB-018/remote-retest-2026-09-30/independent-review.md)未发现报告与 PASS 矛盾之处，但没有在清理后另发请求独立查询资源。历史第七轮报告保留为旧指纹证据；本轮当前证据与 SHA256 见[索引](evidence/OB-018/remote-retest-2026-09-30/index.json)。

同一指纹的本地检查：`go test -mod=readonly -count=1 ./...` 自然退出 0，远端测试包 24 个顶层测试通过且无 skip，Python 工具测试 118 项通过。[normal](../.artifacts/runs/2026-09-30T155913.905445Z-66c28dbd51fa4db6829e4660abe5e089/normal/report-007.json)六命令 PASS、349 个测试/子测试结果，15 组语句覆盖率均至少 85%，最低 86.49%；[五目标 fuzz](../.artifacts/runs/2026-09-30T161043.539990Z-012707c728174601ac6ba00ac252326b/fuzz/report-006.json)各实际运行 30 秒并 PASS；[离线消费者](../.artifacts/runs/2026-09-30T160830.023273Z-b181217a1e4d493381f626701e10c1fb/consumer/report-007.json) reader/signer PASS；[benchmark](../.artifacts/runs/2026-09-30T160721.774300Z-4bd2411f551c46a18f093740d31adcd9/benchmark/report-002.json)六个子项各三轮 PASS。消费者[首次运行](../.artifacts/runs/2026-09-30T160517.126193Z-7f176a108aa548a78c31ff0c04b06e52/consumer/report-007.json)虽四条 Go 命令均退出 0，但隔离 `TEMP` 设在工作区内，被证据校验拒绝为 FAIL；移至既有工作区外 `E:\Temp` 后重新实际运行并通过，首次失败保留。

[正式独立 fixture 集成](../.artifacts/runs/2026-09-30T155544.732585Z-ab47c9c6b1db4853ac0727bfbf9737cb/integration/report-001.json)仍缺可信精确版本与 digest/二进制 SHA256，为 BLOCKED；[固定扫描](../.artifacts/runs/2026-09-30T155558.335355Z-3d22cc4dc5d34cb0bb2aa5bc8ad58136/security/report-001.json)缺已获准的工具/数据库准备，为 BLOCKED。Linux、最低 Go 1.25.0 和远端 CI 未运行。远端补充结果不记作 `REAL_OPENBAO`；原 72 项 AC 仍为 66 PASS / 3 PARTIAL / 3 NOT_RUN，19 个 OB 均 BLOCKED。[发布门禁收据](../.artifacts/runs/remote-retest-20260930-final/release-remote-retest-01.json)记录实际退出 1 和 25 项拒绝：两类正式报告、五个 AC、OB-001～018；未出现当前指纹失配拒绝。未暂存、提交、推送或发布。

## 先前第七轮检查点（历史）

当时工作区为 `master` / `6d8251658e76f88aec5935126f303b2c59497691` 加未提交修改；source_identity v2 为 `dee54e4f2f49c24f71e8cd412064eac0286b2ba3f415c1850de05e82ef1d093b`，173 个输入。该轮获授权最多 120 次串行 API 请求、20 分钟，仅限 `https://kms.jiup9.com:443` 的非生产 `sdk-test`，通过已受控注入的 AppRole 凭据执行。凭据未写入命令参数、源码或证据。实际入口为 `rtk proxy python -B .artifacts/runs/remote-full-fixture-20260930-07/fixture.py --execute --max-total-requests 120`。

[不可变夹具报告](../.artifacts/runs/remote-full-fixture-20260930-07/report.json)为 **PASS**：精确计数 112/120 次，Core [39/39 场景、40 次请求](../.artifacts/runs/remote-sdk-full-20260930-06-core/remote/result.json)与 Transit [22/22 场景、27 次请求](../.artifacts/runs/remote-sdk-full-20260930-06-transit/remote/result.json)均通过；两个 SDK 子进程自然退出 0，报告/日志 SHA256 收据复核匹配，前后源码指纹一致。9 项本轮专用 mount、策略、令牌等资源全部记为 `removed`，清理状态 PASS，无待回收项。服务自报版本为 2.6.2，这不是可信发布物 digest。HMAC 专用测试 key 的元数据结构探测只记录字段类型与匹配布尔，实际 `keys` 字段缺失；在此形状下 `transit_hmac_key`、`transit_hmac`、`transit_hmac_verify` 均通过。持久化的 257 个本轮 JSON/日志文件经定向扫描，未发现注入的 RoleID/SecretID 或原始凭据字段。此前[第六轮失败](../.artifacts/runs/remote-full-fixture-20260930-06/report.json)仍保留：107 次请求，HMAC 元数据解析报 `invalid_response`，9 项资源回收 PASS；其后的两份定向诊断未读到元数据，也保留为失败记录。

本轮只证明固定测试身份、专用资源和目标服务上的这些 SDK 场景。正常 `go test ./...` 没有远端连接；故障注入、响应丢失、UNKNOWN/不重放等由本地测试验证，未对远端做破坏性实验。KV List 的线协议已按用户明确批准改为固定 `GET ?list=true` 并同步契约；HMAC 专用 key 缺失或空 `keys` 的保守预检已同步实现/契约，公开 Go API 未改变。独立只读审查先发现新夹具 run_id 与三层预算不一致，修复后复核无阻断；[审查记录](evidence/OB-018/remote-2026-09-30/independent-review.md)另列证据边界。

同一指纹的本地正式证据为 [normal](../.artifacts/runs/2026-09-30T040616.117162Z-7582862938784f7fa40424b5bc4900dc/normal/report-007.json) PASS（349 项测试结果，新 race profile 的 15 组 Go 语句覆盖率均至少 85%）、[五目标 fuzz](../.artifacts/runs/2026-09-30T040730.684857Z-3544922cb3c047ffb6c245aa6339a1c4/fuzz/report-006.json) PASS、[两个离线消费者](../.artifacts/runs/2026-09-30T041038.162482Z-6279a23c28194e25ad63d636533e27d8/consumer/report-007.json) PASS、[benchmark](../.artifacts/runs/2026-09-30T041120.578170Z-b3f2d9e30a3e4d85a6d157c8235d7fc1/benchmark/report-002.json) PASS（六个启用子项各三轮）。[工具收据](../.artifacts/runs/remote-20260930-current/tooling-tooling-20260930-03.json)覆盖 118 项 Python 工具测试；示例测试/构建及 remote tag 仅编译的当前收据位于 [本轮本地产物目录](../.artifacts/runs/remote-20260930-current/)。[正式独立 fixture 集成](../.artifacts/runs/2026-09-30T041201.992465Z-fcc2d88df3ea479c8dc1ae4c486cc9b3/integration/report-001.json)因缺精确版本与可信 binary/digest 为 BLOCKED；[固定扫描器](../.artifacts/runs/2026-09-30T041213.812598Z-d275ea4aa02b4639b8638d8e5876059d/security/report-001.json)因无已准备的工具/数据库与下载授权为 BLOCKED。Linux、最低 Go 1.25.0 与远端 CI 未运行。当前 `rtk proxy python -B scripts/verify-release.py` 自然退出 1，[不可变门禁收据](../.artifacts/runs/remote-20260930-current/verify-release-release-20260930-03.json)及[日志](../.artifacts/runs/remote-20260930-current/verify-release-release-20260930-03.log)列出 25 项拒绝：integration、scans、AC-001/065/066/070/071，以及 OB-001 至 OB-018；没有当前指纹失配或缺正式目标的新问题。上述远端补充结果不记为 `REAL_OPENBAO`，也不使原 OB/AC 或发布门禁自动通过；未暂存、提交、推送或发布。

以下为此前检查点与入口实现的历史记录，旧指纹及其中的“当前/最终”仅指写入当时。

## 先前续跑检查点（历史）

当前工作树的 source_identity v2 为 `376c3dbba0d9340762216bd161170f435eb3834daeb76342aa6eb99b6496a85d`，仍在 `master` / `6d8251658e76f88aec5935126f303b2c59497691` 的未提交工作区。下文原有 `32dfe9e8...` 及相应正式报告属于入口实现时的历史检查点，不能代表当前基线。当前 `rtk proxy go test -mod=readonly -count=1 ./tests/remote ./internal/authn .` 三个包通过；`rtk proxy python -B -m unittest discover -s scripts/tests -v` 118 项通过。当前 `rtk proxy python -B scripts/verify-release.py` 返回 1，首先拒绝旧台账、旧逐项 AC 和旧正式报告的指纹归属；外部真实集成、扫描与 Linux 等门禁仍未满足。

用户授权仅在非生产 `sdk-test` 内创建并回收本轮专用夹具，AppRole mount 固定为 `auth/approle`；凭据从受控 Windows 用户环境定向注入子进程，未写进命令、报告或仓库。两次全场景夹具续跑各使用 53 次请求，均在 SDK Core 的 KV 嵌套目录 List 处停止；[第一次续跑报告](../.artifacts/runs/remote-full-fixture-20260930-02/report.json)和[第二次续跑报告](../.artifacts/runs/remote-full-fixture-20260930-03/report.json)各自记录 9 项专用资源及登录令牌全部回收，不能据此宣称全功能通过。第二轮预算中的[斜杠诊断](../.artifacts/runs/remote-list-diagnostic-20260930-01/report.json)使用 17 次请求、[方法诊断](../.artifacts/runs/remote-list-method-diagnostic-20260930-01/report.json)使用 18 次请求，各自 4 项资源回收成功；加上第二次续跑的 53 次，该轮共 88/120 次。预算时间窗口结束后不得沿用剩余次数。

两轮诊断复用固定测试前缀，但各自创建并回收了独立的 KV mount/seed：前轮带末尾斜杠的 HTTP `LIST` 对管理员和受限身份均返回 404；后轮同样带斜杠的 `GET ?list=true` 对两种身份均返回 200 并列出本轮种子键。后轮的无斜杠 `LIST` 亦为 404。由于这不是同一资源、同一会话内的严格 A/B，结果强烈提示目标入口对 `LIST` 方法不兼容，尚不能唯一归因。目录末尾斜杠不是已证实的根因，已撤销相应的试探性 SDK 改动。既有[接口契约](spec/02-接口契约.md)明确要求 `LIST` 并禁止静默换为 GET，当前等待明确契约决定；在此之前保持原线协议，将该服务的 KV List 与后续全场景结果记为未通过。上述定向诊断属于远端补充证据，不是正式独立集成或发布门禁证据。尚无未回收的本轮资源记录；下次续跑仍须核对现场。

同一当前指纹的正式本地证据为：[normal 报告](../.artifacts/runs/2026-09-30T013854.912393Z-32c236d5ceac49cdb190e4ee43814ce7/normal/report-007.json) PASS（334 个测试结果，race 新 profile 的 15 组 Go 语句覆盖率均不低于 85%）、[fuzz 报告](../.artifacts/runs/2026-09-30T014050.352673Z-e62f8c3634494bcebf941458a9fe8f9f/fuzz/report-006.json) PASS（五目标各自执行）、[离线消费者报告](../.artifacts/runs/2026-09-30T014446.303984Z-dd1380f6567e4b62905707d8afe48685/consumer/report-007.json) PASS（两个独立消费者）、[benchmark 报告](../.artifacts/runs/2026-09-30T014522.743270Z-50dcd385f6124a279a062aa011e72c99/benchmark/report-002.json) PASS。消费者首次未提供 `--offline-proxy` 的[BLOCKED 报告](../.artifacts/runs/2026-09-30T014348.048784Z-3f73ca5346004d98a1bcb1527696ee14/consumer/report-001.json)保留，随后显式使用已有本地代理重跑。正式[独立 fixture 集成](../.artifacts/runs/2026-09-30T014617.193750Z-51e5c9a0e14c472c8e25b7572ea7ac88/integration/report-001.json)因缺精确版本与可信 binary/digest 为 BLOCKED；[固定扫描器](../.artifacts/runs/2026-09-30T014630.689878Z-5384c8212dd04c59a4b49c71568385b6/security/report-001.json)因无准备/下载授权为 BLOCKED。benchmark 不是发布硬门槛。上述证据若随后修改指纹输入即转为历史。

独立只读 `risk_reviewer` 对诊断证据复核后指出：前轮和后轮并非严格的同资源/同会话方法 A/B，故不能唯一归因到代理或服务哪一层；已据此修正本报告和交接措辞。其余确认包括两轮请求计数准确、清理均通过、常规 KV 读取在失败前通过，且既有契约禁止静默切换。若后续明确批准线协议调整，应使用已有成功证据对应的非空前缀尾 `/` 加固定 `GET ?list=true`，并核对根/嵌套路径、权限、404 和无写入重放；当前没有实施此变更。审查是静态判断，未替代远端复测。

以下内容保留入口实现时的历史记录，出现“当前”或“最终”字样时仅指当时的检查点。

**本地入口实现与回归完成；指定服务的 TLS/健康连通性及新凭据的 `sdk-test` 身份自查已通过，SDK 业务场景仍阻塞，正式交付门禁未通过；未发布。**

已经收到测试账号全场景授权和 namespace `sdk-test`。先前独立入口预检未发出请求；随后按用户两次连接要求，对 `https://kms.jiup9.com:443` 分别完成三次只读 HTTPS 探测：[首次凭据报告](../.artifacts/runs/remote-connect-20260930-01/report.json)的身份自查为 403，[新凭据报告](../.artifacts/runs/remote-connect-20260930-02/report.json)为 200 且有限身份字段校验通过。凭据由交互式遮蔽输入提供，未写入 argv、URL、源码、配置或报告，也未注入正式 runner；交互输入仍属于工具调用，不能宣称它未经过工具。没有创建远端资源。环境性质、KV 测试资源和独占回收范围仍未确认；没有把“测试账号”推断为服务器一定非生产，也没有枚举业务目录寻找资源。

## 1. 基线与实际改动

工作目录为 `E:\xen\code\claude\project\jiupiao\openbao-sdk-go`。分支仍为 `master`，HEAD 仍为 `6d8251658e76f88aec5935126f303b2c59497691`。实际比较基线是本轮开始时的未提交工作树快照，而非单独的 HEAD：

- [初始快照](../.artifacts/runs/remote-20260930-start/baseline.json)：v2 `771ac8fe1964aaebc7dd258d1060a34bbb8781e823ea86b182f8a465d9f8be0c`，163 个输入。
- 最终 source_identity v2：`32dfe9e8dbfa041033bb2ae2ce10488af173b9fceb4fda8388f7af450764471e`，172 个输入。所有当前正式报告的前后标识与此一致。
- [证据索引](evidence/OB-018/remote-2026-09-30/index.json)记录报告、日志、profile 及两次连接补充报告的实际 SHA256；[工作区与进程审计](evidence/OB-018/remote-2026-09-30/workspace-audit.json)记录入口实现时的差异范围、Git 和自有 PID 核对；[入口实现时的复核](evidence/OB-018/remote-2026-09-30/final-check.json)保留当时零远端请求的历史检查点，不能代表随后两次连接探测。

本轮新增 [remote-test.py](../scripts/remote-test.py)、[Python 回归](../scripts/tests/test_remote_runner.py) 和 [tests/remote](../tests/remote/)；仅局部扩展 [command_runner.py](../scripts/command_runner.py) 与 [reporting.py](../scripts/reporting.py)，提供默认关闭的输出抑制选项。原有 SDK Go 源码、go.mod/go.sum、LICENSE 与开始快照逐字节一致。原有 `.codex/config.toml` 修改未被改动。

| 执行单元 | 结果与范围 |
| --- | --- |
| 独立入口、输入与凭据隔离 | 已完成。显式 `--execute`，默认只读，固定 HTTPS 主机/端口，显式 namespace；Go 编译/版本探测不继承凭据；使用已有 RunRecord 与进程执行器 |
| SDK 场景和本地负向回归 | 已完成并通过本地 TLS 模拟。健康、身份预算/精确路径能力、指定版本 KV、Ready/Close、隔离 CAS/软删恢复/回收、受限身份负例、Transit、PKI CSR |
| 独立审查及修复 | 已完成。所有成立发现已修复，保留四组 RED/GREEN；审查不替代运行验证 |
| 冻结后本地正式检查 | 已完成适用 Windows 检查，结果见下一节；未安装工具或下载新依赖 |
| 真实服务全场景测试 | TLS/匿名健康连通性 PASS；新凭据在 `sdk-test` 的身份自查 PASS；SDK 业务场景 BLOCKED/NOT_RUN，尚无业务场景 PASS |
| 文档、原 OB/AC 和证据 | 已更新，未建立第二套任务台账；远端报告只放补充字段，不作为 REAL_OPENBAO |

普通 `go test ./...` 不编译 [live_test.go](../tests/remote/live_test.go)。实际远端目标须带 `remote` tag，并通过入口定向运行 `TestRemoteService`。默认不写；隔离模式要求独占前缀、写入及永久回收配置。写入 CAS=0 防覆盖，UNKNOWN 不重放；清理前逐版本核对归属，204 后独立读取确认不存在。检查点保留合成归属标记的 SHA256，便于响应丢失后核对，不保存 KV 内容。

入口不操作 mount、策略、namespace、Token 或业务密钥。PKI 采用本地 CSR，未启用 Issue；吊销只在独立配置获准时执行，签发/吊销记录不冒称已删除。详细配置及恢复步骤见 [remote-testing.md](remote-testing.md)。

## 2. 当前基线的真实验证

环境：Windows amd64、Go 1.26.3、Python 3.13.13；Go 离线验证使用现有缓存，独立消费者使用既有签名 sumdb 离线镜像。表中 Go 命令由 `record.py` 保存实际 argv/cwd、自然退出码和日志 hash；RTK 仅为外层包装。未关闭 vet、降低覆盖阈值、使用 overlay/replace 或跳过失败用例。

| 实际入口/命令 | 结果 | 当前证据 |
| --- | --- | --- |
| `rtk proxy python -B -m unittest discover -s scripts/tests -v`，由 `record.py --evidence-class COMMAND_TOOLING` 记录 | PASS，114 项，45.124 秒；自然退出 0 | [正式工具收据](../.artifacts/runs/remote-20260930-final/tooling-remote-tools-final-02.json) |
| `rtk proxy python -B scripts/normal-test.py` | PASS，6/6 命令；329 个通过测试结果；fresh race profile 的 15 组 Go 语句覆盖率均 ≥85%，最低 85.498% | [normal](../.artifacts/runs/2026-09-29T165725.812263Z-321c6ee572ef4288a8bc00025afa68c9/normal/report-007.json) |
| `rtk proxy python -B scripts/fuzz-test.py` | PASS，五个目标各运行 30 秒并自然退出 0 | [fuzz](../.artifacts/runs/2026-09-29T165920.872032Z-bb1573c765134ef38fd77e27c7c1fc07/fuzz/report-006.json) |
| `rtk proxy python -B scripts/benchmark-test.py` | PASS，四个 benchmark 的六个子项各三轮；Executor 每轮 256 个独立延迟样本 | [benchmark](../.artifacts/runs/2026-09-29T170254.315373Z-e15334b9020946aaaf6bb7c1cfbe62ae/benchmark/report-002.json) |
| `rtk proxy python -B scripts/consumer-test.py --offline-proxy .artifacts/offline-consumer-proxy` | PASS，reader/signer 各自 tidy/test；目标均有 run/pass，无 skip | [独立消费者](../.artifacts/runs/2026-09-29T170426.812059Z-1fd03f7c7a4c44dd9b17e77d81f40356/consumer/report-007.json) |
| `go test -mod=readonly -count=1 -json -timeout=120s ./examples/...` | PASS，9 项 run/pass、零 skip | [示例测试](../.artifacts/runs/remote-20260930-final/examples-test-remote-examples-test-01.json) |
| `go build -mod=readonly ./examples/...` | PASS | [示例构建](../.artifacts/runs/remote-20260930-final/examples-build-remote-examples-build-01.json) |
| `go test -mod=readonly -c -tags=remote -o .artifacts/runs/remote-20260930-final/remote.test.exe ./tests/remote` | PASS，仅编译，未运行 live 目标 | [tagged 编译](../.artifacts/runs/remote-20260930-final/remote-compile-remote-build-tag-compile-01.json) |
| `rtk proxy python -B scripts/integration-test.py` | BLOCKED：缺精确 OpenBao 发布物/可信摘要及匹配锁；未执行真实 fixture 测试 | [集成前置报告](../.artifacts/runs/2026-09-29T165724.705236Z-ae37b218786b49619756b12f17e616cb/integration/report-001.json) |
| `rtk proxy python -B scripts/security-test.py` | BLOCKED：固定扫描器/数据库下载未获授权；扫描未运行 | [扫描前置报告](../.artifacts/runs/2026-09-29T165732.324781Z-4b722c1f2e7a441f938363a59f229292/security/report-001.json) |
| `scripts/ci-summary.py` 本地白名单导出与汇总 | Windows tooling/normal 导出及校验 PASS；其余六个 Linux job 缺报告，汇总 FAIL；未触发远端 CI | [本地 CI 汇总](../.artifacts/runs/remote-20260930-final/ci-summary.json) |
| `rtk proxy python -B scripts/verify-release.py`，由 record 包装 | FAIL，子进程自然退出 1，共 25 项拒绝；无当前指纹失配项 | [门禁收据](../.artifacts/runs/remote-20260930-final/verify-release-remote-release-01.json)、[逐项原因](../.artifacts/runs/remote-20260930-final/verify-release-remote-release-01.log) |

Fuzz 实际执行计数：Path 146483、Document 5341、PKICSR 184194、Encoding 20817、Decode 486568；各目标保留独立收据与日志。覆盖率来自本轮自然成功的 race，未读取失败前旧 profile。Benchmark 仅为可复跑基线，不新增发布硬门槛，不从均值推算分位数，也不声称性能改善或生产 SLA。

## 3. 真实远端范围与资源

在仅设置已确认的地址/namespace 后，实际运行了 `python -B scripts/remote-test.py --execute --mode isolated --run-id remote-isolated-preflight-20260930-01`。入口在任何 Go 探测或请求前拒绝缺失输入：

- [远端补充报告](../.artifacts/runs/remote-isolated-preflight-20260930-01/remote/report-001.json)：BLOCKED，命令列表为空，无 API 请求。
- [外层命令收据](../.artifacts/runs/remote-20260930-final/remote-preflight-remote-preflight-final-01.json)：真实子进程自然退出 2；外层 record 按非零退出记 FAIL，不把它写成业务断言失败或远端成功。
- 更早的 [只读预检](../.artifacts/runs/remote-live-preflight-20260930-01/remote/report-001.json)同样保留为 BLOCKED，没有覆盖或改写。

用户随后两次要求使用所给测试凭据进行连接检查。[首次连接补充报告](../.artifacts/runs/remote-connect-20260930-01/report.json)记录固定目标 `https://kms.jiup9.com:443`、namespace `sdk-test`、实际 Python argv/cwd、前后源码标识及三次 GET：不带凭据和 namespace 的 `/v1/sys/health` 以默认信任链和主机名验证 TLS 后返回 HTTP 200，`initialized=true`、`sealed=false`、`standby=false`；不带凭据以及带首次凭据的 `sdk-test` `/v1/auth/token/lookup-self` 均返回 HTTP 403。单独的 403 不能确定首次凭据失效、namespace 不匹配或接口权限受限。

[新凭据连接补充报告](../.artifacts/runs/remote-connect-20260930-02/report.json)是另一不可覆盖 run_id：同一健康端点为 200，匿名 `lookup-self` 为 403，带新凭据的 `sdk-test` `lookup-self` 为 200，有限身份字段形状校验通过；自然退出码 0。两次检查均使用限定输出的一次性 HTTPS 探针，不是 SDK 远端测试入口或正式 REAL_OPENBAO 集成。服务自报版本 `2.6.2`，这不是可信发布物摘要。没有切换 namespace/主机/端口、执行 KV 操作或写入。

| 真实服务场景 | 状态 |
| --- | --- |
| TLS/匿名健康连通性 | PASS，固定目标返回 HTTP 200；仅证明网络、TLS 与健康端点可达 |
| `sdk-test` 身份自查 | 首次凭据 BLOCKED（HTTP 403，原因未定）；新凭据 PASS（HTTP 200，有限身份字段有效）。仅证明该自查请求被接受 |
| 精确路径能力、指定 KV 版本/Ready | NOT_RUN，缺已批准的精确测试资源及正式 runner 的受控凭据注入 |
| KV 隔离 CAS、旧 CAS 冲突、软删恢复、最终回收 | NOT_RUN，缺确切 mount、测试资源、独占前缀与回收配置 |
| 受限身份权限负例 | NOT_RUN，缺独立受限身份和明确禁止路径；不会把无效身份的 403 当成功 |
| Transit | NOT_RUN，缺专用既有 key、类型与范围；要求无建钥/管理权限 |
| PKI CSR/吊销 | NOT_RUN，缺角色、允许域名、可信根与独立操作范围 |

真实远端请求数为 **6 次只读 GET**（两次连接探测各三次），创建资源为 **0**，待回收远端资源为 **0**。先前的两份入口预检各自仍是零请求的历史记录。本地 TLS fixture 在测试结束时回收；连接探针均自然退出，自有命令 PID 查询无存活匹配，无 `cleanup_incomplete`。构建产物、缓存与脱敏证据保留在授权位置。未清理不明归属的共享进程或原有工作区内容。

## 4. 失败记录、独立审查与限制

最初的 Python/Go RED 分别证明入口缺失和配置函数缺失；后续独立运行暴露的 Python 导入路径问题已修复。首次失败及所有重跑保存在 [开始阶段证据目录](../.artifacts/runs/remote-20260930-start/)。工具全集首次当前运行使用普通 COMMAND_RECEIPT，随后重新实际运行并生成正式 COMMAND_TOOLING 收据，未手改证据类别或 hash。

[独立 risk_reviewer 记录](evidence/OB-018/remote-2026-09-30/independent-review.md)列出成立问题及修复：身份 TTL/次数预算和 BLOCKED 汇总；无效受限身份的 403 假通过；root/Transit 建钥与管理权限；旧 CAS 和 DELETE 的 UNKNOWN；未知创建后的归属恢复；目标上下文和请求分类。四组 RED 均保留，最后 [GREEN-04](../.artifacts/runs/remote-20260930-start/remote-review-green-remote-review-green-04.json)及冻结后 normal race 通过。审查者只读，verifier 独立执行冻结后本地检查；主代理未把自审称为独立审查。

仍需环境保证的限制：metadata 删除不支持 CAS，归属读取与删除之间存在并发窗口，必须使用维护者保证无人并发写入的独占前缀。本地模拟和两次连接探针无法证明具体业务路径 ACL、非生产属性或 SDK 业务行为。Linux、最低 Go 1.25.0 和远端 CI 未验证。健康端点自报 `2.6.2`，但没有可信镜像 digest；服务锁没有改成 PINNED。

## 5. 原台账、门禁原因与最小下一步

[acceptance-results.json](../acceptance-results.json)仍为 **66 PASS / 3 PARTIAL / 3 NOT_RUN**；[逐项规则核对](evidence/OB-018/remote-2026-09-30/acceptance-reconciliation.json)记录本轮匹配和缺失类别。[task-status.json](../task-status.json)的 19 个 OB 仍为 BLOCKED，依赖未满足的任务没有升级 VERIFIED。OB-003/016/018 单独关联远端补充证据；原始失败、R1/R2/R3、七阶段及结构重构记录均保留。

门禁 25 项拒绝分别是：integration、scans 两类正式报告未通过；AC-001、AC-065、AC-066、AC-070、AC-071 未满足；OB-001 至 OB-018 的验收/依赖链未 VERIFIED。当前 BLOCKED 报告均匹配最终指纹，属于外部条件缺失，不是旧报告失配。AC-072/OB-019 的真实业务迁移继续在本轮范围外。

| 仍缺条件 | 影响 | 最小解除动作 |
| --- | --- | --- |
| 已批准的精确 KV v2 mount/path/version/非敏感标记；环境性质、独占前缀及清理动作；向正式 runner 受控注入新凭据 | OB-003/016/018 远端补充范围 | 新凭据的身份自查已通过；取得非敏感资源定位后，仅对已确认资源使用新 run_id 执行相应场景，不重复提交聊天凭据或申请同一项全场景许可 |
| 可选 Transit、PKI、受限身份输入 | 对应可选远端场景 | 提供准确资源定位与范围；缺项不阻塞已具备条件的 KV 场景 |
| 精确 OpenBao 版本和可信发布物摘要 | OB-001/016、AC-001/065/066/070 | 核验独立 fixture 发布物，冻结前固定锁，再重建受影响正式证据；外部服务冒烟不能替代 |
| 固定 govulncheck/gitleaks 与漏洞数据库准备条件 | OB-018、AC-070/071 | 提供已准备的隔离扫描环境或明确工具/数据库准备授权，然后真实扫描 |
| Linux、最低 Go 及远端 CI 条件 | OB-001/018、AC-001/070 的完整交付范围 | 提供已可用隔离平台，以同字节源码运行；远端操作另需明确授权 |

公开 SDK API、Client → engine → officialSender 分层、module 地址、Apache-2.0 LICENSE、api/v2 v2.7.0、Go 1.25.0 声明下限及数据/错误/认证/重试语义均未改变。没有擅自暂存、提交、推送、创建分支/工作树、发布或部署。当前结论是**本地远端测试入口完成，指定服务连通性、健康和新凭据身份自查通过；SDK 业务测试及交付门禁仍阻塞**。
