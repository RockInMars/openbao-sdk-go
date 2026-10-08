# Security

当前工程门禁及实际验证范围见[当前验证状态](docs/current-status.md)。私密安全报告请发送至维护者确认的邮箱 [git@whereisit.cc](mailto:git@whereisit.cc)。支持版本策略和响应时限尚待维护者确认；不要在公开 issue 中附 Token、SecretID、私钥、秘密正文或可利用细节。

## 运行边界

只提供显式 HTTPS 配置并验证 CA 与服务名，默认不读取 BAO/VAULT/代理环境变量。开发 HTTP 仅能通过专用测试构造器访问字面量 loopback IP，禁止以关闭 TLS 验证解决错误。

每个 Client 的地址/Namespace/身份不可变。业务必须在传入路径、role、密钥名之前完成租户、用户、终端授权。OpenBao ACL 是第二层边界，不替代业务身份认证。

敏感字节和 KV Document 默认 fmt/slog 脱敏、JSON 序列化报错。RevealCopy/RevealJSON 是明确的秘密出口；不得记录其结果。Zero 是尽力擦除自有缓冲，不承诺擦除 Go 字符串、GC/TLS/HTTP 栈副本、swap 或 core dump。涉及高强度内存防护的部署应额外限制 core dump、调试和主机访问。

构造失败会清理 SDK 已创建的 RoleID / TLS PEM 私钥副本，调用方原始值保持其所有权。已解析 TLS 私钥与运行时副本的存留不由此保证。Provider 快照只有在显式交付协议成立时由 SDK 清理；未知自定义 provider 默认借用，关闭 Client 不擦除共享 provider。`NewStaticToken` 的长期内部副本随 provider 生命周期保留。具体约定见[认证与快照所有权](docs/authentication.md)。

默认写入、签发、crypto、认证操作单次；写响应丢失保留 UNKNOWN。不能把 SDK 的一次尝试等同服务端 exactly-once，也不能由客户端保证代理或下游没有隐藏重试。禁止自动重登后重放业务 403。

配置文件路径属于受信任部署输入，应位于受权限保护的目录，不允许不受信用户替换为链接、设备或管道。SDK 对普通文件、大小、密钥配对进行校验，不提供面向敌对目录的完整文件系统沙箱。

PKI Issue 不持久化终端私钥。业务层需保存完整材料并记录准确版本引用。Transit 平台私钥保持不导出，运行时没有管理、创建、轮换、删除/销毁入口。

## 验证

`make security-test` 固定使用 govulncheck v1.1.4 和 gitleaks v8.24.3（其实际 Go module 为 github.com/zricethezav/gitleaks/v8）。缺工具、下载或漏洞数据库失败不能判为无漏洞。历史下载失败与本轮环境/授权阻塞分别保留；当前检查结论以[自动摘要](docs/current-status.md)所绑定的实际报告为准。

集成 fixture 只启动自身拥有的临时 loopback 实例；CA/解封材料/管理员 Token 不写入 Git 或报告。管理员只初始化 fixture，业务断言使用受限身份；退出只清理自己创建的资源。
