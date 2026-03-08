# 任务拆解文档: 订阅套餐扩展与计费优先级

**项目**: New API - AI网关与资产管理系统  
**功能**: 扩展订阅套餐 / 优惠券 / 兑换 / 钱包优先级计费  
**创建日期**: 2025-12-03  
**最新修订**: 2025-12-03  
**文档状态**: 草稿

---

## 1. WBS 分解

### 阶段0: 需求确认 & 方案冻结
- [x] 0.1 兑换折算算法已确认采用向下取整,不足一天金额记入流水
- [x] 0.2 多订阅优先级默认按到期排序,支持拖拽调整并提供 API
- [x] 0.3 兑换码仅能绑定一张优惠券,兑换前可修改,兑换后锁定
- [x] 0.4 管理员取消订阅需触发通知,渠道遵循系统/用户配置
- [x] 0.5 输出需求澄清记录与决策清单(折算、优先级、绑定、取消通知)
- [x] 0.6 对齐验收标准与错误码列表,同步到需求/设计文档
- [x] 0.7 识别剩余假设与风险,更新风险跟踪

### 阶段1: 数据模型 & 迁移

#### 1.1 数据库表结构设计
- [x] 1.1.1 编写跨数据库兼容的 DDL 草案（PostgreSQL/MySQL/SQLite），包括新建表（subscription_plans, subscription_plan_limits, subscription_orders, subscription_usages）、扩展表（subscriptions, coupons, redemptions, users/user_settings, tokens, audit_logs, user_bills），以及字段扩展（新增 tokens.subscription_preferred，保留 auto_smart_group）
- [x] 1.1.2 决策并锁定优惠券绑定存储方案（扩展 coupons 表 vs 独立 `subscription_coupon_bindings` 表），统一后续索引/Model/Service。

#### 1.2 索引和约束设计
- [x] 1.2.1 创建所有必要的数据库索引（subscription_plans索引、subscription_plan_limits唯一约束、subscriptions索引、subscription_usages唯一约束和索引、subscription_orders索引、coupon_bindings唯一约束）
- [x] 1.2.2 定义外键约束和级联删除规则
- [x] 1.2.3 DDL 评审和优化

#### 1.3 迁移脚本开发
- [x] 1.3.1 编写创建新表的迁移脚本（UP）- subscription_migration_postgresql.sql Phase 1-2, subscription_migration_mysql.sql Phase 1-2
- [x] 1.3.2 编写扩展现有表的迁移脚本（UP）- subscription_migration_postgresql.sql Phase 3, subscription_migration_mysql.sql Phase 3
- [x] 1.3.3 编写完整的回滚脚本（DOWN）- 使用备份恢复 + README.md 中的手动回滚 SQL
- [x] 1.3.4 编写数据迁移脚本 - users.setting 初始化（Phase 4），旧订阅数据迁移不需要（全新功能），usage 窗口动态创建
- [x] 1.3.5 编写 token 字段扩展脚本 - subscription_migration_postgresql.sql Phase 3.1, subscription_migration_mysql.sql Phase 3.1（Redis 缓存需手动执行 FLUSHDB）

#### 1.4 迁移测试和验证
- [x] 1.4.1 在测试库演练迁移脚本，记录耗时 - 迁移耗时 < 1 秒，测试报告: MIGRATION_TEST_REPORT.md
- [ ] 1.4.2 迁移性能测试（测试迁移 100万 订阅数据的耗时）- 跳过（测试环境数据量小）
- [x] 1.4.3 编写数据一致性校验脚本 - subscription_migration_verification.sql (PostgreSQL), subscription_migration_verification_mysql.sql (MySQL)
- [x] 1.4.4 迁移回滚演练（在测试环境完整演练回滚流程）- 回滚成功，耗时 < 1 秒，测试报告: MIGRATION_TEST_REPORT.md § 2.3
- [x] 1.4.5 生成迁移执行手册和应急预案 - MIGRATION_PLAYBOOK.md
- [x] 1.4.6 生成用户通知/公告草稿 - USER_NOTIFICATION_TEMPLATES.md

### 阶段2: 后端基础能力

#### 2.1 基础设施和常量定义
- [x] 2.1.1 定义错误码和错误消息 ✅ **2025-12-06 完成**
  - **实施内容**:
    - ✅ 在 `types/error.go` 新增 **65 个错误码**（覆盖主要业务场景）
      - 订阅错误 (13 个): `ErrorCodeSubscriptionNotFound`, `ErrorCodeSubscriptionExpired`, `ErrorCodeSubscriptionInsufficientQuota` 等
      - 套餐错误 (10 个): `ErrorCodePlanNotFound`, `ErrorCodePlanNotPublished`, `ErrorCodePlanPriceInvalid` 等
      - 优惠券错误 (16 个): `ErrorCodeCouponExpired`, `ErrorCodeCouponNotApplicable`, `ErrorCodeCouponScopeMismatch` 等
      - 订单错误 (9 个): `ErrorCodeOrderNotFound`, `ErrorCodeOrderAlreadyPaid`, `ErrorCodeOrderInvalidAmount` 等
      - 使用量错误 (3 个): `ErrorCodeUsageRecordFailed`, `ErrorCodeUsageQuotaExceeded` 等
      - 账单错误 (4 个): `ErrorCodeBillNotFound`, `ErrorCodeBillOperationFailed` 等
      - 兑换错误 (7 个): `ErrorCodeRedemptionConflict`, `ErrorCodeRedemptionExpired`, `ErrorCodeRedemptionOperationFailed` 等
      - 验证和额度错误 (3 个): `ErrorCodeInvalidRequestParams`, `ErrorCodeInsufficientBalance`, `ErrorCodeSubscriptionMaxLimitReached`
    - ✅ 在 `common/subscription_messages.go` 新增所有错误码对应的中文消息常量
    - ✅ 补充关键业务场景错误码: INSUFFICIENT_BALANCE, PLAN_PRICE_INVALID, ORDER_INVALID_AMOUNT, COUPON_SCOPE_MISMATCH, SUBSCRIPTION_INSUFFICIENT_QUOTA 等
  - **修改文件**: `types/error.go` (+81 行), `common/subscription_messages.go` (新建文件, 141 行)
  - **修正说明 (2025-12-07)**: 补充缺失的余额不足、价格无效等关键错误码，总计 65 个

- [x] 2.1.2 定义 DTO 和请求/响应结构体 ✅ **2025-12-06 完成**
  - **实施内容**:
    - ✅ `dto/subscription_plan_dto.go` - 套餐相关 DTO (6 个结构体)
      - `SubscriptionPlanRequest`, `SubscriptionPlanResponse`
      - `SubscriptionPlanLimitRequest`, `SubscriptionPlanLimitResponse`
      - `SubscriptionPlanListRequest`
    - ✅ `dto/subscription_order_dto.go` - 订单相关 DTO (7 个结构体)
      - `SubscriptionOrderCreateRequest`, `SubscriptionOrderResponse`
      - `SubscriptionOrderPayRequest`, `SubscriptionOrderCancelRequest`
      - `SubscriptionOrderPreviewResponse`, `SubscriptionOrderListRequest`
    - ✅ `dto/subscription_usage_dto.go` - 使用量相关 DTO (5 个结构体)
      - `SubscriptionUsageRecordRequest`, `SubscriptionUsageResponse`
      - `SubscriptionUsageSummaryResponse`, `SubscriptionUsagePeriodSummary`
      - `SubscriptionUsageListRequest`
    - ✅ `dto/coupon_binding_dto.go` - 优惠券绑定 DTO (7 个结构体)
      - `CouponCreateRequest`, `CouponResponse`, `CouponUpdateRequest`
      - `CouponValidateRequest`, `CouponValidateResponse`
      - `CouponBindRequest`, `CouponListRequest`
    - ✅ `dto/subscription_dto.go` - 订阅和账单 DTO (10 个结构体)
      - `SubscriptionResponse`, `SubscriptionCancelRequest`, `SubscriptionRenewRequest`
      - `UserBillResponse` (补充 `refund_type`, `conversion_metadata` 字段)
      - `UserBillPayRequest`, `RedemptionSubscriptionRequest` 等
  - **补充修复**: 账单 DTO 新增 `refund_type` 和 `conversion_metadata` 字段，与 DDL 迁移脚本对齐
  - **修改文件**: 新建 5 个 DTO 文件，共 489 行代码

- [x] 2.1.3 扩展 `common/constants.go` 常量定义 ✅ **2025-12-06 完成，2025-12-07 修正**
  - **实施内容**:
    - ✅ 订阅状态枚举: `pending`, `active`, `expired`, `cancelled`
    - ✅ 套餐状态枚举: `draft`, `active`, `archived`
    - ✅ 计费周期枚举: `monthly`, `yearly`, `custom`, `five_hours`, `day`, `week`, `month`
    - ✅ 限额周期类型: `five_hours`, `day`, `week`, `month`
    - ✅ 兑换选项枚举: `stack`(叠加), `coexist`(共存), `convert`(转换)
    - ✅ 优惠券状态/作用域/折扣类型
    - ✅ 订单状态、支付渠道、账单状态
    - ✅ 兑换类型: `quota`(额度兑换), `subscription`(订阅兑换)
    - ✅ 货币单位: `USD`, `CNY`, `EUR`
    - ✅ **补充**: Token 订阅偏好设置 (`subscription_preferred` 字段语义)
      - **重要**: DDL 中 `tokens.subscription_preferred` 默认值为 **FALSE**（即默认不优先使用订阅）
      - `TokenSubscriptionPreferredEnabled = true` - 启用订阅优先扣费
      - `TokenSubscriptionPreferredDisabled = false` - 不优先使用订阅，仅使用钱包余额（默认）
    - ✅ **补充**: 订阅优先级常量 (Highest=1, High=10, Normal=50, Low=90, Lowest=99)
    - ✅ **补充**: 订阅系统配置键常量 (`OptionKeySubscription*`)
    - ✅ **补充**: 默认值常量 (与 `SUBSCRIPTION_MAX_PER_USER` 等配置对齐)
  - **修改文件**: `common/constants.go` (+158 行)
  - **修正说明 (2025-12-07)**: 修正 Token 默认值注释，与 DDL 保持一致（默认 false）

- [x] 2.1.4 定义系统配置项 ✅ **2025-12-06 完成，2025-12-07 修正**
  - **实施内容**:
    - ✅ 配置键定义 (5 个):
      - `SUBSCRIPTION_AUTO_WALLET_DEFAULT` - 自动兜底默认值 (默认: false)
      - `SUBSCRIPTION_EXPIRY_NOTICE_DAYS` - 到期提醒天数 (默认: 7, 范围: 1-90)
      - `SUBSCRIPTION_MAX_PER_USER` - 每用户最大订阅数 (默认: 10, 范围: 1-100)
      - `SUBSCRIPTION_QUOTA_LOW_THRESHOLD` - 额度低阈值 (默认: 0.2, 范围: 0.0-1.0)
      - `SUBSCRIPTION_V2_ENABLED` - feature flag (默认: false)
    - ✅ 配置初始化: 在 `model/option.go:InitOptionMap` 中加载默认值到 OptionMap
    - ✅ 配置验证: 在 `controller/option.go:UpdateOption` 中添加参数校验
      - **补充**: 添加布尔类型校验（`SUBSCRIPTION_AUTO_WALLET_DEFAULT`, `SUBSCRIPTION_V2_ENABLED`）
      - **补充**: 整数范围校验（`SUBSCRIPTION_EXPIRY_NOTICE_DAYS`: 1-90, `SUBSCRIPTION_MAX_PER_USER`: 1-100）
      - **补充**: 浮点范围校验（`SUBSCRIPTION_QUOTA_LOW_THRESHOLD`: 0.0-1.0）
    - ✅ 与 DDL 迁移脚本 Phase 3 中的配置项完全对齐
  - **修改文件**: `model/option.go` (+7 行), `controller/option.go` (+46 行，含布尔校验)
  - **修正说明 (2025-12-07)**: 补充布尔类型合法性校验，确保配置完整验证

- [x] 2.1.5 系统设置后端实现 ✅ **2025-12-06 完成，2025-12-07 修正**
  - **实施内容**:
    - ✅ **配置存储**: 基于现有 `options` 表（DB）+ OptionMap（内存缓存）
    - ✅ **缓存策略**:
      - 创建 `common/subscription_config.go` 实现线程安全配置缓存
      - 使用 RWMutex 保护并发访问
      - 提供 `InitSubscriptionConfig()` 初始化配置
      - 提供 `UpdateSubscriptionConfig()` 热更新配置
    - ✅ **Admin API（读/写）**:
      - 读取: 现有 `GetOptions` API 自动包含订阅配置（已在 InitOptionMap 中注册）
      - 写入: 现有 `UpdateOption` API 支持订阅配置更新（已添加完整验证逻辑）
      - 权限校验: 复用现有 Admin 权限体系
    - ✅ **校验规则（完整）**:
      - `SUBSCRIPTION_AUTO_WALLET_DEFAULT`: 布尔类型（ParseBool 校验）
      - `SUBSCRIPTION_V2_ENABLED`: 布尔类型（ParseBool 校验）
      - `SUBSCRIPTION_EXPIRY_NOTICE_DAYS`: 1-90 天（整数范围校验）
      - `SUBSCRIPTION_MAX_PER_USER`: 1-100 个（整数范围校验）
      - `SUBSCRIPTION_QUOTA_LOW_THRESHOLD`: 0.0-1.0 (浮点范围校验)
    - ✅ **便捷访问函数**:
      - `IsSubscriptionEnabled()` - 判断订阅系统是否启用
      - `GetSubscriptionMaxPerUser()` - 获取每用户最大订阅数
      - `GetSubscriptionExpiryNoticeDays()` - 获取到期提醒天数
      - `GetSubscriptionQuotaLowThreshold()` - 获取额度低阈值
      - `GetAutoWalletFallbackDefault()` - 获取自动兜底默认值
    - ✅ **配置热更新**: 在 `model/option.go:updateOptionMap` 中监听配置变更，自动刷新缓存
  - **修改文件**: `common/subscription_config.go` (新建文件, 140 行), `model/option.go` (+5 行)
  - **修正说明 (2025-12-07)**: 补充布尔类型配置的 ParseBool 校验逻辑
  - **单元测试**: 待后续 2.2 阶段统一实现

#### 2.2 Model 层实现（优先完成，是 Service 层的基础）
- [x] 2.2.1 实现 `model/subscription_plan.go`
  - SubscriptionPlan 结构体定义（含 GORM 标签）
  - SubscriptionPlanLimit 结构体定义
  - 仓储方法：Create, Update, GetByID, List, Delete
  - 模型验证：name 唯一、价格 >0、周期合法
  - 单元测试：CRUD 操作、约束验证
- [x] 2.2.2 实现 `model/subscription.go`
  - Subscription 结构体扩展（新增 priority, auto_wallet_fallback 等）
  - 仓储方法：Create, Update, GetByID, GetActiveByUser, UpdatePriority
  - 状态转换方法：Activate, Expire, Cancel
  - 单元测试：状态转换、优先级排序
- [x] 2.2.3 实现 `model/subscription_usage.go`
  - SubscriptionUsage 结构体定义
  - 仓储方法：Create, Update, GetBySubscriptionAndPeriod, ResetWindow
  - 原子更新方法：AtomicIncrement（使用 SQL WHERE 条件）
  - 单元测试：窗口计算、原子更新
- [x] 2.2.4 实现 `model/subscription_order.go`
  - SubscriptionOrder 结构体定义
  - 仓储方法：Create, GetByID, GetByUser, UpdateStatus
  - 快照序列化：PlanSnapshot, CouponSnapshot
  - 单元测试：订单创建、快照序列化
- [x] 2.2.5 扩展 `model/coupon.go`
  - 新增字段：scope, bind_user_id, bind_redemption_id
  - 新增方法：BindToRedemption, UnbindRedemption, IsReserved
  - 状态管理：active → reserved → used
  - 单元测试：绑定/解绑、状态转换
- [x] 2.2.6 扩展 `model/redemption.go`
  - 新增字段：type（quota/subscription）, payload（JSON）
  - Payload 结构体定义（plan_id, duration_days, bind_coupon_id）
  - 新增方法：CreateSubscriptionRedemption
  - 单元测试：订阅兑换创建、payload 序列化
- [x] 2.2.7 扩展 `model/user.go` 或新建 `model/user_setting.go`
  - 新增 auto_wallet_fallback 字段
  - 配置继承逻辑（用户设置 > 系统默认）
  - 单元测试：配置获取、默认值

#### 2.2.8 优惠券改造 - Model 层实现（新增）

- [x] 2.2.8.1 实现 `model/user_coupon.go`（新增） ✅ 已完成
  - UserCoupon 结构体定义（含 GORM 标签）
    - `id`, `coupon_id`, `user_id`, `code`（UC-UUID格式）
    - `status`（available/locked/used/expired/invalid）
    - `claimed_at`, `used_at`, `order_id`
    - `discount_amount`, `final_amount`（优惠金额和最终支付金额）
    - `created_at`, `updated_at`
  - 仓储方法：
    - Create, GetByID, GetByUserAndCoupon, GetByUser, UpdateStatus
    - GetByCode（根据用户券核销码查询）
  - 领券方法：ClaimCoupon（包含乐观锁）
    - 查询优惠券模板并加载 `version`
    - 校验时间、库存、状态
    - 校验用户领取限制（同一用户同一优惠券只能领取一次）
    - 乐观锁更新库存：`WHERE id = ? AND version = ? AND used_count < total_count`
    - 插入用户优惠券实例（status = 'available'）
  - 核销方法：UseCoupon（包含幂等性检查）
    - 获取用户优惠券并加行锁（FOR UPDATE）
    - 幂等性检查：若已 used 且 order_id 相同，直接返回成功
    - 基础校验：时间、作用域、币种
    - 阈值与计算：满减阈值基于订单原价
    - 原子更新状态为 used
    - 写入 user_bills 账单
  - 状态转换方法：Lock, Use, Expire, Invalidate
  - 单元测试：
    - 领券流程测试（成功、库存不足、重复领取）
    - 核销流程测试（成功、幂等性、阈值不足）
    - 并发测试（1000并发领券，验证无超发）
    - 状态转换测试

- [x] 2.2.8.2 实现 `model/coupon_redemption_binding.go`（新增） ✅ 已完成
  - CouponRedemptionBinding 结构体定义
    - `id`, `coupon_id`, `redemption_id`, `user_id`（专属用户）
    - `status`（reserved/locked）
    - `locked_at`, `created_at`, `updated_at`
  - 仓储方法：
    - Create, GetByRedemption, GetByCoupon, UpdateStatus
  - 绑定方法：BindCouponToRedemption
    - 校验优惠券存在且状态为 active
    - 创建绑定记录（status = 'reserved'）
    - 写入审计日志
  - 锁定方法：LockBinding（reserved → locked）
    - 获取绑定记录并加行锁
    - 校验专属用户（若 user_id 不为空）
    - 原子更新状态为 locked
  - 单元测试：
    - 绑定流程测试
    - 状态转换测试（reserved → locked）
    - 专属用户校验测试
    - 并发绑定测试

- [x] 2.2.8.3 扩展 `model/coupon.go`（补充） ✅ 已完成
  - 新增字段：
    - `type`（discount/full_reduction/instant_reduction）
    - `scope`（wallet/subscription/wallet_subscription）
    - `threshold_amount`（满减阈值，分）
    - `currency`（币种，默认 CNY）
    - `version`（乐观锁版本号）
  - 版本管理方法：IncrementVersion（乐观锁）
    - 原子更新：`SET version = version + 1 WHERE id = ? AND version = ?`
  - 验证方法：
    - ValidateType（校验类型枚举）
    - ValidateScope（校验作用域枚举）
    - ValidateCurrency（校验币种）
    - ValidateThreshold（校验满减阈值 >= 0）
  - 单元测试：
    - 乐观锁机制测试（并发更新）
    - 字段验证测试（类型、作用域、币种）
    - 阈值验证测试

- [x] 2.2.8.4 扩展 `model/user_bill.go`（补充） ✅ 已完成
  - 新增字段：
    - `coupon_id`（使用的优惠券模板ID）
    - `user_coupon_id`（使用的用户优惠券ID）
    - `discount_amount`（优惠金额，分）
    - `original_amount`（原价，分）
    - `final_amount`（实付，分）
    - `refund_type`（退款类型：manual/auto/none）
  - 单元测试：
    - 优惠券相关字段的 CRUD 测试
    - 账单创建时包含优惠券信息测试

- [x] 2.2.8.5 扩展 `model/redemption.go`（补充） ✅ 已完成
  - 新增字段：
    - `bound_user_id`（专属用户ID）
  - 专属用户校验方法：ValidateBoundUser
    - 若 bound_user_id 不为空，校验是否匹配当前用户
    - 不匹配则返回错误 REDEMPTION_USER_MISMATCH
  - 单元测试：
    - 专属用户校验测试（匹配/不匹配）
    - 兑换码绑定优惠券场景测试

#### 2.2.9 Model 层问题修复 ✅ **2025-12-07 完成（二次修正）**
- [x] **修复 1: 按需触发滚动窗口实现** (`model/subscription_usage.go`)
  - **核心逻辑重构**：窗口开始时间由用户首次实际使用触发，不是订阅开始时间
  - 新增 `GetActiveUsageWindow()` / `GetActiveUsageWindowWithTx()` - 获取活跃窗口（不创建）
  - 新增 `GetOrCreateActiveUsageWindow()` / `GetOrCreateActiveUsageWindowWithTx()` - 获取或创建活跃窗口（按需触发）
  - 新增 `ConsumeQuotaWithStrategy()` / `CheckAndConsumeQuotaWithStrategy()` - 带策略的额度消耗
  - 新增 `CalculateFixedWindowBounds()` - 计算固定窗口边界（替代原 CalculateWindowBoundsWithStrategy）
  - 新增 `CalculateRollingWindowEnd()` - 计算滚动窗口结束时间
  - 新增 `IsWindowExpired()` / `GetTimeUntilWindowEnd()` - 窗口状态检查
  - **滚动窗口规则**：
    1. 窗口开始时间 = 用户首次实际使用计费的时间
    2. 窗口结束时间 = 开始时间 + 周期时长
    3. 窗口结束后**不会立即开始新窗口**
    4. 新窗口开始时间 = 上个窗口结束后用户下次首次使用的时间
- [x] **修复 2: 兑换选项补充** (`model/subscription.go`, `common/constants.go`)
  - 在订阅验证中补充 `coexist` 和 `convert` 兑换选项
  - 完整选项：stack（叠加）、coexist（共存）、convert（转换）、replace（替换）、extend（延期）
- [x] **修复 3: 优惠券绑定状态机** (`model/coupon.go`, `common/constants.go`)
  - 新增 `CouponStatusReserved` 常量（已预留：绑定但未使用，可解绑）
  - 新增函数：`ReserveCouponToUser/Tx`（预留优惠券）、`UnbindCouponFromUser/Tx`（解绑）、`LockCoupon/Tx`（锁定）
  - 新增辅助方法：`IsReserved()`、`IsLocked()`、`CanUnbind()`
  - 状态流转：active → reserved（绑定）→ locked（兑换使用）
- [x] **修复 4: 兑换码类型校验和载荷字段** (`model/redemption.go`)
  - `RedemptionSubscriptionPayload` 新增 `DurationDays` 和 `PricePaid` 字段
  - 在 `Redeem()` 中添加类型校验，拒绝 subscription 类型使用额度兑换接口
  - 新增辅助方法：`GetDurationSeconds()`、`IsCoexistMode()`、`IsConvertMode()`
- [x] **修复 5: 用户兜底配置持久化** (`model/user.go`)
  - 新增 `AutoWalletFallback bool` 字段，用于持久化用户级别兜底配置
- [x] **修复 6: 套餐验证逻辑** (`model/subscription_plan.go`, `common/constants.go`)
  - 计费周期验证仅允许 `monthly`/`yearly`/`custom`（不包含 five_hours/day/week/month）
  - 新增 `WindowStrategyNatural` 常量作为 `fixed` 的别名
  - 窗口策略验证支持 `rolling`/`fixed`/`natural`

#### 2.3 Service 层 - 套餐管理服务
- [x] 2.3.1 实现 `service/subscription_plan_service.go`
  - CreatePlan（创建套餐 + 限额配置）
  - UpdatePlan（更新套餐，需事务）
  - GetPlan（获取套餐详情 + 限额）
  - ListPlans（列表查询 + 筛选）- 支持多条件筛选（status/billingCycle/minPrice/maxPrice）
  - PublishPlan / UnpublishPlan（上下架）
  - ValidateModelWhitelist（校验模型列表有效性）
  - ValidateChannelGroups（校验渠道分组有效性）
  - 事件触发：SUB_PLAN_CHANGED（通过 NotifyRootUser 实现）
  - 单元测试：CRUD、校验逻辑、事务回滚

#### 2.4 Service 层 - Usage 滚动窗口服务（核心算法，高风险）
- [x] 2.4.1 实现 `service/subscription_usage_service.go` 基础框架
- [x] 2.4.2 实现窗口时间计算逻辑
  - CalculateWindowStart（计算窗口开始时间）
  - CalculateWindowEnd（计算窗口结束时间）
  - 支持 5小时、自然日、自然周、自然月
- [x] 2.4.3 实现窗口状态判定
  - IsWindowExpired（检查窗口是否过期）
  - ShouldResetWindow（是否需要刷新窗口）
- [x] 2.4.4 实现原子预扣接口
  - TryPreConsume（尝试预扣，使用 SQL: `UPDATE ... WHERE used + quota <= limit`）
  - TryPreConsumeWithStrategies（支持每周期不同策略）
  - TryPreConsumeWithTx（事务内预扣，支持手动缓存控制）
  - 支持多周期同时预扣（5小时 AND 日 AND 周 AND 月）
  - 返回预扣上下文（用于后续回滚或调整）
  - contextId 生成使用毫秒时间戳+原子序列号+随机字节，确保高并发唯一性
- [x] 2.4.5 实现 Redis Lua 脚本（如果 SQL 性能不够） ✅ 跳过（当前 SQL 性能足够）
  - 编写 Lua 脚本实现原子预扣
  - 集成到 TryPreConsume 方法
  - 备注：当前 SQL 实现性能足够，暂不实现
- [x] 2.4.6 实现 usage 回滚接口
  - RollbackPreConsume（预扣失败时恢复）
  - RollbackPreConsumeByContext（通过上下文对象直接回滚，不依赖缓存）
  - 支持部分回滚（例如只回滚某个周期）
- [x] 2.4.7 实现差额调整接口
  - AdjustUsage（根据实际消耗调整，支持 ± delta）
  - 用于 PostConsumeQuota 场景（预扣 vs 实际消耗不一致）
- [x] 2.4.8 实现窗口刷新逻辑
  - RefreshWindow（供定时任务调用）
  - BatchRefreshExpiredWindows（批量刷新过期窗口）
  - StartPeriodicCleanup（启动定时清理任务）
- [x] 2.4.9 并发测试
  - 模拟 1000 并发请求预扣同一订阅
  - 验证 used_quota 不会超过 limit_quota
  - 验证无死锁和竞态条件
  - 测试用例：TestConcurrentPreConsume1000_ContextIdUniqueness、TestConcurrentPreConsume1000_CacheConsistency、TestConcurrentPreConsume1000_RaceCondition

#### 2.5 Service 层 - 订阅优先级服务 ✅ **2025-12-15 完成**
- [x] 2.5.1 实现 `service/subscription_priority_service.go` ✅ 已完成
  - **核心功能**:
    - InitializeSubscriptionPriority（新订阅按 end_at 动态插入到正确位置）
    - UpdateUserPriority（用户自定义调整优先级，返回 HTTP 409 冲突）
    - GetSortedSubscriptions（获取排序后的订阅列表，priority → end_at → created_at）
    - ValidatePriorityConflict（校验优先级冲突）
    - BatchUpdatePriorities（批量更新优先级，含完整性校验）
    - ReorderSubscriptions（拖拽重排序，含完整性校验）
    - NormalizePriorities（规范化优先级为连续整数 1, 2, 3...）
  - **测试覆盖**:
    - 单元测试：参数校验、边界条件
    - SQLite 集成测试：动态插入、排序逻辑、冲突检测、完整性校验

#### 2.6 Service 层 - 优惠券服务扩展
- [x] 2.6.1 优惠券领取 ✅ **2025-12-15 完成**
  - **实施内容**:
    - ✅ 创建 `service/coupon_service.go` 实现优惠券领取服务
    - ✅ `ClaimCoupon` - 领取优惠券（事务+乐观锁，扣减库存+插入 `user_coupons`）
    - ✅ `ValidateCouponClaimable` - 校验优惠券是否可领取（状态、时间、库存）
    - ✅ `CheckUserClaimLimit` - 检查用户领取限制（已领取检查、每用户限制）
    - ✅ 审计日志记录（异步写入）
    - ✅ 单元测试覆盖：领券成功、库存不足、重复领取、过期优惠券、尚未生效、无效状态
  - **修改文件**: `service/coupon_service.go` (新建, 245 行), `test/service/coupon_service_test.go` (新建, 13 个测试用例)
- [x] 2.6.2 优惠券核销 ✅ **2025-12-16 完成**
  - **实施内容**:
    - ✅ UseCoupon（幂等校验 `order_id` + 行锁）
    - ✅ CalculateDiscount（折扣/满减/立减），CheckCouponThreshold
    - ✅ 单元测试：核销成功、幂等、阈值不足
  - **修改文件**: `service/coupon_service.go` (+185 行)
- [x] 2.6.3 绑定/解绑/专属用户 ✅ **2025-12-16 完成**
  - **实施内容**:
    - ✅ BindCouponToRedemption / UnbindCoupon / ValidateBoundUser
    - ✅ UseBoundCoupon（兑换时自动消费绑定的优惠券）
    - ✅ 单元测试：绑定/解绑、专属用户校验
  - **修改文件**: `service/coupon_service.go` (+220 行)
- [x] 2.6.4 列表与预览 ✅ **2025-12-16 完成**
  - **实施内容**:
    - ✅ GetUserCoupons（状态筛选）、GetCouponByCode、PreviewCouponUsage
    - ✅ 单元测试：列表查询、筛选、预览
  - **修改文件**: `service/coupon_service.go` (+180 行)
- [x] 2.6.5 退款处理 ✅ **2025-12-16 完成**
  - **实施内容**:
    - ✅ ProcessRefundWithCoupon（记录 coupon_id/user_coupon_id/discount_amount/final_amount/refund_type，券不恢复）
    - ✅ 单元测试：退款流程、优惠券不恢复
  - **修改文件**: `service/coupon_service.go` (+135 行), `test/service/coupon_service_test.go` (+547 行, 新增 26 个测试用例)

#### 2.7 Service 层 - 兑换码服务扩展 ✅ **2025-12-17 完成**
- [x] 2.7.1 扩展 `service/redemption_service.go` - 支持订阅类型 ✅
  - **实施内容**:
    - ✅ 创建 `service/redemption_service.go` 实现兑换码服务
    - ✅ `RedeemSubscription` - 主入口，支持所有兑换选项
    - ✅ 支持 stack/coexist/convert/replace/extend 兑换模式
    - ✅ 集成 CouponService 和 SubscriptionPriorityService
- [x] 2.7.2 实现同套餐叠加逻辑 ✅
  - ✅ `ExtendSubscription` - 延长当前订阅有效期
  - ✅ `executeStackRedemption` - 执行叠加兑换（事务内）
  - ✅ 记录兑换延长流水（UserBill）
  - ✅ 记录审计日志（AuditLog）
- [x] 2.7.3 实现更贵套餐二选一逻辑 ✅
  - ✅ `CheckSubscriptionConflict` - 检测套餐冲突（same_plan/more_expensive/different_plan）
  - ✅ `PromptUserChoice` - 返回选择提示给前端
  - ✅ `ConflictCheckResult` - 冲突检测结果（含可选选项、推荐选项、折算预览）
- [x] 2.7.4 实现折算逻辑（convert 选项）✅
  - ✅ `CalculateRemainValue` - 计算剩余价值（使用 decimal.Decimal）
  - ✅ `ConvertToDays` - 按新套餐日均价折算天数（向下取整）
  - ✅ `executeConvertRedemption` - 执行折算兑换
  - ✅ `recordConversionBillWithTx` - 记录折算流水（含不足一天金额）
  - ✅ `ConversionInfo` - 折算详情结构体
  - ✅ 使用 shopspring/decimal 类型避免精度问题
- [x] 2.7.5 实现并存逻辑（coexist 选项）✅
  - ✅ `executeCoexistRedemption` - 创建新订阅，保留旧订阅
  - ✅ 检查用户订阅数量限制（`SUBSCRIPTION_MAX_PER_USER`）
  - ✅ 初始化订阅优先级（动态插入）
- [x] 2.7.6 实现兑换时优惠券自动消费 ✅
  - ✅ `RedeemWithCoupon` - 兑换 + 消费绑定的优惠券
  - ✅ `redeemWithBoundCoupon` - 带绑定优惠券的兑换流程
  - ✅ 集成 `CouponService.UseBoundCouponWithPlan`
  - **修改文件**: `service/redemption_service.go` (新建, 993 行)

#### 2.8 Service 层 - 订单服务
- [x] 2.8.1 实现 `service/subscription_order_service.go` ✅ (2025-12-19 修复完成)
  - CreateOrder（创建订单 + 套餐/优惠券快照）
  - ProcessPayment（处理支付：余额/第三方）
  - ActivateSubscription（支付成功后激活订阅）
  - InitializeUsageWindows（初始化 usage 窗口）
  - RecordBill（写入 user_bills 账单）
  - RecordAudit（写入审计日志）
  - 事务管理：订单-支付-激活-账单 原子性
  - 单元测试：订单创建、支付流程、事务回滚
  - **问题修复** (2025-12-19):
    - ✅ 修复 bill_id 未写回订单问题
    - ✅ 修复货币换算错误（根据 Currency 字段进行正确换算）
    - ✅ 修复取整方向问题（扣款向上取整，退款向下取整）
    - ✅ 添加汇率/QuotaPerUnit 的 0/负数保护
    - ✅ 接入优惠券链路（coupon_code → userCouponId 映射）
    - ✅ 增强 CreateOrder 幂等性（验证 coupon 和 payment_channel）
    - ⚠️ 余额缓存一致性风险（部分修复，需后续改进）
  - **实现日志**: `Implementation Logs/task-2-8-fix_2025-12-19T1231_e08c1ec3.md`
- [x] 2.8.2 幂等性设计与测试 ✅ (2025-12-19 增强完成)
  - 幂等键存储与校验（订单创建/支付回调/激活）
  - 幂等冲突处理与错误码
  - **增强**: 验证 coupon 和 payment_channel 匹配性
  - 幂等单测与并发用例（待补充）

#### 2.9 Service 层 - 计费服务（核心逻辑） ✅ **2025-12-18 完成**
- [x] 2.9.1 实现 `service/subscription_billing.go` 基础框架 ✅
  - SubscriptionBillingService 服务结构体定义
  - BillingContext 计费上下文 DTO
  - PreConsumeContext 预扣上下文
  - 单例模式获取服务实例
- [x] 2.9.2 实现订阅筛选逻辑 ✅
  - SelectCandidateSubscriptions（筛选可用订阅）
  - 过滤条件：status=active && 模型匹配 && 渠道匹配
  - 检查 token.subscription_preferred（未启用订阅优先则跳过订阅计费）
- [x] 2.9.3 实现订阅优先级遍历和预扣 ✅
  - TryDeductFromSubscriptions（按优先级遍历订阅）
  - 对每个订阅调用 UsageService.TryPreConsume
  - 返回成功的订阅 ID 和 usage 上下文
- [x] 2.9.4 实现自动兜底处理 ✅
  - CheckAutoWalletFallback（检查兜底配置）
  - FallbackToWallet（失败时切换到余额扣费）
  - RecordFallbackEvent（记录 fallback 事件）
- [x] 2.9.5 实现订阅优先关闭跳过逻辑 ✅
  - SkipSubscriptionBilling（跳过订阅扣费）
  - ReturnSkipReason（返回跳过原因：subscription_preferred_disabled）
- [x] 2.9.6 单元测试：筛选逻辑、优先级遍历、fallback ✅

#### 2.10 Service 层 - Redis 缓存实现 ✅ **2025-12-18 完成**
- [x] 2.10.1 设计缓存策略 ✅
  - `subscription:active:<user_id>` - 用户活跃订阅列表
  - `subscription:usage:<subscription_id>` - 订阅 usage 哈希
  - TTL 与订阅 end_at 对齐
- [x] 2.10.2 实现缓存写入逻辑 ✅
  - CacheActiveSubscriptions（缓存用户订阅）
  - CacheSubscriptionUsage（缓存 usage 数据）
  - RefreshCacheTTL（刷新 TTL）
- [x] 2.10.3 实现缓存失效逻辑 ✅
  - InvalidateOnStatusChange（状态变更时失效）
  - InvalidateOnPriorityChange（优先级调整时失效）
  - InvalidateOnUsageUpdate（usage 更新时同步缓存）
- [x] 2.10.4 实现缓存读取逻辑 ✅
  - GetActiveSubscriptionsFromCache（优先从缓存读取）
  - GetUsageFromCache（优先从缓存读取）
  - FallbackToDatabase（缓存未命中时回源）
- [x] 2.10.5 性能监控 ✅
  - 添加缓存命中率监控指标
  - 目标：命中率 >95%，访问耗时 ≤2ms

#### 2.11 Controller 层 - Admin 套餐管理 API ✅ **2025-12-18 完成**
- [x] 2.11.1 实现 `controller/subscription_plan.go` ✅
  - `GET /api/admin/subscription-plans` - 套餐列表（支持筛选/分页）
  - `POST /api/admin/subscription-plans` - 创建套餐
  - `PUT /api/admin/subscription-plans/:id` - 编辑套餐
  - `POST /api/admin/subscription-plans/:id/publish` - 上架套餐
  - `POST /api/admin/subscription-plans/:id/unpublish` - 下架套餐
  - `DELETE /api/admin/subscription-plans/:id` - 删除套餐（软删除）
  - 权限校验：Admin only
  - 参数验证：价格 >0、周期合法、模型/渠道有效
  - 单元测试：handler 测试、权限测试

#### 2.12 Controller 层 - Admin 优惠券管理 API ✅ **2025-12-22 完成**
- [x] 2.12.1 实现 `controller/subscription_coupon.go` ✅
  - `GET /api/admin/subscription-coupons` - 优惠券列表
  - `POST /api/admin/subscription-coupons` - 创建优惠券
  - `PUT /api/admin/subscription-coupons/:id` - 更新优惠券
  - `DELETE /api/admin/subscription-coupons/:id` - 删除优惠券
  - `POST /api/admin/subscription-coupons/:id/bind-redemption` - 绑定兑换码
  - `DELETE /api/admin/subscription-coupons/:id/unbind` - 解绑兑换码
  - `GET /api/admin/subscription-coupons/:id/bindings` - 查看绑定状态
  - 权限校验：Admin only
  - 单元测试：绑定/解绑逻辑

#### 2.13 Controller 层 - Admin 订阅管理 API ✅ **2025-12-18 完成**
- [x] 2.13.1 实现 `controller/admin_subscription.go` ✅
  - `GET /api/admin/subscriptions` - 全量订阅列表（支持筛选：用户/状态/套餐）
  - `GET /api/admin/subscriptions/:id` - 订阅详情（含 usage 历史）
  - `POST /api/admin/subscriptions/:id/cancel` - 取消订阅
    - 输入：取消原因
    - 更新状态为 cancelled，end_at=now
    - 写入审计日志
    - 触发通知（根据系统/用户配置）
  - `POST /api/admin/subscriptions/:id/refund` - 记录人工退款
    - 输入：退款金额、原因
    - 写入 user_bills（type=manual_refund，仅记录）
    - 写入审计日志
  - 权限校验：Admin only
  - 单元测试：取消流程、退款记录

#### 2.14 Controller 层 - User 订阅查询和购买 API ✅ **2025-12-18 完成**
- [x] 2.14.1 实现 `controller/user_subscription.go` ✅
  - `GET /api/user/subscription-plans` - 可购买套餐列表（仅 active 状态）
  - `GET /api/user/subscription-plans/:id` - 套餐详情（含限额、模型/渠道）
  - `POST /api/user/subscriptions/orders` - 创建订单 + 支付
    - 输入：plan_id, coupon_id (可选)
    - 校验套餐状态、优惠券有效性
    - 计算折扣、生成订单
    - 调用支付服务（余额/第三方）
    - 成功后激活订阅
  - `GET /api/user/subscriptions/active` - 我的订阅列表
    - 返回：基础信息 + 状态 + 剩余天数
    - 返回：各周期 usage（已用/限额/剩余/倒计时）
    - 返回：USD 换算显示
    - 返回：auto_wallet_fallback 状态
    - 返回：优先级顺序
  - `GET /api/user/subscriptions/:id` - 订阅详情
  - `GET /api/user/subscriptions/:id/history` - 订阅历史（订单/兑换/取消记录）
  - 权限校验：User only，仅访问自己的订阅
  - 单元测试：购买流程、查询逻辑

#### 2.15 Controller 层 - User 订阅配置 API
- [x] 2.15.1 实现用户订阅配置接口 ✅ **2025-12-23 完成**
  - `PUT /api/user/subscriptions/:id/auto-wallet` - 切换自动兜底开关
    - 输入：enabled (true/false)
    - 更新 subscriptions.auto_wallet_fallback
    - 刷新缓存
  - `PUT /api/user/subscriptions/priorities` - 批量更新订阅优先级
    - 输入：订阅 ID 列表（按优先级顺序）
    - 验证所有订阅属于当前用户
    - 更新 priority 字段
    - 刷新缓存
  - `GET /api/user/settings/auto-wallet-fallback` - 获取用户级别兜底配置
  - `PUT /api/user/settings/auto-wallet-fallback` - 更新用户级别兜底配置
  - 单元测试：配置更新、权限验证
  - **实施内容**:
    - ✅ `controller/subscription_user.go` 新增 4 个 API 处理函数
      - `UpdateSubscriptionAutoWallet` - 切换订阅自动兜底开关
      - `BatchUpdateSubscriptionPriorities` - 批量更新订阅优先级
      - `GetUserAutoWalletFallback` - 获取用户级别兜底配置
      - `UpdateUserAutoWalletFallback` - 更新用户级别兜底配置
    - ✅ `model/user.go` 新增 `UpdateUserAutoWalletFallback` 函数
    - ✅ `router/api-router.go` 新增路由组配置
      - `/api/user/subscriptions/:id/auto-wallet` (PUT)
      - `/api/user/subscriptions/priorities` (PUT)
      - `/api/user/settings/auto-wallet-fallback` (GET/PUT)
    - ✅ `test/controller/subscription_user_config_test.go` 新建测试文件
      - 6 个测试函数，全部通过
      - 覆盖：启用/禁用自动兜底、优先级重排序、权限验证、未授权访问

#### 2.16 Controller 层 - User 优惠券/兑换码 API
- [x] 2.16.1 实现 `controller/user_coupon.go`
  - `POST /api/user/coupons/claim` - 用户领取优惠券（校验库存、限领）
  - `GET /api/user/coupons` - 我的优惠券列表（支持状态筛选）
  - `POST /api/user/payment/coupon/preview` - 优惠券使用预览（可选）
  - 权限校验：User only，限制访问本人数据
  - 单元测试：领取、列表、预览
- [x] 2.16.2 扩展 `controller/redemption.go`
  - `POST /api/user/redemptions/use` - 兑换订阅（支持绑定优惠券自动消费）
    - 输入：redemption_code, redeem_option (stack/coexist/convert，可选)
    - 校验兑换码有效性、未使用、专属用户
    - 判断套餐关系（同套餐/更贵/不同）
    - 同套餐：直接叠加；更贵套餐：coexist 创建新订阅，convert 折算延期
    - 若绑定优惠券：锁定绑定记录，创建/锁定 `user_coupons`，成功后标记 `used`
  - 写入订单/账单/审计
  - 单元测试：兑换流程、折算逻辑、绑定优惠券校验
- [x] 2.16.3 幂等性设计与测试
  - 幂等键校验（兑换/领取请求）
  - 重复请求的结果幂等返回
  - 幂等冲突错误码与日志

#### 2.17 API 契约测试
- [x] 2.17.1 定义 API 契约（OpenAPI/Swagger 规范）
  - 套餐管理 API 契约
  - 优惠券管理 API 契约
  - 订阅管理 API 契约
  - 兑换 API 契约
- [x] 2.17.2 编写契约测试用例
  - 请求/响应格式验证
  - 错误码验证
  - 权限验��
- [x] 2.17.3 生成 API 文档（供前端参考）

### 阶段3: 计费链路改造

#### 3.1 扩展数据结构
- [x] 3.1.1 扩展 `relay/common.RelayInfo` 结构体
  - 新增 `BillingSource` 字段（subscription/wallet/fallback）
  - 新增 `SubscriptionId` 字段（订阅 ID）
  - 新增 `SkipReason` 字段（跳过原因：subscription_preferred_disabled 等）
- [x] 3.1.2 更新数据传递链路
  - controller → service → relay 传递计费上下文
  - 响应中包含计费提示信息

#### 3.2 Feature Flag 配置
- [x] 3.2.1 实现 feature flag 控制
  - 配置项：`SUBSCRIPTION_V2_ENABLED`
  - 支持按用户ID灰度（user_id % 100 < threshold）
  - 支持按百分比灰度
- [x] 3.2.2 准备灰度切换脚本
  - 开启灰度脚本（1% → 10% → 50% → 100%）
  - 关闭 flag 快速回退脚本
- [x] 3.2.3 缓存/迁移联动
  - flag 开启时刷新相关缓存
  - flag 关闭时回退到旧逻辑

#### 3.3 改造预扣费逻辑
- [x] 3.3.1 改造 `service/pre_consume_quota.go`
  - 检查 feature flag，决定是否启用订阅扣费
  - 调用 `subscription_billing.SelectCandidateSubscriptions`
  - 筛选可用订阅列表
- [x] 3.3.2 接入订阅判定逻辑
  - 调用 `subscription_billing.TryDeductFromSubscriptions`
  - 按优先级遍历订阅并预扣
  - 成功：记录 `BillingSource=subscription`
- [x] 3.3.3 处理订阅额度不足
  - 返回新错误码：`SUBSCRIPTION_LIMIT_REACHED`
  - 检查 `auto_wallet_fallback` 配置
  - fallback=true：切换到余额扣费
  - fallback=false：返回错误给用户
- [x] 3.3.4 处理订阅优先关闭
  - 检查 `token.subscription_preferred`
  - 如果为 false，跳过订阅判定
  - 设置 `SkipReason=subscription_preferred_disabled`
  - 在响应中添加提示字段
- [x] 3.3.5 性能优化
  - 添加性能监控埋点（订阅判定耗时）
  - 优化订阅筛选查询（SQL优化 + 索引）
  - 优化缓存策略（预热、TTL调优）
  - 目标：P99 ≤5ms

#### 3.4 改造后扣费逻辑
- [ ] 3.4.1 改造 `service/quota.go#PostConsumeQuota`
  - 检查 `BillingSource` 字段
  - 如果来源是订阅，调用 usage 调整逻辑
- [ ] 3.4.2 实现订阅 usage 差额调整
  - 计算预扣量 vs 实际消耗的差额
  - 调用 `subscription_usage_service.AdjustUsage`
  - 支持返还（实际<预扣）或补扣（实际>预扣）
- [ ] 3.4.3 实现 fallback 双重账单记录
  - 场景：订阅额度不足触发 fallback
  - 记录订阅部分消耗（如果有）
  - 记录余额部分消耗
  - 确保两条 bill 记录一致性
  - 写入审计日志
- [ ] 3.4.4 实现错误回滚机制
  - 预扣失败时恢复订阅 usage
  - 支付失败时回滚预扣
  - 确保数据一致性

#### 3.5 日志和审计
- [ ] 3.5.1 添加计费来源日志
  - 记录每次请求的计费来源（订阅/余额/fallback）
  - 记录订阅 ID 和 usage 扣减明细
  - 日志级别：Info
- [ ] 3.5.2 添加订阅扣费审计记录
  - object_type=subscription_billing
  - metadata 包含：user_id, subscription_id, quota, periods
  - 每次成功扣费写入审计
- [ ] 3.5.3 添加 fallback 审计记录
  - 记录 fallback 触发原因
  - 记录双重账单 ID
  - 用于合规追溯

#### 3.6 性能测试
- [ ] 3.6.1 压测订阅判定路径
  - 模拟 1000 QPS 请求
  - 测试缓存命中情况
  - 测试缓存未命中情况
  - 目标：P99 ≤5ms
- [ ] 3.6.2 压测 usage 更新
  - 模拟 2000 QPS 并发更新
  - 验证原子性（无超扣）
  - 验证无死锁
  - 目标：2k QPS 稳定运行
- [ ] 3.6.3 输出性能报告和优化清单
  - 记录性能瓶颈点
  - 制定优化方案（SQL/缓存/索引）

### 阶段4: 前端实现

#### 4.0 前端基础准备
- [ ] 4.0.1 定义前端数据模型和类型定义（如使用 TypeScript/PropTypes）
  - SubscriptionPlan 模型
  - Subscription 模型
  - SubscriptionUsage 模型
  - Coupon 模型
  - Redemption 模型
- [ ] 4.0.2 设计 API 客户端封装
  - 扩展 `web/src/helpers/api.js`
  - 新增订阅相关 API 方法
  - 统一错误处理（订阅特有错误码）
- [ ] 4.0.3 定义错误处理和 Toast 提示规范
  - `SUBSCRIPTION_LIMIT_REACHED` 提示文案
  - fallback 提示文案
  - 兑换冲突提示文案
- [ ] 4.0.4 设计状态管理方案
  - 订阅列表状态管理
  - 购买流程状态管理
  - 兑换流程状态管理

#### 4.1 前端 Admin - 套餐管理界面
- [ ] 4.1.1 实现套餐列表页面
  - 文件：`web/src/components/admin/SubscriptionPlanList.jsx`
  - 功能：表格展示（名称/价格/周期/状态/操作）
  - 功能：筛选（状态/搜索）+ 分页
  - 功能：上下架按钮
  - 功能：删除确认对话框
- [ ] 4.1.2 实现套餐创建/编辑 Modal
  - 文件：`web/src/components/admin/SubscriptionPlanModal.jsx`
  - 表单字段：基础信息（名称/描述/SKU/价格/币种）
  - 表单字段：计费周期（monthly/yearly/custom）
  - 表单字段：生效期和停售时间
  - 表单字段：自动兜底默认值开关
- [ ] 4.1.3 实现周期限额配置 UI（动态行）
  - 组件：`web/src/components/admin/PlanLimitConfig.jsx`
  - 支持添加/删除周期行
  - 周期类型选择（5小时/日/周/月）
  - 限额输入（quota 单位）+ USD 换算显示
  - 启用/禁用开关
  - 滚动窗口说明文案
- [ ] 4.1.4 实现模型/渠道多选组件
  - 组件：`web/src/components/admin/ModelChannelSelector.jsx`
  - 模型列表多选（从后端 API 获取）
  - 渠道分组多选
  - 选中状态展示
- [ ] 4.1.5 实现套餐预览和 SKU 管理
  - 预览所有配置信息
  - SKU 自动生成或手动输入
  - 表单验证和提交

#### 4.2 前端 Admin - 优惠券/兑换码管理界面
- [ ] 4.2.1 实现优惠券列表页面
  - 文件：`web/src/components/admin/CouponList.jsx`
  - 表格：优惠券信息 + 绑定状态
  - 筛选：scope(subscription/quota) + 状态
- [ ] 4.2.2 实现优惠券创建 Modal（支持订阅 scope）
  - 文件：`web/src/components/admin/CouponModal.jsx`
  - scope 选择：subscription/quota/all
  - applicable_plan_ids 多选（仅 scope=subscription 时）
  - 折扣类型：折扣/立减
  - 发行量、使用次数限制
- [ ] 4.2.3 实现优惠券绑定兑换码 UI
  - 组件：`web/src/components/admin/CouponBindingModal.jsx`
  - 选择用户（用户搜索）
  - 选择兑换码（已创建的兑换码列表）
  - 绑定确认和提示（不可逆操作）
- [ ] 4.2.4 实现兑换码列表和导出功能
  - 文件：`web/src/components/admin/RedemptionCodeList.jsx`
  - 列表：兑换码 + 类型 + 绑定优惠券状态
  - 导出为 CSV/Excel
- [ ] 4.2.5 实现绑定状态查看
  - 展示优惠券绑定的兑换码
  - 展示兑换码绑定的优惠券
  - 状态：reserved/used

#### 4.3 前端 Admin - 订阅管理界面
- [ ] 4.3.1 实现全量订阅列表页面
  - 文件：`web/src/components/admin/SubscriptionManagement.jsx`
  - 表格：用户/套餐/状态/开始结束时间/操作
  - 筛选：用户ID/邮箱、套餐、状态（pending/active/expired/cancelled）
  - 分页和排序
- [ ] 4.3.2 实现订阅详情查看（含 usage 历史）
  - Modal：`web/src/components/admin/SubscriptionDetailModal.jsx`
  - 基础信息展示
  - 各周期 usage 使用情况
  - 历史记录时间线（购买/兑换/调整/取消）
- [ ] 4.3.3 实现订阅取消功能（含原因填写）
  - Modal：`web/src/components/admin/CancelSubscriptionModal.jsx`
  - 输入取消原因（必填）
  - 确认对话框（警告立即失效）
  - 成功提示（会触发用户通知）
- [ ] 4.3.4 实现人工退款记录 UI
  - Modal：`web/src/components/admin/RefundRecordModal.jsx`
  - 输入退款金额（仅记录，不实际操作）
  - 输入退款原因
  - 记录到账单和审计日志

#### 4.4 前端 User - 套餐购买流程
- [ ] 4.4.1 实现套餐列表页面（用户端）
  - 文件：`web/src/components/user/SubscriptionPlans.jsx`
  - 卡片式展示（名称/价格/周期/特性）
  - 仅展示 active 状态的套餐
  - 突出推荐套餐
  - 购买按钮
- [ ] 4.4.2 实现套餐详情页面（周期限额展示）
  - Modal：`web/src/components/user/PlanDetailModal.jsx`
  - 周期限额列表（5小时/日/周/月）
  - USD 换算显示
  - 可用模型和渠道列表
  - 提示不支持的模型
- [ ] 4.4.3 实现优惠券单选组件
  - 组件：`web/src/components/user/CouponSelector.jsx`
  - 展示用户可用的优惠券列表
  - 单选（仅能选一张）
  - 显示折扣金额预览
  - 不可用优惠券置灰
- [ ] 4.4.4 实现价格明细展示（原价/优惠/实付）
  - 组件：`web/src/components/user/PriceBreakdown.jsx`
  - 原价
  - 优惠券折扣
  - 最终应付金额（突出显示）
- [ ] 4.4.5 实现购买流程和支付确认
  - Modal：`web/src/components/user/CheckoutModal.jsx`
  - 订单信息确认
  - 支付方式选择（余额/第三方）
  - 支付中状态
  - 支付成功/失败提示
  - 跳转到"我的订阅"

#### 4.5 前端 User - 兑换流程
- [ ] 4.5.1 实现兑换码输入和验证 UI
  - 页面：`web/src/components/user/RedeemSubscription.jsx`
  - 兑换码输入框
  - 实时验证（格式/有效性）
  - 验证成功显示套餐信息
- [ ] 4.5.2 实现同套餐叠加提示
  - 提示：延长 X 天
  - 新的结束时间
  - 确认兑换按钮
- [ ] 4.5.3 实现贵套餐二选一对话框（并存/折算）
  - Modal：`web/src/components/user/RedemptionConflictModal.jsx`
  - 选项1：并存（保留现有，新增新订阅）
  - 选项2：折算延期（计算说明 + 预览天数）
  - 两个选项的优缺点说明
  - 用户选择并确认
- [ ] 4.5.4 实现折算说明和确认流程
  - 展示折算公式
  - 旧订阅剩余价值 = 剩余天数 × 日均价
  - 新增天数 = floor(剩余价值 / 新套餐日均价)
  - 不足一天的金额说明（记入流水）
  - 最终延期结果预览
  - 确认按钮

#### 4.6 前端 User - 我的订阅页面
- [ ] 4.6.1 实现订阅列表页面（我的订阅）
  - 页面：`web/src/components/user/MySubscriptions.jsx`
  - 卡片式展示所有订阅
  - 状态标签：待生效/生效中/即将到期/已过期/已取消
  - 剩余天数提示
- [ ] 4.6.2 实现多周期余量展示（5小时/日/周/月）
  - 组件：`web/src/components/user/UsageProgress.jsx`
  - 每个周期一个进度条
  - 显示：已用/总量/剩余
  - 颜色提示（低于20%变红）
- [ ] 4.6.3 实现 USD 换算显示
  - quota → USD 换算
  - 显示在进度条旁边
  - 格式化为货币格式（$0.00）
- [ ] 4.6.4 实现倒计时组件（窗口刷新/到期）
  - 组件：`web/src/components/user/Countdown.jsx`
  - 窗口刷新倒计时（例如：3小时后刷新）
  - 订阅到期倒计时（例如：15天后到期）
  - 实时更新
- [ ] 4.6.5 实现自动兜底开关
  - 组件：Toggle 开关
  - 显示当前状态（继承系统默认 or 用户设置）
  - 切换时调用 API 更新
  - 说明文案（启用后额度不足自动使用余额）
- [ ] 4.6.6 实现订阅优先级拖拽 UI
  - 组件：`web/src/components/user/SubscriptionPriorityList.jsx`
  - 可拖拽排序的订阅列表
  - 拖拽后实时保存（调用 API）
  - 优先级数字显示
  - 说明：越靠前的订阅优先扣费
- [ ] 4.6.7 实现订阅历史记录（订单/兑换/取消）
  - 组件：`web/src/components/user/SubscriptionHistory.jsx`
  - 时间线展示
  - 类型：购买/兑换/延期/折算/取消
  - 详情展示（金额/优惠券/操作人）

#### 4.7 前端 - 更新 Token 管理 UI
- [ ] 4.7.1 新增“订阅优先计费”字段并保留“自动分组”
  - 文件：`web/src/components/token/TokenSettings.jsx`
  - 自动分组：保持原语义（分组不可用时自动回退）
  - 订阅优先：控制是否进入订阅扣费逻辑
- [ ] 4.7.2 更新字段描述和提示文案
  - 开启：请求将遵循系统分组与订阅优先扣费
  - 关闭：跳过订阅扣费，直接使用余额/令牌分组
  - Tooltip 详细说明

#### 4.8 前端 - 国际化文案更新
- [ ] 4.8.1 更新 `web/src/i18n/locales/zh.json`（中文）
  - 订阅相关所有文案
  - 错误提示文案
  - 按钮和标签文案
- [ ] 4.8.2 更新 `web/src/i18n/locales/en.json`（英文）
- [ ] 4.8.3 更新 `web/src/i18n/locales/ja.json`（日文）
- [ ] 4.8.4 更新 `web/src/i18n/locales/ru.json`（俄文）
- [ ] 4.8.5 更新 `web/src/i18n/locales/fr.json`（法文）
- [ ] 4.8.6 更新 `web/src/i18n/locales/vi.json`（越南文）

#### 4.9 前端 - 系统设置中心
- [ ] 4.9.1 实现系统级配置页面（Admin）
  - 文件：`web/src/components/admin/SystemSettings.jsx`
  - 自动兜底默认值开关
  - 到期提醒天数配置
  - 额度低阈值配置
  - 通知渠道配置
- [ ] 4.9.2 实现迁移后首次提醒入口
  - Banner 提示（首次登录后显示）
  - 说明 auto_wallet_fallback 默认关闭
  - 引导用户到设置页面
  - 可关闭（记录到 localStorage）

#### 4.10 前端联调和自测
- [ ] 4.10.1 前后端联调测试
  - 测试所有 API 接口
  - 验证请求/响应格式
  - 验证错误处理
- [ ] 4.10.2 功能自测
  - 测试套餐购买完整流程
  - 测试兑换码完整流程
  - 测试我的订阅页面所有功能
  - 测试优先级拖拽
  - 测试多语言切换

### 阶段5: 定时任务 & 通知 & 监控

#### 5.1 订阅调度器实现
- [ ] 5.1.1 实现 `cron/subscription_scheduler.go` 基础框架
  - 定时任务调度器初始化
  - 配置执行频率（每小时一次）
  - 错误处理和重试机制
- [ ] 5.1.2 实现滚动窗口刷新任务
  - 批量查询过期窗口的订阅
  - 调用 `subscription_usage_service.RefreshWindow`
  - 记录刷新日志
  - 性能优化（批量处理）
- [ ] 5.1.3 实现到期检查任务
  - 查询即将到期的订阅（根据 SUBSCRIPTION_EXPIRY_NOTICE_DAYS）
  - 触发到期通知
  - 自动标记过期订阅（end_at < now）
  - 更新订阅状态为 expired
  - 写入审计日志，必要时写账单（无金额变更则仅审计）
- [ ] 5.1.4 实现限额告警任务
  - 查询usage接近限额的订阅（根据 SUBSCRIPTION_QUOTA_LOW_THRESHOLD）
  - 触发额度低通知
  - 记录告警日志
- [ ] 5.1.5 实现订阅状态同步任务
  - 同步过期订阅状态
  - 刷新缓存
  - 清理无效数据
  - 写入审计日志，必要时写账单（状态/退款等）

#### 5.2 通知模板实现
- [ ] 5.2.1 实现额度低通知模板
  - 模板文件：`templates/notification/subscription_quota_low.html`
  - 内容：订阅名称、周期、剩余额度、剩余百分比
  - 多语言支持
  - CTA：查看我的订阅 / 购买新套餐
- [ ] 5.2.2 实现即将到期通知模板
  - 模板文件：`templates/notification/subscription_expiring.html`
  - 内容：订阅名称、到期时间、剩余天数
  - CTA：续费 / 购买新套餐
- [ ] 5.2.3 实现管理员取消通知模板
  - 模板文件：`templates/notification/subscription_cancelled.html`
  - 内容：取消原因、退款说明
  - CTA：联系客服
- [ ] 5.2.4 实现自动兜底提示模板（首次触发时）
  - 模板文件：`templates/notification/auto_fallback_triggered.html`
  - 内容：订阅额度不足，已自动使用余额
  - 提示：可在设置中关闭自动兜底
  - CTA：查看设置 / 购买新套餐
- [ ] 5.2.5 集成通知配置和用户偏好设置
  - 支持多渠道：邮件/站内通知/第三方（Webhook）
  - 用户偏好：可选择接收哪些通知
  - 管理员强制通知（取消订阅）
  - 频率限制（防止通知轰炸）

#### 5.3 监控指标和大盘
- [ ] 5.3.1 添加 Prometheus 指标
  - `subscription_usage_percent{period}` - 订阅使用率（按周期）
  - `subscription_billing_source_total{source}` - 计费来源统计（subscription/wallet/fallback）
  - `subscription_limit_hit_total` - 订阅限额触发次数
  - `subscription_active_count` - 活跃订阅数
  - `subscription_expiring_count` - 即将到期订阅数
  - `subscription_fallback_rate` - fallback 触发率
  - `subscription_判定_latency_ms` - 订阅判定耗时
  - `subscription_usage_update_qps` - usage 更新 QPS
  - `subscription_cache_hit_rate` - 缓存命中率
- [ ] 5.3.2 更新 Grafana 面板
  - 订阅概览大盘
    - 活跃订阅数趋势
    - 到期订阅数
    - 新购/兑换订阅数（每日）
  - 计费来源大盘
    - 订阅 vs 余额比例饼图
    - fallback 触发率趋势
    - 各套餐使用分布
  - 性能大盘
    - 订阅判定 P99/P95/P50 耗时
    - usage 更新 QPS 趋势
    - 缓存命中率
    - 数据库查询耗时
- [ ] 5.3.3 配置告警规则
  - 性能告警
    - 订阅判定 P99 > 10ms（警告）
    - 订阅判定 P99 > 20ms（严重）
    - usage 更新失败率 > 1%
    - 缓存命中率 < 90%
  - 业务告警
    - fallback 触发率 > 30%（异常）
    - 订阅到期率 > 20%（即将有大量用户流失）
    - 订阅使用量异常增长（可能的滥用）
  - 数据一致性告警
    - usage 超限检测（used > limit）
    - 缓存与数据库不一致
    - 订单支付成功但订阅未激活

#### 5.4 审计日志实现
- [ ] 5.4.1 实现 plan CRUD 审计
  - object_type=subscription_plan
  - action: create/update/publish/unpublish/delete
  - operator_id: 操作管理员 ID
  - metadata: plan详情、变更前后对比
- [ ] 5.4.2 实现订单审计（购买/支付/退款）
  - object_type=subscription_order
  - action: create/paid/failed/refunded
  - metadata: plan_id, user_id, 金额、优惠券、支付渠道
- [ ] 5.4.3 实现优惠券绑定审计
  - object_type=coupon_binding
  - action: bind/unbind/use
  - metadata: coupon_id, redemption_id, user_id
- [ ] 5.4.4 实现兑换审计（叠加/折算）
  - object_type=redemption
  - action: redeem_stack/redeem_coexist/redeem_convert
  - metadata: redemption_code, plan_id, redeem_option, 折算详情
- [ ] 5.4.5 实现取消审计（管理员/用户）
  - object_type=subscription
  - action: cancel_by_admin/cancel_by_user
  - metadata: subscription_id, 取消原因、剩余天数
- [ ] 5.4.6 审计日志查询和导出功能
  - Admin 端查询界面
  - 按时间/类型/操作人筛选
  - 导出为 CSV/Excel

### 阶段6: QA / 发布

#### 6.1 单元测试
- [ ] 6.1.1 plan CRUD 单元测试
  - 套餐创建/更新/删除测试
  - 模型/渠道校验测试
  - 上下架逻辑测试
  - 限额配置验证测试
  - 目标覆盖率：>85%
- [ ] 6.1.2 usage 滚动窗口单元测试
  - 窗口时间计算测试（5小时/日/周/月）
  - 窗口过期判定测试
  - 原子预扣测试（SQL WHERE 条件）
  - 并发预扣测试（1000并发，验证无超扣）
  - 回滚机制测试
  - 差额调整测试
  - 目标覆盖率：>90%（核心算法）
- [ ] 6.1.3 优惠券绑定单元测试
  - 绑定/解绑测试
  - 状态转换测试（active→reserved→used）
  - 绑定冲突测试
  - 绑定后不可修改测试
- [ ] 6.1.4 兑换折算单元测试
  - 同套餐叠加测试
  - 贵套餐并存测试
  - 贵套餐折算测试（向下取整）
  - 折算精度测试（decimal）
  - 边界情况测试（剩余0天、不足1天）
- [ ] 6.1.5 计费优先级单元测试
  - 订阅筛选测试（状态/模型/渠道）
  - 优先级排序测试
  - 预扣遍历测试
  - fallback触发测试
  - 订阅优先关闭跳过测试

#### 6.2 集成测试
- [ ] 6.2.1 购买->扣费->fallback 流程测试
  - 完整购买流程（选择套餐→选优惠券→支付→激活）
  - 订阅扣费流程（请求→判定→预扣→后扣）
  - fallback 流程（额度不足→切换余额→双重账单）
  - 错误处理（支付失败→回滚）
- [ ] 6.2.2 多订阅并存场景测试
  - 多订阅优先级排序
  - 优先级调整（拖拽）
  - 按优先级扣费
  - 多订阅状态同步
- [ ] 6.2.3 订阅优先关闭跳过测试
  - token.subscription_preferred=false
  - 跳过订阅判定
  - 直接使用余额
  - 响应包含 skip_reason
- [ ] 6.2.4 兑换码流程测试
  - 同套餐叠加流程
  - 贵套餐并存流程
  - 贵套餐折算流程
  - 优惠券绑定兑换流程
- [ ] 6.2.5 优惠券绑定和消费测试
  - 管理员绑定优惠券到兑换码
  - 用户兑换时自动消费优惠券
  - 绑定状态同步
  - 兑换后优惠券状态=used

#### 6.3 性能测试和优化
- [ ] 6.3.1 压测订阅判定路径（目标 ≤5ms）
  - 测试场景：1000 QPS 并发请求
  - 测试条件1：缓存命中（热数据）
  - 测试条件2：缓存未命中（冷启动）
  - 测试条件3：多订阅用户（5个订阅）
  - 记录 P50/P95/P99 耗时
  - 目标：P99 ≤5ms
- [ ] 6.3.2 压测 usage 更新（目标 2k QPS）
  - 测试场景：2000 QPS 并发更新
  - 测试条件1：不同订阅（无锁竞争）
  - 测试条件2：相同订阅（锁竞争）
  - 验证原子性（无超扣）
  - 验证无死锁
  - 记录成功率和错误率
  - 目标：成功率 >99.9%，2k QPS 稳定运行
- [ ] 6.3.3 性能瓶颈分析和优化
  - 使用 pprof 分析 CPU 热点
  - 使用慢查询日志分析数据库瓶颈
  - 优化 SQL 查询（索引/JOIN）
  - 优化缓存策略（预热/TTL）
  - 优化锁粒度（减少竞争）
  - 输出性能优化报告

#### 6.4 文档更新
- [ ] 6.4.1 更新 API 文档
  - 文件：`docs/api/web_api.md`
  - 新增订阅相关所有 API 接口
  - 请求/响应示例
  - 错误码说明
  - 认证和权限说明
- [ ] 6.4.2 更新二次开发说明手册
  - 文件：`docs/二次开发说明手册.md`
  - 订阅模块架构说明
  - 滚动窗口算法说明
  - 计费流程说明
  - 扩展点说明
- [ ] 6.4.3 编写订阅使用指南
  - 文件：`docs/guides/subscription_guide.md`
  - 如何创建套餐
  - 如何配置周期限额
  - 如何绑定模型和渠道
  - 用户如何购买订阅
  - 用户如何管理订阅
  - 自动兜底配置说明
- [ ] 6.4.4 编写优惠券使用指南
  - 文件：`docs/guides/coupon_guide.md`
  - 如何创建优惠券
  - 如何绑定兑换码
  - 绑定后不可修改的说明
  - 优惠券使用限制
- [ ] 6.4.5 编写兑换码使用指南
  - 文件：`docs/guides/redemption_guide.md`
  - 如何创建兑换码
  - 同套餐叠加说明
  - 贵套餐二选一说明
  - 折算逻辑说明（附计算示例）
- [ ] 6.4.6 编写 FAQ（常见问题）
  - 文件：`docs/faq/subscription_faq.md`
  - 订阅额度用完怎么办？
  - 如何关闭自动兜底？
  - 多个订阅如何扣费？
  - 兑换码折算怎么计算？
  - 订阅到期后会怎样？
  - 如何申请退款？

#### 6.5 发布准备
- [ ] 6.5.1 配置 feature flag
  - 配置项：`SUBSCRIPTION_V2_ENABLED`
  - 默认值：false（灰度前关闭）
  - 支持动态调整（无需重启）
  - 支持用户ID灰度
  - 支持百分比灰度
- [ ] 6.5.2 准备灰度发布策略
  - 第一阶段：1% 流量（观察 24小时）
    - 选择 100个 测试用户
    - 观察指标：错误率、性能、fallback率
    - 决策：继续/回滚
  - 第二阶段：10% 流量（观察 12小时）
    - 观察指标同上
    - 重点关注性能和数据一致性
  - 第三阶段：50% 流量（观察 6小时）
    - 观察指标同上
    - 确认无异常后全量
  - 第四阶段：100% 全量发布
- [ ] 6.5.3 定义每个灰度阶段的观察指标
  - 错误率：< 0.1%
  - 订阅判定耗时：P99 < 5ms
  - usage 更新成功率：> 99.9%
  - fallback 触发率：< 30%
  - 缓存命中率：> 95%
  - 数据一致性：无 usage 超限
- [ ] 6.5.4 准备灰度回滚 SOP（标准操作流程）
  - 回滚触发条件
    - 错误率 > 1%
    - 性能 P99 > 20ms
    - 数据一致性问题
    - 大量用户投诉
  - 回滚步骤
    1. 关闭 feature flag（立即生效）
    2. 清除相关缓存
    3. 验证回滚成功（错误率恢复正常）
    4. 通知相关人员
    5. 分析问题原因
  - 回滚后数据处理
    - 已创建的订阅保留（不回退）
    - usage 数据保留
    - 账单数据保留
    - 仅计费逻辑回退到旧版本

#### 6.6 线上验证
- [ ] 6.6.1 验证数据迁移准确性
  - 抽样检查 1000个 用户的订阅数据
  - 验证订阅数量一致
  - 验证 usage 初始值正确
  - 验证用户余额未变化
- [ ] 6.6.2 验证计费来源正确性
  - 检查 audit_logs 中的计费记录
  - 验证订阅扣费记录正确
  - 验证 fallback 记录正确
  - 验证双重账单一致性
- [ ] 6.6.3 验证性能指标
  - 实时监控订阅判定耗时（P99 ≤5ms）
  - 实时监控 usage 更新 QPS（峰值 2k QPS）
  - 实时监控缓存命中率（>95%）
  - 实时监控数据库 QPS
- [ ] 6.6.4 验证用户体验
  - 测试套餐购买流程（端到端）
  - 测试兑换码流程（各种场景）
  - 测试我的订阅页面（所有功能）
  - 测试多语言切换
  - 收集用户反馈

#### 6.7 发布执行和监控
- [ ] 6.7.1 准备值班安排
  - 确定发布负责人
  - 确定值班人员（后端/前端/DBA）
  - 准备应急联系方式
  - 准备升级流程
- [ ] 6.7.2 执行发布
  - 第一阶段灰度（1%，观察 24小时）
  - 第二阶段灰度（10%，观察 12小时）
  - 第三阶段灰度（50%，观察 6小时）
  - 全量发布（100%）
- [ ] 6.7.3 实时监控
  - 持续观察 Grafana 面板
  - 关注告警通知
  - 检查错误日志
  - 关注用户反馈渠道
- [ ] 6.7.4 问题处理
  - 快速响应告警
  - 分析错误日志
  - 决策：修复/回滚
  - 执行相应操作
- [ ] 6.7.5 发布复盘
  - 记录发布过程
  - 总结遇到的问题
  - 改进发布流程
  - 更新文档和 SOP

#### 6.8 用户教育和支持
- [ ] 6.8.1 准备用户公告
  - 公告内容：订阅功能上线、自动兜底默认关闭
  - 发布渠道：站内通知/邮件/公告栏
  - 多语言版本
- [ ] 6.8.2 准备引导教程
  - 首次登录引导（Banner提示）
  - 功能亮点介绍
  - 设置指引
- [ ] 6.8.3 准备客服培训材料
  - 订阅功能介绍
  - 常见问题解答
  - 问题排查流程
- [ ] 6.8.4 建立反馈渠道
  - 收集用户反馈
  - 跟踪问题处理
  - 持续优化

---

## 2. 里程碑与依赖
| 里程碑 | 内容 | 依赖 |
|--------|------|------|
| M1 数据模型可用 | 完成阶段0-1 | 无 |
| M2 后端 API Ready | 阶段2 | M1 |
| M3 计费链路切换 | 阶段3 | M2 |
| M4 前端联调完成 | 阶段4 | M2 |
| M5 监控/通知上线 | 阶段5 | M3 |
| M6 发布与验收 | 阶段6 | M3,M4,M5 |

---

## 3. 假设与风险跟踪

### 3.1 关键假设（Assumptions）

本节列出所有关键假设，这些假设需要在实施过程中验证。

#### 3.1.1 技术假设
| 编号 | 假设 | 验证方法 | 风险等级 | 状态 |
|------|------|----------|---------|------|
| A1 | PostgreSQL/MySQL/SQLite 的 UPDATE WHERE 语句可以保证原子性 | 并发测试（1000并发） | 高 | 待验证 |
| A2 | Redis 缓存命中率可达 95% 以上 | 性能测试和监控数据 | 中 | 待验证 |
| A3 | 订阅判定+预扣逻辑可在 5ms 内完成（缓存命中） | 性能基准测试 | 高 | 待验证 |
| A4 | Usage 更新可支持 2k QPS | 压力测试 | 高 | 待验证 |
| A5 | Decimal 类型足以解决浮点精度问题 | 边界值测试 | 中 | 待验证 |
| A6 | 现有数据库连接池可支撑新增负载 | 容量规划和压测 | 中 | 待验证 |
| A7 | Redis 内存容量足够存储所有活跃订阅缓存 | 容量评估（预估10万用户） | 中 | 待验证 |

#### 3.1.2 业务假设
| 编号 | 假设 | 验证方法 | 风险等级 | 状态 |
|------|------|----------|---------|------|
| A8 | 大部分用户同时拥有的订阅数量 ≤ 3 | 用户调研 | 低 | 待验证 |
| A9 | 折算兑换场景占比 < 10% | 运营数据分析 | 低 | 待验证 |
| A10 | 用户可接受默认关闭自动兜底（迁移后） | 用户测试和反馈 | 中 | 待验证 |
| A11 | 订阅额度主要在5小时和日周期消耗 | 业务分析 | 低 | 待验证 |
| A12 | 优惠券绑定主要用于第三方渠道售卖 | 运营确认 | 低 | 已确认 |
| A13 | 用户不会频繁调整订阅优先级（<1次/天） | 用户行为分析 | 低 | 待验证 |

#### 3.1.3 数据假设
| 编号 | 假设 | 验证方法 | 风险等级 | 状态 |
|------|------|----------|---------|------|
| A14 | 存量订阅数据可完整迁移到新结构 | 迁移脚本测试 | 高 | 待验证 |
| A15 | 迁移过程不超过计划停机时间窗口（<30分钟） | 迁移演练 | 高 | 待验证 |
| A16 | 历史账单数据可追溯到订阅来源 | 数据完整性检查 | 中 | 待验证 |
| A17 | Token 字段扩展不影响现有功能 | 回归测试 | 中 | 待验证 |

### 3.2 风险跟踪矩阵

#### 3.2.1 技术风险

| 风险ID | 风险描述 | 影响 | 概率 | 优先级 | 缓解措施 | 应急预案 | 负责人 | 状态 |
|--------|---------|------|------|--------|---------|---------|--------|------|
| R1 | 滚动窗口 UPDATE 竞争严重导致性能问题 | 高 | 中 | P1 | 1. 使用 `UPDATE ... WHERE used + quota <= limit`<br>2. Redis 缓存预判<br>3. 必要时使用 Lua 脚本 | 降级到固定窗口或关闭滚动窗口功能 | 后端 | Open |
| R2 | 并发更新导致超扣或漏扣 | 高 | 中 | P1 | 1. 数据库乐观锁<br>2. Redis Lua 脚本原子操作<br>3. 完整的并发测试 | 人工核对账单并补偿 | 后端 | Open |
| R3 | 缓存与数据库数据不一致 | 高 | 中 | P1 | 1. 写入数据库后立即刷新缓存<br>2. 设置合理 TTL<br>3. 监控缓存命中率 | 清空缓存强制从数据库重新加载 | 后端 | Open |
| R4 | 迁移过程数据丢失或损坏 | 高 | 低 | P1 | 1. 迁移前完整备份<br>2. 演练回滚流程<br>3. 数据一致性校验脚本 | 执行回滚，从备份恢复 | DBA | Open |
| R5 | Feature Flag 切换导致服务不稳定 | 中 | 低 | P2 | 1. 灰度发布策略<br>2. 实时监控关键指标<br>3. 快速回滚机制 | 关闭 Feature Flag，回退到旧逻辑 | DevOps | Open |
| R6 | 定时任务窗口刷新性能不足 | 中 | 中 | P2 | 1. 批量处理<br>2. 分批执行<br>3. 性能优化 | 延长执行间隔或分时段执行 | 后端 | Open |
| R7 | 跨数据库兼容性问题 | 中 | 低 | P2 | 1. 使用 GORM 抽象层<br>2. 避免数据库特定语法<br>3. 全数据库测试 | 针对特定数据库提供专用实现 | 后端 | Open |
| R8 | Redis 内存不足导致缓存失效 | 中 | 低 | P2 | 1. 容量规划<br>2. 设置 LRU 淘汰策略<br>3. 监控内存使用 | 扩容 Redis 或清理冷数据 | DevOps | Open |

#### 3.2.2 业务风险

| 风险ID | 风险描述 | 影响 | 概率 | 优先级 | 缓解措施 | 应急预案 | 负责人 | 状态 |
|--------|---------|------|------|--------|---------|---------|--------|------|
| R9 | 自动兜底默认关闭导致大量限额错误 | 高 | 高 | P1 | 1. 发布前公告<br>2. 首次触发提示<br>3. 准备 FAQ<br>4. 客服培训 | 临时调整系统默认值为 true | 运营 | Open |
| R10 | 多订阅优先级逻辑导致用户困惑 | 中 | 中 | P2 | 1. 清晰的 UI 排序展示<br>2. 默认按到期时间<br>3. 账单标识来源<br>4. 用户教育 | 简化为单一优先级规则 | 产品 | Open |
| R11 | 兑换折算逻辑计算错误 | 高 | 低 | P1 | 1. 统一 service 实现<br>2. 使用 decimal 类型<br>3. 完整的单元测试<br>4. 审计上下文 | 人工核对并补偿 | 后端/财务 | Open |
| R12 | 优惠券绑定被滥用 | 中 | 中 | P2 | 1. 绑定后券进入 reserved 状态<br>2. 仅能被对应兑换码消费<br>3. 记录绑定操作者<br>4. 审计追溯 | 封禁滥用账号，回收优惠券 | 运营 | Open |
| R13 | 用户误操作取消订阅 | 中 | 低 | P3 | 1. 二次确认弹窗<br>2. 提示退款需线下处理<br>3. 保留取消记录 | 允许管理员恢复订阅（24小时内） | 客服 | Open |
| R14 | 套餐定价调整影响存量订单 | 低 | 低 | P3 | 1. 订单保存套餐快照<br>2. 历史数据不受影响 | 无需应急（设计已保护） | 产品 | Open |

#### 3.2.3 运维风险

| 风险ID | 风险描述 | 影响 | 概率 | 优先级 | 缓解措施 | 应急预案 | 负责人 | 状态 |
|--------|---------|------|------|--------|---------|---------|--------|------|
| R15 | 灰度发布期间部分用户体验不一致 | 中 | 高 | P2 | 1. 按用户维度灰度<br>2. 监控两组用户指标<br>3. 快速回滚机制 | 加快灰度进度或全部回滚 | DevOps | Open |
| R16 | 监控告警配置不完善错过问题 | 中 | 中 | P2 | 1. 完整的监控指标<br>2. 合理的告警阈值<br>3. 告警演练 | 人工巡检补充监控盲点 | DevOps | Open |
| R17 | 日志量激增导致存储压力 | 低 | 中 | P3 | 1. 日志分级<br>2. 采样策略<br>3. 定期清理 | 调整日志级别，增加存储 | DevOps | Open |

#### 3.2.4 安全与合规风险

| 风险ID | 风险描述 | 影响 | 概率 | 优先级 | 缓解措施 | 应急预案 | 负责人 | 状态 |
|--------|---------|------|------|--------|---------|---------|--------|------|
| R18 | 未授权访问他人订阅数据 | 高 | 低 | P1 | 1. 权限校验<br>2. 用户隔离<br>3. 安全审计 | 封禁账号，安全加固 | 安全 | Open |
| R19 | 审计日志不完整影响合规 | 中 | 低 | P2 | 1. 完整记录关键操作<br>2. 日志不可篡改<br>3. 定期审查 | 补充遗漏日志，加强审计 | 合规 | Open |
| R20 | 支付数据泄露 | 高 | 低 | P1 | 1. 加密存储<br>2. 最小权限访问<br>3. 安全审计 | 通知受影响用户，加强安全 | 安全 | Open |

#### 3.2.5 用户体验风险

| 风险ID | 风险描述 | 影响 | 概率 | 优先级 | 缓解措施 | 应急预案 | 负责人 | 状态 |
|--------|---------|------|------|--------|---------|---------|--------|------|
| R21 | 订阅页面加载缓慢 | 中 | 中 | P2 | 1. 前端懒加载<br>2. 接口性能优化<br>3. 缓存优化 | 简化页面展示内容 | 前端 | Open |
| R22 | 折算结果展示不清晰 | 中 | 中 | P2 | 1. 详细的计算说明<br>2. 示例展示<br>3. 用户测试 | 提供计算器工具 | 前端/产品 | Open |
| R23 | 多语言翻译不准确 | 低 | 中 | P3 | 1. 专业翻译<br>2. 母语者审核<br>3. 用户反馈机制 | 更新翻译文本 | 产品 | Open |

### 3.3 风险优先级定义

- **P1（严重）**: 影响核心功能或数据安全，必须在发布前解决
- **P2（重要）**: 影响用户体验或系统稳定性，应在发布前解决
- **P3（一般）**: 影响较小，可在后续版本优化

### 3.4 风险状态说明

- **Open**: 风险已识别，缓解措施待执行
- **Mitigating**: 正在执行缓解措施
- **Monitoring**: 缓解措施已执行，持续监控中
- **Closed**: 风险已解除或接受
- **Occurred**: 风险已发生，正在执行应急预案

### 3.5 风险审查计划

- **每周审查**: 检查高优先级风险（P1）的缓解进度
- **双周审查**: 检查所有风险状态，更新假设验证结果
- **里程碑审查**: 在每个阶段完成时，全面评估风险状况
- **发布前审查**: 确认所有 P1 风险已缓解，P2 风险有应急预案

---

**修订记录**
| 版本 | 日期 | 说明 |
|------|------|------|
| 0.1 | 2025-12-03 | 初稿 |
| 0.2 | 2025-12-03 | 增加滚动窗口、多订阅排序、优惠券绑定、兑换折算等任务 |
| 0.3 | 2025-12-04 | 完整细化所有阶段任务，明确执行步骤和验收标准 |

---

## 4. 任务统计

### 总体统计
- **总阶段数**: 6个阶段（阶段0-6）
- **估算总任务数**: 约 200+ 细分任务
- **核心里程碑**: 6个（M1-M6）

### 各阶段任务数估算
- **阶段0**: 需求确认 & 方案冻结（已完成）
- **阶段1**: 数据模型 & 迁移（~25 tasks）
- **阶段2**: 后端基础能力（~80 tasks）
- **阶段3**: 计费链路改造（~20 tasks）
- **阶段4**: 前端实现（~50 tasks）
- **阶段5**: 定时任务 & 通知 & 监控（~25 tasks）
- **阶段6**: QA / 发布（~35 tasks）

### 关键路径
```
阶段1（数据模型）
    ↓
阶段2（后端服务）[关键：滚动窗口算法]
    ↓
阶段3（计费链路）[关键：性能优化 ≤5ms]
    ↓
阶段4（前端实现）
    ↓
阶段5（监控告警）
    ↓
阶段6（测试发布）[关键：灰度发布]
```

### 高风险任务（需重点关注）
1. **阶段2.4**: Usage 滚动窗口服务（核心算法，并发安全）
2. **阶段3.3**: 改造预扣费逻辑（性能要求 P99 ≤5ms）
3. **阶段3.4**: Fallback 双重账单（数据一致性）
4. **阶段6.3**: 性能测试和优化（2k QPS 压测）
5. **阶段6.5**: 灰度发布策略（回滚机制）

### 建议的并行任务
- 阶段2（后端）与阶段4（前端）可部分并行（API契约就绪后）
- 阶段5（监控）可在阶段2-4期间准备
- 阶段6.1-6.2（单元测试/集成测试）可在开发过程中同步进行

---

## 5. 补充说明

### 任务细化原则
1. **MECE 原则**: 任务之间相互独立、完全穷尽
2. **可验收**: 每个任务都有明确的完成标准
3. **可追踪**: 每个任务都可独立跟踪进度
4. **合理粒度**: 单个任务 0.5-2 天工作量

### 与原 tasks.md 的差异
- **原版本**: 粗粒度任务（~30个）
- **当前版本**: 细粒度任务（~200个）
- **主要改进**:
  1. 补充了索引创建、数据校验等基础任务
  2. 细化了滚动窗口算法的实现步骤
  3. 明确了前端组件级别的任务
  4. 增加了性能测试和优化任务
  5. 完善了灰度发布和回滚流程
  6. 增加了用户教育和支持任务

### 使用建议
1. **执行前**: 先完成阶段0（需求确认）和阶段1（数据模型）
2. **执行中**: 按顺序执行阶段2-6，部分任务可并行
3. **风险控制**: 重点关注高风险任务，提前准备应急方案
4. **进度跟踪**: 建议使用项目管理工具（如 Jira/Trello）跟踪任务进度
5. **灵活调整**: 根据实际情况调整任务优先级和执行顺序
