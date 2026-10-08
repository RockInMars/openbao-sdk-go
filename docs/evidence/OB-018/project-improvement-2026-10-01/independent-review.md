# 本轮独立审查与契约核对

主代理根据独立 `risk_reviewer`、`contract_guard` 的实际报告整理。工作区为 `E:\xen\code\claude\project\jiupiao\openbao-sdk-go`，`master` / HEAD `6d8251658e76f88aec5935126f303b2c59497691`，包括保留的原有未提交修改；HEAD 不是完整审查基线。最终候选 v2 为 `8a1b5313ccda18142539b6cc167ee987d1f07fed1368662970fd09a5897d628f`。阶段检查在各自冻结范围完成，后续只影响其他文件或注释的改动不被冒充为新的全仓审查。

| 范围与阶段基线 | 独立结论 | 证据边界 |
| --- | --- | --- |
| 构造失败副本清理，`093a7fbd…` | risk_reviewer 未发现所审范围内可证实缺陷 | 校验后克隆；失败仅清理本次 SDK 副本；调用方与成功客户端的所有权保留。未执行测试。 |
| Provider 所有权，`2df6cb68…`；最低 Go 作业与证据，`fb9fa3ef…` | provider 变更后契约静态 PASS；risk_reviewer 未发现擦错第三方秘密或工具链门禁替代问题 | 自定义 provider 默认借用；可选交付协议核对实际实例，包装器不会继承内建所有权。主/最低版本和平台证据分别核验。未证明最低 Go 实际兼容。 |
| 新 manifest、Python 夹具、Go 消费方，`846b34e8…` | risk_reviewer 未发现确定的新缺陷；contract_guard 静态项 PASS，真实运行 BLOCKED | 约束、额度、期限、原子创建冲突、归属、子收据与清理核对；没有访问目标服务或执行远端测试。 |
| 扫描来源、状态摘要与 CI 消费链，`846b34e8…` | 发现一项 findings 摘要校验遗漏，见下文 | 原始报告与导出附件的完整性缺口，不是已发生的秘密泄漏。 |
| findings 修复，`529549a6…` | risk_reviewer 定点确认原问题关闭，未发现直接回归 | 后续到最终候选仅修正两处测试输入，所审生产实现未变；运行结论由 verifier 提供。 |

## 已关闭发现：gitleaks 附件摘要未参与验收

原 `ci_export.py` 为 `count-only-v1` 附件记录摘要，`evidence.py` 却只读取 JSON 是否为空数组；改变附件字节而保留原报告摘要仍可能通过。主代理增加原始 `findings_sha256`，本地校验和打包前均核对它；CI 校验导出类型、原摘要关联及导出文件摘要，之后仍要求没有未解决 findings。脱敏不再绕过附件完整性检查。

新增回归的 [RED 收据](../../../../.artifacts/improvement-20261001/findings-findings-red-01.json)记录 6 个测试实际执行、3 个新增方法失败，子测试展开共 6 次断言失败，退出 1；没有导入或语法错误。首次 [GREEN 尝试](../../../../.artifacts/improvement-20261001/findings-findings-green-01.json)仍失败两项：测试把原本的 `[]` 重写为相同字节，没有制造篡改。将输入修正为追加换行、保留原断言后，[最终 GREEN](../../../../.artifacts/improvement-20261001/findings-findings-green-02.json)为 6/6 PASS、退出 0；[完整工具测试](../../../../.artifacts/improvement-20261001/tooling-final-tooling-02.json)为 152 项 PASS、退出 0。所有首次失败保留，不合并为一次通过。

## 仍须由运行条件保证的边界

远端身份 GET 与资源 DELETE 不是原子操作；维护者必须保证本轮前缀独占。资源替换、归属不明、期限届满或子进程结果不明时保留待处理记录。AppRole 登录派生 Token 只有在专用 auth mount 成功禁用后才算撤销，依据为 [OpenBao auth disable](https://openbao.org/docs/commands/auth/disable/)与已核对的 2.6.2/2.6.3 `RevokePrefix` 路径；`Client.Close` 不承担服务器端撤销。

没有新一轮远端授权或运行证据，旧 120 请求/20 分钟窗口不能结转。扫描器实际输出、真实漏洞数据库、可信服务发布物及最低 Go/Linux 环境尚未取得本轮完整运行证据。静态审查、模拟测试、正式集成、发布批准是分别记录的结论；此文件不授予发布批准。
