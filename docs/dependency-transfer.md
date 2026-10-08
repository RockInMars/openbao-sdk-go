# OB-001 公共依赖转移：联网导出、离线复验（R3）

本工具实现 R2 交接中允许的“经校验的公共依赖离线材料”路径，不更改 SDK 接口、官方候选版本、安全语义或验收要求。

当前工具链与验证结果见[兼容性](compatibility.md)和[实施交接](implementation-handoff.md)；下方 R3 的失败与未验证记录保留历史含义。

依赖导出、离线验证和内部重放各使用独立临时 HOME、缓存和配置目录。`APPDATA` 与 `XDG_CONFIG_HOME` 指向该 scratch 的 `config`，不会继承个人配置。每个临时环境在首个普通 Go 命令之前，先通过既有日志入口执行 `go telemetry off`，分别记录 `export-telemetry-off`、`verify-telemetry-off`、`replay-telemetry-off`；准备失败即中止。该设置只作用于对应临时目录，避免短命令结束后的遥测子进程与目录清理竞争，不修改全局 Go 设置、不放宽清理或签名校验。命令含义见 [Go 官方遥测说明](https://go.dev/doc/telemetry)。

**R3 历史状态：工具本地测试通过；当时环境实际导出在 DNS 阶段失败，没有生成真实依赖包。完整官方依赖导出、签名材料正向重放及 SDK 正式编译均未验证。**

## 1. 在能联网的隔离开发机运行

使用本交付包的 SDK 源码目录。需要 Python 3.10+、Make、经过来源核验的固定 Go 工具链以及能访问公共 Go 模块代理和校验数据库的网络。官方候选仍为 `github.com/openbao/openbao/api/v2@v2.7.0`。R3 当时 SDK 内的 Go 1.23.2 不是已核实的上游最低版本；当前声明下限为 Go 1.25.0，主验证版本见兼容性表。

```bash
cd openbao-sdk-go
make dependency-export
```

默认输出（**仅真实导出及其离线重放全部成功后才创建**）：

```text
.artifacts/openbao-public-dependencies.zip
.artifacts/openbao-public-dependencies.zip.sha256
```

已有同名输出时拒绝覆盖。需要另一个输出位置或显式的可信无认证 HTTPS 模块代理时：

```bash
python3 scripts/dependency-bundle.py export \
  --output /absolute/new/path/openbao-public-dependencies.zip \
  --proxy https://proxy.golang.org
```

不接受 HTTP、带用户名密码/查询参数的代理、`direct` 回退或本地个人缓存。不会继承个人 HOME、netrc、BAO/VAULT Token、代理环境、workspace、overlay 或 checksum 例外。

导出使用临时 SDK 源码副本和全新 GOPATH/GOMODCACHE/GOCACHE。不会改变原 SDK 的 go.mod/go.sum，不会修改任何调用项目。其顺序是：读取固定工具链和根模块 → 真实下载精确官方候选 → 核对实际上游 go.mod → tidy、完整模块下载、模块图和 verify → 整理原始模块包与已签名校验记录 → 用第二套全新缓存及本地 file proxy 做 Go 重放 → 成功后原子发布转移 ZIP。

如 Go 报依赖要求更高工具链，先依据实际错误和取得的上游材料核实，安装经过校验的固定版本，再通过 PATH 切换本 SDK。工具始终使用 `GOTOOLCHAIN=local`，不自动升级工具链。

## 2. 回传哪些材料

仅回传成功生成的公共依赖 ZIP 和 SHA256 文件。它们包括原始 `.mod/.info/.zip`、Go 的已签名 checksum lookup/tile 数据、解析后的根 go.mod/go.sum 和源码输入指纹。**不包括个人缓存、凭据、私钥、SDK 源码正文、编译产物、服务端二进制或工具链。**

源码指纹只用于绑定对应 SDK 的 `.go` 与根模块输入。ZIP 哈希只证明传输完整性，不是第三方源码真实性证明；接收端仍必须通过 Go 的签名日志验证。校验文件应通过可信通道传递，不把“与 ZIP 一同附带”自动等同于可信发布者签名。

导出失败时不会创建空的或合成的“依赖包”。此时只提供打印出的 `report.json` 和相关失败日志；不能将测试 fixtures 当作可用依赖。

## 3. 在接收端使用真实 Go 独立复验

保留同一份 SDK `.go`、go.mod、go.sum 输入。源码指纹不匹配或固定 Go 版本不同会失败；不要修改清单绕过检查。

```bash
python3 scripts/dependency-bundle.py verify \
  --bundle /absolute/path/openbao-public-dependencies.zip \
  --sha256 "$(awk '{print $1}' /absolute/path/openbao-public-dependencies.zip.sha256)" \
  --destination /absolute/new/path/verified-public-deps
```

接收端先检查 ZIP 条目类型、路径、重复名、大小和文件哈希。随后只将解析后的 go.mod 放入临时验证模块，**不预装包内 go.sum 或 `.ziphash`，不复用个人模块缓存**。真实 Go 从本地 file proxy 读取原始模块和已签名 lookup/tile 数据，在 `GOSUMDB=sum.golang.org` 下重算并验证；缺少或损坏的校验材料必须失败。校验成功后才发布指定的新目录；不执行包中任何脚本。

该步骤即便通过，也只证明 `OFFLINE_DEPENDENCY_REPLAY`，不是 SDK 编译、OpenBao 兼容、扫描或发布通过。

## 4. 依赖复验成功后继续原流程

保持已核实的固定工具链，用新目录作为唯一模块代理：

```bash
export GOPROXY="file:///absolute/new/path/verified-public-deps/proxy"
make dependency-check
make normal-test
```

`dependency-check` 的 `--prepare` 会允许真实 Go 在本 SDK 中更新 go.mod/go.sum；审查其实际差异。禁止手填校验和、`replace` 官方模块、overlay 或改用替代发送器。

依赖通过后再按原计划准备临时 OpenBao 程序/固定 digest 镜像，运行集成和消费者验证。扫描器及 OpenBao 服务端不在此公共模块转移包内，不能因为取得 SDK 依赖而假定这些外部条件也已满足。

## 5. 本轮验证范围

本轮增加 18 项转移工具测试，与既有 17 项工具测试共同运行。覆盖：环境隔离、源输入范围、拒绝符号链接、路径与 ZIP 防护、篡改检测、拒绝覆盖、输入指纹绑定、失败不发包、拒绝 seeded go.sum/ziphash，以及真实 Go 对未签名合成 lookup 数据的拒绝。

这些合成 fixtures 只验证拒绝和传输逻辑，不代表任何 OpenBao 模块通过校验。真实依赖正向导出仍为 BLOCKED，离线正向复验为 NOT_RUN。

## 6. 资料依据

协议机制依据 Go 官方模块说明的 Module proxy protocol 与 Authenticating modules 章节：
https://go.dev/ref/mod

本轮也读取了本地固定 Go 1.23.2 的 `src/cmd/go/internal/modfetch/sumdb.go`，核对 file proxy 的 sumdb/supported、lookup、tile 处理路径。没有从网页片段重建官方 OpenBao 模块。

### 当前消费者入口的额外限制

本轮核对发现，现有 `scripts/consumer-test.py` 的临时 SDK 代理后面固定追加 `https://proxy.golang.org`，不会自动采用调用者提供的 file proxy。故本工具不承诺已经解除独立消费者的离线运行阻塞；先恢复 OB-001 与正式根包编译，后续再按真实消费者失败和测试修正该入口。本轮未修改消费者脚本，也未把其失败标为通过。
