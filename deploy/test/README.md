# 真实 OpenBao 临时测试环境

**仅支持本次启动并拥有的新建 local/dev/test 实例；不能连接现有地址。**

两种输入之一：

```sh
# 由可信来源核验真实发布物后填入，下面变量不是已验证的值。
export BAO_TEST_VERSION="$VERIFIED_SERVER_VERSION"
export BAO_TEST_BINARY="/absolute/path/to/verified/bao"
export BAO_TEST_BINARY_SHA256="$VERIFIED_BINARY_SHA256"
make integration-test
```

或设置 `BAO_TEST_VERSION` 和 `BAO_TEST_IMAGE=完整镜像@sha256:经过核验的64位摘要`，不同时设置binary选项。不得用浮动latest或伪造checksum。环境变量BAO_TEST_ADDRESS/BAO_TEST_TOKEN被拒绝；禁止远程Docker上下文。

脚本内部生成临时HTTPS CA/证书，在127.0.0.1动态端口启动inmem实例，校验实际版本，初始化并解封，创建sdk-a/sdk-b Namespace和最小权限fixture。管理员只用于fixture准备；断言使用runtime/control/maintenance身份，以及自定义sdk-role AppRole。

临时PKI、Transit角色/密钥和权限不复用于生产。控制身份没有终端KV私钥读取权限；unknown crypto key不得upsert。45秒真实AppRole测试覆盖短租期续期与重新认证；集成测试整体5分钟上限。

所有秘密在临时内存或0700目录/0600文件中；服务器原始stdout/stderr不进入测试报告。Close只清理本次实例/容器和临时目录，失败仍执行清理。`compose.yaml`是参考拓扑，自动执行器使用受控native process或docker run，不依赖用户已运行的Compose。

可审查HCL示例在policies/，自动fixture权限由 `internal/testenv/policies.go` 定义并在单测中检查范围。修改一处需同步另一处。本次只对环境校验器和权限生成逻辑运行了单测，没有真实服务测试通过证据。
