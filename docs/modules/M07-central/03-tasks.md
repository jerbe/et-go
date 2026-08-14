# M07-central：账号验证与玩家档案

> `M07` 是原 ET 的兼容审计编号；Go 代码按 Central 运行对象和数据边界
> 组织在 `module/central`、`db`、`cmd/server`。

## 目标边界

目标项目证据：

- `cn.etetet.central/Scripts/Hotfix/Server/R2Central_AccountLoginHandler.cs`
- `cn.etetet.central/Scripts/Hotfix/Server/FiberInit_Central.cs`
- `cn.etetet.central/Proto/CentralInner_S_50000.proto`

Go 责任：

```text
Central Scene
  → MessageSender/MailBox
  → AccountStore
  → 密码校验与迁移
  → AccessToken 生成

Gate G2Game_Login
  → PlayerProfileStore
  → (account_id, zone_id) 唯一档案
  → PlayerID
```

## 任务状态

| 任务 | Go 实现 | 验证 |
|---|---|---|
| 保持 `50001/50002` 和请求响应字段 | `messages.go`、`proto/central_inner.proto`、`proto_codec.go` | `handler_r2central_account_login_test.go` |
| DB 账号查询、重复账号显式报错 | `dbAccountStore.FindByUsername` | Store 测试；Mongo 集成入口 |
| 新密码使用 Argon2id | `password_helper.go`、HTTP 注册 | `TestHashAndVerifyArgon2id`、HTTP 注册测试 |
| 旧 MD5 读取并在首次成功登录升级 | `VerifyPassword` + `UpdatePassword` | `TestHandleAccountLoginUpgradesLegacyMD5` |
| 登录成功生成配置化 AccessToken | `HandleAccountLoginWithStore` | signed/legacy Token 测试 |
| 缺少 DB/Store 不切换到内存 Profile | `accountStoreFromScene`、`loadOrCreatePlayerProfile` | 缺依赖错误测试 |
| 玩家档案按账号和 Zone 唯一 | `uniquePlayerProfile`、Mongo migration v2 | 重复档案测试、migration 定义测试 |
| Mongo 写入更新不覆盖 immutable `_id` | `db.Save` 更新字段过滤 | `TestSaveUpdateFieldsExcludesID` |
| Mongo 全局递增语义 | `db.Increment` 先 `$setOnInsert`、再独立 `$inc` | `DBComponentCRUDAndIncrement` integration；避免同路径 update conflict |
| Token 撤销/审计/限流索引 | Mongo migration v4/v5/v6 | migration 定义测试；真实 MongoDB 未验收 |

## 与目标 MD5 行为的关系

目标 C# 直接对请求密码做 MD5，再查询 `PasswordHash`。Go 保持输入字段和
消息编号不变，但不把 MD5 作为新账号存储算法：

- `password_algorithm=argon2id`：使用 Argon2id 校验；
- `password_algorithm=md5` 或历史数据未标记：按旧 MD5 校验；
- 旧密码首次成功登录时生成 Argon2id 并持久化；
- 不支持的算法、损坏的 Argon2id 编码和非法 MD5 长度直接返回错误。

这是数据存储安全升级，不是 wire 协议差异。旧账号需要真实 MongoDB
数据验证；没有 `ETGO_MONGO_URI` 时集成测试只能明确跳过。

## 错误边界

- 空请求、空 Store、DB Manager 缺失：返回明确错误；
- 用户不存在或密码错误：返回 `100009002`，不泄露账号是否存在；
- 重复档案：返回 `ErrPlayerProfileDuplicate`；
- 密码升级写失败：返回 `ErrPasswordUpgradeFailed`，不生成 Token；
- Token 配置缺失：返回 `login.ErrTokenConfigRequired`；
- DB 查询、插入、递增失败：向上层传播，不创建内存替代数据。

## 验证命令

```bash
go test -count=1 ./module/central ./db
go test -tags=integration -v ./db -run 'Test(DBComponentCRUDAndIncrement|RunMigrationsSerializesOwners)Integration'
```

第二条命令需要 `ETGO_MONGO_URI`；未设置时的 `SKIP` 不是 MongoDB 通过。
