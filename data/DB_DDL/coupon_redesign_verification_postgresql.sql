-- =============================================================================
-- 优惠券与兑换码改造迁移验证 - PostgreSQL 版本
-- =============================================================================
-- 数据库: PostgreSQL
-- 创建日期: 2025-12-09
-- 版本: v1.0
-- 说明: 验证 coupon_redesign_migration_postgresql.sql 执行结果
-- =============================================================================

\echo '============================================================================='
\echo '开始验证优惠券与兑换码改造迁移（PostgreSQL）'
\echo '============================================================================='
\echo ''

-- =============================================================================
-- Phase 1: 验证新表结构
-- =============================================================================

\echo '--- Phase 1: 验证新表结构 ---'
\echo ''

-- 1.1 验证 user_coupons 表
\echo '1.1 检查 user_coupons 表是否存在...'
SELECT
    CASE
        WHEN EXISTS (
            SELECT 1 FROM information_schema.tables
            WHERE table_name = 'user_coupons'
        ) THEN '✓ user_coupons 表已创建'
        ELSE '✗ user_coupons 表不存在'
    END AS result;

-- 验证 user_coupons 表字段
\echo '1.2 验证 user_coupons 表字段...'
SELECT
    column_name,
    data_type,
    character_maximum_length,
    column_default,
    is_nullable
FROM information_schema.columns
WHERE table_name = 'user_coupons'
ORDER BY ordinal_position;

-- 验证 user_coupons 表索引
\echo '1.3 验证 user_coupons 表索引...'
SELECT
    indexname,
    indexdef
FROM pg_indexes
WHERE tablename = 'user_coupons'
ORDER BY indexname;

-- 1.4 验证 coupon_redemption_bindings 表
\echo '1.4 检查 coupon_redemption_bindings 表是否存在...'
SELECT
    CASE
        WHEN EXISTS (
            SELECT 1 FROM information_schema.tables
            WHERE table_name = 'coupon_redemption_bindings'
        ) THEN '✓ coupon_redemption_bindings 表已创建'
        ELSE '✗ coupon_redemption_bindings 表不存在'
    END AS result;

-- 验证 coupon_redemption_bindings 表字段
\echo '1.5 验证 coupon_redemption_bindings 表字段...'
SELECT
    column_name,
    data_type,
    character_maximum_length,
    column_default,
    is_nullable
FROM information_schema.columns
WHERE table_name = 'coupon_redemption_bindings'
ORDER BY ordinal_position;

-- 验证 coupon_redemption_bindings 表索引
\echo '1.6 验证 coupon_redemption_bindings 表索引...'
SELECT
    indexname,
    indexdef
FROM pg_indexes
WHERE tablename = 'coupon_redemption_bindings'
ORDER BY indexname;

-- =============================================================================
-- Phase 2: 验证现有表扩展
-- =============================================================================

\echo ''
\echo '--- Phase 2: 验证现有表扩展 ---'
\echo ''

-- 2.1 验证 coupons 表新增字段
\echo '2.1 验证 coupons 表新增字段...'
SELECT
    CASE
        WHEN EXISTS (
            SELECT 1 FROM information_schema.columns
            WHERE table_name = 'coupons' AND column_name = 'type'
        ) THEN '✓ coupons.type 字段已添加'
        ELSE '✗ coupons.type 字段缺失'
    END AS type_check
UNION ALL
SELECT
    CASE
        WHEN EXISTS (
            SELECT 1 FROM information_schema.columns
            WHERE table_name = 'coupons' AND column_name = 'threshold_amount'
        ) THEN '✓ coupons.threshold_amount 字段已添加'
        ELSE '✗ coupons.threshold_amount 字段缺失'
    END
UNION ALL
SELECT
    CASE
        WHEN EXISTS (
            SELECT 1 FROM information_schema.columns
            WHERE table_name = 'coupons' AND column_name = 'currency'
        ) THEN '✓ coupons.currency 字段已添加'
        ELSE '✗ coupons.currency 字段缺失'
    END
UNION ALL
SELECT
    CASE
        WHEN EXISTS (
            SELECT 1 FROM information_schema.columns
            WHERE table_name = 'coupons' AND column_name = 'version'
        ) THEN '✓ coupons.version 字段已添加'
        ELSE '✗ coupons.version 字段缺失'
    END;

-- 2.2 验证 user_bills 表新增字段
\echo '2.2 验证 user_bills 表新增字段...'
SELECT
    CASE
        WHEN EXISTS (
            SELECT 1 FROM information_schema.columns
            WHERE table_name = 'user_bills' AND column_name = 'coupon_id'
        ) THEN '✓ user_bills.coupon_id 字段已添加'
        ELSE '✗ user_bills.coupon_id 字段缺失'
    END AS coupon_id_check
UNION ALL
SELECT
    CASE
        WHEN EXISTS (
            SELECT 1 FROM information_schema.columns
            WHERE table_name = 'user_bills' AND column_name = 'user_coupon_id'
        ) THEN '✓ user_bills.user_coupon_id 字段已添加'
        ELSE '✗ user_bills.user_coupon_id 字段缺失'
    END
UNION ALL
SELECT
    CASE
        WHEN EXISTS (
            SELECT 1 FROM information_schema.columns
            WHERE table_name = 'user_bills' AND column_name = 'discount_amount'
        ) THEN '✓ user_bills.discount_amount 字段已添加'
        ELSE '✗ user_bills.discount_amount 字段缺失'
    END
UNION ALL
SELECT
    CASE
        WHEN EXISTS (
            SELECT 1 FROM information_schema.columns
            WHERE table_name = 'user_bills' AND column_name = 'original_amount'
        ) THEN '✓ user_bills.original_amount 字段已添加'
        ELSE '✗ user_bills.original_amount 字段缺失'
    END
UNION ALL
SELECT
    CASE
        WHEN EXISTS (
            SELECT 1 FROM information_schema.columns
            WHERE table_name = 'user_bills' AND column_name = 'final_amount'
        ) THEN '✓ user_bills.final_amount 字段已添加'
        ELSE '✗ user_bills.final_amount 字段缺失'
    END;

-- 2.3 验证 redemptions 表新增字段
\echo '2.3 验证 redemptions 表新增字段...'
SELECT
    CASE
        WHEN EXISTS (
            SELECT 1 FROM information_schema.columns
            WHERE table_name = 'redemptions' AND column_name = 'bound_user_id'
        ) THEN '✓ redemptions.bound_user_id 字段已添加'
        ELSE '✗ redemptions.bound_user_id 字段缺失'
    END AS result;

-- =============================================================================
-- Phase 3: 验证数据迁移
-- =============================================================================

\echo ''
\echo '--- Phase 3: 验证数据迁移 ---'
\echo ''

-- 3.1 验证绑定关系迁移
\echo '3.1 验证绑定关系迁移...'
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
\echo '3.2 验证迁移数据完整性...'
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

\echo ''
\echo '--- Phase 4: 验证默认值填充 ---'
\echo ''

-- 4.1 验证 coupons 表默认值
\echo '4.1 验证 coupons 表字段默认值...'
SELECT
    COUNT(*) AS total_coupons,
    COUNT(CASE WHEN type IS NULL THEN 1 END) AS null_type_count,
    COUNT(CASE WHEN threshold_amount IS NULL THEN 1 END) AS null_threshold_count,
    COUNT(CASE WHEN currency IS NULL THEN 1 END) AS null_currency_count,
    COUNT(CASE WHEN version IS NULL THEN 1 END) AS null_version_count,
    CASE
        WHEN COUNT(CASE WHEN type IS NULL THEN 1 END) = 0
             AND COUNT(CASE WHEN threshold_amount IS NULL THEN 1 END) = 0
             AND COUNT(CASE WHEN currency IS NULL THEN 1 END) = 0
             AND COUNT(CASE WHEN version IS NULL THEN 1 END) = 0
        THEN '✓ coupons 表默认值填充验证通过'
        ELSE '✗ coupons 表存在 NULL 值'
    END AS default_value_check
FROM coupons;

-- =============================================================================
-- Phase 5: 验证索引创建
-- =============================================================================

\echo ''
\echo '--- Phase 5: 验证索引创建 ---'
\echo ''

-- 5.1 验证 user_bills 表索引
\echo '5.1 验证 user_bills 表新索引...'
SELECT
    CASE
        WHEN EXISTS (
            SELECT 1 FROM pg_indexes
            WHERE tablename = 'user_bills' AND indexname = 'idx_bill_coupon'
        ) THEN '✓ idx_bill_coupon 索引已创建'
        ELSE '✗ idx_bill_coupon 索引缺失'
    END AS idx_coupon_check
UNION ALL
SELECT
    CASE
        WHEN EXISTS (
            SELECT 1 FROM pg_indexes
            WHERE tablename = 'user_bills' AND indexname = 'idx_bill_user_coupon'
        ) THEN '✓ idx_bill_user_coupon 索引已创建'
        ELSE '✗ idx_bill_user_coupon 索引缺失'
    END;

-- 5.2 验证 redemptions 表索引
\echo '5.2 验证 redemptions 表新索引...'
SELECT
    CASE
        WHEN EXISTS (
            SELECT 1 FROM pg_indexes
            WHERE tablename = 'redemptions' AND indexname = 'idx_redemptions_bound_user'
        ) THEN '✓ idx_redemptions_bound_user 索引已创建'
        ELSE '✗ idx_redemptions_bound_user 索引缺失'
    END AS result;

-- =============================================================================
-- Phase 6: 数据统计
-- =============================================================================

\echo ''
\echo '--- Phase 6: 数据统计 ---'
\echo ''

-- 6.1 统计各表数据量
\echo '6.1 统计各表数据量...'
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
\echo '6.2 统计优惠券类型分布...'
SELECT
    type,
    COUNT(*) AS count
FROM coupons
GROUP BY type
ORDER BY count DESC;

-- 6.3 统计优惠券作用域分布
\echo '6.3 统计优惠券作用域分布...'
SELECT
    scope,
    COUNT(*) AS count
FROM coupons
GROUP BY scope
ORDER BY count DESC;

-- =============================================================================
-- 完成提示
-- =============================================================================

\echo ''
\echo '============================================================================='
\echo '验证完成（PostgreSQL）'
\echo '============================================================================='
\echo ''
\echo '如发现任何 ✗ 标记，请检查迁移脚本执行情况'
\echo '如所有检查均显示 ✓，则迁移成功'
\echo ''
\echo '============================================================================='
