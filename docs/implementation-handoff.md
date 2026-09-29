# openbao-sdk-go 实施交接 — R3

当前工作目录：`/mnt/data/openbao-r3/openbao-sdk-go`。当前源码指纹：`459e4262e9d1665e9ff6018fda0797046daf361b55a2052c230903c0c80e3399`。

最早阻塞仍为 OB-001。本轮在 R2 原源码上增加公共依赖转移工具，未修改 98 个 Go 文件、go.mod、go.sum、公开签名、原始方案或任务依赖。

## 下一条可执行命令

**在能够取得公共 Go 依赖的隔离开发机、使用本交付包源码执行：**

```bash
cd openbao-sdk-go
make dependency-export
```

成功才产生 `.artifacts/openbao-public-dependencies.zip` 和对应 `.sha256`。需要将它们通过可信通道回传；不是上传整个个人缓存，也不需要生产 Token 或私钥。当前本会话未产生这两个真实文件。

接收后按 `docs/dependency-transfer.md` 运行 `scripts/dependency-bundle.py verify`，真实 Go 从新的空缓存和本地 file proxy 独立认证，不预装传入 go.sum。再以验证后的代理执行 `make dependency-check` 和 `make normal-test`。固定工具链不足时只依据真实上游材料切换，禁止猜版本、auto 下载或替换官方模块。

## 本轮验证

- 35 项 Python 工具测试通过，其中 18 项新增；只证明转移/拒绝逻辑。
- 64 个自有 Go 顶层 Test 函数通过，race 与对应 vet 通过；另外运行到 6 个 fuzz 种子入口，不与 Test 数量混加。
- 原五个指定 fuzz 目标分别运行 30 秒并取得退出码 0。初次 path 包装调用和初次 direct/consumer 包装调用的中断日志保留，不计作正常通过。
- 正式 normal-test 返回 2（其内部七条 Go 命令各返回 1）；官方依赖未取得。
- integration/consumer/security 入口均返回 2，真实服务、消费与扫描主体未通过。
- 原 72 项验收：8 PASS / 59 PARTIAL / 5 NOT_RUN；任务：0 VERIFIED / 14 IN_PROGRESS / 5 BLOCKED。工具测试不升级 SDK 验收。

## 下一阶段待解决

获得真实依赖后优先运行官方适配器路径，并用失败测试修复实际问题。OpenBao 二进制/镜像、扫描工具以及固定 Go 工具链不包含在本转移包中，另需核验。`consumer-test.py` 当前固定追加网络代理，独立消费者的纯离线运行还需后续修正并验证，不承诺此包已解除 OB-017。OB-019继续缺真实项目源码。

原始方案在 `docs/spec/`；唯一台账 `task-status.json`；验收 `acceptance-results.json`；本轮证据在 `docs/evidence/OB-001/resume03/`。全新隔离目录没有 .git，未重建 Git 历史、提交、push 或发布；审查为作者自审。无会话结束后的后台任务。
