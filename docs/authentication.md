# 认证与生命周期

## 外部 Token

`auth.NewStaticToken(sensitive.Bytes)` 或 `auth.NewTokenFile(path)` 提供 TokenProvider。外部 Token 的续期由调用者/Agent 管理，SDK 不同时争抢续期；每次请求使用新的完整快照。文件原子替换后下一次请求读取新内容；缺失、空、非法或过期时拒绝，不无期限复用旧凭据。

Token 文件是受信任部署文件，建议权限 0600、父目录受保护。读取限制 64 KiB，拒绝控制字符。不在配置日志中显示内容。ValidUntil 为零表示提供者未声明期限，并非服务端保证永不失效。

## 快照所有权

`TokenProvider` / `SecretIDProvider` 的既有方法和快照字段保持不变。SDK 默认借用自定义 provider 返回的 `sensitive.Bytes`，只清理自己 `RevealCopy` 得到的副本；provider 保留句柄的管理责任，并保证有效期与并发安全。结构体或 `Bytes` 的值复制共享擦除句柄，不能据此推断可以 `Zero`。

明确交付独立快照的 provider 可额外实现 `SnapshotOwnedByConsumer(actual any) bool`。SDK 在原有 panic 防护边界内、获取快照之前调用该方法，并传入实际 provider；返回 true 表示此次调用返回的敏感句柄（包括与 error 同时返回的值）交给消费者，在成功、校验失败、错误或取消后清理。查询必须快速返回；每次交付的句柄不得与 provider 自身、其他调用或其他消费者共享。返回 false 或没有该方法时维持借用行为。

三个内建 provider 每次成功返回独立快照，并且只有 `actual` 是同一个内建实例时才交付所有权。第三方包装者即使提升了该方法，也不会自动把自己改写的返回值交付 SDK；若包装者明确交付，须自行实现方法并保证上述不变量。直接调用内建 `Snapshot` / `Current` 的使用者负责 `Zero` 返回值。Client 关闭不会擦除可能由多个 Client 共享的 provider；`NewStaticToken` 内部持有的长期副本随 provider 生命周期保留。

## ManagedAppRole

`auth.Config{Mode: auth.ManagedAppRole, AppRole: &auth.AppRoleConfig{Mount, RoleID, SecretIDProvider}}`。RoleID 也使用敏感包装。SecretIDProvider 提供 Generation 与 Use（ReusableSecretID/SingleUseSecretID）；一次性 generation 在请求发起前登记，即使响应未知也不会自动再次消费同一 generation。

服务端真实 TTL 减去请求耗时用于期限计算，55%–65%剩余租期进行续期/重新认证。失败退避、singleflight、到期失败关闭；认证不使用业务并发槽。支持正租期、无限使用次数的 Token；缺失租期、空 Token、有限使用次数不发布成可用状态。

到达最大租期后，旧快照过期与重新登录之间可能有短暂不可用窗口，此时 `State.Ready=false`，获取凭据返回 `authentication_failed`。SDK 不承诺该交界处每次业务请求都成功，也不会继续使用已过期快照；后续成功登录会恢复可用状态。真实集成检查到期拒绝、最终恢复及 namespace 隔离，不通过重放业务写入掩盖这个窗口。

SDK 不从业务 403 推断认证失效，不重放业务写入。后端 ACL 撤权可能早于本地租期，State.Ready 仅表达本地可用快照，不代表远端权限检查。

## 生命周期

`New` 校验并构造；`Start(serviceCtx)` 登录并启动必要生命周期；`Close(shutdownCtx)` 停止新请求，关闭认证循环，在关闭预算内等已准入请求，然后取消剩余调用并释放资源。重复 Close 幂等。首次 Start 的服务 Context 一旦取消，不通过再次 Start 掩盖取消或延长该 Client 生命周期；构造新 Client 才能开始新服务生命周期。

`New` 完成配置校验后才克隆 RoleID 和内存 TLS 私钥；后续构造失败统一清理这些 SDK 自有副本，成功时由 Client 持有至关闭。原始输入句柄始终由调用方管理。这里的清理不覆盖 TLS 已解析私钥、HTTP/Go 运行时可能保留的其他副本，不承诺绝对内存擦除。

Close 不撤销外部 Token，不吊销证书或删除 KV，也不承诺调用方提供的 Observer/Provider 永不阻塞；扩展接口必须遵守 Context 和快速返回约定。
