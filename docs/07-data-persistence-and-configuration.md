# 数据、持久化与启动配置

## 1. 配置加载模型

`config.Load(dir)` 读取：

```text
startmachineconfig.json   必需
startprocessconfig.json   必需
startsceneconfig.json     必需
startzoneconfig.json      必需
startareaconfig.json      可选
startsecurityconfig.json  可选；HTTP/Realm/Central/Router 进程实际必需
```

加载策略是严格的：

- 目录不存在：错误；
- 必需文件缺失：错误；
- JSON 解析失败：错误；
- 可选 Area 文件存在但格式错误：错误；
- 不把空配置当成功。

`Config.Validate(processID)` 检查：

- ID 为正；
- Machine/Process/Scene/Zone 引用存在；
- ID 不重复；
- SceneType、Scene Name 非空；
- SceneType 必须属于当前 Go runtime 支持的集合；未知类型在启动前拒绝，
  不创建空白 Fiber；
- `NetInner` Scene 和配置了 `peers` 的 Process 必须有正的 `innerPort`；
- 当前 Process 至少有一个 Scene；
- Map 的 `mapTargets` 只允许 Map 场景，当前最多一个目标；
- Map target 必须存在、是 Map、非自身、与源 Map 同一 Process；
- Zone Name、DBAddr、DBName 非空；
- Area ID/Name 合法。

## 2. 启动配置字段

### 2.1 Machine

```json
{
  "id": 1,
  "innerIP": "10.0.0.10",
  "outerIP": "203.0.113.10",
  "watcherPort": 9000
}
```

Machine 只描述机器地址和 watcher 端口，不等于 OS 主机名。

### 2.2 Process

```json
{
  "id": 1,
  "machineId": 1,
  "innerPort": 10000
}
```

Process ID 是逻辑进程 ID。它用于 ActorID 路由，不是 `os.Getpid()`。
`innerPort` 是本 Process 的 NetInner KCP 监听端口；当配置 `peers` 时必须为
正数。

跨进程 peer 配置示例：

```json
[
  {
    "id": 1,
    "machineId": 1,
    "innerPort": 10001,
    "peers": [
      {
        "processId": 2,
        "address": "10.0.0.12:10002",
        "secret": "replace-with-deployment-secret"
      }
    ]
  },
  {
    "id": 2,
    "machineId": 2,
    "innerPort": 10002,
    "peers": [
      {
        "processId": 1,
        "address": "10.0.0.11:10001",
        "secret": "replace-with-deployment-secret"
      }
    ]
  }
]
```

`peers` 的校验规则：

- ProcessID 必须存在、不能是自身、不能重复；
- address 和 secret 必须非空；
- 当前 Process 有 peer 时 `innerPort` 必须为正；
- 两端必须互相声明对方，并使用相同的 handshake secret；单侧配置或 secret
  不匹配时不能形成有效 peer；
- 较小 ProcessID 主动拨号，较大 ProcessID 只接受连接；
- secret 只用于当前 HMAC handshake；生产部署仍需独立密钥管理和 TLS
  方案，不能把共享 secret 当作完整传输安全。

有 peer 配置但没有显式 `NetInner` Scene 时，`cmd/server` 自动创建一个
NetInner Fiber；显式配置 NetInner 时则复用该 Scene。

### 2.3 Security

登录、账号和 Router 相关 Process 默认必须配置签名 Token key ring。配置文件
`startsecurityconfig.json` 使用对象而不是数组；如果必须兼容原 ET 客户端，
可以显式设置 `accessTokenFormat: "legacy"`，此时只生成旧 SimpleToken 格式：

```json
{
  "accessTokenFormat": "signed",
  "accessTokenCurrentKeyId": "primary-2026",
  "accessTokenKeys": [
    {
      "id": "primary-2026",
      "secret": "at-least-32-byte-secret-configured-by-secret-manager"
    },
    {
      "id": "previous-2025",
      "secret": "previous-at-least-32-byte-secret-for-rotation"
    }
  ],
  "legacyTokenKey": "old-key-only-during-migration",
  "allowLegacyTokens": true,
  "corsAllowedOrigins": [
    "https://client.example.com"
  ],
  "httpRequireTLS": true,
  "httpTLSCertFile": "/etc/et-go/tls/server.crt",
  "httpTLSKeyFile": "/etc/et-go/tls/server.key",
  "loginRateLimitPerMinute": 30
}
```

规则：

- `accessTokenCurrentKeyId` 必须命中 `accessTokenKeys`；
- 每个签名 secret 至少 32 bytes；
- `accessTokenFormat` 只能是 `signed` 或 `legacy`，省略时默认为 `signed`；
- 生成 Token 只使用 current key，验证允许 key ring 中的旧 key；
- `accessTokenFormat="legacy"` 时必须同时设置
  `allowLegacyTokens=true` 和 `legacyTokenKey`；生成格式才会切换为原 ET
  SimpleToken；
- `allowLegacyTokens=true` 时必须显式提供 `legacyTokenKey`，用于验证旧 Token；
- 旧 XOR Token 只用于显式迁移窗口，不是默认生产算法；
- `GenerateAccessToken`/`VerifyAccessToken` 在未安装安全配置时返回
  `login.ErrTokenConfigRequired`，不会隐式启用历史 key；
- HTTP Scene 必须配置非空、精确匹配的 `corsAllowedOrigins`，不接受 `*`；
- HTTP 已支持 `httpTLSCertFile`、`httpTLSKeyFile` 和 `httpRequireTLS`；
  `httpRequireTLS=true` 时证书或私钥缺失会直接拒绝启动；
- 每次新 TLS 握手都会重新读取证书和私钥文件；部署层必须以原子方式替换
  匹配的证书/私钥，文件损坏时新连接直接失败；
- 登录已支持 `loginRateLimitPerMinute`；生产 HTTP Fiber 使用 MongoDB 原子
  bucket 跨 Process 限流，独立测试可使用进程内固定窗口组件；
- AccessToken 已提供撤销存储接口、进程内 Memory 实现、MongoDB 共享适配器、
  TTL migration 和启动 wiring；真实 MongoDB 验收、限流故障策略、审计保留
  策略、密钥托管和生产验收仍未完成；
- `cmd/server` 发现当前 Process 使用 HTTP/Realm/Central/Router 但没有签名
  key ring，且 `accessTokenFormat` 不是 `legacy` 时直接拒绝启动。

### 2.4 Scene

```json
{
  "id": 9003,
  "processId": 1,
  "zone": 1,
  "sceneType": "Realm",
  "name": "Realm",
  "outerPort": 10003
}
```

Scene ID 是配置身份。Scene 根 Entity 仍有独立的 runtime `InstanceID`。

Map 场景可以额外配置：

```json
{
  "id": 1001,
  "processId": 1,
  "zone": 1,
  "sceneType": "Map",
  "name": "Map1",
  "outerPort": 0,
  "mapTargets": ["Map2"],
  "navMeshFile": "maps/map1/navmesh.bin"
}
```

- `mapTargets` 是目标 Map 的 Scene 名称，当前最多一个；
- `cmd/server` 在所有本进程 Map Fiber 创建成功后把目标 runtime ActorID 注入；
- 跨 Process target discovery 尚未实现，配置校验会直接拒绝；
- 配置了 `navMeshFile` 时，Map Fiber 初始化会立即调用
  `move.NewFinder`；没有注册 `move.RegisterFinderFactory`，或资源加载失败，
  启动直接失败；
- 没有配置 `navMeshFile` 的 Map 可以启动，但其 Unit 没有 Finder，寻路请求
  返回 `move.ErrFinderMissing`，不会直线兜底。

`outerPort` 的含义依赖场景：

- Realm/Gate/KCP：监听端口，也可能作为对外地址端口；
- Router/HTTP：HTTP、Router UDP 和 Router TCP 监听端口；
- 未设置正端口时可以在测试中使用 `0` 请求临时端口；
- `Config.Validate` 会拒绝网络场景同时缺少 `outerPort` 和
  `Process.InnerPort` 的配置；
- Process 启用 peer 或显式 NetInner 时，Realm/Gate/HTTP/Router/RouterNode
  等外部监听 Scene 必须显式配置 `outerPort`，不能回退并占用 NetInner 的
  `innerPort`；
- 生产对外 advertised address 必须是正端口；测试代码如需临时端口，
  必须显式绕过生产启动校验并主动传入 `:0`。

### 2.5 Zone

```json
{
  "id": 1,
  "name": "dev",
  "dbName": "etgo",
  "dbAddr": "mongodb://127.0.0.1:27017",
  "serverURL": "http://127.0.0.1:10000",
  "isLogic": true
}
```

Zone 同时决定：

- Scene 所属逻辑区；
- Location/Central/Map 的查找范围；
- DBManager 获取哪一个 MongoDB 数据库；
- Router 的逻辑区列表。

## 3. 地址解析

`network.ResolveSceneListenAddr(scene, preferInner)` 根据：

```text
Scene
  → StartSceneConfig
  → StartProcessConfig
  → StartMachineConfig
  → host + port
```

地址优先级：

- `preferInner=true`：InnerIP，其次 OuterIP；
- `preferInner=false`：OuterIP，其次 InnerIP；
- Scene `OuterPort` 优先；
- Scene 没有端口时使用 Process `InnerPort`；
- Machine 不存在或无地址：错误；
- Scene 配置找不到：错误。

代码里仍可看到 `host := ""` 和 `switch`，它们只是构造过程，不是 `127.0.0.1` 默认绑定。无地址时会返回 `ErrSceneMachineMissing`。

## 4. MongoDB 访问层

### 4.1 Client

`db.Client` 封装：

- MongoDB client；
- database；
- `Collection`；
- `Close`。

`db.New` 会连接并 Ping，连接失败直接返回错误。

### 4.2 DBManagerComponent

DBManager 按 Zone 缓存 DBComponent：

```text
GetZoneDB(zone)
  → 命中 zoneDbs：返回缓存
  → ensureConfig
  → 查找 Zone 配置
  → clientFactory
  → NewDBComponent
  → 缓存
```

缺少 config、Zone、DBAddr、DBName 或 Client 创建失败都会返回错误。

### 4.3 DBComponent

提供：

- `FindOne`；
- `Query`；
- `Insert`；
- `Save`；
- `Remove`；
- `Increment`。

写操作按实体 ID 或分桶 key 使用 CoroutineLock，避免同进程内并发写冲突。

`DBComponent` 没有把空 Client 当可用数据库；底层 collection 不存在时返回错误。

`Save` 先提取并校验实体 ID，调用方传入的 ID 与实体自身 ID 不一致时返回
错误。实体 BSON 会被转换为 `$set` 字段并移除 `_id`，避免 MongoDB 对
immutable `_id` 字段执行更新；实体序列化失败也不会继续写库。

`Increment` 遵循原 ET/C# 的两步语义：先用 upsert + `$setOnInsert` 确保默认
文档存在，再用单独的原子 `$inc` 返回递增后的值。不能在同一条 upsert 中
同时对同一路径使用 `$setOnInsert` 和 `$inc`，否则现代 MongoDB 会把它判定为
更新路径冲突。

## 5. 持久化模型

当前主要实体：

| 类型 | 集合/用途 |
|---|---|
| `CAccount` | 账号、用户名、password_hash |
| `CPlayerProfile` | 玩家档案、AccountID、ZoneID |
| `CIncrement` | 全局递增 ID |
| `BagComponent` | 背包 BSON snapshot |
| `WarehouseComponent` | 仓库 BSON snapshot |
| `Replay` | Lockstep 输入和快照 |

持久化实体实现 `db.IDBEntityCollection`，由 Map 的 `UnitDumperComponent` 定时保存。

同一 `(account_id, zone_id)` 查询到多条玩家档案时，Central 返回
`ErrPlayerProfileDuplicate`，不会任意使用第一条；v2 migration 的唯一索引
防止新数据继续产生该状态，历史重复数据仍需显式清理。

## 6. UnitDumper

```text
TimerComponent
  → UnitDumper.dump
  → DBManager.GetZoneDB
  → UnitComponent.GetAll
  → 遍历可持久化组件
  → 异步 DB.Save
```

Save 失败会记录集合、实体 ID 和错误。

当前仍有运营性问题：

- 定时保存没有事务；
- 多组件之间没有同一快照点；
- 保存前已在 Unit 所属 Fiber 内完成 BSON 序列化，并把不可变 snapshot
  交给异步 goroutine；这解决了异步任务继续读取 live Component 的竞态，
  但不等于多组件事务；
- UnitDumper 先把不可变快照写入 `unit_dump_queue`，再写业务集合，成功后
  删除队列记录；进程重启后的下一次 dump 会查询并重放未删除任务；
- MongoDB 网络故障会进行有限次 retry，失败快照进入当前进程内存
  dead-letter；dead-letter 尚未持久化；
- 多组件仍不是同一 MongoDB 事务快照。

这些不能由单纯的 `go test` 证明，属于持久化可靠性 TODO。

## 7. BSON 与跨 Map 迁移

Map 迁移拆为两部分：

```text
Unit snapshot
  → ID / ConfigID / UnitType / Position / Rotation

Transfer components
  → component type
  → component bytes
```

目标端：

1. 反序列化 Unit；
2. 通过 component factory 创建迁移组件；
3. 恢复 Bag/Numeric/Warehouse；
4. 重新创建 Move/AOI/MailBox；
5. 加入 Scene/UnitComponent；
6. AOI.Enter；
7. 通知客户端；
8. Location ownership 切换。

未知 component factory 返回 `ErrTransferComponentUnsupported`。不跳过、不产生部分成功。

## 8. 数据安全

当前已知风险：

1. 旧账号仍可能保留 MD5；成功登录时会升级为 Argon2id；
2. AccessToken 已使用配置化 HMAC key ring；旧 XOR 只在显式迁移配置下接受；
3. 配置中可能包含 MongoDB URI；
4. HTTP 默认仍可使用明文；生产必须显式配置 `httpRequireTLS` 和证书文件；
5. CORS 使用显式 allowlist；
6. MongoDB 原子 bucket 登录限流、Token 撤销接口、新连接 TLS 证书热轮换和
   MongoDB 共享撤销 wiring 已实现，但真实 MongoDB 验收、限流故障策略、
   密钥托管和审计存储仍未完成。

Realm 生成的 Gate Token 使用 `crypto/rand`，但保留原 C# wire format：
`D5.accountID.D4` 经 UTF-8 Hex 编码后交给客户端。随机数生成失败会向登录
流程返回错误，不注册空 Token 或伪造成功状态。

代码 TODO：

```text
TODO(security)
  → MongoDB 共享 Token 撤销的真实连接、故障恢复和多进程验收；
  → TLS 密钥托管、证书文件原子替换约束和生产验收；
  → 限流故障策略、审计保留策略和运营协议。
```

密码 Argon2id 和旧 MD5 首次登录升级已经实现；剩余 TODO 集中在
AccessToken、传输层和运营安全，不再把密码升级描述成未实现。

## 9. Migration

`db/migrations` 当前提供六个版本：

```text
1: account.username unique index
2: player_profile(account_id, zone_id) unique compound index
3: account.password_algorithm marker for legacy MD5/Argon2id
4: access_token_revocation.expires_at TTL index
5: login_audit.created_at query index
6: login_rate_limit_buckets.expires_at TTL index
```

nil Client 现在返回 `ErrClientRequired`，不会假装 migration 成功。

`db.RunMigrations` 已提供启动 runner：

```text
cmd/server.bootstrapServer
  → Config.Validate
  → 当前 Process 是否包含 DB 场景
  → 按 Zone db.New + Ping
  → db.RunMigrations(migrations.All())
  → 关闭 client
  → 创建 World/Fiber
```

`RunMigrationsWithOptions` 通过 MongoDB `schema_migration_locks` 实现多
Process lease lock；`RunMigrations` 使用默认租约参数。MongoDB integration
测试会并发启动两个 Client，要求同一 migration 的 `Up` 只执行一次。

当前 migration 记录在 `schema_migrations`，按正整数版本排序；版本名
冲突、migration 执行失败、写入版本记录失败都会阻止业务启动。migration
的 `Up` 必须幂等，因为进程可能在变更完成、版本记录写入前崩溃。

当前 runner 使用 `schema_migration_locks` 的 MongoDB 租约锁协调多个
Process：每次运行拥有随机 owner，执行期间由心跳续租，等待锁和数据库
操作都绑定调用方 context；租约丢失、取消或释放失败都会返回错误。不能
把锁丢失后的 migration 继续当作成功。

已注册版本：

```text
1: account_username_unique_index
   → account.username unique index
2: player_profile_account_zone_unique_index
   → player_profile.account_id + zone_id unique compound index
3: account_password_algorithm_marker
   → 为已有 password_hash 标记 md5 或 argon2id；不在 migration 中重算密码，
   旧 MD5 在首次成功登录时升级
4: access_token_revocation_expires_at_ttl
   → 为 Token 撤销记录创建 `expires_at` TTL index
5: login_audit_created_at_index
   → 为登录审计记录创建 `created_at` 查询索引，不擅自设置保留期限
6: login_rate_limit_bucket_expires_at_ttl
   → 为 MongoDB 登录限流 bucket 创建 `expires_at` TTL index
```

UnitDumper 已有 Fiber 内不可变 snapshot、durable queue、有限 Save 重试、
取消感知的带超时 shutdown drain 和有界内存 dead-letter；Map transfer 已有
source journal 和配置化 target durable ledger；仍未完成的持久化可靠性项目
是跨组件一致性事务、恢复扫描和 MongoDB 故障恢复。
它们不能被 migration runner 或单条 queue 记录代替。

## 10. 配置加载样例（不是完整业务运行配置）

下面的配置只用于说明四个必需 JSON 的目录结构和字段关系，不能保证完整登录、进图或切图链路可用。端口 `0` 表示测试代码显式请求临时端口，不适合对外发布。

目录布局：

```text
config/dev/
├── startmachineconfig.json
├── startprocessconfig.json
├── startsceneconfig.json
└── startzoneconfig.json
```

仓库当前的 `data/config/json/` 已提供一份可运行的本地 HTTP 部署配置，
数据库为 `etgo_dev`，监听 `127.0.0.1:18080`，并使用开发用途的 AccessToken
密钥。它用于验证启动、migration、账号注册/登录和 MongoDB 限流，不代表
完整 Realm/Gate/Map 游戏拓扑，也不能直接用于生产。

```json
// startmachineconfig.json
[
  {
    "id": 1,
    "innerIP": "127.0.0.1",
    "outerIP": "127.0.0.1"
  }
]
```

```json
// startprocessconfig.json
[
  {
    "id": 1,
    "machineId": 1,
    "innerPort": 0
  }
]
```

```json
// startsceneconfig.json
[
  {
    "id": 9003,
    "processId": 1,
    "zone": 1,
    "sceneType": "Realm",
    "name": "Realm",
    "outerPort": 0
  },
  {
    "id": 9004,
    "processId": 1,
    "zone": 1,
    "sceneType": "Gate",
    "name": "Gate",
    "outerPort": 0
  }
]
```

Map target 示例：

```json
[
  {
    "id": 1001,
    "processId": 1,
    "zone": 1,
    "sceneType": "Map",
    "name": "Map1",
    "outerPort": 0,
    "mapTargets": ["Map2"],
    "navMeshFile": "maps/map1/navmesh.bin"
  },
  {
    "id": 1002,
    "processId": 1,
    "zone": 1,
    "sceneType": "Map",
    "name": "Map2",
    "outerPort": 0,
    "mapTargets": ["Map1"],
    "navMeshFile": "maps/map2/navmesh.bin"
  }
]
```

```json
// startzoneconfig.json
[
  {
    "id": 1,
    "name": "dev",
    "dbName": "etgo",
    "dbAddr": "mongodb://127.0.0.1:27017"
  }
]
```

如果要启动真实业务服务，还必须按 Scene 类型补齐 Location、Central、Map、
Home Map、HTTP/Router 地址、Finder 工厂和 MongoDB。Lockstep 默认 Room Fiber
已经注入 Go `RoomStateSnapshot v2` provider，快照包含
`Room/FrameBuffer/RoomPlayer/LSWorld`；原 C# MemoryPack/TrueSync wire、
客户端恢复、replay/hash-mismatch 协议和跨版本迁移仍需后续定义与验收。
