-- =============================================================================
-- 订阅系统迁移数据一致性校验脚本 - MySQL 版本
-- =============================================================================
-- 版本: v1.0
-- 日期: 2025-12-04
-- 用途: 验证迁移后的数据完整性和一致性
-- 数据库: MySQL
-- =============================================================================

-- =============================================================================
-- 使用说明
-- =============================================================================
--
-- MySQL:
--   mysql -u root -p new-api < subscription_migration_verification_mysql.sql
--
-- 期望结果:
--   - 所有检查项显示 ✓ 通过
--   - 如果显示 ✗ 失败，查看具体错误信息
-- =============================================================================

SELECT '=========================================================================' AS '';
SELECT '开始数据一致性校验...' AS '';
SELECT '=========================================================================' AS '';
SELECT '' AS '';

-- =============================================================================
-- 第一部分: 表结构验证
-- =============================================================================

SELECT '【1. 表结构验证】' AS '';
SELECT '' AS '';

-- 1.1 验证新表创建
SELECT '1.1 验证 8 张新表是否创建成功...' AS '';

SELECT
    CASE
        WHEN COUNT(*) = 8 THEN '  ✓ 所有 8 张新表创建成功'
        ELSE CONCAT('  ✗ 只创建了 ', COUNT(*), ' 张表，期望 8 张')
    END AS result
FROM information_schema.tables
WHERE table_schema = DATABASE()
  AND table_name IN (
    'subscription_plans',
    'subscription_plan_limits',
    'coupons',
    'subscription_orders',
    'subscriptions',
    'subscription_usages',
    'user_bills',
    'audit_logs'
);

SELECT '' AS '';

-- 1.2 验证字段扩展
SELECT '1.2 验证现有表字段扩展...' AS '';

-- tokens.subscription_preferred 存在
SELECT
    CASE
        WHEN COUNT(*) = 1 THEN '  ✓ tokens.subscription_preferred 字段存在'
        ELSE '  ✗ tokens.subscription_preferred 字段不存在'
    END AS result
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'tokens'
  AND COLUMN_NAME = 'subscription_preferred';

-- tokens.auto_smart_group 存在
SELECT
    CASE
        WHEN COUNT(*) = 1 THEN '  ✓ tokens.auto_smart_group 字段存在'
        ELSE '  ✗ tokens.auto_smart_group 字段不存在'
    END AS result
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'tokens'
  AND COLUMN_NAME = 'auto_smart_group';

-- redemptions.type 存在
SELECT
    CASE
        WHEN COUNT(*) = 1 THEN '  ✓ redemptions.type 字段存在'
        ELSE '  ✗ redemptions.type 字段不存在'
    END AS result
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'redemptions'
  AND COLUMN_NAME = 'type';

-- redemptions.payload 存在
SELECT
    CASE
        WHEN COUNT(*) = 1 THEN '  ✓ redemptions.payload 字段存在'
        ELSE '  ✗ redemptions.payload 字段不存在'
    END AS result
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'redemptions'
  AND COLUMN_NAME = 'payload';

SELECT '' AS '';

-- 1.3 验证配置项插入
SELECT '1.3 验证系统配置项是否插入...' AS '';

SELECT
    CASE
        WHEN COUNT(*) >= 5 THEN CONCAT('  ✓ 系统配置项验证通过（', COUNT(*), ' 个配置项）')
        ELSE CONCAT('  ✗ 订阅配置项不足，期望 5 个，实际 ', COUNT(*))
    END AS result
FROM options
WHERE `key` LIKE 'SUBSCRIPTION_%';

SELECT '' AS '';

-- =============================================================================
-- 第二部分: 索引验证
-- =============================================================================

SELECT '【2. 索引验证】' AS '';
SELECT '' AS '';

SELECT '2.1 验证关键索引是否创建...' AS '';

-- subscription_plans 索引
SELECT
    CASE
        WHEN COUNT(*) >= 2 THEN '  ✓ subscription_plans 索引验证通过'
        ELSE CONCAT('  ✗ subscription_plans 索引缺失，期望 2 个，实际 ', COUNT(*))
    END AS result
FROM information_schema.STATISTICS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'subscription_plans'
  AND INDEX_NAME IN ('idx_subscription_plans_sku', 'idx_subscription_plans_status');

-- subscriptions 索引
SELECT
    CASE
        WHEN COUNT(*) >= 4 THEN '  ✓ subscriptions 索引验证通过'
        ELSE CONCAT('  ✗ subscriptions 索引缺失，期望 4 个，实际 ', COUNT(*))
    END AS result
FROM information_schema.STATISTICS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'subscriptions'
  AND INDEX_NAME IN ('idx_subscriptions_user_status', 'idx_subscriptions_user_priority', 'idx_subscriptions_end_at_status', 'idx_subscriptions_plan_id');

-- coupons 索引
SELECT
    CASE
        WHEN COUNT(*) >= 5 THEN '  ✓ coupons 索引验证通过'
        ELSE CONCAT('  ✗ coupons 索引缺失，期望 5 个，实际 ', COUNT(*))
    END AS result
FROM information_schema.STATISTICS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'coupons'
  AND INDEX_NAME LIKE 'idx_coupons_%';

SELECT '' AS '';

-- =============================================================================
-- 第三部分: 数据迁移验证
-- =============================================================================

SELECT '【3. 数据迁移验证】' AS '';
SELECT '' AS '';

-- 3.1 验证 users.setting 初始化
SELECT '3.1 验证 users.setting 字段初始化...' AS '';

-- 检查空值
SELECT
    CASE
        WHEN COUNT(*) = 0 THEN CONCAT('  ✓ users.setting 无空值（总用户数: ', (SELECT COUNT(*) FROM users), '）')
        ELSE CONCAT('  ✗ 存在 ', COUNT(*), ' 个用户的 setting 字段为空')
    END AS result
FROM users
WHERE setting IS NULL OR setting = '';

-- 检查新键
SELECT
    CASE
        WHEN COUNT(*) = (SELECT COUNT(*) FROM users) THEN '  ✓ 所有用户包含 auto_wallet_fallback 键'
        ELSE CONCAT('  ✗ 只有 ', COUNT(*), '/', (SELECT COUNT(*) FROM users), ' 用户包含 auto_wallet_fallback 键')
    END AS result
FROM users
WHERE JSON_CONTAINS_PATH(setting, 'one', '$.auto_wallet_fallback');

SELECT '' AS '';

-- 3.2 验证 redemptions.type 默认值
SELECT '3.2 验证 redemptions.type 默认值...' AS '';

SELECT
    CASE
        WHEN (SELECT COUNT(*) FROM redemptions) = 0 THEN '  ℹ redemptions 表无数据，跳过验证'
        WHEN COUNT(*) = 0 THEN CONCAT('  ✓ redemptions.type 默认值验证通过（', (SELECT COUNT(*) FROM redemptions), ' 条记录）')
        ELSE CONCAT('  ✗ 存在 ', COUNT(*), ' 条 redemptions 记录的 type 为 NULL')
    END AS result
FROM redemptions
WHERE type IS NULL;

SELECT '' AS '';

-- =============================================================================
-- 第四部分: 数据完整性检查
-- =============================================================================

SELECT '【4. 数据完整性检查】' AS '';
SELECT '' AS '';

SELECT '4.1 表记录统计...' AS '';

SELECT '  subscription_plans' AS table_name, COUNT(*) AS record_count FROM subscription_plans
UNION ALL
SELECT '  subscription_plan_limits', COUNT(*) FROM subscription_plan_limits
UNION ALL
SELECT '  coupons', COUNT(*) FROM coupons
UNION ALL
SELECT '  subscription_orders', COUNT(*) FROM subscription_orders
UNION ALL
SELECT '  subscriptions', COUNT(*) FROM subscriptions
UNION ALL
SELECT '  subscription_usages', COUNT(*) FROM subscription_usages
UNION ALL
SELECT '  user_bills', COUNT(*) FROM user_bills
UNION ALL
SELECT '  audit_logs', COUNT(*) FROM audit_logs;

SELECT '' AS '';

SELECT '4.2 自增 ID 当前值检查...' AS '';

SELECT
    CONCAT('  ', TABLE_NAME) AS table_name,
    AUTO_INCREMENT AS next_auto_increment_value
FROM information_schema.TABLES
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME IN (
    'subscription_plans',
    'subscription_plan_limits',
    'coupons',
    'subscription_orders',
    'subscriptions',
    'subscription_usages',
    'user_bills',
    'audit_logs'
);

SELECT '' AS '';

-- =============================================================================
-- 第五部分: 配置检查
-- =============================================================================

SELECT '【5. 系统配置检查】' AS '';
SELECT '' AS '';

SELECT '5.1 订阅系统配置项...' AS '';

SELECT
    CONCAT('  ', `key`) AS config_key,
    value
FROM options
WHERE `key` LIKE 'SUBSCRIPTION_%'
ORDER BY `key`;

SELECT '' AS '';

-- =============================================================================
-- 第六部分: 用户 setting 抽样检查
-- =============================================================================

SELECT '【6. 用户 setting 抽样检查（前 5 个用户）】' AS '';
SELECT '' AS '';

SELECT
    id,
    username,
    JSON_EXTRACT(setting, '$.auto_wallet_fallback') AS auto_wallet_fallback,
    JSON_EXTRACT(setting, '$.language') AS language,
    JSON_EXTRACT(setting, '$.timezone') AS timezone,
    JSON_CONTAINS_PATH(setting, 'one', '$.notify_type') AS has_old_keys
FROM users
ORDER BY id
LIMIT 5;

SELECT '' AS '';

-- =============================================================================
-- 验证总结
-- =============================================================================

SELECT '=========================================================================' AS '';
SELECT '数据一致性校验完成' AS '';
SELECT '=========================================================================' AS '';
SELECT '' AS '';
SELECT '如果所有检查项显示 ✓ 通过，说明迁移成功。' AS '';
SELECT '如果有 ✗ 失败项，请检查错误信息并执行回滚。' AS '';
SELECT '' AS '';
SELECT '下一步:' AS '';
SELECT '  1. 清理 Redis 缓存: redis-cli FLUSHDB' AS '';
SELECT '  2. 重启服务: systemctl restart new-api' AS '';
SELECT '  3. 执行冒烟测试' AS '';
SELECT '' AS '';
