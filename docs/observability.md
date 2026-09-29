# 观察接口

使用 bao.WithObserver 注入 observe.Observer。事件仅含操作、集群别名、挂载标签、耗时、SDK尝试次数、HTTP状态、错误码与受控RequestID。无 Token、私钥、KV正文、解密明文或上游原始错误。

Observer 同步调用，必须快速返回、不得阻塞，不得在回调里递归调用同一客户端或改变业务结果。SDK恢复回调panic且不因此把成功写入改成失败；不自动创建异步无界队列。接入外部 tracing/metrics 时由调用者做有界采样与低基数字段设计。

挂载标签不是秘密路径；不得自行把每个终端ID/序列号/完整路径添加为 metrics label。不要使用 fmt.Printf("%+v", RevealCopy())；显式揭示接口的返回值不再受包装保护。

ClusterHealth 无Token/Namespace并按健康专用状态解析；CheckReady 只执行传入的受限准确版本读。前者成功不等于后者有权访问。
