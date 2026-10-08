# 临时 Go 环境的遥测清理竞态

候选 e60e 的 Linux [完整 tooling 报告](../../../../.artifacts/tooling-release-linux-20261001-final03.json)真实失败：155 个测试中，`test_real_go_rejects_unsigned_synthetic_sumdb_without_network_fallback` 在 `TemporaryDirectory.__exit__` 删除目录时报 `OSError: [Errno 39] Directory not empty`。Go 命令已自然返回，未签名 sumdb 拒绝和不回网断言未失败。该结果未改写为 PASS，也未忽略清理错误。

[只读现场收据](../../../../.artifacts/release-validation-20261001/telemetry-residual-01.json)显示残留目录只有 `scratch/home/.config/go/telemetry/` 下的 local、upload、Go 1.26.8 counter 和 weekends，重建时间与清理错误一致。核验归档中的 Go 1.25.0 和 1.26.8 `src/cmd/go/main.go` 均包含 `cmdIsGoTelemetryOff`：普通命令可启动独立遥测子进程，`go telemetry off` 则跳过该启动路径。结合残留文件，这是本次清理竞争的原因；没有将业务断言失败归咎环境。

修复使用 [Go 官方支持的关闭命令](https://go.dev/doc/telemetry)，不把只读的 `GOTELEMETRY` 当作可写环境变量。依赖导出、离线验证和内部重放的每个 scratch HOME 都先经既有 `run_go` 记录 `go telemetry off`，使用互不覆盖的 `export-telemetry-off`、`verify-telemetry-off`、`replay-telemetry-off` 日志。`APPDATA` 和 `XDG_CONFIG_HOME` 只指向该 scratch 的 config；全局 Go 配置不变。准备失败立即阻止后续依赖操作和目录发布，签名校验、无网络回退和严格清理保持原要求。

[定向 RED](../../../../.artifacts/release-validation-20261001/telemetry-preparation-red-01.json)在实现前运行了 19 个测试，真实退出 1：配置隔离缺失及三个入口顺序共 3 FAIL、1 ERROR。[GREEN](../../../../.artifacts/release-validation-20261001/telemetry-preparation-green-01.json)在 d318 候选上运行同一组 19 个测试，真实退出 0、无 skip；真实 Go 负例还核对 `go env GOTELEMETRY` 为 off。完整矩阵使用最终当前报告，不能以这组定向测试代替。

独立 `risk_reviewer` 要求把修复覆盖到三个同类短生命周期入口，随后复核实现、失败闭锁与测试断言，未发现确定回归。e60e 的完整失败、RED 以及后续完整运行均保留。原现场残留由最终本轮专用容器回收，不修改共享服务或用户目录。
