# Go 模块任务文档

本目录中的 `Mxx` 只是从原 ET 项目继承的审计编号，不是 Go 的 package
拆分方式。Go 主文档仍按运行时、消息路径、服务流程、领域状态和数据边界
组织：

- [项目总览](../01-project-overview.md)
- [Go 工程架构与运行边界](../02-go-package-architecture.md)
- [消息与网络](../04-messaging-and-network.md)
- [服务流程](../05-service-workflows.md)
- [协议参考](../08-protocol-and-api-reference.md)
- [实现状态](../IMPLEMENTATION_STATUS.md)

任务文档只回答三件事：

1. 目标 ET/C# 行为在 Go 中由哪些运行对象承担；
2. 当前源码、注册点、测试和配置是否已经闭环；
3. 哪些结论依赖 MongoDB、真实客户端、双进程或部署配置，不能由单测
   冒充完成。

状态使用“已实现”“自动化已验证”“真实环境待验收”“明确范围边界”，
不把缺少外部证据写成成功，也不为缺失协议添加 fallback。

当前验收入口：

- [M06-login](./M06-login/03-tasks.md)
- [M07-central](./M07-central/03-tasks.md)
- [M17-http](./M17-http/03-tasks.md)
- [M18-router](./M18-router/03-tasks.md)
