# 2026-09-29 独立审查记录

主代理依据 `/root/gate_review` 的实际只读审查及复核消息整理。本记录不是执行测试的收据，也不是发布批准。审查者角色为 `risk_reviewer`，未修改源码、测试或文档，未再委派。

审查基线为 master / `6d8251658e76f88aec5935126f303b2c59497691` 上的实际未提交差异，包含新增文件，不能仅用 HEAD 代表。先审查工具、CI、门禁，再核对模块迁移、Observer、四个 benchmark 和六个回归测试文件；后续逐项核对修复增量。最后增量为离线消费者路径及测试，最终对应 v2 `cd46dc83078962d602886183bba7aa4a77f60702df5a4a167a4340f529fe3af2`。

| 成立问题 | 影响 | 修复和复核 |
|---|---|---|
| 成功命令可替代必需命令 | 仅有退出 0 可使错误目标通过 | 完整 argv、cwd、目标测试及外层/内层收据一致性校验；RED/GREEN 后只读闭环 |
| CI 上传原始日志 | 条件性秘密泄漏 | 白名单结构化导出，原始摘要和导出摘要分开；原始日志仅留本地/job |
| 动态 Test/Package 名称 | 任意诊断或秘密可混入导出 | 固定源码/规则选择器，动态子项收敛到父项，未知名称固定失败；不得回显原值 |
| A→B→A 源码变化 | 最后恢复可能清除失败 | `baseline_changed` 锁存，报告与消费端均拒绝 |
| 子进程树回收不能确认 | 父进程退出可能掩盖子进程遗留 | `cleanup_incomplete` 明确失败并停止后续命令；只清理本次 PID/组 |
| 集成摘要只检查格式 | 与冻结锁不一致的版本可能通过 | 报告匹配 PINNED 锁；CI artifact 锁与当前 checkout 字节一致 |
| Go 版本探测未进入门禁 | 失败探测或手写版本可被接受 | 独立自然成功的 go-version 收据、日志和工具版本相互核对 |
| 空/null Test 字段 | 畸形事件可能伪装无测试包 | 按字段存在判断，拒绝空值/非字符串；真实 no-test-files 仅在无 Test 字段时识别 |
| 离线代理解析成 UNC | 本地链接可将检查带到网络共享 | resolve 后、任何目录探测前拒绝 UNC 及带主机名的 file URL；负向测试证明不触及目录 |

前八项的红绿证据分别在 `.artifacts/runs/review-red/`、`review-green/`、`normal-parser-red/`、`normal-parser-green/`；UNC 回归见 `.artifacts/runs/consumer-offline-red/offline-resolved-share-offline-03.json` 和对应 `consumer-offline-green/offline-resolved-share-offline-04.json`。最终全部工具回归见 `.artifacts/runs/final-tools/tooling-windows-05.json`。

修复后的限定只读复核未发现剩余成立问题。实际 Go 行为由独立 verifier 的命令收据证明，不能用本审查代替。未进行真实 OpenBao、固定安全扫描器、Linux、最低 Go 或远端 CI 验证，也未审查/执行真实业务迁移。
