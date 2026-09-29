> 当前续作状态见 `IMPLEMENTATION_REPORT_R2.md`。下文保留R1历史报告，不代表本轮验证结果。

# openbao-sdk-go 续作实施报告 R1

记录时间（环境UTC）：2026-09-28T11:32:23.851721+00:00  
当前源码SHA256：`54b0a506d504078aef74886f1ccf072e165d38116211d4f863b6499f9943d38d`  
原源码SHA256：`91e13af25d1501acdc26a11d0df83f81a683d0f016baab607718dcded2e52384`

## 交付结论

本轮在上一轮源码上继续修复与验证，没有重建SDK或重写已验证模块。**当前仍不是可发布版本：官方依赖编译、真实OpenBao、独立模块消费及扫描门禁未通过。** 不做生产/预发布环境操作、远端发布或实际业务迁移。

上一轮源包SHA256为`bd4d36a0bad2cdbedacf53e3190c61d0bebee3f8793db6274fad064648088581`。安全恢复后372个条目与输入一致，原始01–06方案副本逐字节一致。当前没有`.git`，没有伪造分支或提交历史；本轮差异以原归档和源码指纹为依据。

## 本轮实际变更

| 变更 | 修复前 | 修复后与验证 |
|---|---|---|
| 请求关闭 | Start之前发出的健康检查没有run Context，Close超时后请求仍存活 | 增加无后台任务的独立请求生命周期，Close取消已准入健康请求；失败测试复现，关闭组含race重复20轮通过，最终整体回归重新通过 |
| PEM材料 | 标准PEM解码可寻找后续有效块，使损坏首块/中间块被忽略 | 新增私有非跳跃解码器，TLS CA/mTLS/证书/CSR/私钥/Transit公钥拒绝损坏块；正确PKCS8、EC、CRLF双向TLS正例仍通过 |
| KV计划删除 | 所有非空deletion_time被当作已删除，未来自动删除时间也读不了 | 保留准确版本，区分未来计划与已到删除时间；空数据/404仍拒绝，未来时间不能伪证已删除；本地回归通过 |
| JSON Unicode | 不配对的UTF-16转义经标准解码替换字符而丢失原信息 | 原始JSON入口线性校验代理对，拒绝有损转义；合法代理对、中文、转义字面字符保持正确，数字与秘密包装回归通过 |
| 真实集成准备 | 未覆盖未来自动删除元数据 | 新增受限身份计划删除子用例；仅临时管理员准备元数据，运行时policy不扩权。fixture-builder契约与集成源码编译检查通过，真实服务用例未运行 |
| 测试入口 | 新PEM解析器未进入fuzz与包覆盖检查 | 保留原四项并增加第5项30秒fuzz；pemutil沿用85%语句覆盖门槛，没有降低原门槛 |

四类运行时问题均先保存失败测试，再修复并通过相应测试。fixture准备和fuzz入口也有先红后绿记录。此前原始日志与失败证据未删改。具体决定、限制与代价见`docs/adr/001-client-boundary.md`的R017–R023。没有改变公开方法签名、扩大运行时权限或启用写入自动重放。

## 当前测试结果与证据范围

| 层级 | 本轮结果 | 不能据此推断 |
|---|---|---|
| 直接自有包测试 | 64个顶层Test通过，含race；直接leaf go vet通过 | 不含官方发送器/完整SDK编译 |
| 补充根包与协议测试 | 118个顶层Test通过，含race；66个子测试/种子事件；无测试用例FAIL/SKIP | 仅替换私有官方发送器工厂为测试发送器，不是生产配置，不证明官方adapter或真实服务 |
| 模糊测试 | 5项各30秒，make fuzz-test exit0 | 普通种子执行数不叠加为新的fuzz项目 |
| Python工具测试 | 9项通过，make tooling-test exit0 | 不替代SDK功能/安全扫描 |
| 双向TLS | 正常PKCS8/EC/CRLF身份成功，损坏材料拒绝 | 属于本地真实TLS握手，不是OpenBao服务器证据 |
| 集成源码 | 补充发送器模式下编译检查通过，-run=^$ | 未执行任何真实OpenBao集成用例 |
| 消费者源码 | 两份源码在共享临时模块下补充测试通过 | 不是独立go.mod依赖消费通过 |

64直接测试与118补充测试重叠，不能相加。上一轮可比补充基线106个顶层Test，本轮118个；直接包57增至64。补充测试含8个“无测试文件”的包，直接测试含3个；它们是编译项，不是被跳过的验收用例。全部测试名、包和状态见`docs/evidence/resume01-test-index.json`。

## 实际正式命令

| 命令 | 实际退出码 | 原因/解释 |
|---|---:|---|
| GOWORK=off go test -count=1 ./... | 1 | 缺少真实api/v2模块及对应go.sum，未进入完整正式测试 |
| GOWORK=off go test -race -count=1 -coverprofile=coverage.out ./... | 1 | 同上；完整覆盖率未取得 |
| GOWORK=off go vet ./... | 1 | 官方依赖缺失 |
| GOWORK=off go mod verify | 1 | 模块代理DNS拒绝 |
| GOWORK=off go build ./... | 1 | 官方依赖缺失 |
| make normal-test | 2 | 上述正式验证命令未通过 |
| make integration-test | 2 | 缺精确版本且来源校验的bao程序或digest锁定镜像 |
| make consumer-test | 2 | reader/signer各自go mod tidy均exit1，失败于上游官方依赖下载 |
| make security-test | 2 | 固定版本govulncheck/gitleaks获取命令各exit1，扫描主体未运行 |
| make release-check | 2 | 当前任务、验收与正式证据不满足门禁，正确拒绝发布 |
| make tooling-test | 0 | 9项通过 |
| make fuzz-test | 0 | 5项通过 |

实际日志及formal报告原样归档在`docs/evidence/OB-016/17/18/resume01-formal-artifacts/`，路径映射为`docs/evidence/resume01-archive-index.json`。每条实际命令元数据绑定本轮源码指纹，不把工具运行失败归为业务测试断言失败。

## 覆盖率

覆盖指标是Go语句，不是已核实的物理行指标。保留直接覆盖报告和显式叶子包instrumentation报告；后者允许根包契约驱动叶子代码，但不instrument根包，避免当前Go1.23 overlay限制。曾尝试根包coverprofile失败的原始日志也保留。

| 自有叶子包 | 合并语句覆盖率 |
|---|---:|
| auth | 86.49% |
| baoerr | 100.00% |
| internal/authn | 90.94% |
| internal/engine | 86.30% |
| internal/jsondoc | 90.11% |
| internal/pkiutil | 96.55% |
| internal/transitutil | 95.40% |
| internal/pemutil | 100.00% |
| kv | 88.00% |
| pki | 98.57% |
| sensitive | 100.00% |

11个自有叶子包达到85%，**不等于整体覆盖率门禁通过**。根包功能门面与官方适配器尚无正式覆盖率，完整KV/PKI/Transit域也不得用缺少门面的局部数字冒充通过。每个profile的独立分子、分母、合并值和范围见`docs/evidence/OB-018/resume01-leaf-coverage-summary.json`。

## OB任务状态

0 VERIFIED / 14 IN_PROGRESS / 5 BLOCKED。未跳过原依赖链：OB-001未解阻，不因下游本地测试通过就标任务完成。

| 任务 | 标题 | 状态 | 本轮事实 |
|---|---|---|---|
| OB-001 | 固定依赖基线与公开类型 | BLOCKED | 恢复归档与原方案副本一致；重新下载api/v2@v2.7.0仍遭DNS阻断。公开边界在本轮补充测试中重新通过；完整官方依赖、最低Go版本、go.sum与服务版本基线未核实。 |
| OB-002 | 秘密包装与无损 JSON 文档 | IN_PROGRESS | 新增严格UTF-16转义校验：拒绝会丢信息的不配对代理项；合法Unicode、数字与秘密包装回归通过。 |
| OB-003 | 配置、路径与 TLS 默认值 | IN_PROGRESS | TLS材料使用非跳跃PEM解析；损坏首块不能被跳过。CA/mTLS负例及PKCS8/EC/CRLF双向TLS正例通过。 |
| OB-004 | 统一请求、错误与操作注册表 | IN_PROGRESS | 404未来deletion_time不再误判已删除；无效JSON转义被统一响应解析器拒绝。原有单次写入、UNKNOWN、重定向与取消回归通过。 |
| OB-005 | 生命周期与外部 Token 模式 | IN_PROGRESS | 修复Start前健康请求在Close预算耗尽后未取消；关联独立请求生命周期，保持New无网络/后台任务及原关闭预算。 |
| OB-006 | AppRole 单次登录与凭据提供器 | IN_PROGRESS | 本轮直接/补充契约回归重新通过；未重写模块。官方依赖及必需前置验收未满足。 |
| OB-007 | 续期、重登、过期与关闭 | IN_PROGRESS | 本轮直接/补充契约回归重新通过；未重写模块。官方依赖及必需前置验收未满足。 |
| OB-008 | KV 创建与准确版本读取 | IN_PROGRESS | 未来计划删除时间不再使完整有效版本不可读；404/缺数据仍拒绝；不回退latest或变更版本。 |
| OB-009 | KV CAS、元数据、列表与软删除 | IN_PROGRESS | 本轮直接/补充契约回归重新通过；未重写模块。官方依赖及必需前置验收未满足。 |
| OB-010 | PKI 签发、CSR 与本地材料校验 | IN_PROGRESS | 证书、CSR和PKCS8私钥拒绝跳过损坏PEM块；信任链与匹配校验回归通过。 |
| OB-011 | PKI 读取、链和吊销 | IN_PROGRESS | 共享证书解析器加固，查询及链处理在本轮契约回归中重新通过。 |
| OB-012 | Transit 版本化签名与公钥 | IN_PROGRESS | 公钥拒绝损坏PEM首块；版本/算法/原文与摘要/验签边界回归通过。 |
| OB-013 | Transit 加密、重包裹与 HMAC | IN_PROGRESS | 本轮直接/补充契约回归重新通过；未重写模块。官方依赖及必需前置验收未满足。 |
| OB-014 | 诊断和可观测钩子 | IN_PROGRESS | 健康请求纳入独立关闭生命周期；诊断、就绪、Observer回归通过。 |
| OB-015 | 故障、隔离与模糊测试加固 | IN_PROGRESS | 保留四项fuzz并增加第5项PEM解码fuzz，全部实际运行30秒通过；补充跨空间及关闭回归通过。 |
| OB-016 | 临时真实 OpenBao 集成环境 | BLOCKED | 新增计划删除有限权限集成子用例及fixture-builder契约测试；业务policy未扩权。真实OpenBao缺来源核验程序/镜像，未执行集成。 |
| OB-017 | 示例与两个独立消费者验证 | BLOCKED | 两份消费者源码补充测试与凭据恢复回归通过；独立go.mod消费者仍在依赖下载阶段失败，不能称独立消费通过。 |
| OB-018 | 发布门禁、文档与安全审查 | BLOCKED | 重跑正式构建、测试、消费、扫描与发布准备；官方依赖缺失。保存当轮日志及叶子覆盖率，完整发布门禁仍未通过。 |
| OB-019 | 既有项目接入与回滚演练 | BLOCKED | 未提供真实业务源码/隔离迁移条件；没有进行项目接入、回滚或部署。 |

## 72项验收

9 PASS：AC-002～007、047、064、068。  
58 PARTIAL：AC-008～046、048～063、066、069～070。  
5 NOT_RUN：AC-001、065、067、071、072。

所有原场景/观察要求与编号保留，逐项重新关联当前测试证据。PASS为限定本地观察通过，不代表正式运行时或发布通过。准备命令已经失败但未进入实质验收的条目仍NOT_RUN。全部72行及测试/证据见`docs/acceptance-results.md`和根目录`acceptance-results.json`。

## 审查与尚未解决的事项

本轮仅作者自审，当前无注册Codex执行环境，没有独立审查者。没有发布tag、push、生成正式module地址、签署许可证或访问实际项目。

仍需：真实api/v2@v2.7.0完整模块及go.mod、兼容Go工具链与go.sum；精确且来源核验的OpenBao；官方发送器的全部构建/故障/权限测试；完整覆盖率、独立消费者、govulncheck/gitleaks、独立审查及真实项目接入。当前Go1.23.2仅是已运行自有代码的工具链，不是官方依赖兼容性承诺。go.sum留空而非伪造。

## 下一接手入口

从OB-001继续，阅读`docs/implementation-handoff.md`。先获取真实依赖、核对NewConfig/NewRequest/RawRequestWithContext与最低Go要求；再执行正式门禁并修复真实失败。不要以补充测试结果代替官方/集成/发布报告。

## 最终交付校验

当前98个Go文件（43个测试文件）。本轮源码差异：28个文件，其中17个修改、11个新增、0个删除。`openbao-sdk-go-resume01.patch`仅包括代码/测试/构建配置；完整文档和证据包含在新ZIP中。补丁在原归档的临时副本上先`git apply --check --whitespace=error-all`、再实际应用，均退出0，应用后全部源码字节与当前目录一致。

`gofmt`检查98个Go文件退出0。普通`git diff --check`退出129（没有.git），不记为通过；原归档为基准的`git diff --no-index --check`源码限定检查退出1（存在差异，空白诊断为空）；带`--whitespace=error-all`的补丁校验与应用退出0。打包脚本曾误期待no-index的退出码0，该错误及对照复现记录已保留。原始证据日志不改写，旧日志尾随空白不纳入源码补丁检查。

新交付文件为`openbao-sdk-go-implementation-resume01.zip`，旧`openbao-sdk-go-implementation.zip`保留不动。文件清单与SHA256在新包内SHA256SUMS.txt；外部交付校验JSON记录CRC、清单匹配、原证据/原方案保留及基础秘密材料检查。基础打包检查不能替代未运行的govulncheck/gitleaks。
