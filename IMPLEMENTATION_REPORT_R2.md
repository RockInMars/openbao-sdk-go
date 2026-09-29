# OpenBao SDK 续作报告 R2

**结论：OB-001没有解除，SDK V1发布门禁未达到。**

本轮优先核对真实官方依赖，不扩展运行时功能。R1压缩包已恢复并检查CRC及源码指纹。98个Go文件、go.mod、go.sum均与R1逐字节一致，official_sender.go未修改。

## 1. OB-001实际尝试

`go mod download -json github.com/openbao/openbao/api/v2@v2.7.0` 返回1，错误发生在proxy.golang.org域名解析；Go proxy和GitHub的curl探测返回6。保留TLS验证的公共HTTPS DNS探测返回7。下载工具未取得真实模块文件。Codex注册环境列表为空。

浏览器能够查看包文档和精确tag的client.go，但没有取得完整api/go.mod、模块包、Sum或GoModSum。不能据此宣布候选依赖已经下载或编译通过。实际工具链Go1.23.2仍只是本地运行记录，上游最低Go版本未知。

对应证据：`docs/evidence/OB-001/resume02-*.json`、`resume02-reference-review.md`。

## 2. 实际新增的实现

| 文件 | 内容 |
|---|---|
| scripts/dependency-check.py | 真实下载与依赖基线预检；记录实际文件、go.mod最低版本、校验和、图与verify结果；任何失败非零退出 |
| scripts/tests/test_dependency_check.py | 8项新测试，覆盖下载错误、缺材料、缺checksum、错误pin、替换模块、空/错go.sum、JSON输出、环境污染 |
| Makefile | 增加make dependency-check |
| docs/dependency-bootstrap.md | 固定工具链、预检作用范围和实际恢复条件 |

工具测试使用显式标注的合成元数据fixture，仅证明工具解析和拒绝逻辑，既不写入真实go.sum，也不是官方模块通过的证据。预检实际运行到下载步骤即返回BLOCKED，后续tidy/verify没有执行。

预检禁止GOENV保存配置、workspace、overlay、checksum例外对取证的影响；固定当前PATH工具链，避免自动猜测升级。只允许在真实下载检查后由Go更新本SDK的go.mod/go.sum，不修改使用方仓库。

## 3. 本轮实际测试结果

| 验证 | 退出码 | 结果范围 |
|---|---:|---|
| make dependency-check | 2 | 子脚本1；真实下载步骤失败，预检BLOCKED |
| make normal-test | 2 | test/race/vet/mod verify/build/module-graph/test-list各返回1 |
| make integration-test | 2 | 缺来源核验的临时OpenBao程序或镜像，业务用例未运行 |
| make consumer-test | 2 | 两个独立消费者均在下载上游模块阶段失败 |
| make security-test | 2 | 两个固定扫描工具获取失败，扫描主体未运行 |
| make tooling-test | 0 | 17项工具测试通过，包括8项新增测试 |
| 直接自有Go包race测试 | 0 | 64个顶层Test函数通过，0失败/跳过；对应vet返回0 |
| 五项模糊测试 | 各0 | 每项30秒；不是仅运行种子 |
| make release-check | 2 | 正确保持不允许发布 |
| git diff --check | 129 | 没有.git，不冒充Git差异检查通过 |

本轮不运行overlay或替代发送器。直接自有包测试不能推导根包或官方适配器通过。正式全库编译在依赖阶段失败，故不能用其日志中的“0个测试失败”推导成功。

首次路径模糊测试被外层命令45秒期限终止，原日志标记INTERRUPTED；后续完整30秒重跑实际返回0，未删除中断记录。没有遗留进程。

## 4. 任务与验收

任务状态仍为：OB-001、016、017、018、019 BLOCKED；OB-002～015 IN_PROGRESS；0 VERIFIED。

72项当前验收：**8 PASS / 59 PARTIAL / 5 NOT_RUN**。PASS为AC-003～007、047、064、068；NOT_RUN为AC-001、065、067、071、072，其余PARTIAL。

AC-002从R1的PASS收紧为PARTIAL：公开类型代码没有变化，但本轮没有在官方依赖下编译根包，也不再用R1的补充编译结果充当当前通过证据。R1记录保留在各项prior_review中。其他通过项均核对本轮实际测试名字和退出结果。

task-status.json保留原19项依赖，acceptance-results.json保留原72项编号和观察要求，没有通过修改范围使状态变绿。

## 5. 官方源码核对范围

参考：
- https://pkg.go.dev/github.com/openbao/openbao/api/v2
- https://raw.githubusercontent.com/openbao/openbao/api/v2.7.0/api/client.go

观察NewConfig的默认重试、DisableEnvironment、NewClient环境分支、请求ClientToken、Namespace重写及重试/重定向配置。本轮仅静态核对这些文档，没有足够证据认定official_sender.go有必须立即修复的签名问题，更没有完整依赖来运行其失败测试，因此未修改它。

## 6. 交付与后续条件

本轮交付包含原SDK和新预检工具、全部历史证据、更新台账、72项验收、源码差异补丁及交接记录。原方案和历史证据保持不变。没有发布远端版本、接入生产、使用真实凭据或声称独立审查通过。

当前缺少的是**能够取得并校验真实公共Go依赖的执行环境**，而不是另一个SDK设计方案。下一步在该环境的R2 SDK目录执行`make dependency-check`，成功后继续`make normal-test`及真实服务、独立消费者、安全扫描门禁；详细要求见`docs/implementation-handoff.md`。
