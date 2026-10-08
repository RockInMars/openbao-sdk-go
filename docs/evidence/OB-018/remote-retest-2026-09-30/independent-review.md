# sdk-test 第八轮复测独立只读审查（2026-09-30）

基线为 `master` / HEAD `6d8251658e76f88aec5935126f303b2c59497691` 加保留的未提交修改，source_identity v2 `e9d371d3a35385803a651db667a05cbd8fb3bb6cd36b03b23bf24ffd6139f14e`。独立 `risk_reviewer` 静态审查了本次 Python/Go 全场景范围白名单、一次性夹具，以及执行后主/子报告和收据；未修改文件、运行测试、连接服务器或读取凭据值。

执行前未发现阻断项：夹具、`scripts/remote-test.py`、`tests/remote/config.go` 及对应测试的新前缀、run ID 与路径一致；上一轮及诊断报告的预期 SHA256、精确计数、资源 `removed` 和清理 PASS 均匹配。新运行目录在启动前不存在。夹具仍先做实时身份与精确路径能力检查；120 次总请求上限、Core/Transit 的 43/32 次子上限与清理预留仍受约束，异常结果会停止或记录不确定清理。

执行后未发现与 PASS 矛盾的证据：主报告 SHA256 为 `5fbec9f73c7ad8aa6bcdcfb9cfeff0e9b73e859c875752bc63cbfd67fd14cfb2`，状态/清理为 PASS、请求计数 EXACT，45 次管理请求加 40/27 次子请求为 112/120；33 个步骤均 PASS，9 项父夹具资源均标记 `removed`。Core 39/39、Transit 22/22 个案均 PASS，分类计数合计分别为 40/27；主/子报告、结果和日志 SHA256 匹配，相关进程自然退出 0，前后及当前重算源码指纹相同。Core 中两条证书为 `revoked_record_retained`，随后父夹具删除了同一前缀 PKI mount，报告记录 HTTP 204。

证据：[主报告](../../../../.artifacts/runs/remote-full-fixture-20260930-08/report.json)、[Core 报告](../../../../.artifacts/runs/remote-sdk-full-20260930-07-core/remote/report-004.json)、[Transit 报告](../../../../.artifacts/runs/remote-sdk-full-20260930-07-transit/remote/report-004.json)。此审查只核对报告与代码证据，**未在清理后独立连接服务器查询资源是否不存在**；结论仅覆盖本次非生产 `sdk-test` 专用夹具，不替代正式独立集成或发布批准。20 分钟控制依赖运行时剩余时间和清理预留，极端文件写入延迟等情况仍可能令清理不完整，必须按报告状态处理。
