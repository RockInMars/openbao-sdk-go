# 错误与恢复

公开 `baoerr.Error` 包含 Code、Operation、HTTPStatus、RequestID、Attempts、Effect、Message。Message 为固定脱敏说明；不透出上游原始错误正文或 URL。Context 取消与超时保留 errors.Is；使用 `baoerr.IsCode` 和 `baoerr.HasUnknownOutcome` 判断，不做字符串包含匹配。

- 本地参数/路径/配置或发送前取消：NONE。
- 已发送的写/签发/认证响应丢失、超时、模糊响应：UNKNOWN。
- 明确 API 参数/权限拒绝可以为 NONE；模糊代理对象不能当成同等证据。404 始终不代表业务资源确定不存在；写 404 未提供明确 API 拒绝时保留 UNKNOWN。
- KV CAS 仅匹配已确认的 400 错误形态；其他 400 不映射冲突。
- 验签 false 是调用成功的验证结果，网络故障返回 error，不当成 false吞掉。

同一个 Effect=UNKNOWN 可能需要业务日志、operation_id、resource_generation、精确 KV 版本和唯一任务所有权协作核对。SDK 不拥有数据库事务或补偿任务，也不补发证书。`examples/credentialworkflow` 展示 durable claim 和读取原引用恢复，但其 Journal/Store 的跨副本一致性必须由调用项目实现。

只读最多 3 次 SDK 尝试；受限 Retry-After 超出剩余预算则结束。重试、等待凭据/并发和读取响应都共享操作预算。线级尝试数与 SDK Attempts 不等价，代理必须自行禁用写请求隐藏重放。
