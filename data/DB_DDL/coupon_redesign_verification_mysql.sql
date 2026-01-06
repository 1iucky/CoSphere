-- =============================================================================
-- 优惠券与兑换码改造迁移验证 - MySQL 版本
-- =============================================================================
-- 数据库: MySQL 5.7+
-- 创建日期: 2025-12-09
-- 版本: v1.0
-- 说明: 验证 coupon_redesign_migration_mysql.sql 执行结果
-- =============================================================================

SELECT '=============================================================================' AS '';
SELECT '开始验证优惠券与兑换码改造迁移（MySQL）' AS '';
SELECT '=============================================================================' AS '';
SELECT '' AS '';

-- =============================================================================
-- Phase 1: 验证新表结构
-- =============================================================================

SELECT '--- Phase 1: 验证新表结构 ---' AS '';
SELECT '' AS '';

-- 1.1 验证 user_coupons 表
SELECT '1.1 检查 user_coupons 表是否存在...' AS '';
SELECT
    CASE
        WHEN COUNT(*) > 0 THEN '✓ user_coupons 表已创建'
        ELSE '✗ user_coupons 表不存在'
    END AS result
FROM information_schema.tables
WHERE table_schema = DATABASE()
  AND table_name = 'user_coupons';

-- 验证 user_coupons 表字段
SELECT '1.2 验证 user_coupons 表字段...' AS '';
SELECT
    column_name,
    data_type,
    character_maximum_length,
    column_default,
    is_nullable,
    column_comment
FROM information_schema.columns
WHERE table_schema = DATABASE()
  AND table_name = 'user_coupons'
ORDER BY ordinal_position;

-- 验证 user_coupons 表索引
SELECT '1.3 验证 user_coupons 表索引...' AS '';
SELECT
    index_name,
    column_name,
    non_unique
FROM information_schema.statistics
WHERE table_schema = DATABASE()
  AND table_name = 'user_coupons'
ORDER BY index_name, seq_in_index;

-- 1.4 验证 coupon_redemption_bindings 表
SELECT '1.4 检查 coupon_redemption_bindings 表是否存在...' AS '';
SELECT
    CASE
        WHEN COUNT(*) > 0 THEN '✓ coupon_redemption_bindings 表已创建'
        ELSE '✗ coupon_redemption_bindings 表不存在'
    END AS result
FROM information_schema.tables
WHERE table_schema = DATABASE()
  AND table_name = 'coupon_redemption_bindings';

-- 验证 coupon_redemption_bindings 表字段
SELECT '1.5 验证 coupon_redemption_bindings 表字段...' AS '';
SELECT
    column_name,
    data_type,
    character_maximum_length,
    column_default,
    is_nullable,
    column_comment
FROM information_schema.columns
WHERE table_schema = DATABASE()
  AND table_name = 'coupon_redemption_bindings'
ORDER BY ordinal_position;

-- 验证 coupon_redemption_bindings 表索引
SELECT '1.6 验证 coupon_redemption_bindings 表索引...' AS '';
SELECT
    index_name,
    column_name,
    non_unique
FROM information_schema.statistics
WHERE table_schema = DATABASE()
  AND table_name = 'coupon_redemption_bindings'
ORDER BY index_name, seq_in_index;

-- =============================================================================
-- Phase 2: 验证现有表扩展
-- =============================================================================

SELECT '' AS '';
SELECT '--- Phase 2: 验证现有表扩展 ---' AS '';
SELECT '' AS '';

-- 2.1 验证 coupons 表新增字段
SELECT '2.1 验证 coupons 表新增字段...' AS '';
SELECT
    CASE
        WHEN SUM(column_name = 'type') > 0 THEN '✓ coupons.type 字段已添加'
        ELSE '✗ coupons.type 字段缺失'
    END AS type_check
FROM information_schema.columns
WHERE table_schema = DATABASE()
  AND table_name = 'coupons'
UNION ALL
SELECT
    CASE
        WHEN SUM(column_name = 'threshold_amount') > 0 THEN '✓ coupons.threshold_amount 字段已添加'
        ELSE '✗ coupons.threshold_amount 字段缺失'
    END
FROM information_schema.columns
WHERE table_schema = DATABASE()
  AND table_name = 'coupons'
UNION ALL
SELECT
    CASE
        WHEN SUM(column_name = 'currency') > 0 THEN '✓ coupons.currency 字段已添加'
        ELSE '✗ coupons.currency 字段缺失'
    END
FROM information_schema.columns
WHERE table_schema = DATABASE()
  AND table_name = 'coupons'
UNION ALL
SELECT
    CASE
        WHEN SUM(column_name = 'version') > 0 THEN '✓ coupons.version 字段已添加'
        ELSE '✗ coupons.version 字段缺失'
    END
FROM information_schema.columns
WHERE table_schema = DATABASE()
  AND table_name = 'coupons';

-- 2.2 验证 user_bills 表新增字段
SELECT '2.2 验证 user_bills 表新增字段...' AS '';
SELECT
    CASE
        WHEN SUM(column_name = 'coupon_id') > 0 THEN '✓ user_bills.coupon_id 字段已添加'
        ELSE '✗ user_bills.coupon_id 字段缺失'
    END AS coupon_id_check
FROM information_schema.columns
WHERE table_schema = DATABASE()
  AND table_name = 'user_bills'
UNION ALL
SELECT
    CASE
        WHEN SUM(column_name = 'user_coupon_id') > 0 THEN '✓ user_bills.user_coupon_id 字段已添加'
        ELSE '✗ user_bills.user_coupon_id 字段缺失'
    END
FROM information_schema.columns
WHERE table_schema = DATABASE()
  AND table_name = 'user_bills'
UNION ALL
SELECT
    CASE
        WHEN SUM(column_name = 'discount_amount') > 0 THEN '✓ user_bills.discount_amount 字段已添加'
        ELSE '✗ user_bills.discount_amount 字段缺失'
    END
FROM information_schema.columns
WHERE table_schema = DATABASE()
  AND table_name = 'user_bills'
UNION ALL
SELECT
    CASE
        WHEN SUM(column_name = 'original_amount') > 0 THEN '✓ user_bills.original_amount 字段已添加'
        ELSE '✗ user_bills.original_amount 字段缺失'
    END
FROM information_schema.columns
WHERE table_schema = DATABASE()
  AND table_name = 'user_bills'
UNION ALL
SELECT
    CASE
        WHEN SUM(column_name = 'final_amount') > 0 THEN '✓ user_bills.final_amount 字段已添加'
        ELSE '✗ user_bills.final_amount 字段缺失'
    END
FROM information_schema.columns
WHERE table_schema = DATABASE()
  AND table_name = 'user_bills';

-- 2.3 验证 redemptions 表新增字段
SELECT '2.3 验证 redemptions 表新增字段...' AS '';
SELECT
    CASE
        WHEN COUNT(*) > 0 THEN '✓ redemptions.bound_user_id 字段已添加'
        ELSE '✗ redemptions.bound_user_id 字段缺失'
    END AS result
FROM information_schema.columns
WHERE table_schema = DATABASE()
  AND table_name = 'redemptions'
  AND column_name = 'bound_user_id';

-- =============================================================================
-- Phase 3: 验证数据迁移
-- =============================================================================

SELECT '' AS '';
SELECT '--- Phase 3: 验证数据迁移 ---' AS '';
SELECT '' AS '';

-- 3.1 验证绑定关系迁移
SELECT '3.1 验证绑定关系迁移...' AS '';
SELECT
    (SELECT COUNT(*) FROM coupons WHERE bind_redemption_id IS NOT NULL) AS old_bindings_count,
    (SELECT COUNT(*) FROM coupon_redemption_bindings) AS new_bindings_count,
    CASE
        WHEN (SELECT COUNT(*) FROM coupon_redemption_bindings) >=
             (SELECT COUNT(*) FROM coupons WHERE bind_redemption_id IS NOT NULL)
        THEN '✓ 绑定关系迁移验证通过'
        ELSE '✗ 绑定关系迁移验证失败：新表数据数量少于旧数据'
    END AS migration_check;

-- 3.2 验证迁移数据完整性
SELECT '3.2 验证迁移数据完整性（前10条）...' AS '';
SELECT
    crb.id,
    crb.coupon_id,
    crb.redemption_id,
    crb.status,
    c.code AS coupon_code,
    c.name AS coupon_name
FROM coupon_redemption_bindings crb
LEFT JOIN coupons c ON crb.coupon_id = c.id
LIMIT 10;

-- =============================================================================
-- Phase 4: 验证默认值填充
-- =============================================================================

SELECT '' AS '';
SELECT '--- Phase 4: 验证默认值填充 ---' AS '';
SELECT '' AS '';

-- 4.1 验证 coupons 表默认值
SELECT '4.1 验证 coupons 表字段默认值...' AS '';
SELECT
    COUNT(*) AS total_coupons,
    SUM(CASE WHEN type IS NULL THEN 1 ELSE 0 END) AS null_type_count,
    SUM(CASE WHEN threshold_amount IS NULL THEN 1 ELSE 0 END) AS null_threshold_count,
    SUM(CASE WHEN currency IS NULL THEN 1 ELSE 0 END) AS null_currency_count,
    SUM(CASE WHEN version IS NULL THEN 1 ELSE 0 END) AS null_version_count,
    CASE
        WHEN SUM(CASE WHEN type IS NULL THEN 1 ELSE 0 END) = 0
             AND SUM(CASE WHEN threshold_amount IS NULL THEN 1 ELSE 0 END) = 0
             AND SUM(CASE WHEN currency IS NULL THEN 1 ELSE 0 END) = 0
             AND SUM(CASE WHEN version IS NULL THEN 1 ELSE 0 END) = 0
        THEN '✓ coupons 表默认值填充验证通过'
        ELSE '✗ coupons 表存在 NULL 值'
    END AS default_value_check
FROM coupons;

-- =============================================================================
-- Phase 5: 验证索引创建
-- =============================================================================

SELECT '' AS '';
SELECT '--- Phase 5: 验证索引创建 ---' AS '';
SELECT '' AS '';

-- 5.1 验证 user_bills 表索引
SELECT '5.1 验证 user_bills 表新索引...' AS '';
SELECT
    CASE
        WHEN COUNT(*) > 0 THEN '✓ idx_bill_coupon 索引已创建'
        ELSE '✗ idx_bill_coupon 索引缺失'
    END AS idx_coupon_check
FROM information_schema.statistics
WHERE table_schema = DATABASE()
  AND table_name = 'user_bills'
  AND index_name = 'idx_bill_coupon'
UNION ALL
SELECT
    CASE
        WHEN COUNT(*) > 0 THEN '✓ idx_bill_user_coupon 索引已创建'
        ELSE '✗ idx_bill_user_coupon 索引缺失'
    END
FROM information_schema.statistics
WHERE table_schema = DATABASE()
  AND table_name = 'user_bills'
  AND index_name = 'idx_bill_user_coupon';

-- 5.2 验证 redemptions 表索引
SELECT '5.2 验证 redemptions 表新索引...' AS '';
SELECT
    CASE
        WHEN COUNT(*) > 0 THEN '✓ idx_redemptions_bound_user 索引已创建'
        ELSE '✗ idx_redemptions_bound_user 索引缺失'
    END AS result
FROM information_schema.statistics
WHERE table_schema = DATABASE()
  AND table_name = 'redemptions'
  AND index_name = 'idx_redemptions_bound_user';

-- =============================================================================
-- Phase 6: 数据统计
-- =============================================================================

SELECT '' AS '';
SELECT '--- Phase 6: 数据统计 ---' AS '';
SELECT '' AS '';

-- 6.1 统计各表数据量
SELECT '6.1 统计各表数据量...' AS '';
SELECT
    'coupons' AS table_name,
    COUNT(*) AS row_count
FROM coupons
UNION ALL
SELECT
    'user_coupons',
    COUNT(*)
FROM user_coupons
UNION ALL
SELECT
    'coupon_redemption_bindings',
    COUNT(*)
FROM coupon_redemption_bindings
UNION ALL
SELECT
    'user_bills',
    COUNT(*)
FROM user_bills
UNION ALL
SELECT
    'redemptions',
    COUNT(*)
FROM redemptions;

-- 6.2 统计优惠券类型分布
SELECT '6.2 统计优惠券类型分布...' AS '';
SELECT
    type,
    COUNT(*) AS count
FROM coupons
GROUP BY type
ORDER BY count DESC;

-- 6.3 统计优惠券作用域分布
SELECT '6.3 统计优惠券作用域分布...' AS '';
SELECT
    scope,
    COUNT(*) AS count
FROM coupons
GROUP BY scope
ORDER BY count DESC;

-- =============================================================================
-- 完成提示
-- =============================================================================

SELECT '' AS '';
SELECT '=============================================================================' AS '';
SELECT '验证完成（MySQL）' AS '';
SELECT '=============================================================================' AS '';
SELECT '' AS '';
SELECT '如发现任何 ✗ 标记，请检查迁移脚本执行情况' AS '';
SELECT '如所有检查均显示 ✓，则迁移成功' AS '';
SELECT '' AS '';
SELECT '=============================================================================' AS '';
