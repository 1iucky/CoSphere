# 优惠券与兑换码改造数据库迁移指南

本目录包含优惠券与兑换码改造的增量迁移脚本。

## 📋 文件清单

### 迁移脚本
- `coupon_redesign_migration_postgresql.sql` - PostgreSQL 增量迁移脚本
- `coupon_redesign_migration_mysql.sql` - MySQL 增量迁移脚本

### 验证脚本
- `coupon_redesign_verification_postgresql.sql` - PostgreSQL 迁移验证脚本
- `coupon_redesign_verification_mysql.sql` - MySQL 迁移验证脚本

## 🎯 迁移内容

### 新建表
1. **user_coupons** - 用户优惠券实例表
   - 记录用户领取的优惠券
   - 支持核销状态跟踪
   - 关联订单和优惠金额

2. **coupon_redemption_bindings** - 兑换码-优惠券绑定关系表
   - 支持兑换码绑定特定优惠券
   - 支持专属用户绑定
   - 锁定状态管理

### 扩展表

#### coupons 表新增字段
- `type` - 优惠券类型（discount/full_reduction/instant_reduction）
- `threshold_amount` - 满减阈值（分）
- `currency` - 币种（当前固定 CNY）
- `version` - 乐观锁版本号

#### user_bills 表新增字段
- `coupon_id` - 使用的优惠券模板ID
- `user_coupon_id` - 使用的用户优惠券ID
- `discount_amount` - 优惠金额（分）
- `original_amount` - 原价（分）
- `final_amount` - 实付金额（分）

#### redemptions 表新增字段
- `bound_user_id` - 专属用户ID（NULL表示不限用户）

### 数据迁移
- 将 `coupons.bind_redemption_id` 关系迁移到 `coupon_redemption_bindings` 表
- 保留旧字段以兼容现有代码

## 🚀 执行步骤

### 前置条件
1. ✅ 已执行基础订阅系统迁移脚本
   - `subscription_migration_postgresql.sql` (PostgreSQL)
   - `subscription_migration_mysql.sql` (MySQL)
2. ✅ 已备份数据库

### PostgreSQL 执行步骤

```bash
# 1. 连接数据库
psql -U your_username -d your_database

# 2. 执行迁移脚本
\i /path/to/coupon_redesign_migration_postgresql.sql

# 3. 执行验证脚本
\i /path/to/coupon_redesign_verification_postgresql.sql

# 4. 检查验证结果，确认所有检查项显示 ✓
```

### MySQL 执行步骤

```bash
# 1. 连接数据库
mysql -u your_username -p your_database

# 2. 执行迁移脚本
source /path/to/coupon_redesign_migration_mysql.sql;

# 3. 执行验证脚本
source /path/to/coupon_redesign_verification_mysql.sql;

# 4. 检查验证结果，确认所有检查项显示 ✓
```

## ✅ 验证检查项

验证脚本会检查以下内容：

### Phase 1: 新表结构
- ✓ user_coupons 表创建
- ✓ user_coupons 表字段完整性
- ✓ user_coupons 表索引
- ✓ coupon_redemption_bindings 表创建
- ✓ coupon_redemption_bindings 表字段完整性
- ✓ coupon_redemption_bindings 表索引

### Phase 2: 现有表扩展
- ✓ coupons 表新增字段（type, threshold_amount, currency, version）
- ✓ user_bills 表新增字段（coupon_id, user_coupon_id, discount_amount, original_amount, final_amount）
- ✓ redemptions 表新增字段（bound_user_id）

### Phase 3: 数据迁移
- ✓ 绑定关系迁移数量验证
- ✓ 迁移数据完整性验证

### Phase 4: 默认值填充
- ✓ coupons 表字段默认值验证（无 NULL 值）

### Phase 5: 索引创建
- ✓ user_bills 表新索引（idx_bill_coupon, idx_bill_user_coupon）
- ✓ redemptions 表新索引（idx_redemptions_bound_user）

### Phase 6: 数据统计
- 各表数据量统计
- 优惠券类型分布
- 优惠券作用域分布

## ⚠️ 注意事项

### 1. 旧字段保留
迁移脚本保留了以下旧字段以兼容现有代码：
- `coupons.bind_redemption_id`
- `coupons.bind_locked_at`
- `coupons.bind_user_id`

建议观察一段时间（如 1-2 周）后，确认新系统运行稳定再删除这些字段。

### 2. 删除旧字段（可选）
如需删除旧字段，可取消迁移脚本中 Phase 4 的注释：

```sql
-- PostgreSQL
ALTER TABLE coupons DROP COLUMN IF EXISTS bind_redemption_id;
ALTER TABLE coupons DROP COLUMN IF EXISTS bind_locked_at;
ALTER TABLE coupons DROP COLUMN IF EXISTS bind_user_id;

-- MySQL
ALTER TABLE coupons DROP COLUMN IF EXISTS bind_redemption_id;
ALTER TABLE coupons DROP COLUMN IF EXISTS bind_locked_at;
ALTER TABLE coupons DROP COLUMN IF EXISTS bind_user_id;
```

### 3. 回滚准备
- 迁移前务必备份数据库
- 保存备份至少 7 天
- 建议先在测试环境验证

### 4. 幂等性
所有迁移脚本支持重复执行，不会破坏已有数据：
- 使用 `IF NOT EXISTS` 创建表和字段
- 使用 `ON CONFLICT DO NOTHING` (PostgreSQL) / `ON DUPLICATE KEY UPDATE` (MySQL) 防止重复迁移

## 📊 迁移统计

执行完成后，验证脚本会输出以下统计信息：

```
--- Phase 6: 数据统计 ---

6.1 统计各表数据量...
table_name                    | row_count
------------------------------|----------
coupons                       | 150
user_coupons                  | 0
coupon_redemption_bindings    | 25
user_bills                    | 5000
redemptions                   | 300

6.2 统计优惠券类型分布...
type              | count
------------------|------
discount          | 100
full_reduction    | 30
instant_reduction | 20

6.3 统计优惠券作用域分布...
scope                  | count
-----------------------|------
wallet_subscription    | 80
wallet                 | 40
subscription           | 30
```

## 🔗 相关文档

- 业务需求规范: `.spec-workflow/specs/subscribe/coupon_redemption_redesign.md`
- 系统设计文档: `.spec-workflow/specs/subscribe/design.md`
- API 接口文档: 见规范第三章

## 📞 支持

如遇到问题，请检查：
1. 数据库版本是否符合要求（PostgreSQL 9.5+, MySQL 5.7+）
2. 前置迁移脚本是否已执行
3. 用户权限是否足够（需要 CREATE/ALTER/INSERT 权限）

---

**版本**: v1.0
**创建日期**: 2025-12-09
**最后更新**: 2025-12-09
