-- =============================================================================
-- 订阅系统迁移数据一致性校验脚本
-- =============================================================================
-- 版本: v1.0
-- 日期: 2025-12-04
-- 用途: 验证迁移后的数据完整性和一致性
-- 数据库: PostgreSQL / MySQL
-- =============================================================================

-- =============================================================================
-- 使用说明
-- =============================================================================
--
-- PostgreSQL:
--   psql -U root -d new-api -f subscription_migration_verification.sql
--
-- MySQL:
--   mysql -u root -p new-api < subscription_migration_verification.sql
--
-- 期望结果:
--   - 所有检查项显示 ✓ 通过
--   - 如果显示 ✗ 失败，查看具体错误信息
-- =============================================================================

\echo '========================================================================='
\echo '开始数据一致性校验...'
\echo '========================================================================='
\echo ''

-- =============================================================================
-- 第一部分: 表结构验证
-- =============================================================================

\echo '【1. 表结构验证】'
\echo ''

-- 1.1 验证新表创建
\echo '1.1 验证 8 张新表是否创建成功...'

DO $$
DECLARE
    expected_tables TEXT[] := ARRAY[
        'subscription_plans',
        'subscription_plan_limits',
        'coupons',
        'subscription_orders',
        'subscriptions',
        'subscription_usages',
        'user_bills',
        'audit_logs'
    ];
    v_table_name TEXT;
    table_count INT;
    missing_tables TEXT := '';
BEGIN
    FOREACH v_table_name IN ARRAY expected_tables
    LOOP
        SELECT COUNT(*) INTO table_count
        FROM information_schema.tables
        WHERE table_schema = 'public'
          AND tables.table_name = v_table_name;

        IF table_count = 0 THEN
            missing_tables := missing_tables || v_table_name || ', ';
        END IF;
    END LOOP;

    IF missing_tables = '' THEN
        RAISE NOTICE '  ✓ 所有 8 张新表创建成功';
    ELSE
        RAISE EXCEPTION '  ✗ 缺失表: %', TRIM(TRAILING ', ' FROM missing_tables);
    END IF;
END $$;

\echo ''

-- 1.2 验证字段扩展
\echo '1.2 验证现有表字段扩展...'

DO $$
DECLARE
    field_count INT;
BEGIN
    -- 验证 tokens.subscription_preferred
    SELECT COUNT(*) INTO field_count
    FROM information_schema.columns
    WHERE table_name = 'tokens'
      AND column_name = 'subscription_preferred';

    IF field_count = 0 THEN
        RAISE EXCEPTION '  ✗ tokens.subscription_preferred 字段不存在';
    END IF;

    -- 验证 tokens.auto_smart_group
    SELECT COUNT(*) INTO field_count
    FROM information_schema.columns
    WHERE table_name = 'tokens'
      AND column_name = 'auto_smart_group';

    IF field_count = 0 THEN
        RAISE EXCEPTION '  ✗ tokens.auto_smart_group 字段不存在';
    END IF;

    -- 验证 redemptions.type
    SELECT COUNT(*) INTO field_count
    FROM information_schema.columns
    WHERE table_name = 'redemptions'
      AND column_name = 'type';

    IF field_count = 0 THEN
        RAISE EXCEPTION '  ✗ redemptions.type 字段不存在';
    END IF;

    -- 验证 redemptions.payload
    SELECT COUNT(*) INTO field_count
    FROM information_schema.columns
    WHERE table_name = 'redemptions'
      AND column_name = 'payload';

    IF field_count = 0 THEN
        RAISE EXCEPTION '  ✗ redemptions.payload 字段不存在';
    END IF;

    RAISE NOTICE '  ✓ 所有字段扩展验证通过';
END $$;

\echo ''

-- 1.3 验证配置项插入
\echo '1.3 验证系统配置项是否插入...'

DO $$
DECLARE
    config_count INT;
BEGIN
    SELECT COUNT(*) INTO config_count
    FROM options
    WHERE key LIKE 'SUBSCRIPTION_%';

    IF config_count < 5 THEN
        RAISE EXCEPTION '  ✗ 订阅配置项不足，期望 5 个，实际 %', config_count;
    END IF;

    RAISE NOTICE '  ✓ 系统配置项验证通过（% 个配置项）', config_count;
END $$;

\echo ''

-- =============================================================================
-- 第二部分: 索引验证
-- =============================================================================

\echo '【2. 索引验证】'
\echo ''

\echo '2.1 验证关键索引是否创建...'

DO $$
DECLARE
    index_count INT;
BEGIN
    -- 验证 subscription_plans 索引
    SELECT COUNT(*) INTO index_count
    FROM pg_indexes
    WHERE tablename = 'subscription_plans'
      AND indexname IN ('idx_subscription_plans_sku', 'idx_subscription_plans_status');

    IF index_count < 2 THEN
        RAISE EXCEPTION '  ✗ subscription_plans 索引缺失';
    END IF;

    -- 验证 subscriptions 索引
    SELECT COUNT(*) INTO index_count
    FROM pg_indexes
    WHERE tablename = 'subscriptions'
      AND indexname IN ('idx_subscriptions_user_status', 'idx_subscriptions_user_priority', 'idx_subscriptions_end_at_status', 'idx_subscriptions_plan_id');

    IF index_count < 4 THEN
        RAISE EXCEPTION '  ✗ subscriptions 索引缺失';
    END IF;

    -- 验证 coupons 索引
    SELECT COUNT(*) INTO index_count
    FROM pg_indexes
    WHERE tablename = 'coupons'
      AND indexname LIKE 'idx_coupons_%';

    IF index_count < 5 THEN
        RAISE EXCEPTION '  ✗ coupons 索引缺失';
    END IF;

    RAISE NOTICE '  ✓ 所有关键索引验证通过';
END $$;

\echo ''

-- =============================================================================
-- 第三部分: 数据迁移验证
-- =============================================================================

\echo '【3. 数据迁移验证】'
\echo ''

-- 3.1 验证 users.setting 初始化
\echo '3.1 验证 users.setting 字段初始化...'

DO $$
DECLARE
    empty_setting_count INT;
    total_users INT;
    users_with_new_keys INT;
BEGIN
    -- 检查空值数量
    SELECT COUNT(*) INTO empty_setting_count
    FROM users
    WHERE setting IS NULL OR setting = '';

    IF empty_setting_count > 0 THEN
        RAISE EXCEPTION '  ✗ 存在 % 个用户的 setting 字段为空', empty_setting_count;
    END IF;

    -- 检查总用户数
    SELECT COUNT(*) INTO total_users FROM users;

    -- 检查新键是否存在
    SELECT COUNT(*) INTO users_with_new_keys
    FROM users
    WHERE setting::jsonb ? 'auto_wallet_fallback';

    IF users_with_new_keys < total_users THEN
        RAISE EXCEPTION '  ✗ 只有 %/% 用户包含 auto_wallet_fallback 键', users_with_new_keys, total_users;
    END IF;

    RAISE NOTICE '  ✓ users.setting 初始化验证通过（% 个用户）', total_users;
END $$;

\echo ''

-- 3.2 验证 redemptions.type 默认值
\echo '3.2 验证 redemptions.type 默认值...'

DO $$
DECLARE
    null_type_count INT;
    total_redemptions INT;
BEGIN
    SELECT COUNT(*) INTO total_redemptions FROM redemptions;

    IF total_redemptions > 0 THEN
        SELECT COUNT(*) INTO null_type_count
        FROM redemptions
        WHERE type IS NULL;

        IF null_type_count > 0 THEN
            RAISE EXCEPTION '  ✗ 存在 % 条 redemptions 记录的 type 为 NULL', null_type_count;
        END IF;

        RAISE NOTICE '  ✓ redemptions.type 默认值验证通过（% 条记录）', total_redemptions;
    ELSE
        RAISE NOTICE '  ℹ redemptions 表无数据，跳过验证';
    END IF;
END $$;

\echo ''

-- =============================================================================
-- 第四部分: 数据完整性检查
-- =============================================================================

\echo '【4. 数据完整性检查】'
\echo ''

-- 4.1 检查表记录统计
\echo '4.1 表记录统计...'

SELECT
    '  subscription_plans' AS table_name,
    COUNT(*) AS record_count
FROM subscription_plans
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

\echo ''

-- 4.2 检查序列当前值
\echo '4.2 序列当前值检查...'

SELECT
    '  ' || sequence_name AS sequence_name,
    last_value
FROM (
    SELECT 'subscription_plans_id_seq' AS sequence_name, last_value FROM subscription_plans_id_seq
    UNION ALL
    SELECT 'subscription_plan_limits_id_seq', last_value FROM subscription_plan_limits_id_seq
    UNION ALL
    SELECT 'coupons_id_seq', last_value FROM coupons_id_seq
    UNION ALL
    SELECT 'subscription_orders_id_seq', last_value FROM subscription_orders_id_seq
    UNION ALL
    SELECT 'subscriptions_id_seq', last_value FROM subscriptions_id_seq
    UNION ALL
    SELECT 'subscription_usages_id_seq', last_value FROM subscription_usages_id_seq
    UNION ALL
    SELECT 'user_bills_id_seq', last_value FROM user_bills_id_seq
    UNION ALL
    SELECT 'audit_logs_id_seq', last_value FROM audit_logs_id_seq
) seqs;

\echo ''

-- =============================================================================
-- 第五部分: 配置检查
-- =============================================================================

\echo '【5. 系统配置检查】'
\echo ''

\echo '5.1 订阅系统配置项...'

SELECT
    '  ' || key AS config_key,
    value
FROM options
WHERE key LIKE 'SUBSCRIPTION_%'
ORDER BY key;

\echo ''

-- =============================================================================
-- 第六部分: 用户 setting 抽样检查
-- =============================================================================

\echo '【6. 用户 setting 抽样检查（前 5 个用户）】'
\echo ''

SELECT
    id,
    username,
    setting::jsonb->>'auto_wallet_fallback' AS auto_wallet_fallback,
    setting::jsonb->>'language' AS language,
    setting::jsonb->>'timezone' AS timezone,
    setting::jsonb ? 'notify_type' AS has_old_keys
FROM users
ORDER BY id
LIMIT 5;

\echo ''

-- =============================================================================
-- 验证总结
-- =============================================================================

\echo '========================================================================='
\echo '数据一致性校验完成'
\echo '========================================================================='
\echo ''
\echo '如果所有检查项显示 ✓ 通过，说明迁移成功。'
\echo '如果有 ✗ 失败项，请检查错误信息并执行回滚。'
\echo ''
\echo '下一步:'
\echo '  1. 清理 Redis 缓存: redis-cli FLUSHDB'
\echo '  2. 重启服务: systemctl restart new-api'
\echo '  3. 执行冒烟测试'
\echo ''
