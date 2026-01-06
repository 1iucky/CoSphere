# 设计规格文档: 订阅套餐扩展与计费优先级

**项目**: New API - AI网关与资产管理系统  
**功能**: 扩展订阅套餐 / 优惠券 / 兑换 / 钱包优先级计费  
**创建日期**: 2025-12-03  
**最新修订**: 2025-12-03  
**文档状态**: 草稿

---

## 1. 架构设计

### 1.1 高层架构
```
┌──────────────────────────────────────────────────────────────┐
│                        前端 Web (React)                      │
│  ┌──────────────────────────────────────────────────────┐   │
│  │ Admin: 套餐/优惠券/兑换码/订阅管理                   │   │
│  └──────────────────────────────────────────────────────┘   │
│  ┌──────────────────────────────────────────────────────┐   │
│  │ User: 套餐选购 / 我的订阅 / 自动兜底 / 优先级调整     │   │
│  └──────────────────────────────────────────────────────┘   │
└──────────────────────────────────────────────────────────────┘
                              ↓ REST API
┌──────────────────────────────────────────────────────────────┐
│                     后端 (Go + Gin + GORM)                   │
│  controller/subscription_plan.go (CRUD/上下架)               │
│  controller/subscription_coupon.go                           │
│  controller/subscription_order.go / user_subscription.go     │
│  controller/redemption.go (订阅/优惠券绑定)                 │
│                  ↓                                           │
│  service/subscription_plan_service.go                        │
│  service/subscription_order_service.go                       │
│  service/subscription_usage_service.go (滚动窗口)            │
│  service/subscription_priority_service.go (多订阅排序)       │
│  service/subscription_billing.go (订阅优先扣费)              │
│  service/coupon_service.go (单券+绑定)                      │
│  service/redemption_service.go (叠加/折算)                  │
│                  ↓                                           │
│  model/subscription_plan.go                                  │
│  model/subscription.go / subscription_usage.go               │
│  model/coupon.go / redemption.go (新增字段)                  │
│  model/user_setting.go / bill.go / audit_log.go              │
└──────────────────────────────────────────────────────────────┘
                              ↓
┌──────────────────────────────────────────────────────────────┐
│    PostgreSQL/MySQL/SQLite              Redis                │
│  subscription_plans                     subscription:active  │
│  subscription_plan_limits               subscription:usage   │
│  subscriptions                          coupon cache         │
│  subscription_usages                    redemption cache     │
│  subscription_orders, user_bills        feature flag         │
└──────────────────────────────────────────────────────────────┘
```

### 1.2 模块职责摘要
- **Controller**: HTTP 层,校验请求、鉴权、调用 service,写审计。
- **Service**: 处理业务规则(限额、滚动窗口、扣费优先级、折算)并与模型/缓存交互。
- **Model**: ORM + 数据验证,包含 Redis 序列化。
- **Scheduler**: 定时刷新窗口、到期检测、通知。

---

## 2. 数据模型

### 2.1 数据库设计
1. `subscription_plans`
   - 新字段: `channel_groups TEXT`, `model_whitelist TEXT`, `allow_wallet_fallback BOOL`, `sku`, `i18n JSON`。
2. `subscription_plan_limits`
   - `period ENUM('five_hours','day','week','month')`, `window_strategy ENUM('rolling','fixed','natural')`, `quota INT`, `unit ENUM('quota')`, `enabled BOOL`。
   - **窗口策略说明**:
     - `rolling`: **按需触发滚动窗口**
       - **首次窗口**: 窗口开始时间 = 用户账号**首次实际使用计费**的时间（不是订阅开始时间）
       - **窗口结束后**: 不会立即开始新窗口
       - **后续窗口**: 新窗口开始时间 = 上个窗口结束后用户**下次首次使用**的时间
       - 此逻辑循环往复：窗口结束 → 等待用户下次使用 → 创建新窗口
     - `fixed`/`natural`: 固定/自然窗口 - 使用自然时间边界（如自然日 UTC 0:00-24:00）
3. `subscription_orders`
   - `plan_snapshot JSON`, `coupon_snapshot JSON`, `price_cents`, `final_price_cents`, `payment_channel ENUM('wallet','third_party','redeem')`, `bill_id`。
4. `subscriptions`
   - `priority INT`(默认按 `end_at`), `auto_wallet_fallback BOOL`, `bind_channel_group`, `status ENUM('pending','active','expired','cancelled')`, `redeem_option ENUM('stack','coexist','convert','replace','extend')`。
5. `subscription_usages`
   - `window_start BIGINT`, `window_end BIGINT`, `used_quota INT`, `limit_quota INT`, 唯一键(`subscription_id`,`period`,`window_start`)。

#### 优惠券改造相关表

6. `user_coupons`（用户券实例）
   - `id` BIGINT PK AUTO_INCREMENT/nextval
   - `coupon_id` BIGINT NOT NULL - 优惠券模板ID
   - `user_id` BIGINT NOT NULL - 用户ID
   - `code` VARCHAR(64) NOT NULL - 用户专属核销码（格式：UC-UUID）
   - `status` VARCHAR(32) NOT NULL DEFAULT 'available' - 状态：available/locked/used/expired/invalid
   - `claimed_at` BIGINT NOT NULL - 领取时间
   - `used_at` BIGINT - 使用时间
   - `order_id` BIGINT - 核销关联订单ID
   - `discount_amount` BIGINT DEFAULT 0 - 优惠金额（分）
   - `final_amount` BIGINT - 最终支付金额（分）
   - `created_at` BIGINT
   - `updated_at` BIGINT
   - UNIQUE KEY (`coupon_id`, `user_id`, `code`)
   - INDEX `idx_uc_user_status` (`user_id`, `status`)
   - INDEX `idx_uc_coupon` (`coupon_id`)

7. `coupon_redemption_bindings`（兑换码绑定关系）
   - `id` BIGINT PK AUTO_INCREMENT/nextval
   - `coupon_id` BIGINT NOT NULL - 优惠券ID
   - `redemption_id` BIGINT NOT NULL - 兑换码ID
   - `user_id` BIGINT - 专属用户ID（可为NULL）
   - `status` VARCHAR(32) NOT NULL DEFAULT 'reserved' - 状态：reserved/locked
   - `locked_at` BIGINT - 锁定时间
   - `created_at` BIGINT
   - `updated_at` BIGINT
   - UNIQUE KEY `uq_binding_redemption` (`redemption_id`)
   - INDEX `idx_binding_coupon` (`coupon_id`)
   - INDEX `idx_binding_user` (`user_id`)

8. `coupons` 表扩展字段
   - `type` VARCHAR(32) NOT NULL DEFAULT 'discount' - 类型：discount/full_reduction/instant_reduction
   - `scope` VARCHAR(32) NOT NULL DEFAULT 'wallet_subscription' - 作用域：wallet/subscription/wallet_subscription
   - `threshold_amount` BIGINT NOT NULL DEFAULT 0 - 满减阈值（分）
   - `currency` VARCHAR(8) NOT NULL DEFAULT 'CNY' - 币种
   - `version` BIGINT NOT NULL DEFAULT 0 - 乐观锁版本号
   - 保留 `code` 为领取码，唯一索引
   - code 生成规则：COUPON + YYYYMMDD + 6位随机大写字母数字（如 COUPON20250109AB12CD）

9. `user_bills` 表扩展字段
   - `coupon_id` BIGINT - 使用的优惠券模板ID
   - `user_coupon_id` BIGINT - 使用的用户优惠券ID
   - `discount_amount` BIGINT DEFAULT 0 - 优惠金额（分）
   - `original_amount` BIGINT - 原价（分）
   - `final_amount` BIGINT - 实付（分）
   - `refund_type` VARCHAR(32) - 退款类型：manual/auto/none
   - INDEX `idx_bill_coupon` (`coupon_id`)
   - INDEX `idx_bill_user_coupon` (`user_coupon_id`)

10. `redemptions` 表扩展字段
    - `bound_user_id` BIGINT - 专属用户ID
    - 已有字段：`type ENUM('quota','subscription')`, `payload JSON(plan_id,duration_days,bind_coupon_id,price_paid)`

11. `user_settings`
    - `auto_wallet_fallback BOOL` (默认继承系统设置,迁移后用户=FALSE)。
12. `audit_logs`
    - 拓展 `object_type`=subscription/order/coupon, `metadata JSON`。

### 2.2 Go 结构体 (节选)
```go
type SubscriptionPlan struct {
    Id        int
    Name      string
    PriceCents int
    Currency  string
    BillingCycle string
    BillingCycleValue int
    AllowWalletFallback bool
    ModelWhitelist []string `gorm:"-"`
    ChannelGroups []string `gorm:"-"`
    Limits    []SubscriptionPlanLimit `gorm:"-"`
}

type Subscription struct {
    Id        int
    UserId    int
    PlanId    int
    Status    string
    StartAt   time.Time
    EndAt     time.Time
    Priority  int
    AutoWalletFallback bool
    RedemptionId *int
    CouponId *int
}

type SubscriptionUsage struct {
    Id             int
    SubscriptionId int
    Period         string
    WindowStart    int64
    WindowEnd      int64
    UsedQuota      int
    LimitQuota     int
}
```

### 2.3 Redis 缓存
- `subscription:active:<user_id>`: `{subscriptions:[{id,plan_snapshot,priority,auto_wallet,...}]}` 用于快速筛选可用订阅。
- `subscription:usage:<subscription_id>`: 哈希 `period -> {window_start,window_end,used,limit}`。
- 缓存刷新:
  - 订阅状态变更、优先级调整、usage 更新后写入。
  - TTL 与 `end_at` 对齐。

---

## 3. 核心流程

### 3.1 套餐管理
1. Admin 创建/编辑套餐:
   - 前端表单包含基本信息、模型/渠道多选、周期限额动态行。
   - Controller 调用 service 校验:
     - 周期不重复,quota>0。
     - 模型/渠道存在且可用。
   - GORM 事务: 写 `subscription_plans` + `subscription_plan_limits`。
   - 缓存/事件 `SUB_PLAN_CHANGED` 通知前端刷新。

### 3.2 优惠券 & 绑定

#### 3.2.1 领券流程（事务 + 乐观锁）

**流程步骤**：
1. 查询优惠券模板并加载 `version`（乐观锁）
2. 校验时间、库存、状态
3. 校验用户领取限制（同一用户同一优惠券只能领取一次）
4. 乐观锁更新库存：`WHERE id = ? AND version = ? AND used_count < total_count`
   - `used_count = used_count + 1`
   - `version = version + 1`
5. 插入用户优惠券实例（`user_coupons` 表，`status = 'available'`）
6. 写入审计日志

**并发安全**：
- 预期并发量级：1000 QPS
- 使用乐观锁防止超发
- `WHERE used_count < total_count` 双重保障

#### 3.2.2 核销流程（支付成功内事务/幂等）

**流程步骤**：
1. 获取用户优惠券并加行锁（`FOR UPDATE`）
2. 幂等性检查：若已 `used` 且 `order_id` 相同，直接返回成功
3. 基础校验：时间、作用域、币种（当前固定 CNY）
4. 阈值与计算：满减阈值基于订单原价，计算优惠金额
   - 折扣券：`discount = orderAmount * discountValue / 100`
   - 满减券：若满足阈值，`discount = discountValue`
   - 立减券：`discount = min(discountValue, orderAmount)`
5. 原子更新用户优惠券状态为 `used`
6. 写入 `user_bills` 账单（包含完整优惠券信息）
7. 写入审计日志

**关键要点**：
- 阈值判断基于订单原价（未扣券前）
- 幂等性通过 `order_id` 判断
- 原子性使用条件更新 `WHERE status = 'available'`
- 退款时优惠券不恢复

#### 3.2.3 兑换码绑定券流程

**创建绑定（管理端）**：
1. 校验优惠券存在且状态为 `active`
2. 创建兑换码记录
3. 创建 `coupon_redemption_bindings` 记录（`status = 'reserved'`）
4. 写入审计日志

**兑换使用（用户端）**：
1. 获取兑换码并加行锁
2. 获取绑定记录并加行锁
3. 校验专属用户（若 `user_id` 不为空）
4. 锁定绑定记录（`reserved` → `locked`）
5. 处理用户优惠券实例（创建或更新为 `locked`）
6. 执行兑换业务
7. 成功后将用户券状态改为 `used`
8. 标记兑换码已使用
9. 写入审计日志

**关键要点**：
- 专属用户校验确保兑换码安全
- 绑定状态机：`coupon_redemption_bindings.status`: `reserved` → `locked`
- 事务原子性：失败回滚，状态恢复

#### 3.2.4 优惠券使用规则

- 优惠券仅允许 `1/订单`：购买页面展示用户可用券列表，选择后调用 `coupon_service.CalculateDiscount(planID, couponID)`
- 绑定兑换码：管理员在创建兑换码时可选择"绑定某优惠券 + 用户账号"
- 绑定后在 `coupon_redemption_bindings` 表创建记录（status=`reserved`），仅能随兑换码一起消费
- 在兑换前允许管理员修改/重新绑定（删除/重建 binding 记录），一旦兑换成功即将 binding.status 改为 `locked` 且不可再变更

### 3.3 订阅购买
```
POST /api/user/subscriptions/orders
    |- 校验计划状态 & 模型/渠道
    |- 若使用优惠券 -> 验证库存
    |- 计算折扣,生成订单 + user_bill
    |- 支付(余额/第三方)
    |- 成功 -> ActivateSubscription()
ActivateSubscription()
    |- 若存在同 plan active -> 叠加? (根据业务: 允许并存)
    |- 初始化 subscription + usages (创建各周期窗口, window_start=0)
    |- 写缓存/审计/通知
```

### 3.4 兑换流程
1. `POST /api/user/redemptions/use`:
   - 校验兑换码有效、未使用。
   - 判断 `payload.plan_id` 与现有订阅关系:
     - **同计划**: ExtendSubscription -> end_at += duration, 使用类型 `stack`。
     - **更贵计划**: 前端弹窗,用户选择
       1. `coexist`: 新订阅立即生效,保留旧订阅。
      2. `convert`: 将旧订阅剩余天数折算为金额 `amount_old = remain_days * (plan_price / cycle_days)`,换算为 `delta_days = floor(amount_old / new_plan_daily_price)` (向下取整),然后延长兑换套餐 `end_at += duration + delta_days`,并记录 `redeem_option='convert'` 及折算余量(不足一天金额写入流水)。
   - 若兑换码绑定优惠券,自动调用 `coupon_service.UseBoundCoupon()`。
   - 写订单/账单/审计。

### 3.5 多订阅优先级管理
- `priority` 为正整数（可以不连续，如 1, 3, 10, 100），值越小优先级越高
- **初始化**: 新订阅激活时，按 `end_at` 在现有 priority 序列中动态插入
  - 先按 `priority ASC` 排序现有订阅
  - 在排序后的列表中，找到第一个 `end_at > newSub.EndAt` 的位置
  - 插入到该位置，后续订阅的 `priority` 都 +1
  - 这样**尊重用户已手动调整的 priority 顺序**
- **排序规则**: `priority ASC` → `end_at ASC` → `created_at ASC`
- **用户调整**: 可通过 API 或前端拖拽 UI 设置任意正整数，只要不与其他订阅冲突
- **规范化**: `NormalizePriorities()` 可将 priority 整理为连续序列（1, 2, 3...），非强制
- **冲突检测**: 返回 HTTP 409 (Conflict)
- `subscription_priority_service` 负责排序、校验冲突、更新数据库+缓存

### 3.6 订阅计费流程
```
PreConsumeQuota()
  |- subscription_billing.SelectCandidate(userId, modelName, channelGroup, relayInfo)
        1. 从缓存取 active subscriptions,过滤: status=active && 模型/渠道匹配 && token.subscription_preferred=true
           - 渠道匹配以订阅绑定分组为准,不使用 token 分组
        2. 按 priority 排序
        3. 对每个 subscription:
            - 获取 periods (plan limits)
            - 调用 UsageService.TryPreConsume(subscriptionID, periodList, quota)
                * resolveWindow(): 滚动窗口处理逻辑:
                  - 首次使用(window_start=0): start=当前时间, end=start+duration
                  - 窗口过期(window_end < now): start=当前时间(用户本次请求时间), end=start+duration
                * UPDATE subscription_usages SET used = used + quota WHERE id=? AND used + quota <= limit
            - 全部成功 -> 返回 context{source=subscription, usageKeys}
  |- 若 context=nil -> 走余额路径
  |- 若失败 && auto_wallet=true -> fallback 到余额; else return SUBSCRIPTION_LIMIT_REACHED
PostConsumeQuota()
  |- 如果 source=subscription:
        * 根据真实使用量修正 usage (±diff)
        * 若 fallback 发生,记录 dual-source bill
  |- 记录日志: `logger.LogInfo(user, subscriptionId, quota, periods)`
```
- 令牌层新增订阅优先开关: `token.SubscriptionPreferred`（与 `auto_smart_group` 解耦）。
  - `subscription_preferred = false` 表示跳过订阅判定，直接走余额/令牌分组。
  - `auto_smart_group` 仍用于分组自动回退/熔断，不影响订阅扣费开关。
  - 令牌分组（含多分组优先级）仅用于余额路径的转发选择。

### 3.7 自动兜底配置
- 系统设置: `SUBSCRIPTION_AUTO_WALLET_DEFAULT`。
- `user_settings.auto_wallet_fallback`: 用户可开关; 订阅创建时拷贝到 `subscriptions.auto_wallet_fallback`。
- **显式设置标记**: `dto.UserSetting.AutoWalletFallbackExplicit` (*bool)
  - `nil`: 用户未显式设置，继承系统默认值
  - `非nil`: 用户已显式设置，使用 `auto_wallet_fallback` 的值
- **计费时配置读取优先级**:
  1. 订阅级别: `subscriptions.auto_wallet_fallback`
  2. 用户级别: 若 `AutoWalletFallbackExplicit != nil` → 使用 `users.auto_wallet_fallback`
  3. 系统级别: 若 `AutoWalletFallbackExplicit == nil` → 使用 `SUBSCRIPTION_AUTO_WALLET_DEFAULT`
- 迁移脚本: 将所有现有用户设置为 `false` 并发送提示(通知/公告)。

### 3.8 取消/人工退款
- 管理端 `POST /api/admin/subscriptions/:id/cancel`:
  - 校验状态=active/pending。
  - 更新 status=cancelled, end_at=now。
  - 写审计、bill(type=manual_refund) (金额由管理员输入,仅用于记录)。
  - 根据系统配置与用户通知偏好推送提醒(邮件/站内/第三方),提示退款需线下处理。

### 3.9 通知 & 审计
- `subscription_scheduler` 每小时执行:
  - 刷新 usage 窗口。
  - 检查 `end_at - now < threshold` -> 发送 `subscription_expiring`。
  - 检查 `used/limit > threshold` -> 发送 `subscription_quota_low`。
- 审计内容: 操作人、对象、动作、上下文(JSON),用于合规追溯。

---

## 4. API 设计(关键)

### 4.1 管理端 API

| 方法 | Path | 描述 |
|------|------|------|
| GET | /api/admin/subscription-plans | 套餐列表/筛选 |
| POST | /api/admin/subscription-plans | 创建/编辑套餐 |
| POST | /api/admin/subscription-plans/:id/publish | 套餐上下架 |
| GET | /api/admin/subscriptions | 查看所有用户订阅,支持取消 |
| POST | /api/admin/subscriptions/:id/cancel | 管理员取消订阅 |
| POST | /api/admin/coupons | 创建优惠券 |
| PUT | /api/admin/coupons/:id | 更新优惠券 |
| GET | /api/admin/coupons | 优惠券列表（支持状态/作用域/关键词筛选） |
| POST | /api/admin/redemptions | 创建兑换码并绑定优惠券 |
| GET | /api/admin/redemptions | 兑换码列表（包含绑定优惠券信息） |

### 4.2 用户端 API

| 方法 | Path | 描述 |
|------|------|------|
| GET | /api/user/subscription-plans | 可购买套餐列表 |
| POST | /api/user/subscriptions/orders | 创建订单+支付（支持使用优惠券） |
| GET | /api/user/subscriptions/active | 当前订阅+usage+自动兜底 |
| PUT | /api/user/subscriptions/:id/auto-wallet | 切换自动兜底 |
| PUT | /api/user/subscriptions/priorities | 批量更新订阅优先级顺序 |
| POST | /api/user/redemptions/use | 兑换订阅（支持绑定优惠券） |
| POST | /api/user/coupons/claim | 用户领取优惠券 |
| GET | /api/user/coupons | 我的优惠券列表（支持状态筛选） |
| POST | /api/user/payment/coupon/preview | 优惠券使用预览（可选，计算折扣） |
| GET | /dashboard/billing/subscription | (已有)扩展返回周期滚动数据 |

响应中需包含:
```json
{
  "subscription": {
    "id": 21,
    "plan": {...},
    "usage": {
      "five_hours": {"window_start": 1735900000, "window_end": 1736088000, "used": 1200, "limit": 5000, "display_usd": 0.12},
      ...
    },
    "auto_wallet_fallback": false,
    "priority": 1
  },
  "billing_hint": {
    "skip_reason": "subscription_preferred_disabled"
  }
}
```

---

## 5. 迁移与配置
1. Feature Flag: `SUBSCRIPTION_V2_ENABLED` 控制新逻辑启用。
2. 数据库迁移:
```sql
ALTER TABLE redemptions ADD COLUMN type VARCHAR(32) DEFAULT 'quota';
ALTER TABLE redemptions ADD COLUMN payload TEXT;
ALTER TABLE coupons ADD COLUMN scope VARCHAR(32) DEFAULT 'quota';
ALTER TABLE coupons ADD COLUMN bind_user_id INT;
ALTER TABLE coupons ADD COLUMN bind_redemption_id INT;
ALTER TABLE subscriptions ADD COLUMN priority INT NOT NULL DEFAULT 1;
ALTER TABLE subscriptions ADD COLUMN auto_wallet_fallback BOOLEAN DEFAULT false;
ALTER TABLE users ADD COLUMN auto_wallet_fallback BOOLEAN DEFAULT false;
CREATE TABLE subscription_plans (...);
CREATE TABLE subscription_plan_limits (...);
CREATE TABLE subscription_orders (...);
CREATE TABLE subscription_usages (...);
```
3. 迁移脚本:
   - 将旧 `subscriptions` 数据转为 `subscription_plans` + `subscription_plan_limits` 的默认记录。
   - 初始化 `subscription_usages` (window_start=0, used=0)。
   - 保留 `token.auto_smart_group`，新增 `token.subscription_preferred`（迁移需同步 Redis 缓存字段）。

---

## 6. 监控与日志
- Prometheus:
  - `subscription_usage_percent{period}`
  - `subscription_billing_source_total{source}`
  - `subscription_limit_hit_total`
- 日志:
  - `logger.LogInfo` on success, `LogWarn` on fallback, `LogError` on failure。
  - Audit log entries for plan CRUD, coupon bind, redeem, cancel.

---

## 7. 错误码规范

### 7.1 业务错误码定义

所有订阅相关的错误码遵循统一规范，确保前后端一致性。

#### 订阅相关错误码
| 错误码 | HTTP状态码 | 说明 | 触发场景 |
|--------|-----------|------|----------|
| `SUBSCRIPTION_LIMIT_REACHED` | 429 | 订阅额度已达上限 | 用户请求时订阅额度已用完且未开启自动兜底 |
| `SUBSCRIPTION_NOT_FOUND` | 404 | 订阅不存在 | 查询或操作不存在的订阅ID |
| `SUBSCRIPTION_EXPIRED` | 403 | 订阅已过期 | 尝试使用已过期的订阅 |
| `SUBSCRIPTION_INACTIVE` | 403 | 订阅未激活 | 订阅状态为 pending/cancelled |
| `SUBSCRIPTION_ALREADY_EXISTS` | 409 | 订阅已存在 | 重复创建相同套餐的订阅（若不允许并存） |
| `SUBSCRIPTION_PRIORITY_CONFLICT` | 409 | 优先级冲突 | 调整优先级时出现冲突 |

#### 套餐相关错误码
| 错误码 | HTTP状态码 | 说明 | 触发场景 |
|--------|-----------|------|----------|
| `PLAN_NOT_FOUND` | 404 | 套餐不存在 | 套餐ID不存在或已删除 |
| `PLAN_NOT_PUBLISHED` | 403 | 套餐未上架 | 尝试购买未上架的套餐 |
| `PLAN_MODEL_WHITELIST_INVALID` | 400 | 模型白名单无效 | 模型ID不存在或不可用 |
| `PLAN_CHANNEL_GROUP_INVALID` | 400 | 渠道分组无效 | 渠道分组ID不存在 |
| `PLAN_PERIOD_DUPLICATE` | 400 | 周期重复 | 限额配置中存在重复的周期类型 |
| `PLAN_PRICE_INVALID` | 400 | 价格无效 | 价格小于等于0或格式错误 |

#### 优惠券相关错误码
| 错误码 | HTTP状态码 | 说明 | 触发场景 |
|--------|-----------|------|----------|
| `COUPON_NOT_FOUND` | 404 | 优惠券不存在 | 优惠券ID不存在 |
| `COUPON_EXPIRED` | 403 | 优惠券已过期 | 优惠券超过有效期 |
| `COUPON_NOT_APPLICABLE` | 400 | 优惠券不适用 | 优惠券不适用于当前套餐 |
| `COUPON_USAGE_LIMIT_REACHED` | 429 | 优惠券使用次数已达上限 | 用户使用次数或总发行量已用完 |
| `COUPON_BINDING_LOCKED` | 403 | 优惠券绑定已锁定无法修改 | `coupon_redemption_bindings.status = locked` |
| `COUPON_ALREADY_BOUND` | 409 | 优惠券已被绑定 | 兑换码已绑定优惠券（一对一约束） |
| `COUPON_BINDING_RESERVED` | 403 | 优惠券绑定处于预留状态 | `coupon_redemption_bindings.status = reserved`，需通过兑换码使用 |
| `COUPON_ALREADY_CLAIMED` | 409 | 用户已领取该优惠券 | 同一用户重复领取同一优惠券 |
| `COUPON_STOCK_INSUFFICIENT` | 429 | 优惠券库存不足 | 领取时库存已用完 |
| `COUPON_ALREADY_USED` | 403 | 优惠券已使用 | 重复使用已核销的优惠券 |
| `COUPON_STATUS_INVALID` | 400 | 优惠券状态无效 | 优惠券状态不是 available |
| `COUPON_SCOPE_MISMATCH` | 400 | 优惠券作用域不匹配 | 优惠券不适用于当前场景（如 wallet 券用于 subscription 订单） |
| `COUPON_THRESHOLD_NOT_MET` | 400 | 未达满减阈值 | 订单金额未达满减门槛 |
| `COUPON_CURRENCY_MISMATCH` | 400 | 币种不匹配 | 优惠券币种与订单不一致 |
| `COUPON_TYPE_INVALID` | 400 | 优惠券类型无效 | 优惠券类型不合法 |
| `COUPON_TEMPLATE_NOT_FOUND` | 404 | 优惠券模板不存在 | 优惠券模板ID不存在 |
| `COUPON_CONCURRENT_CONFLICT` | 409 | 并发冲突 | 乐观锁版本冲突，请重试 |
| `COUPON_USER_MISMATCH` | 403 | 优惠券不属于该用户 | 用户使用他人的优惠券 |
| `COUPON_REDEMPTION_LOCKED` | 403 | 兑换码绑定已锁定 | 绑定已锁定无法使用 |
| `COUPON_BINDING_QUERY_FAILED` | 500 | 查询优惠券绑定信息失败 | 数据库查询绑定关系出错 |
| `COUPON_USE_FAILED` | 500 | 使用优惠券失败 | 消费优惠券时发生技术错误 |
| `COUPON_USER_LIMIT_REACHED` | 429 | 用户领券频率超限 | 单用户领券频率超过 10次/分钟 |
| `COUPON_INSUFFICIENT_QUOTA` | 429 | 优惠券库存不足 | 优惠券库存已用完 |

#### 兑换码相关错误码
| 错误码 | HTTP状态码 | 说明 | 触发场景 |
|--------|-----------|------|----------|
| `REDEMPTION_INVALID` | 400 | 兑换码无效 | 兑换码格式错误或不存在 |
| `REDEMPTION_USED` | 403 | 兑换码已使用 | 兑换码已被使用 |
| `REDEMPTION_EXPIRED` | 403 | 兑换码已过期 | 兑换码超过有效期 |
| `REDEMPTION_DISABLED` | 403 | 兑换码已被禁用 | 兑换码状态为禁用 |
| `REDEMPTION_CONFLICT` | 409 | 兑换冲突 | 用户已有更贵套餐，需选择处理方式 |
| `REDEMPTION_PLAN_MISMATCH` | 400 | 套餐不匹配 | 兑换码绑定的套餐与payload不一致 |
| `REDEMPTION_USER_MISMATCH` | 403 | 兑换码不属于该用户 | 非专属用户使用专属兑换码 |

#### 订单和支付相关错误码
| 错误码 | HTTP状态码 | 说明 | 触发场景 |
|--------|-----------|------|----------|
| `ORDER_NOT_FOUND` | 404 | 订单不存在 | 订单ID不存在 |
| `ORDER_ALREADY_PAID` | 409 | 订单已支付 | 重复支付订单 |
| `INSUFFICIENT_BALANCE` | 402 | 余额不足 | 钱包余额不足以支付订单 |
| `PAYMENT_FAILED` | 500 | 支付失败 | 支付渠道返回失败 |

#### 权限和验证相关错误码
| 错误码 | HTTP状态码 | 说明 | 触发场景 |
|--------|-----------|------|----------|
| `UNAUTHORIZED_OPERATION` | 403 | 无权操作 | 用户尝试操作他人的订阅 |
| `ADMIN_PERMISSION_REQUIRED` | 403 | 需要管理员权限 | 非管理员访问管理接口 |
| `INVALID_REQUEST_PARAMS` | 400 | 请求参数无效 | 参数格式错误或缺失必填字段 |
| `IDEMPOTENCY_KEY_CONFLICT` | 409 | 幂等键冲突 | 使用相同幂等键重复请求 |

#### 窗口和额度相关错误码
| 错误码 | HTTP状态码 | 说明 | 触发场景 |
|--------|-----------|------|----------|
| `USAGE_WINDOW_EXPIRED` | 410 | 使用窗口已过期 | 窗口已过期但尚未刷新 |
| `USAGE_QUOTA_EXCEEDED` | 429 | 额度超限 | 预扣时发现已超过限额 |
| `CONCURRENT_UPDATE_CONFLICT` | 409 | 并发更新冲突 | 使用量更新时的乐观锁冲突 |

### 7.2 错误响应格式

所有 API 错误响应遵循统一格式：

```json
{
  "success": false,
  "message": "订阅额度已达上限",
  "error": {
    "code": "SUBSCRIPTION_LIMIT_REACHED",
    "details": {
      "subscription_id": 123,
      "period": "five_hours",
      "used_quota": 5000,
      "limit_quota": 5000,
      "auto_wallet_fallback": false
    },
    "hint": "您可以在设置中启用自动余额兜底功能"
  }
}
```

### 7.3 错误码实现位置

- **定义**: `common/constants.go` 或 `common/errors.go`
- **国际化**: `web/src/i18n/locales/{lang}.json`
- **文档**: API 文档和用户手册

---

## 8. 技术验收标准

与 requirements.md 第7节的业务验收标准对应，技术验收标准如下：

### 8.1 后端验收标准

#### 数据库和模型层
- [ ] 所有新表已创建并符合跨数据库兼容性（PostgreSQL/MySQL/SQLite）
- [ ] 索引和约束已正确创建，性能测试通过
- [ ] 迁移脚本可正确执行 UP 和 DOWN 操作
- [ ] 数据迁移脚本已验证，存量数据无损转换
- [ ] Model 层单元测试覆盖率 ≥ 80%

#### 服务层
- [ ] 套餐管理服务（CRUD、上下架、校验）实现完成，单元测试通过
- [ ] 滚动窗口算法实现完成，并发测试通过（1000并发无超扣）
- [ ] 订阅优先级服务实现完成，排序逻辑正确
- [ ] 优惠券服务扩展完成（绑定、解绑、验证）
- [ ] 兑换服务支持订阅类型（叠加、折算、并存）
- [ ] 订单服务实现完成（创建、支付、激活）
- [ ] 计费链路改造完成，订阅优先扣费逻辑正确
- [ ] 自动兜底逻辑实现完成，配置继承正确
- [ ] Service 层单元测试覆盖率 ≥ 80%

#### API 层
- [ ] 所有管理员 API 实现完成并通过集成测试
- [ ] 所有用户 API 实现完成并通过集成测试
- [ ] API 响应格式符合规范，错误码正确返回
- [ ] 幂等性机制已实现（金额相关操作）
- [ ] 权限校验正确（管理员/用户隔离）
- [ ] API 文档已生成（Swagger/OpenAPI）

#### 性能要求
- [ ] 订阅判定 + 预扣耗时 ≤ 5ms（缓存命中场景）
- [ ] Usage 更新支持 2k QPS（压测验证）
- [ ] 滚动窗口刷新批量处理性能 ≥ 1000 订阅/秒
- [ ] Redis 缓存命中率 ≥ 95%

#### 数据一致性
- [ ] 订阅 Usage 更新具备原子性（无超扣现象）
- [ ] 订阅与余额不会同时扣费（双重账单防护）
- [ ] 优惠券绑定状态机转换正确
- [ ] 事务回滚机制验证通过

#### 审计和日志
- [ ] 所有关键操作写入 audit_logs（操作人、对象、动作、上下文）
- [ ] 所有支付/兑换操作写入 user_bills
- [ ] 日志包含必要信息（user_id, subscription_id, quota, source）
- [ ] 审计日志可追溯且不可篡改

### 8.2 前端验收标准

#### 管理端
- [ ] 套餐管理页面实现完成（创建、编辑、上下架）
- [ ] 套餐限额配置支持动态添加/删除周期
- [ ] 模型和渠道分组支持多选
- [ ] 优惠券管理页面实现完成
- [ ] 兑换码管理页面支持优惠券绑定
- [ ] 订阅管理页面可查看所有用户订阅
- [ ] 取消订阅功能实现（需填写原因）
- [ ] 人工退款流水录入功能实现

#### 用户端
- [ ] 套餐选购页面实现（展示套餐、周期限额、价格）
- [ ] 购买流程支持选择优惠券（单张）
- [ ] "我的订阅"页面展示所有订阅及详细信息
- [ ] 多周期余量和倒计时显示正确
- [ ] 自动余额兜底开关实现
- [ ] 订阅优先级拖拽调整实现（实时同步后端）
- [ ] 兑换码兑换流程实现（同套餐叠加、更贵套餐二选一）
- [ ] 折算结果展示清晰（新增天数、剩余金额）
- [ ] 历史记录查询（订单、兑换、取消）

#### 国际化
- [ ] 所有新增文案已添加到 i18n 文件（中文、英文）
- [ ] 错误提示信息已国际化
- [ ] 日期时间格式正确（考虑时区）

### 8.3 系统集成验收标准

#### 定时任务
- [ ] 窗口刷新定时任务实现并运行正常
- [ ] 到期提醒定时任务实现并运行正常
- [ ] 额度低阈值提醒定时任务实现并运行正常

#### 通知系统
- [ ] 订阅到期提醒通知实现
- [ ] 额度低提醒通知实现
- [ ] 取消订阅通知实现（遵循用户偏好）
- [ ] 通知渠道支持：站内/邮件/Webhook

#### 监控和告警
- [ ] Prometheus 指标已添加（usage_percent, billing_source, limit_hit）
- [ ] 日志输出格式统一且结构化
- [ ] 关键错误有告警配置

### 8.4 测试验收标准

#### 单元测试
- [ ] Model 层测试覆盖率 ≥ 80%
- [ ] Service 层测试覆盖率 ≥ 80%
- [ ] 所有边界情况已测试（空值、零值、负值、超大值）

#### 集成测试
- [ ] 完整购买流程测试通过
- [ ] 完整兑换流程测试通过（所有分支）
- [ ] 折算逻辑精度测试通过
- [ ] 优先级调整测试通过
- [ ] 自动兜底切换测试通过

#### 性能测试
- [ ] 订阅判定性能测试（P99 ≤ 5ms）
- [ ] Usage 更新压测（2k QPS 无错误）
- [ ] 并发预扣测试（1000 并发无超扣）
- [ ] 滚动窗口刷新性能测试（100万订阅数据）

#### 兼容性测试
- [ ] PostgreSQL 数据库测试通过
- [ ] MySQL 数据库测试通过
- [ ] SQLite 数据库测试通过
- [ ] 浏览器兼容性测试通过（Chrome, Firefox, Safari, Edge）

### 8.5 部署验收标准

#### 迁移
- [ ] 迁移脚本在测试环境演练成功
- [ ] 迁移执行手册已准备
- [ ] 回滚方案已准备并演练
- [ ] 数据一致性校验脚本已准备

#### 灰度发布
- [ ] Feature Flag 已配置（SUBSCRIPTION_V2_ENABLED）
- [ ] 灰度策略已制定（按用户比例）
- [ ] 灰度监控已就绪
- [ ] 回滚方案已准备

#### 用户通知
- [ ] 用户通知/公告已准备（自动兜底默认关闭）
- [ ] 帮助文档/FAQ 已准备
- [ ] 客服培训已完成

---

## 9. 风险与缓解
| 风险 | 影响 | 缓解 |
|------|------|------|
| 滚动窗口实现复杂、写压力大 | 限额统计失准 | 预扣使用 `UPDATE ... WHERE used + quota <= limit` + Redis 缓存 + 乐观锁,必要时用 Lua 脚本 |
| 多订阅优先级导致用户困惑 | 投诉 | 前端提供清晰排序 UI + 默认按到期时间,并在账单中标识来源 |
| 兑换折算逻辑出错 | 财务风险 | 在 service 中统一实现,所有金额使用 decimal,并写审计上下文 |
| 绑定优惠券滥用 | 财务流失 | 绑定后券进入 reserved 状态,仅能被对应兑换码消费,并记录绑定操作者 |
| 自动兜底迁移默认关闭导致大量限额不足错误 | 体验下降 | 发布前公告 + 第一次触发提示用户开启兜底功能 |
| 并发更新导致数据不一致 | 超扣或漏扣 | 使用数据库乐观锁或 Redis Lua 脚本保证原子性 |
| 迁移过程数据丢失 | 业务中断 | 迁移前完整备份，演练回滚流程，制定应急预案 |

---

**修订记录**
| 版本 | 日期 | 说明 |
|------|------|------|
| 0.1 | 2025-12-03 | 初稿 |
| 0.2 | 2025-12-03 | 根据业务澄清补充滚动窗口、多订阅优先级、优惠券绑定与兑换折算逻辑 |
| 1.0 | 2025-12-04 | 新增完整的错误码规范（第7节）和技术验收标准（第8节），与 requirements.md 对齐 |
| 1.1 | 2025-12-07 | 更新窗口策略说明（支持 rolling/fixed/natural）、兑换选项枚举（补充 replace/extend）|
| 1.2 | 2025-12-22 | 业务澄清更新：完善滚动窗口触发逻辑、补充 AutoWalletFallbackExplicit 字段说明、完善错误码状态码映射 |
