# sdk-test 全功能补充测试请求预算（静态复核）

状态：**静态预算与本地模拟；不是全场景远端运行证据**。目标限定为 `https://kms.jiup9.com:443` 的 `sdk-test`，资源名限定于 `sdk-codex-20260930-727a8eec-*`。用户批准本轮总计 120 次/20 分钟；首轮注入 Token 的身份自查已实际消耗 1 次请求并返回 403、零资源，因此 AppRole 续跑夹具最多 119 次。此前[只读连接](../../../../.artifacts/runs/remote-connect-20260930-02/report.json)观察到服务自报版本 2.6.2，但不能替代可信二进制摘要。

| 阶段 | 成功路径估计 | 代码限制 | 说明 |
| --- | ---: | ---: | --- |
| 已执行身份预检 | 1 | 1 | 首轮 403，未创建任何资源；保留不可覆盖的失败报告 |
| AppRole bootstrap | 1 | 1 | 仅向用户确认的 `sdk-test` auth mount 登录；登录前登记令牌待创建，不记录 RoleID、SecretID 或返回 Token |
| 管理预检与资源创建 | 28 | 28 | 身份/能力各 1；4 个 mount 的创建及 accessor 读取各 4；CA、PKI role、Transit config 各 1；3 个 key、1 次轮换、2 个 CAS=0 seed；2 个 policy 各原子创建/读回；AppRole role/role-id/secret-id 各 1；2 个受限 Token |
| SDK core | 40 | 内部 41、外层预留 42 | 基础健康/身份/能力/指定版本/Ready 为 5；额外 KV 读 4；隔离 KV 创建、CAS、软删恢复及所有权清理约 14；受限身份 2；基础 Transit 7；PKI CSR/吊销 2；AppRole login/Ready 2；PKI Issue/读证书/读链/吊销 4；外层一次余量覆盖后台续期与业务请求的并发 |
| SDK transit | 27 | 32 | 同样的基础只读 5；密钥读取与 Encrypt/Rewrap/Decrypt/Sign/Verify/SignDigest/VerifyDigest/ReadPublicKey/HMAC/HMACVerify 共约 22 |
| 管理资源回收 | 15 | 15 | 两个运行 Token 吊销；两个 policy、auth mount、三个 secrets mount 分别先读回内容 hash/accessor 再删除；最后调用 `revoke-self` 撤销 bootstrap Token |
| 合计 | 112 | 119 | 本轮尚余批准预算 119 次，静态最坏上限 118 次，留 1 次余量；SDK 各轮分别低于原单轮 60 次上限。失败路径可能少于或不同于成功路径，实际请求数只从运行收据与 Observer 报告确认。 |

`tests/remote/runner.go` 对 core/transit 分别设置内部 41/32 次上限，AppRole 登录与续期现经同一 Observer 计数；`scripts/remote-test.py` 独立按目标集合和外层 42/32 次预留验收。续跑 fixture 在**首笔写入前**为两轮上限和清理预留预算，运行中超过剩余 119 次立即停止。子进程收据或结果缺失时请求总数标为 UNKNOWN，仅保留下界/上界，不冒充精确计数。Go 读取重试为 1。脚本不自动扩大预算，默认 60 会在远端写入前拒绝。一次性 fixture 的本地模拟覆盖预算拒绝、UNKNOWN、身份变更拒删、子进程超时和环境变量白名单；它不证明真实服务器会按此估计应答。

ACL policy 采用 OpenBao 官方 `cas=-1` 的原子仅创建及 `cas_required=true`，不依赖预读空结果。创建后与删除前读取并比对内容、version、modified 和 cas_required，mount/auth mount 比对 accessor。删除请求本身没有本轮可用的条件删除，因此仍需本轮专用前缀无其他写入者这一协作前提；本地模拟不能排除此类并发替换。AppRole 登录会新签发 Token，续跑夹具将其作为资源登记，成功回收以最后的 `revoke-self` 自然退出收据为准；未知登录结果保留残留风险，绝不标为清理通过。

证据：`tests/remote/config_test.go` 的 core/transit 精确权限测试、`scripts/tests/test_remote_runner.py` 的独立目标与范围测试、`.artifacts/runs/remote-full-fixture-20260930-02/dryrun_test.py` 本地模拟。远端结果如运行，应另写不可变 run_id 报告，不改本表的“静态估算”性质。
