# 认证与生命周期

## 外部 Token

`auth.NewStaticToken(sensitive.Bytes)` 或 `auth.NewTokenFile(path)` 提供 TokenProvider。外部 Token 的续期由调用者/Agent 管理，SDK 不同时争抢续期；每次请求使用新的完整快照。文件原子替换后下一次请求读取新内容；缺失、空、非法或过期时拒绝，不无期限复用旧凭据。

Token 文件是受信任部署文件，建议权限 0600、父目录受保护。读取限制 64 KiB，拒绝控制字符。不在配置日志中显示内容。ValidUntil 为零表示提供者未声明期限，并非服务端保证永不失效。

## ManagedAppRole

`auth.Config{Mode: auth.ManagedAppRole, AppRole: &auth.AppRoleConfig{Mount, RoleID, SecretIDProvider}}`。RoleID 也使用敏感包装。SecretIDProvider 提供 Generation 与 Use（ReusableSecretID/SingleUseSecretID）；一次性 generation 在请求发起前登记，即使响应未知也不会自动再次消费同一 generation。

服务端真实 TTL 减去请求耗时用于期限计算，55%–65%剩余租期进行续期/重新认证。失败退避、singleflight、到期失败关闭；认证不使用业务并发槽。支持正租期、无限使用次数的 Token；缺失租期、空 Token、有限使用次数不发布成可用状态。

SDK 不从业务 403 推断认证失效，不重放业务写入。后端 ACL 撤权可能早于本地租期，State.Ready 仅表达本地可用快照，不代表远端权限检查。

## 生命周期

`New` 校验并构造；`Start(serviceCtx)` 登录并启动必要生命周期；`Close(shutdownCtx)` 停止新请求，关闭认证循环，在关闭预算内等已准入请求，然后取消剩余调用并释放资源。重复 Close 幂等。首次 Start 的服务 Context 一旦取消，不通过再次 Start 掩盖取消或延长该 Client 生命周期；构造新 Client 才能开始新服务生命周期。

Close 不撤销外部 Token，不吊销证书或删除 KV，也不承诺调用方提供的 Observer/Provider 永不阻塞；扩展接口必须遵守 Context 和快速返回约定。
