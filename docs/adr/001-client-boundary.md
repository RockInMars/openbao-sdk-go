# ADR 001 — 实施边界与离线测试

执行已批准的 docs/spec/01、02、03、04，不修改原始接口或任务依赖。

## 本次环境证据

现有工具链为 Go 1.23.2 linux/amd64；没有已有 SDK/真实消费者仓库，没有 Docker、Podman、bao。
直接 Go module download 实际失败（DNS connection refused）；浏览器能够读取候选标签的 client.go，但不是可供 Go 使用的完整模块归档。
不根据主仓库 main/go.mod 推断 api/v2.7.0 的最低 Go 版本。未取得其 go.mod/归档/sum，依赖基线门禁保持阻塞。

## 决定 R-001

先保留方案示例 module path 与固定候选 require v2.7.0，go 指令暂为现有可运行的 1.23.2，只代表自有标准库代码语言基线，不是对完整依赖图最低版本的承诺。
正式依赖恢复后运行 `GOWORK=off go mod download` 并读取真实依赖 go.mod，核对官方 go.mod 并记录最终工具链。不得伪造 go.sum 或降级官方依赖来标绿。
代价：正式模块构建/发布仍有阻塞，必须在真实依赖下重新跑全部门禁。

## 决定 R-002

业务代码依赖内部 Sender 小接口；默认实现只使用官方 api/v2。为验证网络受限时的自有代码，补充独立命令 contract-test，用 Go overlay **只替换私有官方适配器工厂**为 internal/testutil 中的标准库 HTTP 测试发送器。
它不替换任何第三方模块，不是SDK运行模式，不在默认构建中启用，不伪造官方包，不算官方客户端或真实OpenBao兼容测试。
除这一适配器外配置、TLS、请求执行器、认证、公开方法、编码、失败语义都使用真实自有实现。
所有这类证据标为 CONTRACT_WITH_TEST_SENDER，官方依赖测试和发布门禁保持未通过。
代价：官方 RawRequest 特有行为需要恢复依赖后单独验证，合同测试不能证明该适配器兼容。

## 审查方式

本会话串行执行与作者自审；发现的 Codex 工具仅提供读取功能，未调度独立审查者。真实项目接入 OB-019 不编造。

R-003：执行器测试中一次编辑误将 io 导入片段插入两份 JSON fixture，导致严格解析器正确拒绝响应而不重试。恢复原定合法 JSON fixture；未改变生产解析规则。取消测试为 TLS/race 调度留出 200ms 的调用方期限，SDK 预算仍独立测试；期限前后效果断言不变。服务器读取请求体后再等待取消，以便 net/http 能观察连接断开。执行器最终 race 结果见 OB-004/green-executor。

R-004：当前 Go 1.23 的 go test 自动 vet 阶段仍读取被 overlay 替换文件的原始 import，因此补充契约脚本仅对该命令设置 -vet=off。默认 go test/go vet/CI 正式门禁不禁用 vet，仍需要取得真实依赖。补充测试不算官方适配器编译或静态验证。

R-005：ClusterHealth 是明确的根级无身份诊断，允许在 CREATED 状态调用，CLOSING/CLOSED 禁止；业务请求仍须 Start。认证启动分为有总预算的 Refresh 和绑定服务 Context 的 Run，避免重复读取外部凭据或错误地绑定短期登录 Context。

R-006：HTTP 库在取消期间可能仍持有请求 Body reader，协议发送器给它独立 GC 管理的缓冲区，不在 Do 返回时并发清零。输入、结果及 SDK 自有敏感对象仍尽力清零；不宣称消除 Go 字符串、TLS 私钥对象或 HTTP 栈所有内存副本。

R007: CheckReady returns a complete negative readiness result for not-ready/auth/permission/not-found/version-unavailable states; transport, malformed responses, cancellation and programmer errors remain errors. ClusterHealth may run before Start, without credentials.

R008: The first scoped readiness test configured root but asserted named space-a; fixed the fixture to explicitly select named space-a. Kept the named-identity assertion; failed and corrected runs are retained.

R009: Transit V1 uses an uncached read-only key metadata/public-key preflight and one bounded total crypto-operation context. Read permission on the named key is required. Signing additionally verifies the returned signature locally against that exact public version/profile. No key creation, alternate algorithm, context guessing or permission expansion occurs. TLS/protocol fixture crypto is not real OpenBao evidence.

R010: Failure-driven hardening fixed four defects: limits reject values that cannot safely support bounded int-sized buffers and the +1 sentinel; Issue/SignCSR now require TTL>0 exactly as overall design 11.1; void 200 responses reject nonempty/malformed errors envelopes; concurrent Start callers retain their own attempt result instead of a later retry result. Tests first reproduced all four failures. No public contract relaxed.

R011: Authentication Run now waits for a canceled previous attempt to terminate before installing a new loop. A failure-first deterministic-clock test reproduced a stopped loop being mistaken for a live loop. Concurrent renewal still uses the existing single-flight coordinator; auth requests have independent semaphore capacity.


R012: 作者收尾审查的失败驱动修复：显式空CA文件必须报错而非回退系统根；READY但服务Context已取消时再次Start必须返回取消；Transit元数据解码错误记录实际HTTP尝试数，不能把HTTP200填入Attempts；400/401/403的模糊代理对象及404缺失API拒绝证据时写Effect不归零；配置TLS文件打开前拒绝FIFO；业务恢复示例拒绝nil存储回执，不panic。对应red/green证据在OB-018。代价：更保守的UNKNOWN需要业务核对，不提供假定未执行的便捷重试。

R013: 常见Go覆盖工具输出原生语句覆盖率，而01用“行覆盖率”措辞。报告明确指标单位，没有将补充语句覆盖率当成完整行指标/正式覆盖率通过。脚本对正常root和关键物理包、kv/pki/transit域均设85%门槛，缺root证据不通过。代价：严格物理行指标仍需验收方定义与验证，当前release没有通过。

R014: 为在依赖阻塞时检查两份消费源码的类型和协议，新增consumer-contract-test.py，仅将其Go文件临时置于SDK模块下配合测试发送器执行。这不是独立go.mod消费；真正consumer-test仍使用独立目录/临时module proxy且已失败于依赖下载。两者证据不合并。代价：不能证明发布后依赖解析与SDK真实适配器可用。

R015: 不擅自选择项目开源许可证，不创建远端仓库或发布tag。新工作区变更保留供审查；所有记录以源代码SHA256及时间绑定，不伪造Git提交历史。代价：正式发布前需要确定授权和真实module地址。

R016: 最终可执行目标审查发现Makefile中的FuzzDocument和FuzzPKICSR指向错误包；先增加失败的工具契约测试，再改为实际声明所在的kv和pki包。此前四项30秒模糊测试使用正确直接命令，仍保留其原始证据；新增make整体执行作为入口验证。未改变任何模糊断言或运行时实现。

## 续作 R1（在原交付包恢复的源码上继续）

R017: 本轮工作区没有 `.git`；使用原交付 ZIP 和内部 SHA256SUMS 校验后恢复同一份源码，初始372个条目逐字节一致。未重建项目或初始化虚构 Git 历史。差异基准为原 ZIP SHA256 `bd4d36a0bad2cdbedacf53e3190c61d0bebee3f8793db6274fad064648088581`。代价：不能声称保留了未随交付包提供的分支/提交，只交付可核验的文件差异。

R018: AC020/023/059/063 回归确认，Start 前准许的健康请求没有服务 run Context，Close 耗尽预算后无法取消它。为 Client 建立不启动 goroutine 的独立请求生命周期，所有已准入请求都关联它；关闭排空或超时后统一取消。服务 Context、调用方 Context、关闭预算和写 UNKNOWN 规则不变。代价：每请求增加一个可注销取消回调；目标关闭测试含 race 连续20轮通过，完整回归另行重跑。

R019: 默认 pem.Decode 会寻找后续有效块；原先的前缀检查仍会跳过损坏的首块/中间块。新增内部非跳跃 PEM 解码器，用于 TLS CA、mTLS身份、PKI证书/CSR/PKCS8私钥和Transit公钥。只接受从首字节开始且没有跨越另一BEGIN标记的解码；调用者继续执行长度、类型、头和剩余材料校验。私有接口变化，不改变公开类型。代价：之前被忽略的损坏或多余密钥材料现在明确拒绝；正确PKCS8/EC密钥及CRLF的双向TLS仍通过正向测试。

R020: 官方KV v2文档明确 `delete_version_after` 会写入未来的 `deletion_time`。不能把非空时间戳无条件解释为已删除。200完整数据的读取只在destroyed或删除时间已到时拒绝；404的未来时间不是已删除证明，保持not_found_or_hidden；200缺数据仍拒绝。原DeletedAt字段保留原始时间（可能为计划删除），不改字段名、不回退版本。代价：本地截止判断依赖本机时钟，时钟超前时仍保守拒绝；真实服务行为须由新增有限权限集成用例确认，不能用本地HTTP fixture代替。

R021: 无损JSON入口不应接受会被标准库替换为U+FFFD的不配对UTF-16转义。原始JSON校验增加线性扫描，拒绝该类有损转义，仍接受合法代理对、中文、显式U+FFFD和转义的字面反斜杠。不升级Go、不更换JSON库、不改变公开接口。代价：标准库兼容性模式曾接受的非法Unicode值会被拒绝。NewDocument的任意Go对象仍按既有json.Marshal契约编码；此变更不声称控制调用方自定义MarshalJSON的所有转换。

R022: 新PEM解析器加入第5项30秒fuzz；保留原4项及其断言。集成fixture仅由临时管理员准备 `fixture/scheduled` 元数据，业务身份没有新增metadata-write权限。真实集成门禁要求新增计划删除子用例执行通过；缺环境依然失败。新增fixture-builder的HTTP契约测试不是实际OpenBao测试。

R023: 本轮直接覆盖率与跨根包契约测试的叶子包覆盖率分别保存；后者显式-coverpkg排除根包以避开Go1.23 overlay限制，不伪造根包覆盖率。报告只展示实际采集到的叶子指标；即使工具聚合中的部分domain值有数字，也不能在门面文件缺失时当作完整功能域通过。原85%门槛未降低，新增pemutil也纳入同一门槛。
