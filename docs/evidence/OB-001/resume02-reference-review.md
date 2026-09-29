# R2 官方客户端资料核对（仅源码文档核对，非编译结果）

核对日期：2026-09-28。

- 包索引：`https://pkg.go.dev/github.com/openbao/openbao/api/v2` 显示 v2.7.0，并有 NewConfig、NewClient、NewRequest、RawRequestWithContext 文档。
- 精确 tag 源码：`https://raw.githubusercontent.com/openbao/openbao/api/v2.7.0/api/client.go`。
- 浏览器未取得同 tag 的完整 api/go.mod；执行环境下载该文件和模块均失败。

以下源代码行号是浏览器读取到的原始文件行号，不是本地完整模块校验结果：

| 位置 | 静态观察 | 本 SDK 对应行为 |
|---|---|---|
| client.go 322–352 | NewConfig 默认重试为2，DisableEnvironment=true | 工厂显式把MaxRetries置0，使用NewConfig |
| client.go 707–776 | NewClient 在DisableEnvironment=true时不导入环境Token/Namespace | 保持显式DisableEnvironment=true |
| client.go 1311–1358 | NewRequest 初始化ClientToken并复制headers | 每请求填入Token快照，不变更共享客户端身份 |
| client.go 1404–1411 | RawRequestWithContext应用客户端Namespace到请求 | 工厂构造时固定Namespace |
| client.go 1463–1485 | RetryMax来自配置，重定向另有DisableRedirects判断 | 双层禁重定向、MaxRetries=0仍待实际编译和wire测试 |

结论：没有从上述静态资料确定必须修改 official_sender.go 的公开调用签名。本轮不猜测依赖结构、不修改该文件、不制造替代依赖来编译。`go.mod`最低Go要求、完整模块Sum/GoModSum、真实执行请求次数与官方适配器编译均未通过。
