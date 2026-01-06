# 优惠券改造方案文档更新总结

**更新日期**: 2025-12-09
**基于文档**: `coupon_redemption_redesign.md`
**影响文档**: `requirements.md`, `design.md`, `tasks.md`

---

## ✅ 已完成的更新

### 1. requirements.md 更新（已完成）

#### 1.1 FR-2 优惠券改造（已扩展）
- ✅ 补充优惠券类型（discount/full_reduction/instant_reduction）
- ✅ 补充适用范围（wallet/subscription/wallet_subscription）
- ✅ 补充用户券状态管理（available/locked/used/expired/invalid）
- ✅ 补充领取与使用规则（单笔单券、领取限制、乐观锁）
- ✅ 补充兑换码绑定优惠券功能
- ✅ 补充退款规则（优惠券不恢复）
- ✅ 补充并发安全机制（乐观锁、原子操作、幂等性）
- ✅ 补充审计与流水要求

#### 1.2 FR-5 兑换码支持（已扩展）
- ✅ 补充兑换码绑定优惠券功能详细说明
- ✅ 补充专属用户校验机制
- ✅ 补充绑定状态管理（reserved/locked）
- ✅ 补充完整的兑换流程（8步）

#### 1.3 FR-8 取消/退款与审计（已扩展）
- ✅ 补充退款处理详细规则
- ✅ 补充退款场景分类
- ✅ 补充退款元数据记录要求

---

## 📋 待完成的更新

### 2. design.md 需要补充的内容

#### 2.1 数据模型部分（第2节）

**需要在 2.1 数据库设计中补充**：

```markdown
#### 优惠券改造相关表

6. `user_coupons`（用户券实例）
   - `id` BIGINT PK
   - `coupon_id` BIGINT NOT NULL
   - `user_id` BIGINT NOT NULL
   - `code` VARCHAR(64) NOT NULL（用户专属核销码，格式：UC-UUID）
   - `status` VARCHAR(32) NOT NULL DEFAULT 'available'
   - `claimed_at` BIGINT NOT NULL
   - `used_at` BIGINT
   - `order_id` BIGINT（核销关联订单）
   - `discount_amount` BIGINT DEFAULT 0（优惠金额，分）
   - `final_amount` BIGINT（最终支付金额，分）
   - `created_at`, `updated_at` BIGINT
   - UNIQUE KEY (`coupon_id`, `user_id`, `code`)
   - INDEX `idx_uc_user_status` (`user_id`, `status`)
   - INDEX `idx_uc_coupon` (`coupon_id`)

7. `coupon_redemption_bindings`（兑换码绑定关系）
   - `id` BIGINT PK
   - `coupon_id` BIGINT NOT NULL
   - `redemption_id` BIGINT NOT NULL
   - `user_id` BIGINT（专属用户，可为NULL）
   - `status` VARCHAR(32) NOT NULL DEFAULT 'reserved'
   - `locked_at` BIGINT
   - `created_at`, `updated_at` BIGINT
   - UNIQUE KEY `uq_binding_redemption` (`redemption_id`)
   - INDEX `idx_binding_coupon` (`coupon_id`)
   - INDEX `idx_binding_user` (`user_id`)

8. `coupons` 表扩展字段
   - `type` VARCHAR(32) NOT NULL DEFAULT 'discount'
   - `scope` VARCHAR(32) NOT NULL DEFAULT 'wallet_subscription'
   - `threshold_amount` BIGINT NOT NULL DEFAULT 0（满减阈值，分）
   - `currency` VARCHAR(8) NOT NULL DEFAULT 'CNY'
   - `version` BIGINT NOT NULL DEFAULT 0（乐观锁版本号）

9. `user_bills` 表扩展字段
   - `coupon_id` BIGINT（使用的优惠券模板ID）
   - `user_coupon_id` BIGINT（使用的用户优惠券ID）
   - `discount_amount` BIGINT DEFAULT 0（优惠金额，分）
   - `original_amount` BIGINT（原价，分）
   - `final_amount` BIGINT（实付，分）
   - `refund_type` VARCHAR(32)（退款类型：manual/auto/none）
   - INDEX `idx_bill_coupon` (`coupon_id`)
   - INDEX `idx_bill_user_coupon` (`user_coupon_id`)

10. `redemptions` 表扩展字段
    - `bound_user_id` BIGINT（专属用户）
```

#### 2.2 核心流程部分（第3节）

**需要在 3.2 优惠券 & 绑定 之后补充**：

```markdown
### 3.2.1 领券流程（事务 + 乐观锁）

**流程步骤**：
1. 查询优惠券模板并加载 `version`（乐观锁）
2. 校验时间、库存、状态
3. 校验用户领取限制（同一用户同一优惠券只能领取一次）
4. 乐观锁更新库存：`WHERE id = ? AND version = ? AND used_count < total_count`
   - `used_count = used_count + 1`
   - `version = version + 1`
5. 插入用户优惠券实例（`user_coupons` 表，`status = 'available'`）
6. 写入审计日志

**并发安全**：
- 预期并发量级：1000 QPS
- 使用乐观锁防止超发
- `WHERE used_count < total_count` 双重保障

### 3.2.2 核销流程（支付成功内事务/幂等）

**流程步骤**：
1. 获取用户优惠券并加行锁（`FOR UPDATE`）
2. 幂等性检查：若已 `used` 且 `order_id` 相同，直接返回成功
3. 基础校验：时间、作用域、币种（当前固定 CNY）
4. 阈值与计算：满减阈值基于订单原价，计算优惠金额
   - 折扣券：`discount = orderAmount * discountValue / 100`
   - 满减券：若满足阈值，`discount = discountValue`
   - 立减券：`discount = min(discountValue, orderAmount)`
5. 原子更新用户优惠券状态为 `used`
6. 写入 `user_bills` 账单（包含完整优惠券信息）
7. 写入审计日志

**关键要点**：
- 阈值判断基于订单原价（未扣券前）
- 幂等性通过 `order_id` 判断
- 原子性使用条件更新 `WHERE status = 'available'`
- 退款时优惠券不恢复

### 3.2.3 兑换码绑定券流程

**创建绑定（管理端）**：
1. 校验优惠券存在且状态为 `active`
2. 创建兑换码记录
3. 创建 `coupon_redemption_bindings` 记录（`status = 'reserved'`）
4. 写入审计日志

**兑换使用（用户端）**：
1. 获取兑换码并加行锁
2. 获取绑定记录并加行锁
3. 校验专属用户（若 `user_id` 不为空）
4. 锁定绑定记录（`reserved` → `locked`）
5. 处理用户优惠券实例（创建或更新为 `locked`）
6. 执行兑换业务
7. 成功后将用户券状态改为 `used`
8. 标记兑换码已使用
9. 写入审计日志

**关键要点**：
- 专属用户校验确保兑换码安全
- 绑定状态机：`reserved` → `locked`
- 事务原子性：失败回滚，状态恢复
```

#### 2.3 API 设计部分（第4节）

**需要在表格中补充以下 API**：

| 方法 | Path | 描述 |
|------|------|------|
| POST | /api/admin/coupons | 创建优惠券 |
| PUT | /api/admin/coupons/:id | 更新优惠券 |
| GET | /api/admin/coupons | 优惠券列表 |
| POST | /api/admin/redemptions | 创建兑换码并绑定优惠券 |
| POST | /api/user/coupons/claim | 用户领取优惠券 |
| GET | /api/user/coupons | 我的优惠券列表 |
| POST | /api/user/payment/coupon/preview | 优惠券使用预览（可选）|
| POST | /api/user/redemptions/use | 兑换码使用（补充绑定逻辑）|

#### 2.4 错误码部分（第7节）

**需要在 7.1 业务错误码定义中补充**：

```markdown
#### 优惠券相关错误码（补充）

| 错误码 | HTTP状态码 | 说明 | 触发场景 |
|--------|-----------|------|----------|
| `COUPON_ALREADY_CLAIMED` | 409 | 用户已领取该优惠券 | 同一用户重复领取同一优惠券 |
| `COUPON_STOCK_INSUFFICIENT` | 429 | 优惠券库存不足 | 领取时库存已用完 |
| `COUPON_ALREADY_USED` | 403 | 优惠券已使用 | 重复使用已核销的优惠券 |
| `COUPON_STATUS_INVALID` | 400 | 优惠券状态无效 | 优惠券状态不是 available |
| `COUPON_SCOPE_MISMATCH` | 400 | 优惠券作用域不匹配 | 优惠券不适用于当前场景 |
| `COUPON_THRESHOLD_NOT_MET` | 400 | 未达满减阈值 | 订单金额未达满减门槛 |
| `COUPON_CURRENCY_MISMATCH` | 400 | 币种不匹配 | 优惠券币种与订单不一致 |
| `COUPON_TYPE_INVALID` | 400 | 优惠券类型无效 | 优惠券类型不合法 |
| `COUPON_TEMPLATE_NOT_FOUND` | 404 | 优惠券模板不存在 | 优惠券模板ID不存在 |
| `COUPON_CONCURRENT_CONFLICT` | 409 | 并发冲突 | 乐观锁版本冲突 |
| `COUPON_USER_MISMATCH` | 403 | 优惠券不属于该用户 | 用户使用他人的优惠券 |
| `COUPON_REDEMPTION_LOCKED` | 403 | 兑换码绑定已锁定 | 绑定已锁定无法使用 |
| `REDEMPTION_USER_MISMATCH` | 403 | 兑换码不属于该用户 | 非专属用户使用专属兑换码 |
```

---

### 3. tasks.md 需要补充的内容

#### 3.1 Model 层实现部分（2.2节）

**需要在 2.2.7 之后补充**：

```markdown
#### 2.2.8 实现 `model/user_coupon.go`（新增）
- [  ] UserCoupon 结构体定义
- [  ] 仓储方法：Create, GetByID, GetByUserAndCoupon, GetByUser, UpdateStatus
- [  ] 领券方法：ClaimCoupon（包含乐观锁）
- [  ] 核销方法：UseCoupon（包含幂等性检查）
- [  ] 状态转换方法：Lock, Use, Expire, Invalidate
- [  ] 单元测试：领券流程、核销流程、并发测试（1000并发）

#### 2.2.9 实现 `model/coupon_redemption_binding.go`（新增）
- [  ] CouponRedemptionBinding 结构体定义
- [  ] 仓储方法：Create, GetByRedemption, GetByCoupon, UpdateStatus
- [  ] 绑定方法：BindCouponToRedemption
- [  ] 锁定方法：LockBinding（reserved → locked）
- [  ] 单元测试：绑定流程、状态转换

#### 2.2.10 扩展 `model/coupon.go`（补充）
- [  ] 新增字段：type, scope, threshold_amount, currency, version
- [  ] 版本管理方法：IncrementVersion（乐观锁）
- [  ] 验证方法：ValidateType, ValidateScope, ValidateCurrency
- [  ] 单元测试：乐观锁机制、字段验证

#### 2.2.11 扩展 `model/user_bill.go`（补充）
- [  ] 新增字段：coupon_id, user_coupon_id, discount_amount, original_amount, final_amount, refund_type
- [  ] 单元测试：优惠券相关字段

#### 2.2.12 扩展 `model/redemption.go`（补充）
- [  ] 新增字段：bound_user_id
- [  ] 专属用户校验方法：ValidateBoundUser
- [  ] 单元测试：专属用户校验
```

#### 3.2 Service 层实现部分（2.6节）

**需要在 2.6 Service 层 - 优惠券服务扩展 中补充**：

```markdown
#### 2.6 Service 层 - 优惠券服务扩展（详细任务）

- [  ] 2.6.1 实现 `service/coupon_service.go` - 优惠券领取服务
  - ClaimCoupon（领券流程，包含乐观锁和事务）
  - ValidateCouponClaimable（校验是否可领取）
  - CheckUserClaimLimit（校验用户领取限制）
  - 单元测试：领券成功、库存不足、重复领取

- [  ] 2.6.2 实现 `service/coupon_service.go` - 优惠券核销服务
  - UseCoupon（核销流程，包含幂等性检查）
  - CalculateDiscount（计算折扣金额：折扣/满减/立减）
  - ValidateCouponUsable（校验是否可核销）
  - CheckCouponThreshold（校验满减阈值）
  - 单元测试：核销成功、幂等性、阈值不足

- [  ] 2.6.3 实现 `service/coupon_service.go` - 兑换码绑定服务
  - BindCouponToRedemption（绑定优惠券到兑换码）
  - UnbindCoupon（解绑优惠券，仅限未兑换）
  - UseBoundCoupon（兑换时自动消费绑定的优惠券）
  - ValidateBoundUser（校验专属用户）
  - 单元测试：绑定/解绑、专属用户校验

- [  ] 2.6.4 实现 `service/coupon_service.go` - 优惠券列表服务
  - GetUserCoupons（获取用户优惠券列表，支持状态筛选）
  - GetCouponByCode（根据领取码获取优惠券）
  - PreviewCouponUsage（预览优惠券使用效果，可选）
  - 单元测试：列表查询、筛选

- [  ] 2.6.5 实现 `service/coupon_service.go` - 退款处理
  - ProcessRefundWithCoupon（处理带优惠券的订单退款）
  - 记录退款元数据（coupon_id, discount_amount, coupon_restored=false）
  - 单元测试：退款流程、优惠券不恢复
```

---

## 🔧 coupon_redemption_redesign.md 需要修正的问题

### 问题1：DDL 定义缺失字段
**位置**: 第二章 2.1 节 `user_coupons` 表定义（第42-57行）

**修正内容**：在 `user_coupons` 表定义中添加：
```sql
discount_amount BIGINT DEFAULT 0    -- 优惠金额（分）
final_amount BIGINT                 -- 最终支付金额（分）
```

### 问题2：阈值判断描述不一致
**位置**: 第一章第29行

**修正内容**：将
```
- **满减阈值基于"实际支付价格"判断**（约定：下单金额 price，即未扣券前的应付金额）。
```
修改为：
```
- **满减阈值基于"订单原价"判断**（约定：下单金额 price，即未扣券前的应付金额）。
```

### 问题3：章节编号重复
**位置**: 第1084行和第1092行

**修正内容**：
- 第1084行：`## 七、并发与性能`（保持不变）
- 第1092行：`## 八、代码改造点（现有仓库）`（改为"八"）
- 第1332行：`## 九、验收与测试要点`（改为"九"）
- 第1342行：`## 十、后续可选优化`（改为"十"）

---

## 📝 更新优先级建议

**高优先级**（核心功能，必须完成）：
1. ✅ requirements.md FR-2、FR-5、FR-8 更新（已完成）
2. ⏳ design.md 数据模型补充（user_coupons、coupon_redemption_bindings）
3. ⏳ design.md 核心流程补充（领券、核销、绑定）
4. ⏳ tasks.md Model 层任务补充（2.2.8-2.2.12）

**中优先级**（重要但可延后）：
5. ⏳ design.md API 设计补充
6. ⏳ tasks.md Service 层任务补充（2.6详细任务）

**低优先级**（文档修正）：
7. ⏳ coupon_redemption_redesign.md 问题修正

---

## ✅ 验收检查清单

### 文档一致性检查
- [  ] requirements.md 的 FR-2 是否与 coupon_redemption_redesign.md 一致
- [  ] design.md 的数据模型是否完整包含所有新表和字段
- [  ] design.md 的核心流程是否涵盖领券、核销、绑定三大流程
- [  ] tasks.md 是否包含所有 Model 层和 Service 层的实施任务
- [  ] 错误码是否完整定义（至少 13 个优惠券相关错误码）

### 技术实现检查
- [  ] 乐观锁机制是否明确定义（version 字段）
- [  ] 幂等性机制是否明确定义（order_id 判断）
- [  ] 并发安全是否明确定义（条件更新、行锁）
- [  ] 退款规则是否明确定义（优惠券不恢复）
- [  ] 审计日志是否明确定义（所有关键操作）

---

**文档维护**: 本文档将随着实施进度持续更新，所有修改都应记录在此文档中。
