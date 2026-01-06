-- =============================================================================
-- 订阅系统数据库表结构 DDL - MySQL 版本
-- =============================================================================
-- 数据库: MySQL 5.7+
-- 创建日期: 2025-12-04
-- 版本: v1.1（修复自增ID问题）
-- 说明: 订阅套餐、优惠券、兑换码、账单、审计日志表结构
-- =============================================================================

-- =============================================================================
-- Phase 1: 新建核心业务表
-- =============================================================================

-- -----------------------------------------------------------------------------
-- 2.1 subscription_plans [新建表] - 订阅套餐表
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS subscription_plans (
    id BIGINT PRIMARY KEY NOT NULL AUTO_INCREMENT,
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
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 索引
CREATE INDEX idx_subscription_plans_status ON subscription_plans(status);
CREATE INDEX idx_subscription_plans_sku ON subscription_plans(sku);

-- -----------------------------------------------------------------------------
-- 2.2 subscription_plan_limits [新建表] - 套餐限额配置表
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS subscription_plan_limits (
    id BIGINT PRIMARY KEY NOT NULL AUTO_INCREMENT,
    plan_id BIGINT NOT NULL,
    period VARCHAR(32) NOT NULL,
    quota BIGINT NOT NULL,
    unit VARCHAR(32) DEFAULT 'quota',
    enabled BOOLEAN DEFAULT TRUE,
    window_strategy VARCHAR(32) DEFAULT 'rolling',
    created_at BIGINT,
    updated_at BIGINT,
    UNIQUE KEY uk_plan_period (plan_id, period)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 索引：关联 subscription_plans.id（应用层约束）
CREATE INDEX idx_subscription_plan_limits_plan_id ON subscription_plan_limits(plan_id);

-- -----------------------------------------------------------------------------
-- 2.3 coupons [新建表] - 优惠券表
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS coupons (
    id BIGINT PRIMARY KEY NOT NULL AUTO_INCREMENT,
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
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 索引
CREATE UNIQUE INDEX idx_coupons_code ON coupons(code);
CREATE INDEX idx_coupons_status ON coupons(status);
CREATE INDEX idx_coupons_bind_redemption ON coupons(bind_redemption_id);
CREATE INDEX idx_coupons_bind_user ON coupons(bind_user_id);
CREATE INDEX idx_coupons_valid_to ON coupons(valid_to);

-- -----------------------------------------------------------------------------
-- 2.4 subscription_orders [新建表] - 订阅订单表
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS subscription_orders (
    id BIGINT PRIMARY KEY NOT NULL AUTO_INCREMENT,
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
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 索引：关联 users.id, subscription_plans.id, user_bills.id（应用层约束）
CREATE INDEX idx_subscription_orders_user ON subscription_orders(user_id);
CREATE INDEX idx_subscription_orders_plan ON subscription_orders(plan_id);
CREATE INDEX idx_subscription_orders_bill ON subscription_orders(bill_id);
CREATE INDEX idx_subscription_orders_status ON subscription_orders(status);
CREATE INDEX idx_subscription_orders_coupon ON subscription_orders(coupon_id);

-- -----------------------------------------------------------------------------
-- 2.5 subscriptions [新建表] - 用户订阅表
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS subscriptions (
    id BIGINT PRIMARY KEY NOT NULL AUTO_INCREMENT,
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
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 索引：关联 users.id, subscription_plans.id, subscription_orders.id, redemptions.id, coupons.id（应用层约束）
CREATE INDEX idx_subscriptions_user_status ON subscriptions(user_id, status);
CREATE INDEX idx_subscriptions_user_priority ON subscriptions(user_id, priority);
CREATE INDEX idx_subscriptions_end_at_status ON subscriptions(end_at, status);
CREATE INDEX idx_subscriptions_plan_id ON subscriptions(plan_id);

-- -----------------------------------------------------------------------------
-- 2.6 subscription_usages [新建表] - 订阅使用量表（滚动窗口）
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS subscription_usages (
    id BIGINT PRIMARY KEY NOT NULL AUTO_INCREMENT,
    subscription_id BIGINT NOT NULL,
    period VARCHAR(32) NOT NULL,
    window_start BIGINT NOT NULL,
    window_end BIGINT NOT NULL,
    used_quota BIGINT NOT NULL DEFAULT 0,
    limit_quota BIGINT NOT NULL,
    created_at BIGINT,
    updated_at BIGINT,
    UNIQUE KEY uk_sub_period_window (subscription_id, period, window_start)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 索引：关联 subscriptions.id（应用层约束，删除时需级联清理）
CREATE INDEX idx_subscription_usage_window ON subscription_usages(subscription_id, period);
CREATE INDEX idx_subscription_usage_subscription ON subscription_usages(subscription_id);

-- =============================================================================
-- Phase 2: 新建辅助表
-- =============================================================================

-- -----------------------------------------------------------------------------
-- 2.7 user_bills [新建表] - 用户账单流水表
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS user_bills (
    id BIGINT PRIMARY KEY NOT NULL AUTO_INCREMENT,
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
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 索引：关联 users.id（应用层约束）
CREATE INDEX idx_user_bills_user_id_created_at ON user_bills(user_id, created_at);
CREATE INDEX idx_user_bills_source ON user_bills(source_type, source_id);
CREATE INDEX idx_user_bills_trade_no ON user_bills(trade_no);

-- 补充缺失字段（兼容已存在的表）
SET @col_exists = (
    SELECT COUNT(*)
    FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'user_bills'
      AND COLUMN_NAME = 'refund_type'
);

SET @sql = IF(@col_exists = 0,
    'ALTER TABLE user_bills ADD COLUMN refund_type VARCHAR(32) COMMENT ''手工退款等标记''',
    'SELECT ''user_bills.refund_type 已存在，跳过'' AS message'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists = (
    SELECT COUNT(*)
    FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'user_bills'
      AND COLUMN_NAME = 'conversion_metadata'
);

SET @sql = IF(@col_exists = 0,
    'ALTER TABLE user_bills ADD COLUMN conversion_metadata TEXT COMMENT ''兑换折算/不足一天金额等审计信息''',
    'SELECT ''user_bills.conversion_metadata 已存在，跳过'' AS message'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- -----------------------------------------------------------------------------
-- 2.8 audit_logs [新建表] - 审计日志表
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS audit_logs (
    id BIGINT PRIMARY KEY NOT NULL AUTO_INCREMENT,
    user_id BIGINT,
    operator_id BIGINT,
    object_type VARCHAR(64) NOT NULL,
    object_id BIGINT,
    action VARCHAR(64) NOT NULL,
    ip_address VARCHAR(64),
    user_agent TEXT,
    metadata TEXT,
    created_at BIGINT NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 索引：关联 users.id（应用层约束，用户删除时 user_id 设为 NULL）
CREATE INDEX idx_audit_logs_user_created_at ON audit_logs(user_id, created_at);
CREATE INDEX idx_audit_logs_object ON audit_logs(object_type, object_id);
CREATE INDEX idx_audit_logs_created_at ON audit_logs(created_at);

-- =============================================================================
-- Phase 3: 扩展现有表（ALTER TABLE）
-- =============================================================================

-- -----------------------------------------------------------------------------
-- 3.1 扩展 tokens 表 - 新增 subscription_preferred + 保留 auto_smart_group
-- -----------------------------------------------------------------------------

-- 确保 subscription_preferred 字段存在
SET @col_exists = (
    SELECT COUNT(*)
    FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'tokens'
      AND COLUMN_NAME = 'subscription_preferred'
);

SET @sql = IF(@col_exists = 0,
    'ALTER TABLE tokens ADD COLUMN subscription_preferred BOOLEAN DEFAULT FALSE COMMENT ''是否遵循订阅优先扣费：true=优先订阅，false=跳过订阅直接余额''',
    'ALTER TABLE tokens ALTER COLUMN subscription_preferred SET DEFAULT FALSE'
);

PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 确保 auto_smart_group 字段存在（自动智能分组）
SET @col_exists = (
    SELECT COUNT(*)
    FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'tokens'
      AND COLUMN_NAME = 'auto_smart_group'
);

SET @sql = IF(@col_exists = 0,
    'ALTER TABLE tokens ADD COLUMN auto_smart_group BOOLEAN DEFAULT FALSE COMMENT ''是否自动选择分组：true=启用自动智能分组''',
    'ALTER TABLE tokens ALTER COLUMN auto_smart_group SET DEFAULT FALSE'
);

PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 若此前执行过字段重命名，subscription_preferred 可能承载了 auto_smart_group 的旧值
SET @sql = IF(@col_exists = 0,
    'UPDATE tokens SET auto_smart_group = subscription_preferred',
    'SELECT ''tokens.auto_smart_group 已存在，跳过回填'' AS message'
);

PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- -----------------------------------------------------------------------------
-- 3.2 扩展 redemptions 表 - 新增 type 和 payload 字段
-- -----------------------------------------------------------------------------

-- 添加 type 字段（如果不存在）
SET @col_exists = (
    SELECT COUNT(*)
    FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'redemptions'
      AND COLUMN_NAME = 'type'
);

SET @sql = IF(@col_exists = 0,
    'ALTER TABLE redemptions ADD COLUMN type VARCHAR(32) DEFAULT ''quota'' COMMENT ''兑换类型：quota=额度充值, subscription=订阅兑换''',
    'SELECT ''redemptions.type 字段已存在，跳过添加'' AS message'
);

PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 添加 payload 字段（如果不存在）
SET @col_exists = (
    SELECT COUNT(*)
    FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'redemptions'
      AND COLUMN_NAME = 'payload'
);

SET @sql = IF(@col_exists = 0,
    'ALTER TABLE redemptions ADD COLUMN payload TEXT COMMENT ''兑换配置 JSON（用于订阅兑换时存储套餐快照）''',
    'SELECT ''redemptions.payload 字段已存在，跳过添加'' AS message'
);

PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 为存量数据设置默认 type
UPDATE redemptions SET type = 'quota' WHERE type IS NULL;

-- 验证
SELECT
    CASE
        WHEN COUNT(*) = 0 THEN '✓ redemptions.type 验证通过：无 NULL 值'
        ELSE CONCAT('✗ redemptions.type 验证失败：存在 ', COUNT(*), ' 条 NULL 记录')
    END AS verification_result
FROM redemptions
WHERE type IS NULL;

-- -----------------------------------------------------------------------------
-- 3.3 扩展 options 表 - 插入订阅系统配置项
-- -----------------------------------------------------------------------------

-- 插入配置项（如果不存在）
INSERT INTO options (`key`, value)
SELECT 'SUBSCRIPTION_AUTO_WALLET_DEFAULT', 'false'
WHERE NOT EXISTS (SELECT 1 FROM options WHERE `key` = 'SUBSCRIPTION_AUTO_WALLET_DEFAULT');

INSERT INTO options (`key`, value)
SELECT 'SUBSCRIPTION_EXPIRY_NOTICE_DAYS', '7'
WHERE NOT EXISTS (SELECT 1 FROM options WHERE `key` = 'SUBSCRIPTION_EXPIRY_NOTICE_DAYS');

INSERT INTO options (`key`, value)
SELECT 'SUBSCRIPTION_QUOTA_LOW_THRESHOLD', '0.2'
WHERE NOT EXISTS (SELECT 1 FROM options WHERE `key` = 'SUBSCRIPTION_QUOTA_LOW_THRESHOLD');

INSERT INTO options (`key`, value)
SELECT 'SUBSCRIPTION_V2_ENABLED', 'false'
WHERE NOT EXISTS (SELECT 1 FROM options WHERE `key` = 'SUBSCRIPTION_V2_ENABLED');

INSERT INTO options (`key`, value)
SELECT 'SUBSCRIPTION_MAX_PER_USER', '10'
WHERE NOT EXISTS (SELECT 1 FROM options WHERE `key` = 'SUBSCRIPTION_MAX_PER_USER');

-- 验证
SELECT `key`, value
FROM options
WHERE `key` LIKE 'SUBSCRIPTION_%'
ORDER BY `key`;

-- =============================================================================
-- Phase 4: 数据迁移 - users.setting 字段初始化
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

-- -----------------------------------------------------------------------------
-- Step 2: 执行迁移（MySQL JSON 合并）
-- -----------------------------------------------------------------------------

-- ⚠️ 重要：此方案保留现有键，只添加缺失的新键
-- MySQL 使用 JSON_MERGE_PATCH 实现类似 PostgreSQL 的 || 操作

-- 先将 NULL 或 空字符串初始化为空 JSON，避免 JSON_MERGE_PATCH 报错
UPDATE users
SET setting = '{}'
WHERE setting IS NULL OR setting = '';

UPDATE users
SET setting = JSON_MERGE_PATCH(
    COALESCE(
        CASE
            WHEN setting IS NULL OR setting = '' THEN '{}'
            ELSE setting
        END,
        '{}'
    ),
    JSON_OBJECT(
        'auto_wallet_fallback', false,
        'notification_preferences', JSON_OBJECT('email', true, 'in_app', true),
        'language', 'zh-CN',
        'timezone', 'Asia/Shanghai'
    )
)
WHERE setting IS NULL
   OR setting = ''
   OR NOT JSON_CONTAINS_PATH(setting, 'one', '$.auto_wallet_fallback');

-- 输出影响行数
SELECT ROW_COUNT() AS affected_rows;

-- -----------------------------------------------------------------------------
-- Step 3: 迁移后验证
-- -----------------------------------------------------------------------------

-- 验证无空值
SELECT COUNT(*) as empty_setting_count
FROM users
WHERE setting IS NULL OR setting = '';
-- 期望结果: 0

-- 验证所有用户都有 auto_wallet_fallback 键
SELECT COUNT(*) as has_new_keys_count
FROM users
WHERE JSON_CONTAINS_PATH(setting, 'one', '$.auto_wallet_fallback');
-- 期望结果: 等于总用户数

-- 查看迁移后的样例数据（前10条）
SELECT
    id,
    username,
    JSON_EXTRACT(setting, '$.auto_wallet_fallback') as auto_wallet_fallback,
    JSON_EXTRACT(setting, '$.notification_preferences') as notification_preferences,
    JSON_EXTRACT(setting, '$.language') as language,
    JSON_EXTRACT(setting, '$.timezone') as timezone,
    -- 查看是否保留了旧键
    JSON_EXTRACT(setting, '$.notify_type') as old_notify_type,
    JSON_EXTRACT(setting, '$.quota_warning_threshold') as old_quota_warning
FROM users
LIMIT 10;

-- =============================================================================
-- 完成提示
-- =============================================================================
-- 订阅系统数据库迁移完成（MySQL）
--
-- 已完成：
-- - Phase 0: 无需创建序列（MySQL 使用 AUTO_INCREMENT）
-- - Phase 1-2: 新建 8 张核心表和辅助表
-- - Phase 3: 扩展 tokens、redemptions、options 表
-- - Phase 4: 初始化 users.setting 字段
--
-- 下一步：
-- 1. 清理 Redis 中的 token 缓存（由于字段扩展）: redis-cli FLUSHDB
-- 2. 重启服务使新字段名生效: systemctl restart new-api
-- 3. 执行验证检查（参考 README.md）
-- =============================================================================
