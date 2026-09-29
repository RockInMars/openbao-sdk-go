# 续作 R1 外部资料核对（不是集成证据）

核对日期：2026-09-28。

## KV v2 自动删除时间

官方资料： https://openbao.org/docs/secrets/kv/kv-v2/

“Key metadata”示例展示配置delete-version-after后，新写版本的deletion_time晚于created_time。该资料支持区分计划删除和已经删除。本轮本地回归确认原SDK把未来时间当作已删除；修改见ADR R020。不能据网页资料声称已核验指定服务版本或完成真实OpenBao测试。

## Go JSON UTF-16转义

官方资料： https://pkg.go.dev/encoding/json

Unmarshal说明指出不合法UTF-16代理对可被替换为U+FFFD。该兼容行为不是标准库缺陷，但不满足本SDK原始JSON入口的无损要求。本轮首先在实际Go1.23.2环境复现，再在jsondoc前置校验中拒绝；合法Unicode用例不变。修改见ADR R021。

## 官方客户端候选

此前浏览器阅读的api/v2@v2.7.0部分client.go仍不等于完整模块、go.mod、校验和或可供编译的依赖。本轮Go下载命令仍失败；OB001保持阻塞。未以主仓库main分支代替候选模块基线。
