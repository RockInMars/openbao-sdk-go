# 作者自审与修复记录

本次为作者自审，没有调度独立审查者，不宣称第二位工程师/独立代理已批准。

已先复现再修复的事项：

| 问题 | 失败证据 | 修复后的证据 |
|---|---|---|
| 限制值+1溢出、PKI零TTL、void错误正文、Start结果串代次 | OB-015/red-hardening | OB-015/green-hardening |
| 取消后认证循环未重新建立 | OB-015/red-auth-loop-restart | OB-015/green-auth-loop-restart |
| 空CA回退、取消后Start错误READY、Attempts误记HTTP码、模糊4xx写效果归零 | OB-018/red-review-findings | OB-018/green-review-findings |
| 配置FIFO阻塞、业务恢复nil回执panic | OB-018/red-review-nil-fifo | OB-018/green-review-nil-fifo |
| 模糊404被当作确定未写 | OB-018/red-404-write-effect | OB-018/green-404-write-effect、green-404-proof |
| Makefile两项fuzz包路径错误 | OB-018/red-fuzz-make-targets | OB-018/green-fuzz-make-targets、OB-015/make-fuzz-all-final |

每条路径相对docs/evidence，包含命令JSON和原始脱敏日志。最后补充自有代码race回归全部通过；正式官方factory不在此证明范围。

尚未消除的发布风险：官方api/v2工厂未编译；真实服务/Namespace/ACL/认证续期/PKI/Transit兼容没有运行证据；根包和完整域覆盖率缺失；扫描工具主体未运行；无真实项目迁移；无独立审查。不能将这些风险列为“已修复”。

项目未给出开源许可授权，未擅自加入LICENSE或发布远端tag。正式module地址、Go下限和服务端锁仍需确认。关键实施决定R001–R016及影响见docs/adr/001-client-boundary.md；原设计文档未被修改。

完整git diff --check实际返回2：历史原始覆盖率日志的一条Go警告含尾随空格。未篡改原始日志以标绿；排除docs/evidence后的源码/文档检查返回0，两份记录都交付。此格式缺陷不改变任何功能测试状态，完整检查未通过事实保留。


## 续作R1自审

本轮仍由实现者自审，没有注册可用的独立Codex环境，没有第二位审查者的批准。

| 问题/补充 | 红例 | 绿例 |
|---|---|---|
| Start前健康请求无法随Close取消 | OB-005/resume01-health-close-red | OB-005/resume01-health-close-green（20轮race） |
| PEM解析跳过损坏首块/中间块 | OB-010/resume01-strict-pem-red | OB-010/resume01-strict-pem-green；OB-003/resume01-mutual-tls-regression |
| KV未来自动删除被误判为已删除 | OB-008/resume01-kv-scheduled-red | OB-008/resume01-kv-scheduled-green；OB-004/resume01-error-mapping-regression |
| 原始JSON不配对代理字符被有损接受 | OB-002/resume01-unicode-red | OB-002/resume01-unicode-green |
| 自动删除真实集成fixture缺口 | OB-016/resume01-scheduled-fixture-red | OB-016/resume01-scheduled-fixture-green（仅fixture构造测试） |
| 新解析器没有Make fuzz入口 | OB-015/resume01-fuzz-entry-red | OB-015/resume01-fuzz-entry-green |

检查点包括：原始方案及公开签名未修改；原始交付文件未丢失；没有扩大运行时ACL；Close仍不远端撤销凭据；合法证书链/CRLF/mTLS保持可用；准确版本不浮动；未来删除时间不解释200空数据或404为成功；JSON合法代理对/反斜杠不误拒绝；正式门禁不使用测试发送器。

剩余发布风险没有因这些修复消失：完整官方依赖及适配器编译、真实服务、完整门面覆盖率、扫描和独立消费/业务接入均需对应的正式证据。全量状态以本轮实施报告和72项台账为准。

## 续作R2自审（OB-001聚焦）

本轮没有修改Go运行时；新增scripts/dependency-check.py、其8项测试与Make目标。先运行缺入口失败测试，再实现解析、文件存在、精确pin、go.sum匹配、替换拒绝与环境隔离逻辑。全部17项工具测试通过；这是工具本身的测试，不是上游模块校验结果。

实际预检下载失败，未进入完整模块校验；源码检查只用浏览器精确tag资料作补充，不拼凑第三方模块，不修改official_sender.go来迎合无法验证的接口。64项直接自有Go测试、vet及五项30秒fuzz重新通过，本轮不使用overlay或替代发送器。

作者自审检查：没有任何go源码或go.mod/go.sum差异；预检禁止GOENV/workspace/overlay/checksum例外污染；只在验证真实下载后允许本SDK的tidy；未修改原始方案和342个历史证据/规范文件。脚本PASS不等于OB-001/SDK发布完成；公网模块仍需真正取得，服务器/消费者/扫描继续阻塞。没有独立审查者。

Ruling R2-001：新增OB-001实施工具而不改变运行时接口；当前失败源为执行环境取包能力，不为绕过它去重写已验证模块。代价：仍需在能够取得原始模块的隔离环境执行后续步骤。
Ruling R2-002：预检固定GOTOOLCHAIN=local并记录实际工具链，不自动追踪可能变化的工具链。取得上游go.mod后显式选择核验过的工具链；不能据本地1.23.2猜测上游下限。
Ruling R2-003：AC-002由本轮PASS候选收紧至PARTIAL，保留R1补充验证记录。其公开Go类型没有变动，但当前官方依赖不可用，不把旧补充模式的编译结果签成新正式证据。
