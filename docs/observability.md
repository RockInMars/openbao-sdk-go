# 观察接口

使用 bao.WithObserver 注入 observe.Observer。事件仅含操作、集群别名、挂载标签、耗时、SDK尝试次数、HTTP状态、错误码与受控RequestID。无 Token、私钥、KV正文、解密明文或上游原始错误。

Observer 同步调用，必须快速返回、不得阻塞，不得在回调里递归调用同一客户端或改变业务结果。SDK恢复回调panic且不因此把成功写入改成失败；不自动创建异步无界队列。接入外部 tracing/metrics 时由调用者做有界采样与低基数字段设计。

挂载标签不是秘密路径；不得自行把每个终端ID/序列号/完整路径添加为 metrics label。不要使用 fmt.Printf("%+v", RevealCopy())；显式揭示接口的返回值不再受包装保护。

ClusterHealth 无Token/Namespace并按健康专用状态解析；CheckReady 只执行传入的受限准确版本读。前者成功不等于后者有权访问。

最小可执行示例见 [examples/observer](../examples/observer/main.go)。完成公共 `SDK_BAO_*` 配置后，以明确的 `-mount`、`-path`、正整数 `-version` 执行一次 KV 只读；不会显示读取正文。回调只有原子加法，将所有操作归到 `kv_read_version/other`、错误归到 `success/permission_denied/error`，共六个固定桶。ClusterAlias、MountLabel、RequestID 和秘密均不进入输出或标签；输出发生在请求完成后。

```sh
go run ./examples/observer -mount kv -path fixture/item -version 1
```

此命令只可用于已授权的隔离实例。仓库测试通过自身 TLS fixture 检查参数透传和无选项旧调用兼容，并在 race 下并发验证计数。它不是实际 OpenBao 集成或生产性能承诺。性能基线命令及独立采样方法见 [测试说明](testing.md)。
