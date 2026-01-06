-- =============================================================================
-- 订阅系统数据库表结构 DDL - PostgreSQL 版本
-- =============================================================================
-- 数据库: PostgreSQL
-- 创建日期: 2025-12-04
-- 版本: v1.1（修复自增ID问题）
-- 说明: 订阅套餐、优惠券、兑换码、账单、审计日志表结构
-- =============================================================================

-- =============================================================================
-- Phase 0: 创建序列（SEQUENCE）
-- =============================================================================

CREATE SEQUENCE IF NOT EXISTS subscription_plans_id_seq;
CREATE SEQUENCE IF NOT EXISTS subscription_plan_limits_id_seq;
CREATE SEQUENCE IF NOT EXISTS coupons_id_seq;
CREATE SEQUENCE IF NOT EXISTS subscription_orders_id_seq;
CREATE SEQUENCE IF NOT EXISTS subscriptions_id_seq;
CREATE SEQUENCE IF NOT EXISTS subscription_usages_id_seq;
CREATE SEQUENCE IF NOT EXISTS user_bills_id_seq;
CREATE SEQUENCE IF NOT EXISTS audit_logs_id_seq;

-- =============================================================================
-- Phase 1: 新建核心业务表
-- =============================================================================

-- -----------------------------------------------------------------------------
-- 2.1 subscription_plans [新建表] - 订阅套餐表
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS subscription_plans (
    id BIGINT PRIMARY KEY NOT NULL DEFAULT nextval('subscription_plans_id_seq'::regclass),
    sku VARCHAR(64) UNIQUE,
    name VARCHAR(128) NOT NULL,
    description TEXT,
    price_cents BIGINT NOT NULL,
    currency VARCHAR(8) DEFAULT 'USD',
    billing_cycle VARCHAR(32) NOT NULL,
    billing_cycle_value INT DEFAULT 1,
    allow_wallet_fallback BOOLEAN DEFAULT TRUE,
    status VARCHAR(32) DEFAULT 'draft',
    start_at BIGINT,
    end_at BIGINT,
    model_whitelist TEXT,
    channel_groups TEXT,
    extra TEXT,
    created_at BIGINT,
    updated_at BIGINT
);

-- 索引
CREATE INDEX IF NOT EXISTS idx_subscription_plans_status ON subscription_plans(status);
CREATE INDEX IF NOT EXISTS idx_subscription_plans_sku ON subscription_plans(sku);

-- -----------------------------------------------------------------------------
-- 2.2 subscription_plan_limits [新建表] - 套餐限额配置表
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS subscription_plan_limits (
    id BIGINT PRIMARY KEY NOT NULL DEFAULT nextval('subscription_plan_limits_id_seq'::regclass),
    plan_id BIGINT NOT NULL,
    period VARCHAR(32) NOT NULL,
    quota BIGINT NOT NULL,
    unit VARCHAR(32) DEFAULT 'quota',
    enabled BOOLEAN DEFAULT TRUE,
    window_strategy VARCHAR(32) DEFAULT 'rolling',
    created_at BIGINT,
    updated_at BIGINT,
    UNIQUE(plan_id, period)
);

-- 索引：关联 subscription_plans.id（应用层约束）
CREATE INDEX IF NOT EXISTS idx_subscription_plan_limits_plan_id ON subscription_plan_limits(plan_id);

-- -----------------------------------------------------------------------------
-- 2.3 coupons [新建表] - 优惠券表
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS coupons (
    id BIGINT PRIMARY KEY NOT NULL DEFAULT nextval('coupons_id_seq'::regclass),
    code VARCHAR(64) UNIQUE NOT NULL,
    name VARCHAR(128) NOT NULL,
    description TEXT,
    scope VARCHAR(32) DEFAULT 'quota',
    discount_type VARCHAR(32) NOT NULL,
    discount_value BIGINT NOT NULL,
    applicable_plan_ids TEXT,
    total_count BIGINT DEFAULT 1,
    used_count BIGINT DEFAULT 0,
    per_user_limit BIGINT DEFAULT 1,
    valid_from BIGINT NOT NULL,
    valid_to BIGINT NOT NULL,
    status VARCHAR(32) DEFAULT 'active',
    bind_user_id BIGINT,
    bind_redemption_id BIGINT,
    bind_locked_at BIGINT,
    created_by BIGINT NOT NULL,
    created_at BIGINT,
    updated_at BIGINT
);

-- 索引
CREATE UNIQUE INDEX IF NOT EXISTS idx_coupons_code ON coupons(code);
CREATE INDEX IF NOT EXISTS idx_coupons_status ON coupons(status);
CREATE INDEX IF NOT EXISTS idx_coupons_bind_redemption ON coupons(bind_redemption_id);
CREATE INDEX IF NOT EXISTS idx_coupons_bind_user ON coupons(bind_user_id);
CREATE INDEX IF NOT EXISTS idx_coupons_valid_to ON coupons(valid_to);  -- 补充：到期扫描

-- -----------------------------------------------------------------------------
-- 2.4 subscription_orders [新建表] - 订阅订单表
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS subscription_orders (
    id BIGINT PRIMARY KEY NOT NULL DEFAULT nextval('subscription_orders_id_seq'::regclass),
    user_id BIGINT NOT NULL,
    plan_id BIGINT NOT NULL,
    plan_snapshot TEXT NOT NULL,
    coupon_id BIGINT,
    coupon_snapshot TEXT,
    payment_channel VARCHAR(32) NOT NULL,
    price_cents BIGINT NOT NULL,
    discount_cents BIGINT DEFAULT 0,
    final_price_cents BIGINT NOT NULL,
    redemption_id BIGINT,
    bill_id BIGINT,
    status VARCHAR(32) DEFAULT 'pending',
    metadata TEXT,
    created_at BIGINT,
    updated_at BIGINT
);

-- 索引：关联 users.id, subscription_plans.id, user_bills.id（应用层约束）
CREATE INDEX IF NOT EXISTS idx_subscription_orders_user ON subscription_orders(user_id);
CREATE INDEX IF NOT EXISTS idx_subscription_orders_plan ON subscription_orders(plan_id);
CREATE INDEX IF NOT EXISTS idx_subscription_orders_bill ON subscription_orders(bill_id);
CREATE INDEX IF NOT EXISTS idx_subscription_orders_status ON subscription_orders(status);
CREATE INDEX IF NOT EXISTS idx_subscription_orders_coupon ON subscription_orders(coupon_id);  -- 补充：优惠券聚合查询

-- -----------------------------------------------------------------------------
-- 2.5 subscriptions [新建表] - 用户订阅表
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS subscriptions (
    id BIGINT PRIMARY KEY NOT NULL DEFAULT nextval('subscriptions_id_seq'::regclass),
    user_id BIGINT NOT NULL,
    plan_id BIGINT NOT NULL,
    order_id BIGINT,
    status VARCHAR(32) DEFAULT 'pending',
    start_at BIGINT NOT NULL,
    end_at BIGINT NOT NULL,
    priority INT NOT NULL,
    auto_wallet_fallback BOOLEAN DEFAULT FALSE,
    bind_channel_group VARCHAR(64),
    model_whitelist_cache TEXT,
    redemption_id BIGINT,
    coupon_id BIGINT,
    redeem_option VARCHAR(32) DEFAULT 'stack',
    metadata TEXT,
    created_at BIGINT,
    updated_at BIGINT
);

-- 索引：关联 users.id, subscription_plans.id, subscription_orders.id, redemptions.id, coupons.id（应用层约束）
CREATE INDEX IF NOT EXISTS idx_subscriptions_user_status ON subscriptions(user_id, status);
CREATE INDEX IF NOT EXISTS idx_subscriptions_user_priority ON subscriptions(user_id, priority);
CREATE INDEX IF NOT EXISTS idx_subscriptions_end_at_status ON subscriptions(end_at, status);
CREATE INDEX IF NOT EXISTS idx_subscriptions_plan_id ON subscriptions(plan_id);  -- 补充：套餐聚合查询

-- -----------------------------------------------------------------------------
-- 2.6 subscription_usages [新建表] - 订阅使用量表（滚动窗口）
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS subscription_usages (
    id BIGINT PRIMARY KEY NOT NULL DEFAULT nextval('subscription_usages_id_seq'::regclass),
    subscription_id BIGINT NOT NULL,
    period VARCHAR(32) NOT NULL,
    window_start BIGINT NOT NULL,
    window_end BIGINT NOT NULL,
    used_quota BIGINT NOT NULL DEFAULT 0,
    limit_quota BIGINT NOT NULL,
    created_at BIGINT,
    updated_at BIGINT,
    UNIQUE(subscription_id, period, window_start)
);

-- 索引：关联 subscriptions.id（应用层约束，删除时需级联清理）
CREATE INDEX IF NOT EXISTS idx_subscription_usage_window ON subscription_usages(subscription_id, period);
CREATE INDEX IF NOT EXISTS idx_subscription_usage_subscription ON subscription_usages(subscription_id);

-- =============================================================================
-- Phase 2: 新建辅助表
-- =============================================================================

-- -----------------------------------------------------------------------------
-- 2.7 user_bills [新建表] - 用户账单流水表
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS user_bills (
    id BIGINT PRIMARY KEY NOT NULL DEFAULT nextval('user_bills_id_seq'::regclass),
    user_id BIGINT NOT NULL,
    bill_type VARCHAR(64) NOT NULL,
    amount BIGINT NOT NULL,
    balance_before BIGINT NOT NULL,
    balance_after BIGINT NOT NULL,
    source_type VARCHAR(64),
    source_id BIGINT,
    payment_channel VARCHAR(64),
    trade_no VARCHAR(255),
    description TEXT,
    refund_type VARCHAR(32),               -- 手工退款等标记
    conversion_metadata TEXT,              -- 兑换折算/不足一天金额等审计信息
    metadata TEXT,
    operator_id BIGINT,
    created_at BIGINT NOT NULL
);

-- 索引：关联 users.id（应用层约束）
CREATE INDEX IF NOT EXISTS idx_user_bills_user_id_created_at ON user_bills(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_user_bills_source ON user_bills(source_type, source_id);
CREATE INDEX IF NOT EXISTS idx_user_bills_trade_no ON user_bills(trade_no);

-- 补充缺失字段（兼容已存在的表）
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'user_bills' AND column_name = 'refund_type'
    ) THEN
        ALTER TABLE user_bills ADD COLUMN refund_type VARCHAR(32);
        RAISE NOTICE 'user_bills.refund_type 已添加';
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'user_bills' AND column_name = 'conversion_metadata'
    ) THEN
        ALTER TABLE user_bills ADD COLUMN conversion_metadata TEXT;
        RAISE NOTICE 'user_bills.conversion_metadata 已添加';
    END IF;
END$$;

-- -----------------------------------------------------------------------------
-- 2.8 audit_logs [新建表] - 审计日志表
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS audit_logs (
    id BIGINT PRIMARY KEY NOT NULL DEFAULT nextval('audit_logs_id_seq'::regclass),
    user_id BIGINT,
    operator_id BIGINT,
    object_type VARCHAR(64) NOT NULL,
    object_id BIGINT,
    action VARCHAR(64) NOT NULL,
    ip_address VARCHAR(64),
    user_agent TEXT,
    metadata TEXT,
    created_at BIGINT NOT NULL
);

-- 索引：关联 users.id（应用层约束，用户删除时 user_id 设为 NULL）
CREATE INDEX IF NOT EXISTS idx_audit_logs_user_created_at ON audit_logs(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_object ON audit_logs(object_type, object_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at ON audit_logs(created_at DESC);

-- =============================================================================
-- 完成提示
-- =============================================================================
-- Phase 1-2 表结构创建完成
--
-- 接下来将自动执行：
-- - Phase 3: 扩展现有表（tokens、redemptions、options）
-- - Phase 4: 初始化 users.setting 数据
-- =============================================================================
-- =============================================================================
-- 订阅系统 Phase 3: 扩展现有表（ALTER TABLE）- PostgreSQL 版本
-- =============================================================================
-- 数据库: PostgreSQL
-- 创建日期: 2025-12-04
-- 版本: v1.0
-- 前置条件: Phase 1-2 新表已创建
-- =============================================================================

-- -----------------------------------------------------------------------------
-- 3.1 扩展 tokens 表 - 新增 subscription_preferred + 保留 auto_smart_group
-- -----------------------------------------------------------------------------

-- 确保 subscription_preferred 字段存在
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'tokens' AND column_name = 'subscription_preferred'
    ) THEN
        ALTER TABLE tokens ADD COLUMN subscription_preferred BOOLEAN DEFAULT FALSE;
        COMMENT ON COLUMN tokens.subscription_preferred IS '是否遵循订阅优先扣费：true=优先订阅，false=跳过订阅直接余额';
        RAISE NOTICE 'tokens.subscription_preferred 字段已添加';
    ELSE
        ALTER TABLE tokens ALTER COLUMN subscription_preferred SET DEFAULT FALSE;
        COMMENT ON COLUMN tokens.subscription_preferred IS '是否遵循订阅优先扣费：true=优先订阅，false=跳过订阅直接余额';
        RAISE NOTICE 'tokens.subscription_preferred 字段已存在，默认值已设置';
    END IF;
END$$;

-- 确保 auto_smart_group 字段存在（自动智能分组）
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'tokens' AND column_name = 'auto_smart_group'
    ) THEN
        ALTER TABLE tokens ADD COLUMN auto_smart_group BOOLEAN DEFAULT FALSE;
        COMMENT ON COLUMN tokens.auto_smart_group IS '是否自动选择分组：true=启用自动智能分组';
        -- 若此前执行过字段重命名，subscription_preferred 可能承载了 auto_smart_group 的旧值
        UPDATE tokens SET auto_smart_group = subscription_preferred;
        RAISE NOTICE 'tokens.auto_smart_group 字段已添加';
    ELSE
        ALTER TABLE tokens ALTER COLUMN auto_smart_group SET DEFAULT FALSE;
        COMMENT ON COLUMN tokens.auto_smart_group IS '是否自动选择分组：true=启用自动智能分组';
        RAISE NOTICE 'tokens.auto_smart_group 字段已存在，默认值已设置';
    END IF;
END$$;

-- -----------------------------------------------------------------------------
-- 3.2 扩展 redemptions 表 - 新增 type 和 payload 字段
-- -----------------------------------------------------------------------------

-- 添加 type 字段（如果不存在）
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'redemptions' AND column_name = 'type'
    ) THEN
        ALTER TABLE redemptions ADD COLUMN type VARCHAR(32) DEFAULT 'quota';
        RAISE NOTICE 'redemptions.type 字段已添加';
    ELSE
        RAISE NOTICE 'redemptions.type 字段已存在，跳过添加';
    END IF;
END$$;

-- 添加 payload 字段（如果不存在）
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'redemptions' AND column_name = 'payload'
    ) THEN
        ALTER TABLE redemptions ADD COLUMN payload TEXT;
        RAISE NOTICE 'redemptions.payload 字段已添加';
    ELSE
        RAISE NOTICE 'redemptions.payload 字段已存在，跳过添加';
    END IF;
END$$;

-- 为存量数据设置默认 type
UPDATE redemptions SET type = 'quota' WHERE type IS NULL;

-- 验证
DO $$
DECLARE
    null_count INT;
BEGIN
    SELECT COUNT(*) INTO null_count FROM redemptions WHERE type IS NULL;
    IF null_count = 0 THEN
        RAISE NOTICE '✓ redemptions.type 验证通过：无 NULL 值';
    ELSE
        RAISE WARNING '✗ redemptions.type 验证失败：存在 % 条 NULL 记录', null_count;
    END IF;
END$$;

-- -----------------------------------------------------------------------------
-- 3.3 扩展 options 表 - 插入订阅���统配置项
-- -----------------------------------------------------------------------------

-- 插入配置项（如果不存在）
INSERT INTO options (key, value)
VALUES
    ('SUBSCRIPTION_AUTO_WALLET_DEFAULT', 'false'),
    ('SUBSCRIPTION_EXPIRY_NOTICE_DAYS', '7'),
    ('SUBSCRIPTION_QUOTA_LOW_THRESHOLD', '0.2'),
    ('SUBSCRIPTION_V2_ENABLED', 'false'),
    ('SUBSCRIPTION_MAX_PER_USER', '10')
ON CONFLICT (key) DO NOTHING;

-- 验证
SELECT key, value FROM options
WHERE key LIKE 'SUBSCRIPTION_%'
ORDER BY key;

-- =============================================================================
-- 完成提示
-- =============================================================================
-- Phase 3 扩展现有表完成
--
-- 接下来将自动执行：
-- - Phase 4: 初始化 users.setting 数据
--
-- 注意事项：
-- - 迁移完成后需清理 Redis 中的 token 缓存（字段扩展）: redis-cli FLUSHDB
-- - 重启服务使新字段名生效: systemctl restart new-api
-- =============================================================================
-- =============================================================================
-- 订阅系统数据迁移: users.setting 字段初始化 - PostgreSQL
-- =============================================================================
-- 数据库: PostgreSQL
-- 创建日期: 2025-12-04
-- 版本: v1.0
-- ���明: 使用 JSON 合并策略初始化 users.setting，避免覆盖已存在的设置
-- =============================================================================

-- -----------------------------------------------------------------------------
-- Step 1: 迁移前检查
-- -----------------------------------------------------------------------------

-- 检查现有 setting 内容和数量
SELECT
    COUNT(*) as total_users,
    COUNT(CASE WHEN setting IS NULL OR setting = '' THEN 1 END) as empty_setting,
    COUNT(CASE WHEN setting IS NOT NULL AND setting <> '' THEN 1 END) as has_setting
FROM users;

-- 查看已有设置的样例（前10条）
SELECT id, username, setting
FROM users
WHERE setting IS NOT NULL AND setting <> ''
LIMIT 10;

-- 检查是否有非 JSON 格式数据
SELECT COUNT(*) as invalid_json_count FROM users
WHERE setting IS NOT NULL
  AND setting <> ''
  AND NOT (setting::jsonb IS NOT NULL);
-- 期望��果: 0（所有非空 setting 都是有效 JSON）

-- -----------------------------------------------------------------------------
-- Step 2: 执行迁移（方案 1: PostgreSQL JSON 合并）
-- -----------------------------------------------------------------------------

-- ⚠️ 重要：此方案保留现有键，只添加缺失的新键
-- 执行前请确认已完成 Step 1 检查

-- 先将 NULL 或 空字符串初始化为空 JSON，避免 ::jsonb 转换失败
UPDATE users
SET setting = '{}'::text
WHERE setting IS NULL OR setting = '';

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

-- 输出影响行数
-- PostgreSQL 会自动显示 "UPDATE <count>"

-- -----------------------------------------------------------------------------
-- Step 3: 迁移后验证
-- -----------------------------------------------------------------------------

-- 验证无空值
SELECT COUNT(*) as empty_setting_count FROM users
WHERE setting IS NULL OR setting = '';
-- 期望���果: 0

-- 验证所有用户都有 auto_wallet_fallback 键
SELECT COUNT(*) as has_new_keys_count FROM users
WHERE setting::jsonb ? 'auto_wallet_fallback';
-- 期望结果: 等于总用户数

-- 验证 JSON 格式正确性
SELECT COUNT(*) as invalid_json_count FROM users
WHERE setting IS NOT NULL
  AND setting <> ''
  AND NOT (setting::jsonb IS NOT NULL);
-- 期望结果: 0 行

-- 查看迁移后的样例数据（前10条）
SELECT
    id,
    username,
    setting::jsonb ->> 'auto_wallet_fallback' as auto_wallet_fallback,
    setting::jsonb -> 'notification_preferences' as notification_preferences,
    setting::jsonb ->> 'language' as language,
    setting::jsonb ->> 'timezone' as timezone,
    -- 查看是否保留了旧键
    setting::jsonb ->> 'notify_type' as old_notify_type,
    setting::jsonb ->> 'quota_warning_threshold' as old_quota_warning
FROM users
LIMIT 10;

-- -----------------------------------------------------------------------------
-- Step 4: 回滚脚本（仅在迁移失败时使用）
-- -----------------------------------------------------------------------------

-- ⚠️ 警告：此脚本会移除新增的键，仅在迁移失败时使用
-- 取消注释以下代码执行回滚：

-- UPDATE users
-- SET setting = (
--     SELECT jsonb_object_agg(key, value)
--     FROM jsonb_each(setting::jsonb)
--     WHERE key NOT IN ('auto_wallet_fallback', 'notification_preferences', 'language', 'timezone')
-- )::text
-- WHERE setting::jsonb ?| ARRAY['auto_wallet_fallback', 'notification_preferences', 'language', 'timezone'];

-- =============================================================================
-- 完成提示
-- =============================================================================
-- users.setting 迁移完成
--
-- 迁移结果:
-- - ✓ 所有用户的 setting 字段已初始化订阅相关默认值
-- - ✓ 现有键保留，未被覆盖
-- - ✓ JSON 格式验证通过
--
-- 下一步：
-- 1. 更新 dto/user_settings.go，添加新字段定义
-- 2. 测试 GetSetting() 和 SetSetting() 方法
-- =============================================================================
