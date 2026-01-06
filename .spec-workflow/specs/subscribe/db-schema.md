# 数据库模型设计: 订阅套餐扩展

**文件目标**: 梳理订阅套餐、优惠券、兑换码、计费优先级相关的数据表结构、字段含义、索引及迁移注意事项,供数据库迁移和模型编码使用。

**最新修订**: 2025-12-04
**对比基准**: data/DB_DDL/DDL.sql（当前数据库 DDL）
**差距分析**: 详见 db-schema-gap-analysis.md

---

## 0. 现有表 vs 计划新增表

⚠️ **重要说明**: 本文档描述的订阅相关表（subscription_*、coupons、user_bills、audit_logs）在当前数据库中**完全不存在**，需要通过迁移脚本**完全新建**。

### 0.1 现有基础表（data/DB_DDL/DDL.sql）

| 表名 | 用途 | 关键字段 | 是否需要修改 |
|------|------|----------|------------|
| users | 用户表 | id, username, quota, used_quota, **setting TEXT** | ⚠️ 扩展 setting JSON 结构 |
| tokens | 令牌表 | id, user_id, **auto_smart_group boolean default false**, **subscription_preferred boolean default false**, group_priorities | ⚠️ 新增字段 |
| redemptions | 兑换码表 | id, user_id, key, status, quota | ⚠️ 新增 type 和 payload 字段 |
| options | 系统配置表 | key TEXT, value TEXT | ⚠️ 插入订阅配置项 |
| logs | API 日志表 | id, user_id, type, model_name, quota | ✅ 无需修改 |
| top_ups | 充值记录表 | id, user_id, amount bigint, money numeric | ✅ 保留（与 user_bills 并存） |
| quota_data | 用量统计表 | id, user_id, model_name, token_used | ✅ 保留（用途不同） |

### 0.2 计划新增表（需要完全新建）

| 表名 | 用途 | 依赖 | 建表阶段 |
|------|------|------|---------|
| **subscription_plans** | 订阅套餐主表 | 无 | Phase 1 |
| **subscription_plan_limits** | 套餐限额配置 | subscription_plans | Phase 1 |
| **coupons** | 优惠券表 | 无 | Phase 1 |
| **subscription_orders** | 订单表 | subscription_plans, users | Phase 1 |
| **subscriptions** | 订阅表 | subscription_plans, subscription_orders, users | Phase 1 |
| **subscription_usages** | 订阅使用量（滚动窗口） | subscriptions | Phase 1 |
| **user_bills** | 统一账单表 | users | Phase 2 |
| **audit_logs** | 审计日志表 | users | Phase 2 |

### 0.3 现有表字段修改清单

**高优先级修改**:
1. **tokens 字段扩展**
   - 新增字段: `subscription_preferred`（订阅优先扣费开关）
   - 保留字段: `auto_smart_group`（自动智能分组）
   - 默认值: 均为 `false`
   - 风险: 需要清理 Redis 缓存，更新 API 响应

2. **redemptions 新增字段**
   - `type VARCHAR(32) DEFAULT 'quota'` - 兑换码类型（quota/subscription）
   - `payload TEXT` - 扩展数据（JSON 格式）
   - 存量数据: 默认 type='quota'

3. **options 插入配置**
   - `SUBSCRIPTION_AUTO_WALLET_DEFAULT` = 'false'
   - `SUBSCRIPTION_EXPIRY_NOTICE_DAYS` = '7'
   - `SUBSCRIPTION_QUOTA_LOW_THRESHOLD` = '0.2'
   - `SUBSCRIPTION_V2_ENABLED` = 'false'（Feature Flag）
   - `SUBSCRIPTION_MAX_PER_USER` = '10'

**中优先级修改**:
4. **users.setting 初始化**
   - 现有字段: `setting TEXT`
   - 初始化默认值: `{"auto_wallet_fallback":false,...}`
   - 存量用户: 需要 UPDATE 设置默认 JSON

### 0.4 关键差距与决策点

**已确认的差距**:
1. ❌ 所有订阅相关表不存在（8个表需要新建）
2. ⚠️ tokens 字段名和默认值与设计不符
3. ⚠️ redemptions 缺少 type/payload 字段
4. ⚠️ options 无订阅配置项
5. ⚠️ 用户设置方案：使用 users.setting JSON（不新建 user_settings 表）

**待决策的问题**（见 db-schema-gap-analysis.md）:
1. ✅ **外键约束策略**: 不使用 FK，仅索引 + 应用层约束（遵循现有表规范）
2. ✅ **user_bills 与 top_ups**: 并存（top_ups 保留，user_bills 记录所有类型）
3. ✅ **金额字段类型**: 统一使用 BIGINT（已修正文档中的 INT）
4. ✅ **迁移执行窗口**: 无时间要求（研发阶段）
5. ✅ **Feature Flag 灰度**: 无灰度要求（开发时手动控制）

---

## 1. ER 概览

```
User (users) [现有表]
  ├── [users.setting JSON] - 用户设置（包含 auto_wallet_fallback）[扩展]
  ├── Subscription (subscriptions) [新建]
  │       ├── SubscriptionUsage (subscription_usages) [新建]
  │       ├── SubscriptionOrder (subscription_orders) [新建]
  │       └── SubscriptionPlan (subscription_plans) [新建]
  │               └── SubscriptionPlanLimit (subscription_plan_limits) [新建]
  ├── Coupon (coupons) [新建]
  ├── UserBills (user_bills) [新建]
  ├── AuditLog (audit_logs) [新建]
  ├── Redemption (redemptions) [现有表，扩展 type/payload 字段]
  └── Token (tokens) [现有表，新增 subscription_preferred 字段]
```

**说明**:
- **[现有表]**: 已存在于 data/DB_DDL/DDL.sql
- **[新建]**: 需要通过迁移脚本完全新建
- **[扩展]**: 需要在现有表基础上添加字段或修改字段

**数据存储约定**:
- 所有金额使用 **BIGINT** 存储（美分单位），与现有 quota/amount 逻辑一致
- 所有时间戳使用 **BIGINT** 存储（Unix 秒级时间戳，UTC 时区）
- JSON 字段使用 **TEXT** 类型（兼容 PostgreSQL/MySQL/SQLite）
- 需要展示为 USD 时按 `common.QuotaPerUnit` 或价格比率换算

**金额字段类型统一**:
- ✅ users.quota: BIGINT（现有）
- ✅ logs.quota: BIGINT（现有）
- ✅ top_ups.amount: BIGINT（现有）
- ✅ subscription_plans.price_cents: BIGINT（新建，修正原 INT）
- ✅ subscription_orders.price_cents: BIGINT（新建，修正原 INT）
- ✅ user_bills.amount: BIGINT（新建）

---

## 2. 新增表

### 2.1 subscription_plans [新建表]
| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | BIGINT | PK | 自增 |
| sku | VARCHAR(64) | UNIQUE | 套餐 SKU, 用于第三方对接 |
| name | VARCHAR(128) | NOT NULL | 多语言 key, 结合 i18n 表/JSON |
| description | TEXT |  | Markdown/富文本描述 |
| price_cents | BIGINT | NOT NULL | 支付金额(美分/内部单位) ⚠️ 修正: 原 INT 改为 BIGINT |
| currency | VARCHAR(8) | 默认 `USD` | 预留多币种 |
| billing_cycle | VARCHAR(32) | NOT NULL | 计费周期类型: monthly/yearly/custom |
| billing_cycle_value | INT | 默认 1 | 当 custom 时表示天数,month/year 固定 |
| allow_wallet_fallback | BOOLEAN | 默认 true | 套餐级别是否允许余额兜底 |
| status | VARCHAR(32) | 默认 `draft` | 上下架状态: draft/active/inactive |
| start_at | BIGINT |  | 套餐生效时间(秒) |
| end_at | BIGINT |  | 停售时间,0 表示无限 |
| model_whitelist | TEXT | JSON | 可用模型列表 |
| channel_groups | TEXT | JSON | 可使用的渠道分组,为空表示不限 |
| extra | TEXT | JSON | 自定义配置,如封面、tag |
| created_at/updated_at | BIGINT |  | 秒级时间戳 |

**索引**:
- `idx_subscription_plans_status` (status)
- `idx_subscription_plans_sku` (unique)

### 2.2 subscription_plan_limits [新建表]
| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | BIGINT | PK |
| plan_id | BIGINT | NOT NULL | 关联 subscription_plans.id（应用层约束） |
| period | VARCHAR(32) | NOT NULL | five_hours/day/week/month |
| quota | BIGINT | NOT NULL | 上限(内部 quota 单位) ⚠️ 修正: 原 INT 改为 BIGINT |
| unit | VARCHAR(32) | 预留 | 默认 quota |
| enabled | BOOLEAN | 默认 true |
| window_strategy | VARCHAR(32) | 默认 `rolling` |
| created_at/updated_at | BIGINT |  |

**约束**: unique(plan_id, period)
**索引**:
- `idx_subscription_plan_limits_plan_id` (plan_id)

### 2.3 subscription_orders [新建表]
| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | BIGINT | PK |
| user_id | BIGINT | NOT NULL | 关联 users.id（应用层约束） |
| plan_id | BIGINT | NOT NULL | 关联 subscription_plans.id（应用层约束） |
| plan_snapshot | TEXT | NOT NULL | 下单时套餐快照(JSON 格式) |
| coupon_id | BIGINT | 可空 | 使用的优惠券 id（应用层约束） |
| coupon_snapshot | TEXT |  | 优惠券快照(JSON 格式) |
| payment_channel | VARCHAR(32) | NOT NULL | wallet/third_party/redeem |
| price_cents | BIGINT | NOT NULL | 原价 ⚠️ 修正: 原 INT 改为 BIGINT |
| discount_cents | BIGINT | 默认 0 | 优惠金额 ⚠️ 修正: 原 INT 改为 BIGINT |
| final_price_cents | BIGINT | NOT NULL | 实付金额 ⚠️ 修正: 原 INT 改为 BIGINT |
| redemption_id | BIGINT | 可空 | 对应兑换码记录（应用层约束） |
| bill_id | BIGINT | 可空 | 对应 user_bills 记录（应用层约束） |
| status | VARCHAR(32) | 默认 `pending` | pending/paid/failed/refunded |
| metadata | TEXT |  | 扩展字段(JSON 格式) |
| created_at/updated_at | BIGINT |  |

**索引**:
- `idx_subscription_orders_user` (user_id)
- `idx_subscription_orders_plan` (plan_id)
- `idx_subscription_orders_bill` (bill_id)
- `idx_subscription_orders_status` (status)

### 2.4 subscriptions [新建表]
| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | BIGINT | PK |
| user_id | BIGINT | NOT NULL | 关联 users.id（应用层约束） |
| plan_id | BIGINT | NOT NULL | 关联 subscription_plans.id（应用层约束） |
| order_id | BIGINT | 可空 | 关联 subscription_orders.id（应用层约束） |
| status | VARCHAR(32) | 默认 `pending` | pending/active/expired/cancelled |
| start_at | BIGINT | NOT NULL |
| end_at | BIGINT | NOT NULL |
| priority | INT | NOT NULL | 默认为 end_at (Unix) 或按插入顺序,用于排序 |
| auto_wallet_fallback | BOOLEAN | 默认 false | 订阅级别开关 |
| bind_channel_group | VARCHAR(64) | 可空 | 绑定的渠道分组 |
| model_whitelist_cache | TEXT |  | 方便判定(可冗余) |
| redemption_id | BIGINT | 可空 | 兑换码触发时关联（应用层约束） |
| coupon_id | BIGINT | 可空 | 使用的优惠券（应用层约束） |
| redeem_option | VARCHAR(32) | 默认 `stack` | stack/coexist/convert |
| metadata | TEXT |  | JSON 格式，例如折算剩余金额等 |
| created_at/updated_at | BIGINT |  |

**索引**:
- `idx_subscriptions_user_status` (user_id, status)
- `idx_subscriptions_user_priority` (user_id, priority)
- `idx_subscriptions_end_at_status` (end_at, status) -- 用于定时任务检查到期订阅

### 2.5 subscription_usages [新建表]
| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | BIGINT | PK |
| subscription_id | BIGINT | NOT NULL | 关联 subscriptions.id（应用层约束，删除时需级联清理） |
| period | VARCHAR(32) | NOT NULL | five_hours/day/week/month |
| window_start | BIGINT | NOT NULL | UNIX 时间戳 |
| window_end | BIGINT | NOT NULL |
| used_quota | BIGINT | NOT NULL | ⚠️ 修正: 原 INT 改为 BIGINT |
| limit_quota | BIGINT | NOT NULL | ⚠️ 修正: 原 INT 改为 BIGINT |
| created_at/updated_at | BIGINT |  |

**约束**: unique(subscription_id, period, window_start)
**索引**:
- `idx_subscription_usage_window` (subscription_id, period)
- `idx_subscription_usage_subscription` (subscription_id)

### 2.6 coupons [新建表]
**决策**: 根据 requirements.md 6.3 节，优惠券功能需要完全新建。

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | BIGINT | PK | 自增 |
| code | VARCHAR(64) | UNIQUE NOT NULL | 优惠券码 |
| name | VARCHAR(128) | NOT NULL | 优惠券名称 |
| description | TEXT |  | 说明 |
| scope | VARCHAR(32) | 默认 `quota` | quota=额度券, subscription=订阅券 |
| discount_type | VARCHAR(32) | NOT NULL | percentage=折扣, fixed=立减 |
| discount_value | BIGINT | NOT NULL | 折扣值：percentage 时为百分比（85=8.5折），fixed 时为美分 |
| applicable_plan_ids | TEXT |  | JSON 数组，适用套餐 ID 列表 |
| total_count | BIGINT | 默认 1 | 总发行量，-1 表示无限 |
| used_count | BIGINT | 默认 0 | 已使用次数 |
| per_user_limit | BIGINT | 默认 1 | 单用户使用次数限制，-1 表示无限 |
| valid_from | BIGINT | NOT NULL | 生效时间（Unix 秒） |
| valid_to | BIGINT | NOT NULL | 过期时间 |
| status | VARCHAR(32) | 默认 `active` | active=可用, used=已用完, expired=已过期 |
| bind_user_id | BIGINT | 可空 | 绑定到特定用户（第三方售卖场景） |
| bind_redemption_id | BIGINT | 可空 | **已弃用**，绑定关系通过 `coupon_redemption_bindings` 表管理 |
| bind_locked_at | BIGINT | 可空 | **已弃用**，锁定状态通过 `coupon_redemption_bindings.status` 管理 |
| created_by | BIGINT | NOT NULL | 创建者 user_id |
| created_at/updated_at | BIGINT |  |  |

**索引**:
- `idx_coupons_code` (UNIQUE)
- `idx_coupons_status` (status)
- `idx_coupons_bind_user` (bind_user_id)

**约束**:
- CHECK (discount_value > 0)
- CHECK (total_count = -1 OR total_count > 0)
- CHECK (valid_to > valid_from)

**状态机**:
```
active (可用)
  → used (兑换成功) / expired (过期)
```

**注意**: 优惠券与兑换码的绑定关系通过 `coupon_redemption_bindings` 表管理，见 2.8 节。

### 2.7 user_bills [新建表]
**说明**: 统一账单表，记录所有充值、消费、退款流水。与 top_ups 表并存（top_ups 保留用于兼容）。

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | BIGINT | PK | 自增 |
| user_id | BIGINT | NOT NULL | 关联 users.id（应用层约束） |
| bill_type | VARCHAR(64) | NOT NULL | 账单类型（见下方枚举） |
| amount | BIGINT | NOT NULL | 金额（美分），正数=收入，负数=支出 |
| balance_before | BIGINT | NOT NULL | 操作前余额 |
| balance_after | BIGINT | NOT NULL | 操作后余额 |
| source_type | VARCHAR(64) |  | 来源类型：subscription/wallet/coupon/redemption |
| source_id | BIGINT |  | 来源 ID（订单 ID、兑换码 ID 等） |
| payment_channel | VARCHAR(64) |  | 支付渠道：wallet/stripe/manual 等 |
| trade_no | VARCHAR(255) |  | 交易流水号 |
| description | TEXT |  | 描述 |
| metadata | TEXT |  | JSON 扩展字段（如折算详情） |
| operator_id | BIGINT |  | 操作员 ID（人工操作时） |
| created_at | BIGINT | NOT NULL |  |

**bill_type 枚举**:
- `top_up`: 充值（对应 `common.BillTypeRecharge`）
- `subscription`: 订阅购买（对应 `common.BillTypeSubscription`）
- `subscription_renew`: 订阅续费（对应 `common.BillTypeSubscriptionRenew`）
- `refund`: 退款（对应 `common.BillTypeRefund`）
- `consume`: API 消费（对应 `common.BillTypeConsume`）
- `adjustment`: 调整（对应 `common.BillTypeAdjustment`）
- `coupon_discount`: 优惠券抵扣（对应 `common.BillTypeCouponDiscount`）

**索引**:
- `idx_user_bills_user_id` (user_id, created_at DESC)
- `idx_user_bills_source` (source_type, source_id)
- `idx_user_bills_trade_no` (trade_no)

### 2.8 coupon_redemption_bindings [新建表]
**说明**: 优惠券与兑换码的绑定关系表。一个兑换码只能绑定一张优惠券（redemption_id 唯一）。

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | BIGINT | PK | 自增 |
| coupon_id | BIGINT | NOT NULL | 关联 coupons.id |
| redemption_id | BIGINT | UNIQUE NOT NULL | 关联 redemptions.id（一对一） |
| user_id | BIGINT | 可空 | 专属用户 ID（NULL 表示不限用户） |
| status | VARCHAR(32) | 默认 `reserved` | reserved=已预留, locked=已锁定 |
| locked_at | BIGINT | 可空 | 锁定时间（兑换后设置） |
| created_at | BIGINT | NOT NULL |  |
| updated_at | BIGINT | NOT NULL |  |

**索引**:
- `idx_binding_coupon` (coupon_id)
- `uq_binding_redemption` (redemption_id) UNIQUE
- `idx_binding_user` (user_id)

**状态机**:
```
reserved (已预留，可修改/解绑)
  → locked (已锁定，兑换成功后不可变更)
```

**约束**:
- 一个兑换码只能绑定一张优惠券（通过 redemption_id UNIQUE 约束）
- 仅 reserved 状态可解绑或修改
- locked 状态不可变更

### 2.9 audit_logs [新建表]
**说明**: 审计日志表，记录所有关键操作用于合规和追溯。与 logs 表（API 日志）职责不同。

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | BIGINT | PK | 自增 |
| user_id | BIGINT |  | 操作用户 ID |
| operator_id | BIGINT |  | 操作员 ID（管理员操作时） |
| object_type | VARCHAR(64) | NOT NULL | 对象类型（见下方枚举） |
| object_id | BIGINT |  | 对象 ID |
| action | VARCHAR(64) | NOT NULL | 操作类型：create/update/delete/cancel/bind 等 |
| ip_address | VARCHAR(64) |  | 操作 IP |
| user_agent | TEXT |  | User-Agent |
| metadata | TEXT |  | JSON 格式，记录操作详情 |
| created_at | BIGINT | NOT NULL |  |

**object_type 枚举**:
- `subscription`: 订阅
- `subscription_plan`: 套餐
- `subscription_order`: 订单
- `coupon`: 优惠券
- `coupon_binding`: 优惠券绑定
- `redemption`: 兑换码
- `user`: 用户

**索引**:
- `idx_audit_logs_user` (user_id, created_at DESC)
- `idx_audit_logs_object` (object_type, object_id)
- `idx_audit_logs_created_at` (created_at DESC)

---

## 3. 现有表字段扩展

### 3.1 users 表扩展

**现有结构**: 已有 setting 字段（TEXT 类型）

**扩展方案**: 使用 users.setting 字段存储 JSON 格式的用户设置，**不新建 user_settings 表**。

**⚠️ 重要**: 现有 setting 字段可能已存储其他数据，**不能直接覆盖**，需要使用 JSON 合并策略。

**users.setting JSON 结构**:
```json
{
  "auto_wallet_fallback": false,           // 自动余额兜底开关（新增）
  "notification_preferences": {             // 通知偏好（新增）
    "email": true,
    "in_app": true,
    "webhook_url": ""
  },
  "language": "zh-CN",                      // 语言偏好（新增）
  "timezone": "Asia/Shanghai",              // 时区（新增）
  // ... 其他已存在的键保留
}
```

**迁移策略（JSON 合并，不覆盖）**:

**方案 1: PostgreSQL JSON 合并**（推荐）
```sql
-- PostgreSQL 支持 jsonb || 操作符进行合并
UPDATE users
SET setting = COALESCE(
    setting::jsonb,
    '{}'::jsonb
) || '{
    "auto_wallet_fallback": false,
    "notification_preferences": {"email": true, "in_app": true},
    "language": "zh-CN",
    "timezone": "Asia/Shanghai"
}'::jsonb
WHERE setting IS NULL
   OR setting = ''
   OR NOT (setting::jsonb ? 'auto_wallet_fallback');  -- 只初始化缺少新键的记录

-- 验证
SELECT COUNT(*) FROM users
WHERE setting IS NULL
   OR setting = ''
   OR NOT (setting::jsonb ? 'auto_wallet_fallback');
-- 期望结果: 0
```

**方案 2: 应用层合并**（兼容性更好）
```go
// Go 代码示例
func MergeUserSetting(existingSetting string) (string, error) {
    // 默认设置
    defaultSettings := map[string]interface{}{
        "auto_wallet_fallback": false,
        "notification_preferences": map[string]interface{}{
            "email":  true,
            "in_app": true,
        },
        "language": "zh-CN",
        "timezone": "Asia/Shanghai",
    }

    // 解析现有设置
    existing := make(map[string]interface{})
    if existingSetting != "" && existingSetting != "{}" {
        if err := json.Unmarshal([]byte(existingSetting), &existing); err != nil {
            return "", err
        }
    }

    // 合并：现有键保留，只添加缺失的新键
    for key, value := range defaultSettings {
        if _, exists := existing[key]; !exists {
            existing[key] = value
        }
    }

    // 序列化回 JSON
    merged, err := json.Marshal(existing)
    return string(merged), err
}

// 批量迁移
func MigrateUserSettings() error {
    users := []User{}
    db.Find(&users)

    for _, user := range users {
        merged, err := MergeUserSetting(user.Setting)
        if err != nil {
            log.Errorf("Failed to merge setting for user %d: %v", user.ID, err)
            continue
        }

        if merged != user.Setting {
            db.Model(&user).Update("setting", merged)
        }
    }
    return nil
}
```

**方案 3: 保守策略（仅初始化空值）**
```sql
-- 只初始化 NULL 或空字符串，不修改已有设置
UPDATE users
SET setting = '{
    "auto_wallet_fallback": false,
    "notification_preferences": {"email": true, "in_app": true},
    "language": "zh-CN",
    "timezone": "Asia/Shanghai"
}'
WHERE setting IS NULL OR setting = '' OR setting = '{}';

-- 验证
SELECT COUNT(*) FROM users WHERE setting IS NULL OR setting = '';
-- 期望结果: 0
```

**推荐策略**:
- **开发/测试环境**: 使用方案 3（保守策略），简单且安全
- **生产环境**: ��用方案 2（应用层合并），确保数据完整性

**迁移前检查**:
```sql
-- 检查现有 setting 内容
SELECT id, username, setting
FROM users
WHERE setting IS NOT NULL AND setting <> ''
LIMIT 10;

-- 检查是否有非 JSON 格式数据
SELECT COUNT(*) FROM users
WHERE setting IS NOT NULL
  AND setting <> ''
  AND NOT (setting::jsonb IS NOT NULL);  -- PostgreSQL
-- 期望结果: 0（所有非空 setting 都是有效 JSON）
```

**迁移后验证**:
```sql
-- 验证所有用户都有 auto_wallet_fallback 键
SELECT COUNT(*) FROM users
WHERE setting::jsonb ? 'auto_wallet_fallback';
-- 期望结果: 等于总用户数

-- 验证 JSON 格式正确性
SELECT id, username, setting
FROM users
WHERE setting IS NOT NULL
  AND setting <> ''
  AND NOT (setting::jsonb IS NOT NULL)  -- PostgreSQL
LIMIT 10;
-- 期望结果: 0 行
```

### 3.2 tokens 表扩展

**现有结构**: 已有 auto_smart_group 字段（BOOLEAN，默认 false）

**扩展方案**: 保留 auto_smart_group，新增 subscription_preferred

**迁移 SQL**:
```sql
-- PostgreSQL
ALTER TABLE tokens ADD COLUMN IF NOT EXISTS subscription_preferred BOOLEAN DEFAULT false;

-- MySQL
ALTER TABLE tokens ADD COLUMN subscription_preferred BOOLEAN DEFAULT false;

-- SQLite (需要重建表)
-- 参考 SQLite 文档使用临时表方式
```

**字段含义**:
- `subscription_preferred = true`: 遵循系统分组与订阅优先扣费
- `subscription_preferred = false`: 跳过订阅扣费，直接使用余额或令牌分组

**迁移注意事项**:
1. 需要同步更新 Redis 缓存中的 token 数据
2. API 响应新增字段 `subscription_preferred`
3. 前端需新增开关字段并保留自动分组开关

### 3.3 redemptions 表扩展

**现有结构**:
```sql
id, user_id, key, status, name, quota, created_time, redeemed_time, used_user_id, expired_time
```

**扩展方案**: 新增 type 和 payload 字段

**迁移 SQL**:
```sql
ALTER TABLE redemptions ADD COLUMN type VARCHAR(32) DEFAULT 'quota';
ALTER TABLE redemptions ADD COLUMN payload TEXT;
```

**payload JSON 结构**（type=subscription 时）:
```json
{
  "plan_id": 12,                           // 订阅套餐 ID
  "duration_days": 30,                     // 时长（天）
  "bind_coupon_id": 88,                   // 绑定的优惠券 ID（可选）
  "price_paid_cents": 9900,               // 购买价格（美分）
  "redeem_option": "stack"                // 兑换选项：stack/coexist/convert
}
```

**迁移注意事项**:
- 存量兑换码 type 默认设为 'quota'
- quota 字段保留，用于额度类型兑换码

### 3.4 options 表（系统配置）

**现有结构**: key (TEXT, PK), value (TEXT)

**新增配置项**:
```sql
INSERT INTO options (key, value) VALUES
('SUBSCRIPTION_AUTO_WALLET_DEFAULT', 'false'),
('SUBSCRIPTION_EXPIRY_NOTICE_DAYS', '7'),
('SUBSCRIPTION_QUOTA_LOW_THRESHOLD', '0.2'),
('SUBSCRIPTION_V2_ENABLED', 'false'),
('SUBSCRIPTION_MAX_PER_USER', '10');
```

**配置项说明**:
| Key | Value 类型 | 默认值 | 说明 |
|-----|-----------|--------|------|
| SUBSCRIPTION_AUTO_WALLET_DEFAULT | boolean | false | 系统级自动余额兜底默认值 |
| SUBSCRIPTION_EXPIRY_NOTICE_DAYS | integer | 7 | 订阅到期提醒天数 |
| SUBSCRIPTION_QUOTA_LOW_THRESHOLD | float | 0.2 | 额度低阈值（20%） |
| SUBSCRIPTION_V2_ENABLED | boolean | false | 订阅 V2 功能开关（Feature Flag） |
| SUBSCRIPTION_MAX_PER_USER | integer | 10 | 单用户最大订阅数量 |

---

## 4. 数据完整性约束策略

⚠️ **重要决策**: 遵循现有表规范，**不使用 FOREIGN KEY 约束**，采用**索引 + 应用层约束**。

### 4.1 应用层约束说明

| 子表 | 关联字段 | 父表 | 删除策略 | 应用层实现 |
|------|---------|------|---------|-----------|
| subscription_plan_limits | plan_id | subscription_plans(id) | CASCADE | 删除套餐时在 Service 层级联删除限额配置 |
| subscription_orders | user_id | users(id) | RESTRICT | 删除用户前检查是否有订单，有则拒绝删除 |
| subscription_orders | plan_id | subscription_plans(id) | RESTRICT | 删除套餐前检查是否有订单，有则拒绝删除 |
| subscriptions | user_id | users(id) | RESTRICT | 删除用户前检查是否有订阅，有则拒绝删除 |
| subscriptions | plan_id | subscription_plans(id) | RESTRICT | 删除套餐前检查是否有订阅，有则拒绝删除 |
| subscriptions | order_id | subscription_orders(id) | SET NULL | 订单删除时将订阅的 order_id 设为 NULL |
| subscription_usages | subscription_id | subscriptions(id) | CASCADE | 删除订阅时在 Service 层级联删除使用量记录 |
| coupons | bind_redemption_id | redemptions(id) | SET NULL | 兑换码删除时解除绑定（设为 NULL） |
| user_bills | user_id | users(id) | RESTRICT | 删除用户前检查账单，生产环境禁止删除有账单的用户 |
| audit_logs | user_id | users(id) | SET NULL | 用户删除时保留审计日志，user_id 设为 NULL |

### 4.2 索引策略

**所有关联字段都需要创建索引**，以保证查询性能：

```sql
-- 示例：subscriptions 表的关联字段索引
CREATE INDEX idx_subscriptions_user_id ON subscriptions(user_id);
CREATE INDEX idx_subscriptions_plan_id ON subscriptions(plan_id);
CREATE INDEX idx_subscriptions_order_id ON subscriptions(order_id);
```

### 4.3 Service 层约束实现

**删除操作前检查**（RESTRICT 策略）:
```go
// 伪代码示例
func DeleteUser(userID int64) error {
    // 检查是否有订单
    orderCount, _ := db.Model(&SubscriptionOrder{}).Where("user_id = ?", userID).Count()
    if orderCount > 0 {
        return errors.New("无法删除用户：存在订单记录")
    }

    // 检查是否有订阅
    subCount, _ := db.Model(&Subscription{}).Where("user_id = ?", userID).Count()
    if subCount > 0 {
        return errors.New("无法删除用户：存在活跃订阅")
    }

    // 执行删除
    return db.Delete(&User{}, userID).Error
}
```

**级联删除**（CASCADE 策略���:
```go
// 伪代码示例
func DeleteSubscription(subscriptionID int64) error {
    // 先删除关联的使用量记录
    db.Where("subscription_id = ?", subscriptionID).Delete(&SubscriptionUsage{})

    // 再删除订阅
    return db.Delete(&Subscription{}, subscriptionID).Error
}
```

**SET NULL 策略**:
```go
// 伪代码示例
func DeleteOrder(orderID int64) error {
    // 先将关联订阅的 order_id 设为 NULL
    db.Model(&Subscription{}).Where("order_id = ?", orderID).Update("order_id", nil)

    // 再删除订单
    return db.Delete(&SubscriptionOrder{}, orderID).Error
}
```

### 4.4 数据完整性检查任务

**定期巡检脚本**（建议每日执行）:
```sql
-- 检查孤立的 subscription_plan_limits（plan_id 不存在）
SELECT COUNT(*) FROM subscription_plan_limits l
LEFT JOIN subscription_plans p ON l.plan_id = p.id
WHERE p.id IS NULL;

-- 检查孤立的 subscriptions（user_id 不存在）
SELECT COUNT(*) FROM subscriptions s
LEFT JOIN users u ON s.user_id = u.id
WHERE u.id IS NULL;

-- 检查孤立的 subscription_usages（subscription_id 不存在）
SELECT COUNT(*) FROM subscription_usages u
LEFT JOIN subscriptions s ON u.subscription_id = s.id
WHERE s.id IS NULL;
```

### 4.5 注意事项

1. **事务操作**: 所有涉及多表的删除/更新操作必须在事务中执行
2. **错误处理**: Service 层需要清晰的错误提示，说明为何操作被拒绝
3. **性能监控**: 关注删除操作的性能，避免级联删除导致的性能问题
4. **审计日志**: 关键删除操作需要记录到 audit_logs 表

---

## 5. JSON 字段结构定义

### 5.1 subscription_plans.model_whitelist (TEXT/JSON)
```json
["gpt-4", "gpt-3.5-turbo", "claude-3-opus", "claude-3-sonnet"]
```

### 5.2 subscription_plans.channel_groups (TEXT/JSON)
```json
["default", "premium", "enterprise"]
```
空数组 `[]` 表示不限制渠道

### 5.3 subscription_plans.extra (TEXT/JSON)
```json
{
  "cover_image": "https://example.com/plan-cover.jpg",
  "tags": ["popular", "recommended"],
  "features": ["无限模型切换", "优先支持"],
  "sort_order": 100
}
```

### 5.4 subscription_orders.plan_snapshot (TEXT/JSON)
```json
{
  "id": 12,
  "name": "专业版",
  "sku": "PRO-MONTHLY",
  "price_cents": 9900,
  "currency": "USD",
  "billing_cycle": "monthly",
  "billing_cycle_value": 1,
  "limits": [
    {
      "period": "five_hours",
      "quota": 500000,
      "enabled": true
    },
    {
      "period": "day",
      "quota": 1000000,
      "enabled": true
    }
  ],
  "model_whitelist": ["gpt-4", "gpt-3.5-turbo"],
  "channel_groups": ["default"]
}
```

### 5.5 subscription_orders.coupon_snapshot (TEXT/JSON)
```json
{
  "id": 88,
  "code": "SAVE20",
  "name": "20% 折扣券",
  "discount_type": "percentage",
  "discount_value": 80,
  "applicable_plan_ids": [12, 13, 14]
}
```

### 5.6 subscriptions.metadata (TEXT/JSON)
```json
{
  "conversion_from_plan_id": 10,        // 折算来源套餐 ID
  "conversion_remainder_cents": 150,    // 折算剩余金额（不足一天）
  "original_end_at": 1704067200,       // 折算前的原到期时间
  "admin_notes": "客户要求提前升级"      // 管理员备注
}
```

### 5.7 user_bills.metadata (TEXT/JSON)
```json
{
  "conversion_details": {
    "old_plan_id": 10,
    "old_plan_name": "基础版",
    "remaining_days": 15,
    "remaining_value_cents": 4950,
    "new_plan_id": 12,
    "new_plan_name": "专业版",
    "daily_price_new": 330,
    "added_days": 15,
    "remainder_cents": 0
  },
  "coupon_code": "SAVE20",
  "discount_cents": 1980
}
```

### 5.8 audit_logs.metadata (TEXT/JSON)
```json
{
  "action_details": "取消订阅",
  "reason": "用户请求退款",
  "before": {
    "status": "active",
    "end_at": 1735660800
  },
  "after": {
    "status": "cancelled",
    "end_at": 1704067200
  },
  "ip_address": "192.168.1.100",
  "user_agent": "Mozilla/5.0..."
}
```

### 5.9 coupons.applicable_plan_ids (TEXT/JSON)
```json
[12, 13, 14]
```
空数组 `[]` 表示适用于所有套餐

---

## 6. 关键索引与性能建议
1. `subscriptions(user_id, status)` 支撑「查询用户活跃订阅」,再配合 priority 排序。
2. `subscription_usages(subscription_id, period)` 需要频繁读写,使用 `(subscription_id, period, window_start)` unique 便于 UPSERT。
3. `subscription_orders(user_id, status)` 便于列表/对账。
4. `subscription_plan_limits(plan_id)` 常用 join,需索引。
5. `coupons(bind_redemption_id)` 便于查询兑换码绑定的优惠券。
6. `user_bills(user_id, created_at DESC)` 支持用户账单历史查询。
7. `audit_logs(created_at DESC)` 支持管理员审计日志查询。
8. `subscriptions(end_at, status)` 支持定时任务批量检查到期订阅。

---

## 7. 迁移策略（研发阶段）

⚠️ **重要说明**: 当前为研发阶段，无生产流量限制，可随时执行迁移。

### 7.1 表创建顺序

**Phase 0: 验证现有表**
```sql
-- 验证必须存在的表
SELECT COUNT(*) FROM information_schema.tables
WHERE table_name IN ('users', 'tokens', 'redemptions', 'options', 'top_ups');
-- 期望结果: 5
```

**Phase 1: 新建核心业务表**（全部不存在，需要完整建表）
1. `subscription_plans`
2. `subscription_plan_limits`
3. `coupons`
4. `subscription_orders`
5. `subscriptions`
6. `subscription_usages`

**Phase 2: 新建辅助表**（全部不存在，需要完整建表）
7. `user_bills`
8. `audit_logs`

**Phase 3: 扩展现有表**
9. `users.setting` - 初始化 JSON 默认值（字段已存在）
10. `tokens` - 新增 subscription_preferred，保留 auto_smart_group
11. `redemptions` - 新增 type 和 payload 字段
12. `options` - 插入订阅配置项

### 7.2 数据迁移步骤

#### 7.2.1 users.setting 字段初始化

⚠️ **重要**: 使用 JSON 合并策略，避免覆盖已存在的设置。

**步骤 1: 迁移前检查**
```sql
-- 检查现有 setting 内容和数量
SELECT
    COUNT(*) as total_users,
    COUNT(CASE WHEN setting IS NULL OR setting = '' THEN 1 END) as empty_setting,
    COUNT(CASE WHEN setting IS NOT NULL AND setting <> '' THEN 1 END) as has_setting
FROM users;

-- 查看已有设置的样例
SELECT id, username, setting
FROM users
WHERE setting IS NOT NULL AND setting <> ''
LIMIT 10;
```

**步骤 2: 执行迁移（推荐：保守策略）**
```sql
-- 方案 3: 仅初始化空值（开发/测试环境推荐）
-- 只初始化 NULL、空字符串或空 JSON，不修改已有设置
UPDATE users
SET setting = '{
    "auto_wallet_fallback": false,
    "notification_preferences": {"email": true, "in_app": true},
    "language": "zh-CN",
    "timezone": "Asia/Shanghai"
}'
WHERE setting IS NULL OR setting = '' OR setting = '{}';
```

**可选：生产环境 JSON 合并策略**
```sql
-- 方案 1: PostgreSQL JSON 合并（生产环境推荐）
-- 保留现有键，只添加缺失的新键
UPDATE users
SET setting = COALESCE(
    setting::jsonb,
    '{}'::jsonb
) || '{
    "auto_wallet_fallback": false,
    "notification_preferences": {"email": true, "in_app": true},
    "language": "zh-CN",
    "timezone": "Asia/Shanghai"
}'::jsonb
WHERE setting IS NULL
   OR setting = ''
   OR NOT (setting::jsonb ? 'auto_wallet_fallback');
```

**步骤 3: 验证**
```sql
-- 验证无空值
SELECT COUNT(*) FROM users WHERE setting IS NULL OR setting = '';
-- 期望结果: 0

-- 验证所有用户都有新键（如果使用合并策略）
SELECT COUNT(*) FROM users
WHERE setting::jsonb ? 'auto_wallet_fallback';
-- 期望结果: 等于总用户数
```

#### 7.2.2 tokens 表字段扩展
```sql
-- PostgreSQL
ALTER TABLE tokens ADD COLUMN IF NOT EXISTS subscription_preferred BOOLEAN DEFAULT false;
COMMENT ON COLUMN tokens.subscription_preferred IS '是否遵循订阅优先扣费：true=优先订阅，false=跳过订阅直接余额';

-- 验证
SELECT column_name, column_default FROM information_schema.columns
WHERE table_name = 'tokens'
  AND column_name IN ('subscription_preferred', 'auto_smart_group');
-- 期望结果: 两个字段均存在，默认值为 false
```

**迁移后操作**:
- 清理 Redis 中所有 token 缓存
- 更新 API 文档和前端代码（新增订阅优先字段）

#### 7.2.3 redemptions 表字段扩展
```sql
-- 添加新字段
ALTER TABLE redemptions ADD COLUMN type VARCHAR(32) DEFAULT 'quota';
ALTER TABLE redemptions ADD COLUMN payload TEXT;

-- 为存量数据设置默认 type
UPDATE redemptions SET type = 'quota' WHERE type IS NULL;

-- 验证
SELECT COUNT(*) FROM redemptions WHERE type IS NULL;
-- 期望结果: 0
```

#### 7.2.4 options 表系统配置
```sql
INSERT INTO options (key, value) VALUES
('SUBSCRIPTION_AUTO_WALLET_DEFAULT', 'false'),
('SUBSCRIPTION_EXPIRY_NOTICE_DAYS', '7'),
('SUBSCRIPTION_QUOTA_LOW_THRESHOLD', '0.2'),
('SUBSCRIPTION_V2_ENABLED', 'false'),
('SUBSCRIPTION_MAX_PER_USER', '10')
ON CONFLICT (key) DO NOTHING;

-- 验证
SELECT key, value FROM options WHERE key LIKE 'SUBSCRIPTION_%';
-- 期望结果: 5 行
```

### 7.3 回滚策略

**Phase 1 回滚**（如果需要）:
```sql
-- 删除新建的表（按依赖关系逆序）
DROP TABLE IF EXISTS subscription_usages;
DROP TABLE IF EXISTS subscriptions;
DROP TABLE IF EXISTS subscription_orders;
DROP TABLE IF EXISTS subscription_plan_limits;
DROP TABLE IF EXISTS subscription_plans;
DROP TABLE IF EXISTS coupons;
DROP TABLE IF EXISTS user_bills;
DROP TABLE IF EXISTS audit_logs;
```

**Phase 3 回滚**:
```sql
-- tokens 表字段回滚
ALTER TABLE tokens DROP COLUMN IF EXISTS subscription_preferred;

-- redemptions 表字段回滚
ALTER TABLE redemptions DROP COLUMN type;
ALTER TABLE redemptions DROP COLUMN payload;

-- options 表配置回滚
DELETE FROM options WHERE key LIKE 'SUBSCRIPTION_%';

-- users.setting 回滚（可选，一般不需要）
-- 如需回滚，手动修改或重置为空
```

### 7.4 Feature Flag 控制

在 `options` 表中使用 `SUBSCRIPTION_V2_ENABLED` 作为功能开关：

```go
// 伪代码示例
func IsSubscriptionV2Enabled() bool {
    value := GetOption("SUBSCRIPTION_V2_ENABLED")
    return value == "true"
}

// 在路由中根据 Feature Flag 决定使用新旧逻辑
if IsSubscriptionV2Enabled() {
    // 使用新订阅系统
    return handleSubscriptionV2(req)
} else {
    // 使用旧余额系统
    return handleLegacyQuota(req)
}
```

**研发阶段**: 手动控制开关，无需灰度发布
```sql
-- 开启订阅功能
UPDATE options SET value = 'true' WHERE key = 'SUBSCRIPTION_V2_ENABLED';

-- 关闭订阅功能
UPDATE options SET value = 'false' WHERE key = 'SUBSCRIPTION_V2_ENABLED';
```

### 7.5 迁移执行计划（研发阶段简化版）

**执行步骤**:
1. 备份当前数据库
2. 执行 Phase 1 建表 SQL
3. 执行 Phase 2 建表 SQL
4. 执行 Phase 3 扩展 SQL
5. 验证数据完整性
6. 更新代码适配新表结构
7. 通过 Feature Flag 控制开关

**验证清单**:
- [ ] 所有新表创建成功
- [ ] 所有索引创建成功
- [ ] users.setting 初始化完成
- [ ] tokens.subscription_preferred 新增完成
- [ ] redemptions 字段扩展完成
- [ ] options 配置插入完成
- [ ] 数据完整性检查通过
- [ ] 代码更新完成

---

## 8. 数据完整性检查

### 8.1 外键完整性检查
```sql
-- 检查订阅订单关联
SELECT COUNT(*) FROM subscriptions s
LEFT JOIN subscription_orders o ON s.order_id = o.id
WHERE s.order_id IS NOT NULL AND o.id IS NULL;
-- 期望结果: 0

-- 检查套餐限额关联
SELECT COUNT(*) FROM subscription_plan_limits l
LEFT JOIN subscription_plans p ON l.plan_id = p.id
WHERE p.id IS NULL;
-- 期望结果: 0

-- 检查优惠券绑定关联
SELECT COUNT(*) FROM coupons c
LEFT JOIN redemptions r ON c.bind_redemption_id = r.id
WHERE c.bind_redemption_id IS NOT NULL AND r.id IS NULL;
-- 期望结果: 0
```

### 8.2 业务规则完整性检查
```sql
-- 检查使用量不能超过限额
SELECT subscription_id, period, used_quota, limit_quota
FROM subscription_usages
WHERE used_quota > limit_quota;
-- 期望结果: 0 行

-- 检查优惠券状态一致性
SELECT id, code, status, bind_locked_at
FROM coupons
WHERE status = 'used' AND bind_locked_at IS NULL;
-- 期望结果: 0 行（used 状态必须有 bind_locked_at）

-- 检查订阅时间合理性
SELECT id, user_id, start_at, end_at
FROM subscriptions
WHERE start_at >= end_at;
-- 期望结果: 0 行
```

### 8.3 定期监控任务

**每日监控**:
- 订阅使用量异常（used_quota > limit_quota）
- 优惠券绑定状态异常（status 与 bind_locked_at 不一致）
- 账单余额一致性（user_bills.balance_after 与 users.quota 对比）

**每周巡检**:
- 孤立订单（订阅记录无对应订单）
- 过期未清理订阅（end_at < now() 且 status != 'expired'）
- 审计日志缺失（关键操作无对应日志）

---

## 9. 后续扩展预留

### 9.1 多币种支持
- `subscription_plans.currency` 字段预留，目前默认 USD
- 可扩展为 CNY, EUR, JPY 等
- 需要增加汇率转换表 `exchange_rates`

### 9.2 计量单位扩展
- `subscription_plan_limits.unit` 当前固定为 `quota`
- 可扩展为:
  - `tokens`: 按 token 计量（GPT-4 等）
  - `requests`: 按请求次数计量
  - `usd`: 按美元计量（特殊场景）

### 9.3 支付渠道扩展
- `subscription_orders.payment_channel` 当前支持 `wallet`, `third_party`, `redeem`
- 可扩展为:
  - `third_party:stripe`: Stripe 支付
  - `third_party:paypal`: PayPal 支付
  - `third_party:alipay`: 支付宝
  - `third_party:wechat`: 微信支付

### 9.4 订阅高级功能预留
- **家庭套餐**: subscriptions.metadata 中增加 `family_members` 数组
- **企业套餐**: 增加 `organization_id` 字段，关联组织表
- **试用期**: subscription_plans 增加 `trial_days` 字段
- **自动续费**: subscriptions 增加 `auto_renew` 和 `payment_method_id` 字段

### 9.5 分析报表预留
- 订阅续费率分析: 基于 subscription_orders 的 order_type 字段（new/renewal/upgrade）
- 用户生命周期价值: 基于 user_bills 汇总
- 优惠券转化率: 基于 coupons 和 subscription_orders 关联分析

---

**文档版本**: 5.0
**最后修订**: 2025-12-04
**对比基准**: data/DB_DDL/DDL.sql
**差距分析**: db-schema-gap-analysis.md
**审阅状态**: ✅ 所有问题已修正，可开始编写迁移 SQL

**关键修正**:
1. ✅ 移除"已通过 JDBC 分析"误导性描述
2. ✅ 明确标注8个表为"完全新建"
3. ✅ 统一所有金额字段为 BIGINT（修正原 INT）
4. ✅ 明确 users.setting 使用 JSON（不新建 user_settings 表）
5. ✅ 明确 user_bills 与 top_ups 并存策略
6. ✅ **决策完成：不使用 FK，仅索引 + 应用层约束**
7. ✅ **���策完成：研发阶段迁移，无时间限制**
8. ✅ **决策完成：无灰度要求，Feature Flag 手动控制**
9. ✅ **问题修正：移除所有 FK 描述，统一为"应用层约束"**
10. ✅ **问题修正：users.setting 使用 JSON 合并策略，避免数据覆盖**

**下一步**: 编写迁移 SQL 脚本（Stage 1: 数据模型 & 迁移）
