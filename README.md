# openbao-sdk-go

**实施快照，尚未通过发布门禁。不是已发布的 v1.0 SDK。**

这是依据 `docs/spec/01–06` 实施的 Go 运行时 SDK。包含 Core/Auth、KV v2、PKI、Transit、Diagnostics/Observe 的真实实现、测试、示例和验证脚本。管理 API 不在运行时范围。

当前环境可以执行 Go 1.23.2、Git、GCC、Python；不能下载官方模块，没有经过校验的 OpenBao 二进制/镜像。**正常构建和真实集成未通过**。`go.sum` 留空，绝不使用伪造的依赖或校验和。源代码交付不代表生产可用。阅读 `IMPLEMENTATION_REPORT.md`、`acceptance-results.json` 和 `docs/implementation-handoff.md` 后继续。

## 接入与功能文档（R3-D1）

- [SDK 接入说明](docs/sdk-integration-guide.md)：模块引用、认证与配置、完整只读接入示例、权限和错误恢复。
- [功能手册与调用示例](docs/sdk-feature-manual.md)：Core/Auth、KV v2、PKI、Transit、诊断与观察的真实公开接口。
- [首次使用指南](docs/first-use-guide.md)：现有四类示例的环境变量、操作顺序、副作用与预期结果。
- [完整文档导航](docs/README.md)。

以下 R1 段落是历史结果；最新软件状态以 [R3报告](IMPLEMENTATION_REPORT_R3.md)、`task-status.json` 与 `acceptance-results.json` 为准。R3-D1 只增加文档，未解除 OB-001、未改变验收状态，不是新软件发布。已知示例/权限差异见功能手册。

## 续作 R1

在原交付快照上修复关闭、严格PEM、KV计划删除和JSON Unicode四类问题。当前64个直接测试、118个补充契约测试（互有重叠）、5项30秒fuzz通过；正式官方编译/真实集成/独立消费/扫描仍未通过。详见`IMPLEMENTATION_REPORT.md`。当前恢复目录没有`.git`；来源和差异通过原归档与源码指纹核对。

## 模块与基线

- 暂定 module：`git.example.com/infra/openbao-sdk-go`。它是明确的占位仓库域名，没有创建或发布远端仓库。
- 声明依赖候选：`github.com/openbao/openbao/api/v2 v2.7.0`。已读取该标签部分官方源代码，未获取完整模块/go.mod/checksum。
- `go 1.23.2` 只表示当前自有代码的实际测试工具链，**不是已核实的官方依赖最低版本**。OB-001 必须先在可获取依赖的环境核验并更新基线。
- 此处所有接入示例待正式模块地址、依赖和环境验证后使用。不要将示例版本误认为发布记录。

## 边界

每个 Client 固定地址、Namespace 和身份。`New` 不联网；`Start` 使用服务生命周期 Context；`Close` 使用独立的关闭预算，不撤销外部 Token，不吊销证书、不删除秘密。所有请求由统一执行器管理预算、认证快照和错误语义。

SDK 不包含 Gin/Fiber/Gorm/数据库、MQTT、Webhook、游艇或支付业务。业务引用必须保存准确 KV 版本；签发和保存不是一个事务。

## 接入形态

```go
package service

import (
    "context"
    "errors"
    "time"
    bao "git.example.com/infra/openbao-sdk-go"
    "git.example.com/infra/openbao-sdk-go/auth"
)

func Run(ctx context.Context, address, caFile, tokenFile string) (err error) {
    provider, err := auth.NewTokenFile(tokenFile)
    if err != nil { return err }
    client, err := bao.New(bao.Config{
        Address: address,
        ClusterAlias: "configured-cluster",
        Namespace: bao.NamespaceConfig{Mode: bao.NamespaceRoot},
        TLS: bao.TLSConfig{CAFile: caFile},
        Auth: auth.Config{Mode: auth.ExternalToken, TokenProvider: provider},
    })
    if err != nil { return err }
    defer func() {
        shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer cancel()
        err = errors.Join(err, client.Close(shutdown))
    }()
    if err = client.Start(ctx); err != nil { return err }
    <-ctx.Done()
    return ctx.Err()
}
```

named Namespace 必须使用 `Mode: bao.NamespaceNamed, Path: "已明确的路径"`。不根据 `xyouting-test` 这个名称猜测部署。完整运行示例在 `examples/`，输入约定见 `examples/README.md`。

## 功能入口

| 入口 | 能力 |
|---|---|
| `Client.KVv2(mount)` | Create、CAS、ReadVersion/ReadLatest/ReadRef、Metadata、List、显式软删除/恢复 |
| `Client.PKI(mount)` | Issue、SignCSR、ReadCertificate、ReadIssuerChain、Revoke |
| `pki.ValidateBundle` | 独立可信根、密钥匹配、EKU/SAN、有效期校验 |
| `Client.Transit(mount)` | Sign/SignDigest、Verify/VerifyDigest、准确公钥/元数据、Encrypt/Decrypt/Rewrap、HMAC/HMACVerify |
| `Client.ClusterHealth` / `CheckReady` | 集群状态与受限身份准确版本探测，语义独立 |
| `bao.WithObserver` | 同步、应当迅速返回的脱敏事件钩子；调用方不得阻塞 |

公开签名完整定义保存在 `docs/spec/02-接口契约.md`，由 `public_contract_test.go` 固定。

## 验证命令

```sh
# 首先核验下载得到的真实版本与 go.mod，不能将下载失败跳过。
GOWORK=off go mod download
make tooling-test
make normal-test
make fuzz-test
make integration-test
make consumer-test
make security-test
make release-check
```

`make normal-test` 运行未修改 SDK 的 test/race/vet/mod verify/build 与覆盖率检查。真实集成必须提供经校验的**新建本地实例**配置，见 `deploy/test/README.md`；禁止传入外部服务地址或 Token。消费者通过临时 module proxy，而不是本地 replace。

### 明确区分的补充测试

```sh
make contract-test
```

该目标仅通过 Go overlay 替换本仓库的 `official_sender.go` 工厂，接入本地 TLS 测试发送器；**没有伪造 `github.com/openbao/openbao/api/v2` 模块**。除这一测试工厂外的自有 SDK 代码会实际运行。该结果不是官方客户端编译、真实 OpenBao、发布或业务接入证明。Go 1.23 overlay 对自动 vet/根包覆盖率存在限制，因此该目标明确 `-vet=off`；正常门禁从不使用这个选项或 overlay。

## 项目结构

`internal/engine`：传输、响应限制、统一预算、重试、错误；`internal/authn`：凭据生命周期；`sensitive`/`kv`：秘密与无损 JSON；`pki`/`internal/pkiutil`：本地证书校验；`internal/transitutil`：公钥与版本编码；`internal/pemutil`：拒绝跳过损坏块的PEM解码。根包包含功能门面。`internal/testenv`、`tests/integration` 仅由测试使用。

## 交付与审核

`task-status.json` 是唯一任务台账；`acceptance-results.json` 是验收观察结果，不修改原始矩阵。`docs/evidence` 保存真实执行，包括失败和中断。阅读 `docs/testing.md` 理解其证据等级；发布前必须重新执行当前源码的正常门禁并复核所有 BLOCKED/PARTIAL。

许可证未由项目权利人指定；本交付不擅自授予开源许可或代用户选择许可证。第三方依赖的原许可证仍然有效。远端发布前由权利人确定授权并补充适当文件。
