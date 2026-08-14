# 兼容性参考资料

本目录只保存原 ET/C# 行为、协议和迁移背景。它不是 Go 主架构文档。

主文档入口：

1. [Go 项目总览](../01-project-overview.md)
2. [Go 工程架构与运行边界](../02-go-package-architecture.md)
3. [服务流程](../05-service-workflows.md)
4. [实现状态](../IMPLEMENTATION_STATUS.md)

兼容性资料只能用于回答：

- 某个 MsgID/字段的历史含义；
- 客户端需要保持什么 wire 结构；
- 旧密码/Token/快照为什么不能直接替换；
- Go 行为与原实现的差异在哪里。

兼容性资料只说明 wire/行为意图，不证明 Go 生产部署、真实客户端、
真实 MongoDB migration 或压力验收已经完成。

不能用它来决定：

- Go 包应该如何组织；
- Fiber/Scene 应该如何初始化；
- 哪些服务已经配置启用；
- 缺少依赖时是否可以 fallback。

当兼容性资料和当前 Go 代码冲突时，文档必须同时写明：

```text
历史行为
当前 Go 行为
是否需要客户端/配置迁移
阻塞原因或 TODO
```
