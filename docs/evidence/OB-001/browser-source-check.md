# 浏览器查阅与未取得的材料

2026-09-28 只读核验。未将页面文本伪装成可校验的Go模块归档。

- 可读取：https://raw.githubusercontent.com/openbao/openbao/api/v2.7.0/api/client.go 。检查NewConfig/DisableEnvironment/MaxRetries/DisableRedirects、SetClientTimeout、一次性Namespace配置、RawRequest入口与错误读取行为。
- 未取得：https://raw.githubusercontent.com/openbao/openbao/api/v2.7.0/api/go.mod 及Go proxy的v2.7.0.mod/info/zip。容器go mod download实际DNS失败，日志另存。
- 未取得真实服务端发布物/checksum或镜像digest；不能以版本名称补齐证据。
- 固定工具版本的主来源：https://pkg.go.dev/golang.org/x/vuln@v1.1.4/cmd/govulncheck 、https://github.com/gitleaks/gitleaks/releases/tag/v8.24.3 及该标签go.mod。运行时仍需要下载，失败不能说扫描完成。
- CI action固定提交从对应官方release的commit链接取得：https://github.com/actions/checkout/commit/11bd71901bbe5b1630ceea73d27597364c9af683 和 https://github.com/actions/setup-go/commit/d35c59abb061a4a6fb18e82ac0862c26744d6ab5 。这是固定版本选择，不是最新性声明；没有运行远端CI。
