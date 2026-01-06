# 数据库方案与现有 DDL 差距分析

**分析时间**: 2025-12-04
**对比基准**: data/DB_DDL/DDL.sql
**方案文档**: .spec-workflow/specs/subscribe/db-schema.md

---

## 1. 差距总览

### 1.1 表级别差距

| 表名 | 当前状态 | 方案要求 | 差距 | 严重度 |
|------|---------|---------|------|--------|
| subscription_plans | ❌ 不存在 | ✅ 需要新建 | 完全新建 | **高** |
| subscription_plan_limits | ❌ 不存在 | ✅ 需要新建 | 完全新建 | **高** |
| subscription_orders | ❌ 不存在 | ✅ 需要新建 | 完全新建 | **高** |
| subscriptions | ❌ 不存在 | ✅ 需要新建 | 完全新建 | **高** |
| subscription_usages | ❌ 不存在 | ✅ 需要新建 | 完全新建 | **高** |
| coupons | ❌ 不存在 | ✅ 需要新建 | 完全新建 | **高** |
| user_bills | ❌ 不存在 | ✅ 需要新建 | 完全新建 | **高** |
| audit_logs | ❌ 不存在 | ✅ 需要新建 | 完全新建 | **高** |
| users | ✅ 存在 | ⚠️ 扩展 setting 字段 | 字段扩展 | 中 |
| tokens | ✅ 存在 | ⚠️ 新增 subscription_preferred | 字段新增 | **高** |
| redemptions | ✅ 存在 | ⚠️ 新增 type/payload | 字段新增 | **高** |
| options | ✅ 存在 | ⚠️ 插入配置项 | 数据插入 | **高** |

### 1.2 现有表实际结构

**users 表**:
```sql
id bigint, username text, password text, display_name text, role bigint,
status bigint, email text, github_id text, discord_id text, oidc_id text,
wechat_id text, telegram_id text, access_token char(32), quota bigint,
used_quota bigint, request_count bigint, "group" varchar(64), aff_code varchar(32),
aff_count bigint, aff_quota bigint, aff_history bigint, inviter_id bigint,
deleted_at timestamp, linux_do_id text, setting text, remark varchar(255),
stripe_customer varchar(64), google_id text
```
✅ 已有 setting text 字段

**tokens 表**:
```sql
id bigint, user_id bigint, key char(48), status bigint, name text,
created_time bigint, accessed_time bigint, expired_time bigint,
remain_quota bigint, unlimited_quota boolean, model_limits_enabled boolean,
model_limits varchar(1024), allow_ips text, used_quota bigint, "group" text,
deleted_at timestamp, group_priorities varchar(2048),
auto_smart_group boolean default false  -- ⚠️ 需新增 subscription_preferred
```

**redemptions 表**:
```sql
id bigint, user_id bigint, key char(32), status bigint, name text,
quota bigint default 100, created_time bigint, redeemed_time bigint,
used_user_id bigint, deleted_at timestamp, expired_time bigint
-- ⚠️ 缺少 type varchar(32), payload text
```

**options 表**:
```sql
key text PRIMARY KEY, value text
-- ⚠️ 无任何订阅相关配置
```

**top_ups 表**:
```sql
id bigint, user_id bigint, amount bigint, money numeric,
trade_no varchar(255), payment_method varchar(50),
create_time bigint, complete_time bigint, status text
-- ⚠️ 只记录充值，无消费记录
```

---

## 2. 高优先级问题

### 2.1 误导性描述问题 ⚠️

**问题**: db-schema.md Section 0 声称"通过 JDBC 连接分析现有表结构"，但实际上：
- 所有 subscription_* 表不存在
- coupons、user_bills、audit_logs 不存在
- 这些表是**计划新增**，不是"已存在"

**影响**: 严重低估迁移复杂度，误导开发人员

**修复**:
1. 移除"已分析现有结构"措辞
2. 明确标注"计划新增表"
3. 在迁移策略中补充完整建表 SQL

### 2.2 tokens 字段扩展问题 ⚠️

**现状**:
```sql
auto_smart_group boolean default false
```

**方案要求**:
```sql
subscription_preferred boolean default false
```

**差距**:
1. 缺少 `subscription_preferred` 字段
2. 需要保持 `auto_smart_group` 语义不变

**迁移复杂度**:
- 需要 ALTER TABLE ADD COLUMN
- 需要设置默认值
- 需要清理 Redis 缓存（token 缓存可能缺少新字段）
- 需要更新 API 响应字段
- 需要更新前端代码

### 2.3 redemptions 字段缺失问题 ⚠️

**缺失字段**:
- `type VARCHAR(32) DEFAULT 'quota'`
- `payload TEXT`

**影响**:
- 无法支持订阅类型兑换码
- 无法存储优惠券绑定信息
- requirements.md 中的"兑换码绑定优惠券"功能无法实现

**迁移 SQL**:
```sql
ALTER TABLE redemptions ADD COLUMN type VARCHAR(32) DEFAULT 'quota';
ALTER TABLE redemptions ADD COLUMN payload TEXT;
UPDATE redemptions SET type = 'quota' WHERE type IS NULL;
```

### 2.4 options 配置缺失问题 ⚠️

**当前状态**: options 表为空（或只有其他配置）

**需要插入的配置**:
```sql
INSERT INTO options (key, value) VALUES
('SUBSCRIPTION_AUTO_WALLET_DEFAULT', 'false'),
('SUBSCRIPTION_EXPIRY_NOTICE_DAYS', '7'),
('SUBSCRIPTION_QUOTA_LOW_THRESHOLD', '0.2'),
('SUBSCRIPTION_V2_ENABLED', 'false'),
('SUBSCRIPTION_MAX_PER_USER', '10')
ON CONFLICT (key) DO NOTHING;
```

### 2.5 用户设置方案不统一问题 ⚠️

**矛盾点**:
- **数据库**: users.setting TEXT (JSON 格式)
- **需求文档**: 提到 UserSetting 模型
- **任务列表**: 可能假定独立 user_settings 表

**决策**:
- ✅ **采用 users.setting TEXT (JSON)** - 与现有结构一致
- ❌ 不新建 user_settings 表

**理由**:
1. 避免表结构变更（users 表已有 setting 字段）
2. 减少 JOIN 操作开销
3. PostgreSQL 支持 JSON 查询和索引
4. 灵活性更高（易扩展）

**代价**:
- 需要在 Model 层处理 JSON 序列化/反序列化
- 需要定义清晰的 JSON Schema
- 需要迁移脚本初始化默认值

---

## 3. 中优先级问题

### 3.1 金额字段类型不一致 ⚠️

**现状**:
- users.quota: **bigint**
- logs.quota: **bigint**
- top_ups.amount: **bigint**
- top_ups.money: **numeric**

**方案文档**:
- subscription_plans.price_cents: **INT** (不一致！)
- subscription_orders.price_cents: **INT** (不一致！)
- user_bills.amount: **BIGINT** (一致)

**决策**: 统一使用 **BIGINT**
- 原因：与现有 quota/amount 字段一致
- 单位：美分或内部 quota 单位
- 范围：bigint 足够（-9,223,372,036,854,775,808 到 9,223,372,036,854,775,807）

**修正**:
```sql
-- 将所有 INT 改为 BIGINT
price_cents BIGINT NOT NULL,
discount_cents BIGINT DEFAULT 0,
final_price_cents BIGINT NOT NULL
```

### 3.2 外键约束策略冲突 ⚠️

**现状**: DDL.sql 中**没有任何 FOREIGN KEY 约束**

**方案**: db-schema.md 规划了大量 FK 约束（CASCADE, RESTRICT, SET NULL）

**问题**:
1. 引入 FK 会改变现有运维模式
2. FK 约束可能影响性能（级联删除、级联更新）
3. 现有业务逻辑可能依赖应用层约束

**决策**:
- **建议方案 A（保守）**: 订阅相关表使用 FK 约束，现有表不添加
  - 优点：保持现有表的灵活性
  - 缺点：约束策略不一致
- **建议方案 B（激进）**: 所有新表使用 FK 约束，并评估对现有表的影响
  - 优点：数据完整性更好
  - 缺点：需要评估性能影响和回滚成本

**需要用户决策**: 是否引入 FK 约束？

### 3.3 user_bills 与 top_ups 关系不明确 ⚠️

**问题**:
- top_ups 表只记录充值
- user_bills 表记录所有账单（充值、消费、退款）
- 两者数据有重叠（充值记录）

**决策选项**:
1. **方案 A**: user_bills 替代 top_ups（废弃 top_ups）
   - 优点：数据统一
   - 缺点：需要迁移历史数据
2. **方案 B**: user_bills 和 top_ups 并存
   - 优点：保持兼容性
   - 缺点：数据冗余
3. **方案 C**: top_ups 只记录充值，user_bills 记录消费
   - 优点：职责分离
   - 缺点：查询账单历史需要 JOIN

**推荐**: **方案 B（并存）**
- 保持 top_ups 不变（兼容现有代码）
- user_bills 记录所有类型账单
- 充值记录同时写入两表（过渡期）
- 未来逐步迁移查询到 user_bills

### 3.4 quota_data 与 subscription_usages 关系不明确 ⚠️

**问题**:
- quota_data 表：现有用量统计（按 user_id + model_name 汇总）
- subscription_usages 表：订阅滚动窗口使用量（按 subscription_id + period 记录）

**差异**:
| 维度 | quota_data | subscription_usages |
|------|-----------|---------------------|
| 粒度 | 用户 + 模型 | 订阅 + 时间窗口 |
| 时间范围 | 累计 | 滚动窗口 |
| 用途 | 统计分析 | 限额控制 |

**决策**: 两者**共存，职责不同**
- quota_data: 继续用于统计分析、报表
- subscription_usages: 用于实时限额检查

### 3.5 options 键命名规范缺失 ⚠️

**问题**: options 表是简单 KV 结构，无命名空间

**风险**:
- 键冲突（如 `ENABLED` vs `SUBSCRIPTION_V2_ENABLED`）
- 难以批量查询（如查询所有订阅配置）
- 难以版本管理

**建议规范**:
- 使用前缀分组：`SUBSCRIPTION_*`, `PAYMENT_*`, `NOTIFICATION_*`
- 全大写 + 下划线分隔
- 文档化所有配置键

**迁移 SQL 增强**:
```sql
-- 添加注释说明键的用途
COMMENT ON TABLE options IS '系统配置键值对表。键命名规范：大写+下划线，使用前缀分组（如 SUBSCRIPTION_*）';
```

---

## 4. 迁移策略修正

### 4.1 建表顺序（修正后）

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

### 4.2 外键约束策略（待决策）

**选项 A（推荐）**: 新表使用 FK，现有表不改动
```sql
-- 仅在新建表中定义 FK
ALTER TABLE subscription_plan_limits ADD CONSTRAINT fk_plan
  FOREIGN KEY (plan_id) REFERENCES subscription_plans(id) ON DELETE CASCADE;
-- ... 其他新表 FK
```

**选项 B**: 不使用 FK，仅应用层约束 + 索引
```sql
-- 不定义 FK，只创建索引
CREATE INDEX idx_subscription_plan_limits_plan_id ON subscription_plan_limits(plan_id);
```

### 4.3 金额字段类型（修正）

**修正**: 所有金额字段统一使用 **BIGINT**

```sql
-- subscription_plans
price_cents BIGINT NOT NULL,  -- 原 INT 修改为 BIGINT

-- subscription_orders
price_cents BIGINT NOT NULL,
discount_cents BIGINT DEFAULT 0,
final_price_cents BIGINT NOT NULL,

-- user_bills
amount BIGINT NOT NULL,
balance_before BIGINT NOT NULL,
balance_after BIGINT NOT NULL,

-- coupons
discount_value BIGINT NOT NULL
```

### 4.4 users.setting 初始化（补充 - ⚠️ 已修正）

⚠️ **问题**: 现有的 setting 字段可能已经存储了其他数据，直接覆盖为默认 JSON 会丢失数据。

**解决方案**: 使用 JSON 合并策略，详见 db-schema.md Section 3.1

**方案 1: PostgreSQL JSON 合并**（生产环境推荐）
```sql
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

**方案 2: 应用层合并**（详见 db-schema.md Section 3.1）

**方案 3: 保守策略**（开发/测试环境推荐）
```sql
-- 只初始化空值，不修改已有设置
UPDATE users
SET setting = '{
    "auto_wallet_fallback": false,
    "notification_preferences": {"email": true, "in_app": true},
    "language": "zh-CN",
    "timezone": "Asia/Shanghai"
}'
WHERE setting IS NULL OR setting = '' OR setting = '{}';
```

**推荐策略**:
- **开发/测试环境**: 方案 3（简单安全）
- **生产环境**: 方案 1 或方案 2（确保数据完整性）

**JSON Schema 定义**（Go 代码）:
```go
type UserSetting struct {
    AutoWalletFallback      bool                 `json:"auto_wallet_fallback"`
    NotificationPreferences NotificationPrefs    `json:"notification_preferences"`
    Language                string               `json:"language"`
    Timezone                string               `json:"timezone"`
}

type NotificationPrefs struct {
    Email      bool   `json:"email"`
    InApp      bool   `json:"in_app"`
    WebhookURL string `json:"webhook_url,omitempty"`
}
```

### 4.5 FK 描述与约束策略不一致（⚠️ 已修正）

**问题**: 文档声明"不使用 FK 约束"，但表定义中仍标注 "FK -> users.id"。

**影响**:
- 迁移脚本可能误加 FOREIGN KEY
- 与现有表规范（无 FK）不一致
- 运维人员产生困惑

**解决方案**: 统一表述为"关联 xxx（应用层约束）"

**已修正位置**:
- user_bills.user_id: "FK -> users.id" → "关联 users.id（应用层约束）"
- 所有其他表的关联字段已统一为"关联 xxx（应用层约束）"

**验证**:
```bash
grep "FK ->" db-schema.md
# 期望结果: 无匹配（已全部修正）
```

---

## 5. 风险评估与缓解

### 5.1 高风险项

| 风险 | 严重度 | 概率 | 缓解措施 | 状态 |
|------|--------|------|---------|------|
| tokens 字段扩展导致服务不兼容 | 高 | 中 | 1. 提前更新代码与缓存兼容新字段<br>2. 准备回滚脚本 | Open |
| redemptions 扩展导致现有兑换码失效 | 高 | 低 | 1. 默认 type='quota'<br>2. 存量数据验证 | Open |
| **users.setting 覆盖导致数据丢失** | **高** | **高** | 1. 使用 JSON 合并策略<br>2. 迁移前检查<br>3. 保守策略（仅初始化空值） | **已修正** |
| **FK 约束与文档不一致导致迁移错误** | **中** | **中** | 1. 统一表述为"应用层约束"<br>2. 移除所有 FK 描述 | **已修正** |
| user_bills 与 top_ups 数据不一致 | 中 | 高 | 1. 双写策略<br>2. 定期对账 | Open |

### 5.2 缓解措施

**tokens 字段扩展**:
```go
// 兼容代码（过渡期）
type Token struct {
    // ... 其他字段
    AutoSmartGroup        bool `gorm:"column:auto_smart_group" json:"auto_smart_group,omitempty"`
    SubscriptionPreferred bool `gorm:"column:subscription_preferred" json:"subscription_preferred"`
}
```

**迁移验证清单**:
- [ ] DDL 脚本在测试环境执行成功
- [ ] 存量数据迁移完成且验证通过
- [ ] 新代码兼容旧字段名（过渡期）
- [ ] 缓存清理脚本准备完毕
- [ ] 回滚脚本测试通过
- [ ] 监控告警配置完成

---

## 6. 行动计划

### 6.1 立即行动

1. **更新 db-schema.md**
   - 移除"已通过 JDBC 分析"误导性描述
   - 明确标注"计划新增表"
   - 修正金额字段类型（INT → BIGINT）
   - 统一用户设置方案（users.setting JSON）

2. **补充完整建表 SQL**
   - 编写 8 个新表的完整 CREATE TABLE 语句
   - 编写现有表扩展 ALTER TABLE 语句
   - 编写数据初始化 INSERT 语句

3. **编写迁移验证脚本**
   - 验证表结构
   - 验证数据完整性
   - 验证索引创建成功

### 6.2 需要决策的问题

1. **外键约束策略**: 使用 FK 还是仅应用层约束？
   - ✅ **决策**: **不使用 FOREIGN KEY 约束，遵循现有表规范**
   - **理由**: 现有表（DDL.sql）无任何 FK 约束，保持一致性
   - **实施**: 仅创建索引 + 应用层约束
   - **影响**: 需要在 Service 层严格控制数据完整性

2. **user_bills 与 top_ups**: 替代、并存还是分离？
   - ✅ **决策**: **并存**
   - **理由**: 保持向后兼容，避免修改现有代码
   - **实施**:
     - top_ups 保留不变
     - user_bills 记录所有类型账单
     - 充值记录同时写入两表（过渡期）

3. **迁移时间窗口**: 建议凌晨 2-4 点，需要多少时间？
   - ✅ **决策**: **无时间要求（研发阶段）**
   - **理由**: 目前是开发阶段，无生产流量
   - **实施**: 随时可执行迁移，无需等待低峰期

4. **Feature Flag 开关**: 灰度比例和时间表？
   - ✅ **决策**: **无灰度要求**
   - **理由**: 研发阶段，可直接全量或通过 Flag 控制
   - **实施**: options 表保留 SUBSCRIPTION_V2_ENABLED 开关，开发时手动控制

---

**文档版本**: 2.0
**创建时间**: 2025-12-04
**审阅状态**: ✅ 所有决策已完成

**决策总结**:
1. ✅ 不使用 FOREIGN KEY 约束，遵循现有表规范
2. ✅ user_bills 与 top_ups 并存
3. ✅ 研发阶段迁移，无时间限制
4. ✅ 无灰度要求，Feature Flag 手动控制

**下一步**: 编写迁移 SQL 脚本，开始 Stage 1 任务（数据模型 & 迁移）
