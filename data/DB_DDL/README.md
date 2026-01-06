# 订阅系统数据库迁移指南

**版本**: v1.2
**日期**: 2025-12-04
**数据库**: PostgreSQL / MySQL

---

## 📋 快速开始

### PostgreSQL 用户

```bash
# 1. 备份数据库
pg_dump -U postgres -d new-api > backup_$(date +%Y%m%d_%H%M%S).sql

# 2. 执行完整迁移（一键执行所有 Phase 0-4）
psql -U postgres -d new-api -f subscription_migration_postgresql.sql

# 3. 清理 Redis 缓存（tokens 字段扩展）
redis-cli FLUSHDB

# 4. 重启服务
systemctl restart new-api
```

### MySQL 用户

```bash
# 1. 备份数据库
mysqldump -u root -p new-api > backup_$(date +%Y%m%d_%H%M%S).sql

# 2. 执行完整迁移（一键执行所有 Phase 1-4）
mysql -u root -p new-api < subscription_migration_mysql.sql

# 3. 清理 Redis 缓存（tokens 字段扩展）
redis-cli FLUSHDB

# 4. 重启服务
systemctl restart new-api
```

### SQLite 用户

⚠️ **SQLite 支持暂未实现，当前迁移脚本不包含 SQLite 版本，切勿在 SQLite 环境直接执行。**

SQLite 迁移脚本尚未提供。如需支持 SQLite，请参考 PostgreSQL 或 MySQL 脚本手动改写，主要差异：
- SQLite 使用 `INTEGER PRIMARY KEY AUTOINCREMENT` 实现自增
- SQLite 不支持 `ALTER TABLE ... RENAME COLUMN`，需使用 `ALTER TABLE ... RENAME COLUMN ... TO ...` 或重建表
- SQLite JSON 函数支持有限，users.setting 迁移需调整

---

## 📦 迁移内容

### PostgreSQL 版本

**Phase 0: 创建序列**
- 8 个 SEQUENCE 对象（subscription_plans_id_seq, subscription_plan_limits_id_seq, coupons_id_seq, subscription_orders_id_seq, subscriptions_id_seq, subscription_usages_id_seq, user_bills_id_seq, audit_logs_id_seq）

**Phase 1-2: 新建表（8张）**
| 表名 | 说明 | 自增ID | 索引数 |
|------|------|--------|--------|
| subscription_plans | 订阅套餐表 | `nextval('seq')` | 2 |
| subscription_plan_limits | 套餐限额配置 | `nextval('seq')` | 1 + UNIQUE |
| coupons | 优惠券表 | `nextval('seq')` | 5 |
| subscription_orders | 订阅订单表 | `nextval('seq')` | 5 |
| subscriptions | 用户订阅表 | `nextval('seq')` | 4 |
| subscription_usages | 使用量表（滚动窗口） | `nextval('seq')` | 2 + UNIQUE |
| user_bills | 账单流水表 | `nextval('seq')` | 3 |
| audit_logs | 审计日志表 | `nextval('seq')` | 3 |

**Phase 3: 扩展现有表**
- `tokens`: 新增 `subscription_preferred`，保留 `auto_smart_group`（默认值 FALSE）
- `redemptions`: 新增 `type` 和 `payload` 字段
- `options`: 插入 5 个订阅系统配置项（`SUBSCRIPTION_*`）

**Phase 4: 数据迁移**
- `users.setting`: 使用 JSON 合并策略（`||` 操作符）初始化订阅默认值，不覆盖现有键

### MySQL 版本

**Phase 1-2: 新建表（8张）**
| 表名 | 说明 | 自增ID | 索引数 |
|------|------|--------|--------|
| subscription_plans | 订阅套餐表 | `AUTO_INCREMENT` | 2 |
| subscription_plan_limits | 套餐限额配置 | `AUTO_INCREMENT` | 1 + UNIQUE |
| coupons | 优惠券表 | `AUTO_INCREMENT` | 5 |
| subscription_orders | 订阅订单表 | `AUTO_INCREMENT` | 5 |
| subscriptions | 用户订阅表 | `AUTO_INCREMENT` | 4 |
| subscription_usages | 使用量表（滚动窗口） | `AUTO_INCREMENT` | 2 + UNIQUE |
| user_bills | 账单流水表 | `AUTO_INCREMENT` | 3 |
| audit_logs | 审计日志表 | `AUTO_INCREMENT` | 3 |

**Phase 3: 扩展现有表**
- `tokens`: 新增 `subscription_preferred`，保留 `auto_smart_group`（使用动态SQL，默认值 FALSE）
- `redemptions`: 新增 `type` 和 `payload` 字段（使用动态SQL检查）
- `options`: 插入 5 个订阅系统配置项（使用 `INSERT ... SELECT ... WHERE NOT EXISTS`）

**Phase 4: 数据迁移**
- `users.setting`: 使用 `JSON_MERGE_PATCH` 实现 JSON 合并，不覆盖现有键

---

## ✅ 验证检查

### 1. 验证新表创建

**PostgreSQL**:
```sql
SELECT table_name FROM information_schema.tables
WHERE table_schema = 'public'
  AND table_name IN (
    'subscription_plans', 'subscription_plan_limits', 'coupons',
    'subscription_orders', 'subscriptions', 'subscription_usages',
    'user_bills', 'audit_logs'
);
-- 期望结果: 8 行
```

**MySQL**:
```sql
SELECT table_name FROM information_schema.tables
WHERE table_schema = DATABASE()
  AND table_name IN (
    'subscription_plans', 'subscription_plan_limits', 'coupons',
    'subscription_orders', 'subscriptions', 'subscription_usages',
    'user_bills', 'audit_logs'
);
-- 期望结果: 8 行
```

### 2. 验证字段扩展

**PostgreSQL**:
```sql
-- tokens 字段扩展
SELECT column_name FROM information_schema.columns
WHERE table_name = 'tokens'
  AND column_name IN ('subscription_preferred', 'auto_smart_group');
-- 期望结果: 2 行

-- redemptions 新字段
SELECT column_name FROM information_schema.columns
WHERE table_name = 'redemptions' AND column_name IN ('type', 'payload');
-- 期望结果: 2 行
```

**MySQL**:
```sql
-- tokens 字段扩展
SELECT COLUMN_NAME FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'tokens'
  AND COLUMN_NAME IN ('subscription_preferred', 'auto_smart_group');
-- 期望结果: 2 行

-- redemptions 新字段
SELECT COLUMN_NAME FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'redemptions'
  AND COLUMN_NAME IN ('type', 'payload');
-- 期望结果: 2 行
```

### 3. 验证数据迁移

**PostgreSQL**:
```sql
-- users.setting 无空值
SELECT COUNT(*) FROM users WHERE setting IS NULL OR setting = '';
-- 期望结果: 0

-- users.setting 包含新键
SELECT COUNT(*) FROM users WHERE setting::jsonb ? 'auto_wallet_fallback';
-- 期望结果: 等于总用户数
```

**MySQL**:
```sql
-- users.setting 无空值
SELECT COUNT(*) FROM users WHERE setting IS NULL OR setting = '';
-- 期望结果: 0

-- users.setting 包含新键
SELECT COUNT(*) FROM users
WHERE JSON_CONTAINS_PATH(setting, 'one', '$.auto_wallet_fallback');
-- 期望结果: 等于总用户数
```

---

## 🔑 关键设计决策

### 1. 优惠券绑定方案
**决策**: 扩展 coupons 表，添加 `bind_redemption_id`、`bind_user_id`、`bind_locked_at` 字段
**原因**: 简单、性能好、符合业务约束（一张优惠券只能绑定一个对象）

### 2. 应用层约束（无 FOREIGN KEY）
**决策**: 不使用数据库外键，采用应用层约束
**原因**: 与现有表保持一致、跨数据库兼容性好、更灵活

**约束类型**:
- **CASCADE**: 删除父记录时 Service 层级联删除子记录
- **RESTRICT**: 删除前检查子记录，存在则拒绝
- **SET NULL**: 删除父记录时子记录外键设为 NULL

**完整性检查**: 定期运行巡检脚本检测孤立记录

### 3. 自增ID策略
- **PostgreSQL**: `SEQUENCE + DEFAULT nextval('seq'::regclass)`
- **MySQL**: `AUTO_INCREMENT`
- **一致性**: 与现有表（DDL.sql）保持一致

### 4. JSON 合并策略
- **PostgreSQL**: 使用 `jsonb || operator` 实现非覆盖合并
- **MySQL**: 使用 `JSON_MERGE_PATCH` 实现非覆盖合并
- **目标**: 保留现有键，只添加缺失的新键

---

## 📊 索引优化

**补充的关键索引**:
- `subscriptions.plan_id` - 套餐聚合查询
- `subscription_orders.coupon_id` - 优惠券聚合查询
- `coupons.valid_to` - 到期扫描
- `user_bills(user_id, created_at DESC)` - 账单历史查询（复合索引）
- `audit_logs(user_id, created_at DESC)` - 审计日志查询（复合索引）

**性能预估**:
- 用户订阅查询: < 10ms
- 订单列表查询: < 20ms
- 账单流水查询: < 30ms

---

## 🔄 回滚方案

如果迁移失败：

```bash
# 1. 停止服务
systemctl stop new-api

# 2. 恢复数据库
# PostgreSQL
psql -U postgres -d new-api < backup_YYYYMMDD_HHMMSS.sql

# 或 MySQL
mysql -u root -p new-api < backup_YYYYMMDD_HHMMSS.sql

# 3. 重启服务
systemctl start new-api
```

**手动回滚 Phase 3-4**（如果只有 Phase 3-4 失败）:

PostgreSQL:
```sql
-- 回滚 tokens 字段扩展（仅移除 subscription_preferred）
ALTER TABLE tokens DROP COLUMN IF EXISTS subscription_preferred;

-- 回滚 redemptions 字段
ALTER TABLE redemptions DROP COLUMN IF EXISTS type;
ALTER TABLE redemptions DROP COLUMN IF EXISTS payload;

-- 回滚 options 配置
DELETE FROM options WHERE key LIKE 'SUBSCRIPTION_%';

-- 回滚 users.setting（移除新增键）
UPDATE users
SET setting = (
    SELECT jsonb_object_agg(key, value)
    FROM jsonb_each(setting::jsonb)
    WHERE key NOT IN ('auto_wallet_fallback', 'notification_preferences', 'language', 'timezone')
)::text
WHERE setting::jsonb ?| ARRAY['auto_wallet_fallback', 'notification_preferences', 'language', 'timezone'];
```

MySQL:
```sql
-- 回滚 tokens 字段扩展（仅移除 subscription_preferred）
ALTER TABLE tokens DROP COLUMN subscription_preferred;

-- 回滚 redemptions 字段
ALTER TABLE redemptions DROP COLUMN IF EXISTS type;
ALTER TABLE redemptions DROP COLUMN IF EXISTS payload;

-- 回滚 options 配置
DELETE FROM options WHERE `key` LIKE 'SUBSCRIPTION_%';

-- 回滚 users.setting 较复杂，建议从备份恢复
```

---

## ⚠️ 注意事项

### 研发阶段
- ✅ 在测试环境完整演练
- ✅ 记录迁移耗时（参考：100万用户约 30-60秒）
- ✅ 验证应用层约束代码

### 生产环境
- 🔴 选择低流量时段执行
- 🔴 迁移前通知用户
- 🔴 准备回滚方案（至少保留最近3天备份）
- 🔴 迁移后冒烟测试

### tokens 字段扩展影响
- API 新增字段 `subscription_preferred`，`auto_smart_group` 保留
- 若此前已执行字段重命名，迁移会将 `auto_smart_group` 回填为 `subscription_preferred` 的现有值
- Redis 缓存需清理以同步新字段（建议执行 `redis-cli FLUSHDB`）
- 前端代码需同步更新（新增订阅优先开关）

### users.setting 迁移注意
- 现有键会被保留，不会覆盖
- 只有缺少新键的用户会被更新
- 如果用户已有 `auto_wallet_fallback` 键，不会被修改

---

## 📁 文件说明

| 文件名 | 说明 | 大小 | 包含阶段 |
|--------|------|------|---------|
| `DDL.sql` | 现有表结构（参考） | 14 KB | - |
| `README.md` | 本文档 | - | - |
| `subscription_migration_postgresql.sql` | PostgreSQL 完整迁移脚本 | ~20 KB | Phase 0-4 |
| `subscription_migration_mysql.sql` | MySQL 完整迁移脚本 | ~15 KB | Phase 1-4 |

**说明**:
- PostgreSQL 脚本包含 Phase 0（创建序列）到 Phase 4（数据迁移）的完整内容
- MySQL 脚本包含 Phase 1（新建表）到 Phase 4（数据迁移）的完整内容
- **一次性执行**: 每个脚本都是完整的、可独立执行的，无需额外辅助文件
- **幂等性**: 脚本使用 `IF NOT EXISTS` / `IF EXISTS` 检查，支持重复执行

---

## 🔗 相关文档

- **数据库设计**: `.spec-workflow/specs/subscribe/db-schema.md`
- **需求文档**: `.spec-workflow/specs/subscribe/requirements.md`
- **任务追踪**: `.spec-workflow/specs/subscribe/tasks.md`

---

## 📞 支持

如有问题，请在项目 GitHub Issues 中反馈。

**迁移完成标志**: 所有验证检查通过 + 服务正常运行 ✅
