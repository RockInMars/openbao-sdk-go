# 测试证据等级和门禁

## 五类证据

1. `DIRECT_OWN_CODE`：标准库依赖的自有包直接 go test/race；不经过工厂替换。
2. `CONTRACT_WITH_TEST_SENDER`：只替换自有 official_sender.go 工厂，真实执行其余代码并访问临时 TLS httptest。这是 HTTP 契约/故障测试，不是官方 api/v2 编译或真实 OpenBao。
3. `NORMAL_OFFICIAL_CLIENT`：未修改源码、完整官方依赖的正常 test/race/vet/build/mod verify。当前失败在依赖获取。
4. `REAL_OPENBAO`：正式工厂 + 经 SHA256 校验的新建真实临时集群 + Namespace/受限身份。当前未执行业务测试；启动前置检查真实失败。
5. `INDEPENDENT_CONSUMERS`：两个独立 go.mod 通过临时模块代理加载 SDK，禁止 replace。当前下载 SDK 成功、官方依赖下载失败，消费者未构建。

固定版本安全扫描独立归为 `PINNED_SECURITY_SCANNERS`，下载失败不代表发现零漏洞。

## 可复跑命令

`make normal-test` 连续记录各项原始退出码，不以一项通过遮蔽其它失败；`make tooling-test` 验证脚本在缺环境/skip/错误证据类别/旧 source hash 下拒绝发布；`make integration-test` 缺配置 exit2（make 包装后同样非零）；`make consumer-test` 无本地 replace；`make release-check` 只检查，不把台账改为 PASS。

`make fuzz-test` 依次对 Path、Document、PKI CSR、Transit 编码和严格 PEM 解码运行30秒 fuzz（续作R1新增第5项，原4项不删改）。历史中被工具硬超时中断的进程保留 exit_code=null，不转成PASS；后续完整运行独立记证。

## 覆盖率

Go 原生 coverprofile 表示语句覆盖率，不是严格的物理代码行覆盖率。原方案用了“行覆盖率”措辞。本实现明确报告原生 Go 语句覆盖率，**未把它偷换成原方案的行覆盖率已通过**；若发布验收要求独立行指标，须追加定义与采集，原门禁保持未通过。

物理包要求根包、internal/engine、internal/authn、auth、kv、pki、baoerr、sensitive、internal/jsondoc、internal/pkiutil、internal/transitutil、internal/pemutil各≥85%；另外检查 kv/pki/transit 功能门面+类型/校验包组合，避免 root 平均掩盖功能域缺口。types-only的transit/diagnostics/observe不冒充有执行覆盖率。

Go1.23 对 overlay 根包的覆盖率/自动vet会读取原始第三方导入。补充运行因此只覆盖指定自有叶子包，**根包/official_sender.go仍缺正常覆盖率证据**。补充叶子报告绝不能用于 make release-check 中的正常报告。

## 台账

根 task-status.json 保留OB001–019及原依赖。写了代码、仅补充测试通过、前置依赖未验证，不标VERIFIED。acceptance-results.json逐AC记录状态、证据等级和缺口；PARTIAL 表示观察到部分行为但没有完成要求的证据链。原始 docs/spec/04不改写。

每次最终验证用 source_sha256 绑定实际源代码。中间红绿记录保留当时的原始退出码，旧记录不自动视作新源码通过。整个工作是作者自审，没有伪造第二位审查者。

`python3 scripts/consumer-contract-test.py` 仅验证两份消费者源码在SDK同模块下的类型/本地协议。临时目录退出后删除；没有replace指令，也不是独立依赖解析证据，AC067继续等待真正consumer-test通过。

## 续作R1证据范围

本轮原始证据使用 `resume01-` 前缀，历史红绿日志保持不变。`resume01-final-direct-tests`是直接包测试；`resume01-final-contract-tests`是仅替换官方工厂的完整自有代码回归；`resume01-leaf-instrumented-contracts`通过显式-coverpkg只采集关键叶子包，其余未被采集的根包和完整功能域保持未验证。普通覆盖命令失败的原始输出也保留，不用补充命令覆盖它。

真实集成新增 `TestIntegrationKVScheduledDeletion`：临时管理员预配置自动删除元数据，有限权限业务身份写入并按准确版本读取，再显式软删除验证。源码编译检查的 `-run=^$`不执行任何集成用例，不能用于AC065。
