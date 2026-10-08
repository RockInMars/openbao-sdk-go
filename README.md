# openbao-sdk-go

**Go 运行时 SDK；版本以仓库 Git 标签为准，当前工程门禁结果以自动生成的验证状态为准。**

这是依据 `docs/spec/01–06` 实施的 Go 运行时 SDK。包含 Core/Auth、KV v2、PKI、Transit、Diagnostics/Observe 的真实实现、测试、示例和验证脚本。管理 API 不在运行时范围。

准确源码标识、当前报告校验和发布拒绝统一见[当前验证状态](docs/current-status.md)，该页由原有台账及证据生成。具体范围、首次失败与续作见[实施交接](docs/implementation-handoff.md)。历史非生产 `sdk-test` 结果独立于正式 `REAL_OPENBAO`；源码变化后不能沿用旧 PASS。模块地址与 Apache-2.0 许可已由维护者确认；独立消费者支持从本地签名代理在全新缓存中认证依赖。

## 接入与功能文档（R3-D1）

- [SDK 接入说明](docs/sdk-integration-guide.md)：模块引用、认证与配置、完整只读接入示例、权限和错误恢复。
- [功能手册与调用示例](docs/sdk-feature-manual.md)：Core/Auth、KV v2、PKI、Transit、诊断与观察的真实公开接口。
- [首次使用指南](docs/first-use-guide.md)：现有五类示例的环境变量、操作顺序、副作用与预期结果。
- [完整文档导航](docs/README.md)。

以下 R1 段落及 [R3报告](IMPLEMENTATION_REPORT_R3.md) 保留原始历史结论。当前状态以现有台账和实施交接为准；历史通过记录不自动归属本轮源码。

## 续作 R1

在原交付快照上修复关闭、严格PEM、KV计划删除和JSON Unicode四类问题。当前64个直接测试、118个补充契约测试（互有重叠）、5项30秒fuzz通过；正式官方编译/真实集成/独立消费/扫描仍未通过。详见`IMPLEMENTATION_REPORT.md`。当前恢复目录没有`.git`；来源和差异通过原归档与源码指纹核对。

## 模块与基线

- 正式 module：`github.com/RockInMars/openbao-sdk-go`，保留 `RockInMars` 大小写。此工作区未提交或推送到该仓库，未验证远端能够获取本轮快照。
- 固定官方依赖：`github.com/openbao/openbao/api/v2 v2.7.0`；本轮未增加生产依赖。
- `go.mod` 声明下限为 Go 1.25.0；工程主验证工具链固定为 Go 1.26.8。最低版本、平台和主版本的证据分别校验，见 [兼容矩阵](docs/compatibility.md)。
- Apache-2.0 许可见 [LICENSE](LICENSE)。仓库地址及许可确定不代表已有本轮发布版本。

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
    bao "github.com/RockInMars/openbao-sdk-go"
    "github.com/RockInMars/openbao-sdk-go/auth"
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
python -B -m unittest discover -s scripts/tests -v
python -B scripts/normal-test.py
python -B scripts/fuzz-test.py
python -B scripts/benchmark-test.py
python -B scripts/integration-test.py
python -B scripts/consumer-test.py
python -B scripts/security-test.py
python -B scripts/verify-release.py
```

本地按全局规则为上述命令加 `rtk proxy`。依赖准备在源码冻结前单独进行；正式普通检查默认不下载依赖。消费者可用 `--offline-proxy <已准备的本地代理目录>`，在全新缓存中认证公共依赖；没有离线材料时，消费者与扫描器只有获得下载/工具执行授权后才传 `--allow-network`。真实集成必须固定版本和 digest，并仅新建自己的临时实例，见 [测试说明](docs/testing.md)。五目标 fuzz 保留各自收据；benchmark 不新增发布阈值。

### 历史补充测试入口（本轮未使用）

```sh
make contract-test
```

该目标仅通过 Go overlay 替换本仓库的 `official_sender.go` 工厂，接入本地 TLS 测试发送器；**没有伪造 `github.com/openbao/openbao/api/v2` 模块**。除这一测试工厂外的自有 SDK 代码会实际运行。该结果不是官方客户端编译、真实 OpenBao、发布或业务接入证明。Go 1.23 overlay 对自动 vet/根包覆盖率存在限制，因此该目标明确 `-vet=off`；正常门禁从不使用这个选项或 overlay。

## 项目结构

根包 `bao` 保留 `Client` 生命周期和 KV、PKI、Transit、Diagnostics 功能门面；`auth`、`kv`、`pki`、`transit`、`sensitive`、`baoerr`、`diagnostics`、`observe` 提供公开类型与配套能力。包边界与依赖方向见[总体设计](docs/spec/01-总体设计.md)。

`internal/engine` 负责请求预算、传输、响应限制、重试和错误；`internal/authn` 管理凭据生命周期；`internal/pkiutil`、`internal/transitutil`、`internal/pemutil` 分别处理证书、公钥/版本编码和严格 PEM 解码。`internal/testenv` 仅供测试使用。

`tests/integration` 是独立服务集成入口；`tests/remote` 是指定测试服务的受控场景入口。`scripts/` 顶层 `*-test.py` 和 `verify-release.py` 是检查命令，其余模块负责证据、记录和基础工具；运行范围与证据等级见[测试说明](docs/testing.md)。

## 交付与审核

`task-status.json` 是唯一任务台账；`acceptance-results.json` 是验收观察结果，不修改原始矩阵。`docs/evidence` 保存真实执行，包括失败和中断。阅读 `docs/testing.md` 理解其证据等级；发布前必须重新执行当前源码的正常门禁并复核所有 BLOCKED/PARTIAL。

本轮按维护者提供的现有仓库复用许可证，没有另选许可。没有自动 stage、commit、push、发布或生产迁移。
