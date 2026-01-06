-- =============================================================================
-- 优惠券与兑换码改造增量迁移 - MySQL 版本
-- =============================================================================
-- 数据库: MySQL 5.7+
-- 创建日期: 2025-12-09
-- 版本: v1.0
-- 说明: 基于 coupon_redemption_redesign.md 的数据模型变更
-- 前置条件: subscription_migration_mysql.sql 已执行
-- =============================================================================

-- =============================================================================
-- Phase 1: 创建新表
-- =============================================================================

-- -----------------------------------------------------------------------------
-- 1.1 user_coupons [新建表] - 用户券实例表
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS user_coupons (
    id BIGINT PRIMARY KEY NOT NULL AUTO_INCREMENT,
    coupon_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    code VARCHAR(64) NOT NULL COMMENT '用户专属核销码/显示用 (UC-UUID)',
    status VARCHAR(32) NOT NULL DEFAULT 'available' COMMENT '状态: available/locked/used/expired/invalid',
    claimed_at BIGINT NOT NULL,
    used_at BIGINT,
    order_id BIGINT COMMENT '核销关联订单',
    discount_amount BIGINT DEFAULT 0 COMMENT '优惠金额（分）',
    final_amount BIGINT DEFAULT 0 COMMENT '实付金额（分）',
    created_at BIGINT,
    updated_at BIGINT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户优惠券实例表';

-- 唯一索引：防止用户重复领取同一优惠券
CREATE UNIQUE INDEX idx_uc_coupon_user ON user_coupons(coupon_id, user_id);

-- 常规索引
CREATE INDEX idx_uc_user_status ON user_coupons(user_id, status);
CREATE INDEX idx_uc_coupon ON user_coupons(coupon_id);
CREATE INDEX idx_uc_code ON user_coupons(code);
CREATE INDEX idx_uc_order ON user_coupons(order_id);

-- -----------------------------------------------------------------------------
-- 1.2 coupon_redemption_bindings [新建表] - 兑换码-优惠券绑定关系表
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS coupon_redemption_bindings (
    id BIGINT PRIMARY KEY NOT NULL AUTO_INCREMENT,
    coupon_id BIGINT NOT NULL,
    redemption_id BIGINT NOT NULL,
    user_id BIGINT COMMENT '专属用户ID（NULL表示不限用户）',
    status VARCHAR(32) NOT NULL DEFAULT 'reserved' COMMENT '状态: reserved=已绑定未锁定, locked=已锁定/已兑换',
    locked_at BIGINT,
    created_at BIGINT,
    updated_at BIGINT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='兑换码-优惠券绑定关系表';

-- 唯一索引：一个兑换码只能绑定一张优惠券
CREATE UNIQUE INDEX uq_binding_redemption ON coupon_redemption_bindings(redemption_id);

-- 常规索引
CREATE INDEX idx_binding_coupon ON coupon_redemption_bindings(coupon_id);
CREATE INDEX idx_binding_user ON coupon_redemption_bindings(user_id);
CREATE INDEX idx_binding_status ON coupon_redemption_bindings(status);

-- =============================================================================
-- Phase 2: 扩展现有表
-- =============================================================================

-- -----------------------------------------------------------------------------
-- 2.1 扩展 coupons 表 - 新增字段
-- -----------------------------------------------------------------------------

-- 添加 type 字段（优惠券类型）
SET @col_exists = (
    SELECT COUNT(*)
    FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'coupons'
      AND COLUMN_NAME = 'type'
);

SET @sql = IF(@col_exists = 0,
    'ALTER TABLE coupons ADD COLUMN type VARCHAR(32) NOT NULL DEFAULT ''discount'' COMMENT ''优惠券类型: discount=折扣券, full_reduction=满减券, instant_reduction=立减券''',
    'SELECT ''coupons.type 已存在，跳过'' AS message'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 修改 scope 字段默认值（从 'quota' 改为 'wallet_subscription'）
ALTER TABLE coupons ALTER COLUMN scope SET DEFAULT 'wallet_subscription';

-- 添加 threshold_amount 字段（满减阈值）
SET @col_exists = (
    SELECT COUNT(*)
    FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'coupons'
      AND COLUMN_NAME = 'threshold_amount'
);

SET @sql = IF(@col_exists = 0,
    'ALTER TABLE coupons ADD COLUMN threshold_amount BIGINT NOT NULL DEFAULT 0 COMMENT ''满减阈值（分）''',
    'SELECT ''coupons.threshold_amount 已存在，跳过'' AS message'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 添加 currency 字段（币种）
SET @col_exists = (
    SELECT COUNT(*)
    FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'coupons'
      AND COLUMN_NAME = 'currency'
);

SET @sql = IF(@col_exists = 0,
    'ALTER TABLE coupons ADD COLUMN currency VARCHAR(8) NOT NULL DEFAULT ''CNY'' COMMENT ''币种（当前固定 CNY）''',
    'SELECT ''coupons.currency 已存在，跳过'' AS message'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 添加 version 字段（乐观锁版本号）
SET @col_exists = (
    SELECT COUNT(*)
    FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'coupons'
      AND COLUMN_NAME = 'version'
);

SET @sql = IF(@col_exists = 0,
    'ALTER TABLE coupons ADD COLUMN version BIGINT NOT NULL DEFAULT 0 COMMENT ''乐观锁版本号（用于并发控制）''',
    'SELECT ''coupons.version 已存在，跳过'' AS message'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

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
SET @col_exists = (
    SELECT COUNT(*)
    FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'user_bills'
      AND COLUMN_NAME = 'coupon_id'
);

SET @sql = IF(@col_exists = 0,
    'ALTER TABLE user_bills ADD COLUMN coupon_id BIGINT COMMENT ''使用的优惠券模板ID''',
    'SELECT ''user_bills.coupon_id 已存在，跳过'' AS message'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 创建索引
SET @index_exists = (
    SELECT COUNT(*)
    FROM information_schema.STATISTICS
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'user_bills'
      AND INDEX_NAME = 'idx_bill_coupon'
);

SET @sql = IF(@index_exists = 0,
    'CREATE INDEX idx_bill_coupon ON user_bills(coupon_id)',
    'SELECT ''idx_bill_coupon 已存在，跳过'' AS message'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 添加 user_coupon_id 字段
SET @col_exists = (
    SELECT COUNT(*)
    FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'user_bills'
      AND COLUMN_NAME = 'user_coupon_id'
);

SET @sql = IF(@col_exists = 0,
    'ALTER TABLE user_bills ADD COLUMN user_coupon_id BIGINT COMMENT ''使用的用户优惠券ID''',
    'SELECT ''user_bills.user_coupon_id 已存在，跳过'' AS message'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 创建索引
SET @index_exists = (
    SELECT COUNT(*)
    FROM information_schema.STATISTICS
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'user_bills'
      AND INDEX_NAME = 'idx_bill_user_coupon'
);

SET @sql = IF(@index_exists = 0,
    'CREATE INDEX idx_bill_user_coupon ON user_bills(user_coupon_id)',
    'SELECT ''idx_bill_user_coupon 已存在，跳过'' AS message'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 添加 discount_amount 字段
SET @col_exists = (
    SELECT COUNT(*)
    FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'user_bills'
      AND COLUMN_NAME = 'discount_amount'
);

SET @sql = IF(@col_exists = 0,
    'ALTER TABLE user_bills ADD COLUMN discount_amount BIGINT DEFAULT 0 COMMENT ''优惠金额（分）''',
    'SELECT ''user_bills.discount_amount 已存在，跳过'' AS message'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 添加 original_amount 字段
SET @col_exists = (
    SELECT COUNT(*)
    FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'user_bills'
      AND COLUMN_NAME = 'original_amount'
);

SET @sql = IF(@col_exists = 0,
    'ALTER TABLE user_bills ADD COLUMN original_amount BIGINT COMMENT ''原价（分）''',
    'SELECT ''user_bills.original_amount 已存在，跳过'' AS message'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 添加 final_amount 字段
SET @col_exists = (
    SELECT COUNT(*)
    FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'user_bills'
      AND COLUMN_NAME = 'final_amount'
);

SET @sql = IF(@col_exists = 0,
    'ALTER TABLE user_bills ADD COLUMN final_amount BIGINT COMMENT ''实付金额（分）''',
    'SELECT ''user_bills.final_amount 已存在，跳过'' AS message'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- -----------------------------------------------------------------------------
-- 2.3 扩展 redemptions 表 - 新增 bound_user_id 字段
-- -----------------------------------------------------------------------------

SET @col_exists = (
    SELECT COUNT(*)
    FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'redemptions'
      AND COLUMN_NAME = 'bound_user_id'
);

SET @sql = IF(@col_exists = 0,
    'ALTER TABLE redemptions ADD COLUMN bound_user_id BIGINT COMMENT ''专属用户ID（NULL表示不限用户）''',
    'SELECT ''redemptions.bound_user_id 已存在，跳过'' AS message'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 创建索引
SET @index_exists = (
    SELECT COUNT(*)
    FROM information_schema.STATISTICS
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'redemptions'
      AND INDEX_NAME = 'idx_redemptions_bound_user'
);

SET @sql = IF(@index_exists = 0,
    'CREATE INDEX idx_redemptions_bound_user ON redemptions(bound_user_id)',
    'SELECT ''idx_redemptions_bound_user 已存在，跳过'' AS message'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

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
    COALESCE(c.created_at, UNIX_TIMESTAMP()) AS created_at,
    COALESCE(c.updated_at, UNIX_TIMESTAMP()) AS updated_at
FROM coupons c
WHERE c.bind_redemption_id IS NOT NULL
ON DUPLICATE KEY UPDATE id=id;                      -- 防止重复迁移

-- 验证迁移结果
SELECT
    CONCAT('旧数据绑定数量: ',
        (SELECT COUNT(*) FROM coupons WHERE bind_redemption_id IS NOT NULL),
        ', 新表数据数量: ',
        (SELECT COUNT(*) FROM coupon_redemption_bindings)
    ) AS migration_result;

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

-- SELECT '旧字段数据已清空（bind_redemption_id, bind_locked_at）' AS message;

-- -----------------------------------------------------------------------------
-- 4.2 删除旧字段（谨慎操作，建议保留一段时间后再删除）
-- -----------------------------------------------------------------------------

-- ⚠️ 极度危险：以下操作会删除旧字段，仅在完全确认不再需要后执行
-- 取消注释以下代码以执行删除：

-- ALTER TABLE coupons DROP COLUMN IF EXISTS bind_redemption_id;
-- ALTER TABLE coupons DROP COLUMN IF EXISTS bind_locked_at;
-- ALTER TABLE coupons DROP COLUMN IF EXISTS bind_user_id;

-- SELECT '旧字段已删除' AS message;

-- =============================================================================
-- 完成提示
-- =============================================================================

SELECT '=============================================================================' AS message
UNION ALL SELECT '优惠券与兑换码改造增量迁移完成（MySQL）'
UNION ALL SELECT '============================================================================='
UNION ALL SELECT ''
UNION ALL SELECT '已完成：'
UNION ALL SELECT '  ✓ 创建 user_coupons 表'
UNION ALL SELECT '  ✓ 创建 coupon_redemption_bindings 表'
UNION ALL SELECT '  ✓ 扩展 coupons 表（type, scope, threshold_amount, currency, version）'
UNION ALL SELECT '  ✓ 扩展 user_bills 表（coupon_id, user_coupon_id, discount_amount, original_amount, final_amount）'
UNION ALL SELECT '  ✓ 扩展 redemptions 表（bound_user_id）'
UNION ALL SELECT '  ✓ 迁移旧数据到新表'
UNION ALL SELECT ''
UNION ALL SELECT '下一步：'
UNION ALL SELECT '  1. 执行验证脚本: coupon_redesign_verification_mysql.sql'
UNION ALL SELECT '  2. 更新后端代码（model/controller/service）'
UNION ALL SELECT '  3. 重启服务: systemctl restart new-api'
UNION ALL SELECT ''
UNION ALL SELECT '注意事项：'
UNION ALL SELECT '  - 旧字段（bind_redemption_id, bind_locked_at）已保留，建议观察一段时间后再删除'
UNION ALL SELECT '  - 如需回滚，请保留数据库备份'
UNION ALL SELECT '=============================================================================';
