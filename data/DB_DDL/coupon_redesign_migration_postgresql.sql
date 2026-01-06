-- =============================================================================
-- 优惠券与兑换码改造增量迁移 - PostgreSQL 版本
-- =============================================================================
-- 数据库: PostgreSQL
-- 创建日期: 2025-12-09
-- 版本: v1.0
-- 说明: 基于 coupon_redemption_redesign.md 的数据模型变更
-- 前置条件: subscription_migration_postgresql.sql 已执行
-- =============================================================================

-- =============================================================================
-- Phase 0: 创建序列
-- =============================================================================

CREATE SEQUENCE IF NOT EXISTS user_coupons_id_seq;
CREATE SEQUENCE IF NOT EXISTS coupon_redemption_bindings_id_seq;

-- =============================================================================
-- Phase 1: 创建新表
-- =============================================================================

-- -----------------------------------------------------------------------------
-- 1.1 user_coupons [新建表] - 用户券实例表
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS user_coupons (
    id BIGINT PRIMARY KEY NOT NULL DEFAULT nextval('user_coupons_id_seq'::regclass),
    coupon_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    code VARCHAR(64) NOT NULL,                    -- 用户专属核销码/显示用 (UC-UUID)
    status VARCHAR(32) NOT NULL DEFAULT 'available',
    claimed_at BIGINT NOT NULL,
    used_at BIGINT,
    order_id BIGINT,                              -- 核销关联订单
    discount_amount BIGINT DEFAULT 0,             -- 优惠金额（分）
    final_amount BIGINT DEFAULT 0,                -- 实付金额（分）
    created_at BIGINT,
    updated_at BIGINT
);

-- 唯一索引：防止用户重复领取同一优惠券
CREATE UNIQUE INDEX IF NOT EXISTS idx_uc_coupon_user ON user_coupons(coupon_id, user_id);

-- 常规索引
CREATE INDEX IF NOT EXISTS idx_uc_user_status ON user_coupons(user_id, status);
CREATE INDEX IF NOT EXISTS idx_uc_coupon ON user_coupons(coupon_id);
CREATE INDEX IF NOT EXISTS idx_uc_code ON user_coupons(code);
CREATE INDEX IF NOT EXISTS idx_uc_order ON user_coupons(order_id);

-- 注释
COMMENT ON TABLE user_coupons IS '用户优惠券实例表';
COMMENT ON COLUMN user_coupons.code IS '用户专属核销码 (UC-UUID格式)';
COMMENT ON COLUMN user_coupons.status IS '状态: available/locked/used/expired/invalid';
COMMENT ON COLUMN user_coupons.discount_amount IS '实际优惠金额（分）';
COMMENT ON COLUMN user_coupons.final_amount IS '实付金额（分）';

-- -----------------------------------------------------------------------------
-- 1.2 coupon_redemption_bindings [新建表] - 兑换码-优惠券绑定关系表
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS coupon_redemption_bindings (
    id BIGINT PRIMARY KEY NOT NULL DEFAULT nextval('coupon_redemption_bindings_id_seq'::regclass),
    coupon_id BIGINT NOT NULL,
    redemption_id BIGINT NOT NULL,
    user_id BIGINT,                               -- 专属用户（NULL表示不限用户）
    status VARCHAR(32) NOT NULL DEFAULT 'reserved',
    locked_at BIGINT,
    created_at BIGINT,
    updated_at BIGINT
);

-- 唯一索引：一个兑换码只能绑定一张优惠券
CREATE UNIQUE INDEX IF NOT EXISTS uq_binding_redemption ON coupon_redemption_bindings(redemption_id);

-- 常规索引
CREATE INDEX IF NOT EXISTS idx_binding_coupon ON coupon_redemption_bindings(coupon_id);
CREATE INDEX IF NOT EXISTS idx_binding_user ON coupon_redemption_bindings(user_id);
CREATE INDEX IF NOT EXISTS idx_binding_status ON coupon_redemption_bindings(status);

-- 注释
COMMENT ON TABLE coupon_redemption_bindings IS '兑换码-优惠券绑定关系表';
COMMENT ON COLUMN coupon_redemption_bindings.user_id IS '专属用户ID（NULL表示不限用户）';
COMMENT ON COLUMN coupon_redemption_bindings.status IS '状态: reserved=已绑定未锁定, locked=已锁定/已兑换';

-- =============================================================================
-- Phase 2: 扩展现有表
-- =============================================================================

-- -----------------------------------------------------------------------------
-- 2.1 扩展 coupons 表 - 新增字段
-- -----------------------------------------------------------------------------

-- 添加 type 字段（优惠券类型）
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'coupons' AND column_name = 'type'
    ) THEN
        ALTER TABLE coupons ADD COLUMN type VARCHAR(32) NOT NULL DEFAULT 'discount';
        COMMENT ON COLUMN coupons.type IS '优惠券类型: discount=折扣券, full_reduction=满减券, instant_reduction=立减券';
        RAISE NOTICE 'coupons.type 已添加';
    ELSE
        RAISE NOTICE 'coupons.type 已存在，跳过';
    END IF;
END$$;

-- 修改 scope 字段默认值（从 'quota' 改为 'wallet_subscription'）
DO $$
BEGIN
    ALTER TABLE coupons ALTER COLUMN scope SET DEFAULT 'wallet_subscription';
    COMMENT ON COLUMN coupons.scope IS '作用域: wallet=余额充值, subscription=订阅, wallet_subscription=充值+订阅均可';
    RAISE NOTICE 'coupons.scope 默认值已更新为 wallet_subscription';
END$$;

-- 添加 threshold_amount 字段（满减阈值）
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'coupons' AND column_name = 'threshold_amount'
    ) THEN
        ALTER TABLE coupons ADD COLUMN threshold_amount BIGINT NOT NULL DEFAULT 0;
        COMMENT ON COLUMN coupons.threshold_amount IS '满减阈值（分）';
        RAISE NOTICE 'coupons.threshold_amount 已添加';
    ELSE
        RAISE NOTICE 'coupons.threshold_amount 已存在，跳过';
    END IF;
END$$;

-- 添加 currency 字段（币种）
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'coupons' AND column_name = 'currency'
    ) THEN
        ALTER TABLE coupons ADD COLUMN currency VARCHAR(8) NOT NULL DEFAULT 'CNY';
        COMMENT ON COLUMN coupons.currency IS '币种（当前固定 CNY）';
        RAISE NOTICE 'coupons.currency 已添加';
    ELSE
        RAISE NOTICE 'coupons.currency 已存在，跳过';
    END IF;
END$$;

-- 添加 version 字段（乐观锁版本号）
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'coupons' AND column_name = 'version'
    ) THEN
        ALTER TABLE coupons ADD COLUMN version BIGINT NOT NULL DEFAULT 0;
        COMMENT ON COLUMN coupons.version IS '乐观锁版本号（用于并发控制）';
        RAISE NOTICE 'coupons.version 已添加';
    ELSE
        RAISE NOTICE 'coupons.version 已存在，跳过';
    END IF;
END$$;

-- 为存量数据填充默认值
UPDATE coupons SET type = 'discount' WHERE type IS NULL;
UPDATE coupons SET scope = 'wallet_subscription' WHERE scope = 'quota';
UPDATE coupons SET threshold_amount = 0 WHERE threshold_amount IS NULL;
UPDATE coupons SET currency = 'CNY' WHERE currency IS NULL;
UPDATE coupons SET version = 0 WHERE version IS NULL;

-- -----------------------------------------------------------------------------
-- 2.2 扩展 user_bills 表 - 新增优惠券相关字段
-- -----------------------------------------------------------------------------

-- 添加 coupon_id 字段
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'user_bills' AND column_name = 'coupon_id'
    ) THEN
        ALTER TABLE user_bills ADD COLUMN coupon_id BIGINT;
        COMMENT ON COLUMN user_bills.coupon_id IS '使用的优惠券模板ID';
        CREATE INDEX IF NOT EXISTS idx_bill_coupon ON user_bills(coupon_id);
        RAISE NOTICE 'user_bills.coupon_id 已添加';
    ELSE
        RAISE NOTICE 'user_bills.coupon_id 已存在，跳过';
    END IF;
END$$;

-- 添加 user_coupon_id 字段
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'user_bills' AND column_name = 'user_coupon_id'
    ) THEN
        ALTER TABLE user_bills ADD COLUMN user_coupon_id BIGINT;
        COMMENT ON COLUMN user_bills.user_coupon_id IS '使用的用户优惠券ID';
        CREATE INDEX IF NOT EXISTS idx_bill_user_coupon ON user_bills(user_coupon_id);
        RAISE NOTICE 'user_bills.user_coupon_id 已添加';
    ELSE
        RAISE NOTICE 'user_bills.user_coupon_id 已存在，跳过';
    END IF;
END$$;

-- 添加 discount_amount 字段
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'user_bills' AND column_name = 'discount_amount'
    ) THEN
        ALTER TABLE user_bills ADD COLUMN discount_amount BIGINT DEFAULT 0;
        COMMENT ON COLUMN user_bills.discount_amount IS '优惠金额（分）';
        RAISE NOTICE 'user_bills.discount_amount 已添加';
    ELSE
        RAISE NOTICE 'user_bills.discount_amount 已存在，跳过';
    END IF;
END$$;

-- 添加 original_amount 字段
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'user_bills' AND column_name = 'original_amount'
    ) THEN
        ALTER TABLE user_bills ADD COLUMN original_amount BIGINT;
        COMMENT ON COLUMN user_bills.original_amount IS '原价（分）';
        RAISE NOTICE 'user_bills.original_amount 已添加';
    ELSE
        RAISE NOTICE 'user_bills.original_amount 已存在，跳过';
    END IF;
END$$;

-- 添加 final_amount 字段
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'user_bills' AND column_name = 'final_amount'
    ) THEN
        ALTER TABLE user_bills ADD COLUMN final_amount BIGINT;
        COMMENT ON COLUMN user_bills.final_amount IS '实付金额（分）';
        RAISE NOTICE 'user_bills.final_amount 已添加';
    ELSE
        RAISE NOTICE 'user_bills.final_amount 已存在，跳过';
    END IF;
END$$;

-- -----------------------------------------------------------------------------
-- 2.3 扩展 redemptions 表 - 新增 bound_user_id 字段
-- -----------------------------------------------------------------------------

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'redemptions' AND column_name = 'bound_user_id'
    ) THEN
        ALTER TABLE redemptions ADD COLUMN bound_user_id BIGINT;
        COMMENT ON COLUMN redemptions.bound_user_id IS '专属用户ID（NULL表示不限用户）';
        CREATE INDEX IF NOT EXISTS idx_redemptions_bound_user ON redemptions(bound_user_id);
        RAISE NOTICE 'redemptions.bound_user_id 已添加';
    ELSE
        RAISE NOTICE 'redemptions.bound_user_id 已存在，跳过';
    END IF;
END$$;

-- =============================================================================
-- Phase 3: 数据兼容迁移
-- =============================================================================

-- -----------------------------------------------------------------------------
-- 3.1 迁移 coupons.bind_redemption_id → coupon_redemption_bindings
-- -----------------------------------------------------------------------------

-- 迁移已绑定的兑换码关系
INSERT INTO coupon_redemption_bindings (
    coupon_id,
    redemption_id,
    user_id,
    status,
    locked_at,
    created_at,
    updated_at
)
SELECT
    c.id AS coupon_id,
    c.bind_redemption_id AS redemption_id,
    NULL AS user_id,                                -- 旧数据没有专属用户概念，设为NULL
    CASE
        WHEN c.bind_locked_at IS NOT NULL THEN 'locked'
        ELSE 'reserved'
    END AS status,
    c.bind_locked_at AS locked_at,
    COALESCE(c.created_at, EXTRACT(EPOCH FROM NOW())::BIGINT) AS created_at,
    COALESCE(c.updated_at, EXTRACT(EPOCH FROM NOW())::BIGINT) AS updated_at
FROM coupons c
WHERE c.bind_redemption_id IS NOT NULL
ON CONFLICT (redemption_id) DO NOTHING;            -- 防止重复迁移

-- 验证迁移结果
DO $$
DECLARE
    old_count INT;
    new_count INT;
BEGIN
    SELECT COUNT(*) INTO old_count FROM coupons WHERE bind_redemption_id IS NOT NULL;
    SELECT COUNT(*) INTO new_count FROM coupon_redemption_bindings;

    RAISE NOTICE '旧数据绑定数量: %, 新表数据数量: %', old_count, new_count;

    IF new_count >= old_count THEN
        RAISE NOTICE '✓ 数据迁移验证通过';
    ELSE
        RAISE WARNING '✗ 数据迁移验证失败: 新表数据数量少于旧数据';
    END IF;
END$$;

-- =============================================================================
-- Phase 4: 清理与优化（可选）
-- =============================================================================

-- -----------------------------------------------------------------------------
-- 4.1 清空旧字段数据（保留列以兼容旧代码）
-- -----------------------------------------------------------------------------

-- ⚠️ 警告：以下操作会清空旧字段数据，仅在确认迁移成功后执行
-- 取消注释以下代码以执行清理：

-- UPDATE coupons SET bind_redemption_id = NULL, bind_locked_at = NULL
-- WHERE bind_redemption_id IS NOT NULL;

-- RAISE NOTICE '旧字段数据已清空（bind_redemption_id, bind_locked_at）';

-- -----------------------------------------------------------------------------
-- 4.2 删除旧字段（谨慎操作，建议保留一段时间后再删除）
-- -----------------------------------------------------------------------------

-- ⚠️ 极度危险：以下操作会删除旧字段，仅在完全确认不再需要后执行
-- 取消注释以下代码以执行删除：

-- ALTER TABLE coupons DROP COLUMN IF EXISTS bind_redemption_id;
-- ALTER TABLE coupons DROP COLUMN IF EXISTS bind_locked_at;
-- ALTER TABLE coupons DROP COLUMN IF EXISTS bind_user_id;

-- RAISE NOTICE '旧字段已删除';

-- =============================================================================
-- 完成提示
-- =============================================================================

DO $$
BEGIN
    RAISE NOTICE '=============================================================================';
    RAISE NOTICE '优惠券与兑换码改造增量迁移完成（PostgreSQL）';
    RAISE NOTICE '=============================================================================';
    RAISE NOTICE '';
    RAISE NOTICE '已完成：';
    RAISE NOTICE '  ✓ 创建 user_coupons 表';
    RAISE NOTICE '  ✓ 创建 coupon_redemption_bindings 表';
    RAISE NOTICE '  ✓ 扩展 coupons 表（type, scope, threshold_amount, currency, version）';
    RAISE NOTICE '  ✓ 扩展 user_bills 表（coupon_id, user_coupon_id, discount_amount, original_amount, final_amount）';
    RAISE NOTICE '  ✓ 扩展 redemptions 表（bound_user_id）';
    RAISE NOTICE '  ✓ 迁移旧数据到新表';
    RAISE NOTICE '';
    RAISE NOTICE '下一步：';
    RAISE NOTICE '  1. 执行验证脚本: coupon_redesign_verification_postgresql.sql';
    RAISE NOTICE '  2. 更新后端代码（model/controller/service）';
    RAISE NOTICE '  3. 重启服务: systemctl restart new-api';
    RAISE NOTICE '';
    RAISE NOTICE '注意事项：';
    RAISE NOTICE '  - 旧字段（bind_redemption_id, bind_locked_at）已保留，建议观察一段时间后再删除';
    RAISE NOTICE '  - 如需回滚，请保留数据库备份';
    RAISE NOTICE '=============================================================================';
END$$;
