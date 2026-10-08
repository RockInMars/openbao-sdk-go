# 可执行示例与业务恢复示例

包内 `ExampleNew`、`sensitive.ExampleBytes`、`kv.ExampleDocument` 与 `baoerr.ExampleHasUnknownOutcome` 是确定性、无网络的 GoDoc 示例，随 `go test ./...` 执行。它们展示生命周期、共享擦除句柄、文档所有权及 UNKNOWN 的处理责任，不证明任何真实服务权限。下面的 `examples/*` 命令仍需显式配置和执行，可能产生服务端副作用。

示例入口使用明确的SDK_*环境变量，由示例配置层构造Config，SDK本体不读取环境。共同输入：

- SDK_BAO_ADDRESS：HTTPS地址。
- SDK_BAO_CLUSTER：受信任集群别名。
- SDK_BAO_NAMESPACE_MODE：root或named；named时另设SDK_BAO_NAMESPACE。
- SDK_BAO_TOKEN_FILE：受保护的Token文件；可选SDK_BAO_CA_FILE。

`kv-versioned`：额外提供挂载和唯一秘密路径，创建一份非生产演示文档、准确Ref读取和CAS；会产生真实测试写入，不能运行到生产。

`pki-issue-store`：额外提供PKI/KV挂载、PKI角色、唯一generation/path、operation_id、CN、业务证书可信根文件。真实签发→ValidateBundle→整套材料KV写入→准确版本核对。保存失败不能重新运行整个签发命令来当作重试；应在业务持久任务中恢复。同一脚本没有跨进程恢复能力，生产编排参见独立workflow。

`transit-sign`：明确提供已存在密钥、准确正整数版本和受支持profile，演示签名、验签、公钥读取。未创建任何密钥。

`service-bootstrap`：建立客户端、以服务Context运行，收到结束信号时用新关闭Context清理。

`observer`：额外传 `-mount`、`-path`、`-version`，仅准确版本读取一次；输出六个固定操作/错误计数桶，不输出秘密或请求标识。内部 `bootstrap.Run(action, opts ...bao.Option)` 透传 `WithObserver`，原有 `Run(action)` 调用兼容。详见 [观察接口](../docs/observability.md)。

每个 main.go 内的 SDK_* 名称为确切输入契约。配置错误返回非零，输出不包含秘密。运行前阅读源码确认写入范围；本轮示例验证使用正式 SDK 工厂和本地 TLS fixture，尚未证明真实 OpenBao 调用。正式证据以 [交接](../docs/implementation-handoff.md) 为准。

`credentialworkflow`是调用层而非SDKruntime：Issuer/Store/Journal由业务注入。Journal.Claim必须是持久化原子占位，不能用单进程mutex冒充跨副本幂等。Find只能在明确确认缺失时返回ErrNotStored，不能把OpenBao 404/not_found_or_hidden直接当不存在。测试覆盖保存后关联失败恢复、写响应丢失核对、未知签发不补发、代次不符、nil存储回执和注册前两类凭据有效性。


## R3-D1 接入文档

按实际 main 输入编写的逐步说明见 [首次使用指南](../docs/first-use-guide.md)；通用配置、生命周期和只读接入见 [SDK 接入说明](../docs/sdk-integration-guide.md)，参数与结果见 [功能手册](../docs/sdk-feature-manual.md)。

注意：`service-bootstrap` 的 started 不代表远端 ACL 已验证；`pki-issue-store` 是一次性示例，不具备完整材料持久化和跨进程恢复能力。HMAC 的测试权限模板后缀与当前验证请求的既有差异仍在功能手册列出；本轮没有修改 ACL。
