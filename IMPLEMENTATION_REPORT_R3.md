# OpenBao SDK 续作报告 R3

**OB-001仍阻塞，SDK没有达到V1发布条件。本轮推进的是R2交接允许的公共依赖离线转移入口，不是运行时SDK已完成。**

记录时间：2026-09-28T15:55:23.849283+00:00
源码指纹：`459e4262e9d1665e9ff6018fda0797046daf361b55a2052c230903c0c80e3399`

## 本轮实际改动

- 新增 `scripts/dependency-bundle.py`：联网机器用空缓存下载完整公共模块，读取真实上游 go.mod，验证实际校验记录和模块图；在第二套新缓存中通过本地 file proxy 重放签名日志校验，成功后才发布转移包。
- 新增 `scripts/tests/test_dependency_bundle.py`：18项隔离、归档、防篡改、失败拒绝测试。
- Makefile新增 `dependency-export`；使用说明见 `docs/dependency-transfer.md`。
- 98个Go文件、go.mod、go.sum和official_sender.go未修改。不扩展SDK能力，不使用overlay、替代发送器或replace。

## OB-001实际结果

`make dependency-check`返回2，`make dependency-export`返回2；内部真实官方下载命令均返回1，DNS解析失败。没有完整官方模块、上游go.mod或真实校验记录，没有生成公共依赖ZIP。当前固定Go为1.23.2，不是已核实的上游最低要求。Codex注册执行环境列表为空。外部下载入口亦未取得文件。

依赖转移包的正向导出/接收复验未运行成功；新工具的单元测试不能作为官方依赖通过证据。

## 实际检查与退出码

| 检查 | 退出码 | 范围 |
|---|---:|---|
| make dependency-check | 2 | 正式下载阶段阻塞 |
| make dependency-export | 2 | 正式下载阶段阻塞；无输出依赖ZIP |
| make normal-test | 2 | 内部unit/race/vet/verify/build/module-graph/test-list各1 |
| 直接自有Go包test -race | 0 | 64个顶层Test函数通过；不含官方适配器 |
| 自有Go包vet | 0 | 通过 |
| make tooling-test | 0 | 35项通过：既有17＋新增18 |
| 五项指定fuzz | 各0 | 每项30秒；原入口与时长未缩减 |
| make integration-test | 2 | 缺经核验的服务程序/镜像 |
| make consumer-test | 2 | 依赖下载失败 |
| make security-test | 2 | 固定扫描工具获取失败，扫描未完成 |
| make release-check | 2 | 保持拒绝发布 |

中断的包装调用保留原日志，没有以其尾部PASS推定整个命令退出0；随后重跑取得直接测试和五项fuzz的实际退出记录。新工具首轮测试因Go错误文字不包含sumdb而有一条失败，改为同时断言实际“verifying go.mod”阶段与“malformed record data”；非零退出、无外部回退、无go.sum创建断言保留。该决策见证据目录 decisions.md。

## 任务状态

| ID | 任务 | 状态 |
|---|---|---|
| OB-001 | 固定依赖基线与公开类型 | BLOCKED |
| OB-002 | 秘密包装与无损 JSON 文档 | IN_PROGRESS |
| OB-003 | 配置、路径与 TLS 默认值 | IN_PROGRESS |
| OB-004 | 统一请求、错误与操作注册表 | IN_PROGRESS |
| OB-005 | 生命周期与外部 Token 模式 | IN_PROGRESS |
| OB-006 | AppRole 单次登录与凭据提供器 | IN_PROGRESS |
| OB-007 | 续期、重登、过期与关闭 | IN_PROGRESS |
| OB-008 | KV 创建与准确版本读取 | IN_PROGRESS |
| OB-009 | KV CAS、元数据、列表与软删除 | IN_PROGRESS |
| OB-010 | PKI 签发、CSR 与本地材料校验 | IN_PROGRESS |
| OB-011 | PKI 读取、链和吊销 | IN_PROGRESS |
| OB-012 | Transit 版本化签名与公钥 | IN_PROGRESS |
| OB-013 | Transit 加密、重包裹与 HMAC | IN_PROGRESS |
| OB-014 | 诊断和可观测钩子 | IN_PROGRESS |
| OB-015 | 故障、隔离与模糊测试加固 | IN_PROGRESS |
| OB-016 | 临时真实 OpenBao 集成环境 | BLOCKED |
| OB-017 | 示例与两个独立消费者验证 | BLOCKED |
| OB-018 | 发布门禁、文档与安全审查 | BLOCKED |
| OB-019 | 既有项目接入与回滚演练 | BLOCKED |

## 验收结论

72项保持原编号和要求：8 PASS、59 PARTIAL、5 NOT_RUN；逐项结果见 acceptance-results.json。未将转移工具单测映射成AC-001通过，也未将SDK可用和实际接入混写。

## 仍需外部条件与已知限制

需要能够下载并校验真实公共模块的隔离环境。在该环境执行 `make dependency-export` 才能生成回传材料。没有引入个人模块缓存、关闭GOSUMDB或拼装官网片段的替代方案。ZIP传输hash仅校验完整性，Go签名日志认证是独立步骤。

OpenBao服务程序/镜像与扫描器不在公共SDK模块包内。现有消费者脚本固定使用公开网络代理，纯离线消费者入口还需要后续按真实失败修正；本轮未动该脚本。没有真实调用项目源码，因此OB-019继续阻塞。

## 审查与交付边界

本轮是作者自审，无独立审查者。没有Git仓库历史，不伪造提交或push。原始方案和历史证据保留；旧R2根校验清单保存在本轮证据目录，新交付清单独立生成。源码交付ZIP不是公共依赖ZIP，也不是可生产发布版本。
