# 优惠券与兑换码改造实施方案（可执行版）

目标：满足领取码发放、用户领券绑定、充值/订阅单笔单券核销、兑换码绑定专属用户 + 单券、并保持并发安全与审计完备。当前系统默认币种人民币（CNY），单笔仅可用一张券。

---

## 一、枚举与业务规则

- 优惠券类型 `coupon.type`
  - `discount` 折扣（百分比，1-100）
  - `full_reduction` 满减（需满足阈值）
  - `instant_reduction` 立减
- 优惠券适用范围 `coupon.scope`
  - `wallet` 余额充值
  - `subscription` 订阅
  - `wallet_subscription` 充值+订阅均可
- 用户券状态 `user_coupons.status`
  - `available` 可用
  - `locked` 已锁定（核销进行中/兑换绑定）
  - `used` 已核销
  - `expired` 已过期
  - `invalid` 作废
- 绑定表状态 `coupon_redemption_bindings.status`
  - `reserved` 已绑定未锁定
  - `locked` 已锁定（兑换时使用/已失效）

使用规则：
- 单笔仅可用一张券；币种必须与订单一致（当前 CNY）。
- **满减阈值基于"实际支付价格"判断**（约定：下单金额 price，即未扣券前的应付金额）。
- **领取限制**：同一用户同一优惠券只能领取一次；同一用户可领取多张不同优惠券。
- 领取：扣减库存 + 生成用户券实例，原子事务，乐观锁防止超发。
- 核销：校验范围/时间/阈值/状态/归属，锁定并标记 `used`。
- 兑换码：绑定专属 `user_id + coupon_id`，仅该用户可用，已锁定视为优惠失效。
- **退款规则**：优惠券不恢复，退款金额按使用优惠券后的实际支付金额计算。

---

## 二、DDL（PostgreSQL / MySQL）

### 2.1 新表

`user_coupons`（用户券实例）
```
id BIGINT PK AUTO_INCREMENT/nextval
coupon_id BIGINT NOT NULL
user_id BIGINT NOT NULL
code VARCHAR(64) NOT NULL           -- 用户专属核销码/显示用
status VARCHAR(32) NOT NULL DEFAULT 'available'
claimed_at BIGINT NOT NULL
used_at BIGINT
order_id BIGINT                      -- 核销关联订单
created_at BIGINT
updated_at BIGINT
UNIQUE KEY (coupon_id, user_id, code)
INDEX idx_uc_user_status (user_id, status)
INDEX idx_uc_coupon (coupon_id)
```

`coupon_redemption_bindings`
```
id BIGINT PK AUTO_INCREMENT/nextval
coupon_id BIGINT NOT NULL
redemption_id BIGINT NOT NULL
user_id BIGINT                       -- 专属用户
status VARCHAR(32) NOT NULL DEFAULT 'reserved'
locked_at BIGINT
created_at BIGINT
updated_at BIGINT
UNIQUE KEY uq_binding_redemption (redemption_id)
INDEX idx_binding_coupon (coupon_id)
INDEX idx_binding_user (user_id)
```

### 2.2 旧表扩展

`coupons`
```
ADD COLUMN type VARCHAR(32) NOT NULL DEFAULT 'discount';
ADD COLUMN scope VARCHAR(32) NOT NULL DEFAULT 'wallet_subscription';
ADD COLUMN threshold_amount BIGINT NOT NULL DEFAULT 0;     -- 分
ADD COLUMN currency VARCHAR(8) NOT NULL DEFAULT 'CNY';
ADD COLUMN version BIGINT NOT NULL DEFAULT 0;              -- 乐观锁版本号
-- 保留 code 为领取码，唯一索引
-- code 生成规则：前缀(COUPON) + 日期(YYYYMMDD) + 随机码(6位大写字母数字)
-- 示例：COUPON20250109AB12CD
```

`user_bills`（扩展已有表）
```
ADD COLUMN coupon_id BIGINT;                    -- 使用的优惠券模板ID
ADD COLUMN user_coupon_id BIGINT;               -- 使用的用户优惠券ID
ADD COLUMN discount_amount BIGINT DEFAULT 0;    -- 优惠金额（分）
ADD COLUMN original_amount BIGINT;              -- 原价（分）
ADD COLUMN final_amount BIGINT;                 -- 实付（分）
ADD COLUMN refund_type VARCHAR(32);             -- 退款类型 manual/auto/none
ADD INDEX idx_bill_coupon (coupon_id);
ADD INDEX idx_bill_user_coupon (user_coupon_id);
```

`redemptions`
```
ADD COLUMN bound_user_id BIGINT;      -- 专属用户
```

### 2.3 兼容迁移
- 迁移 `coupons.bind_redemption_id/bind_locked_at` → `coupon_redemption_bindings`（status=reserved/locked，user_id 置 NULL）。迁移后可清空旧列值，保留列以兼容旧代码，后续可删除。
- 为 `coupons.code` 确认唯一索引。
- 默认填充值：现存券 `type=discount`、`scope=wallet_subscription`、`currency=CNY`、`threshold_amount=0`。

---

## 三、接口设计（REST API 详细规范）

### 3.1 管理端接口

#### 3.1.1 创建优惠券
**接口**: `POST /api/admin/coupons`

**请求参数**:
```json
{
  "code": "COUPON20250109AB12CD",           // 领取码（必填）：COUPON + YYYYMMDD + 6位随机大写字母数字
  "name": "新年折扣券",                     // 名称（必填）
  "description": "全场8折优惠",             // 描述（可选）
  "type": "discount",                      // 类型（必填）：discount/full_reduction/instant_reduction
  "scope": "wallet_subscription",          // 作用域（必填）：wallet/subscription/wallet_subscription
  "discount_value": 20,                    // 折扣值（必填）：百分比 1-100 或金额（分）
  "threshold_amount": 10000,               // 满减阈值（可选）：单位分，默认 0
  "currency": "CNY",                       // 币种（必填）：当前固定 CNY
  "total_count": 1000,                     // 总发行量（必填）：≥1
  "per_user_limit": 1,                     // 每用户限领数量（必填）：≥1
  "valid_from": 1704672000,                // 生效时间（必填）：Unix 时间戳
  "valid_to": 1736208000                   // 过期时间（必填）：Unix 时间戳
}
```

**校验规则**:
- `type`: 必须为 `discount`、`full_reduction`、`instant_reduction` 之一
- `scope`: 必须为 `wallet`、`subscription`、`wallet_subscription` 之一
- `discount_value`:
  - `discount` 类型: 1-100（百分比）
  - `full_reduction`/`instant_reduction`: >0（金额，单位分）
- `threshold_amount`: ≥0（单位分）
- `valid_to` > `valid_from`

**成功响应** (HTTP 201):
```json
{
  "success": true,
  "data": {
    "id": 123,
    "code": "COUPON20250109AB12CD",
    "name": "新年折扣券",
    "remaining_count": 1000,
    "created_at": 1736352000
  }
}
```

#### 3.1.2 更新优惠券
**接口**: `PUT /api/admin/coupons/{id}`

**限制**:
- 不可修改 `used_count`（已核销数量）
- 库存 `total_count` 只能增加，不能减少（防止已发放的优惠券失效）

**请求参数**: 同创建接口（除 `code` 外均可修改）

#### 3.1.3 创建兑换码并绑定优惠券
**接口**: `POST /api/admin/redemptions`

**请求参数**:
```json
{
  "name": "VIP专属兑换码",
  "type": "subscription",                   // quota/subscription
  "payload": {                              // 订阅类型 payload
    "plan_id": 10,
    "duration_days": 30
  },
  "bound_user_id": 1001,                    // 专属用户 ID（可选，NULL 表示不限用户）
  "coupon_id": 123,                         // 绑定的优惠券 ID（可选）
  "expired_time": 1736352000                // 兑换码过期时间
}
```

**业务逻辑**:
1. 创建兑换码记录
2. 若 `coupon_id` 不为空，写入 `coupon_redemption_bindings` 表
   - `status = 'reserved'`（已绑定未锁定）
   - `user_id = bound_user_id`（可为 NULL）
3. 审计日志记录绑定操作

**成功响应** (HTTP 201):
```json
{
  "success": true,
  "data": {
    "redemption_id": 456,
    "key": "RDM-a1b2c3d4e5f6",
    "binding_status": "reserved",
    "bound_coupon": {
      "id": 123,
      "name": "新年折扣券"
    }
  }
}
```

#### 3.1.4 优惠券列表查询
**接口**: `GET /api/admin/coupons`

**查询参数**:
- `status`: 状态筛选（active/inactive/expired）
- `scope`: 作用域筛选（wallet/subscription/wallet_subscription）
- `keyword`: 关键词搜索（name/code）
- `page`: 页码（从 1 开始）
- `page_size`: 每页数量（默认 20）

#### 3.1.5 兑换码列表查询
**接口**: `GET /api/admin/redemptions`

**返回内容**: 包含绑定优惠券信息和绑定状态

---

### 3.2 用户端接口

#### 3.2.1 领取优惠券
**接口**: `POST /api/user/coupons/claim`

**请求参数**:
```json
{
  "code": "COUPON20250109AB12CD"            // 领取码
}
```

**业务流程（6步事务）**:
1. 查询优惠券模板并加载 `version`（乐观锁）
2. 校验时间、库存、状态
3. 校验用户领取限制（同一用户同一优惠券只能领取一次）
4. 乐观锁更新库存：`WHERE id = ? AND version = ? AND used_count < total_count`
   - `used_count = used_count + 1`
   - `version = version + 1`
5. 插入用户优惠券实例（`user_coupons` 表）
   - `status = 'available'`
   - `code = 'UC-' + UUID`
6. 写入审计日志

**成功响应** (HTTP 200):
```json
{
  "success": true,
  "data": {
    "user_coupon_id": 789,
    "user_coupon_code": "UC-a1b2c3d4-e5f6-7890-abcd-ef1234567890",
    "coupon_name": "新年折扣券",
    "coupon_type": "discount",
    "discount_value": 20,
    "valid_from": 1704672000,
    "valid_to": 1736208000,
    "is_expired": false
  }
}
```

**错误响应示例**:
```json
// 已领取过
{
  "success": false,
  "error": "COUPON_ALREADY_CLAIMED",
  "message": "您已领取过该优惠券"
}

// 库存不足
{
  "success": false,
  "error": "COUPON_STOCK_INSUFFICIENT",
  "message": "优惠券库存不足"
}

// 已过期
{
  "success": false,
  "error": "COUPON_EXPIRED",
  "message": "优惠券已过期"
}
```

#### 3.2.2 我的优惠券列表
**接口**: `GET /api/user/coupons`

**查询参数**:
- `status`: 状态筛选（available/used/expired/invalid）
- `page`: 页码
- `page_size`: 每页数量

**响应示例**:
```json
{
  "success": true,
  "data": {
    "total": 5,
    "items": [
      {
        "id": 789,
        "coupon_id": 123,
        "coupon_name": "新年折扣券",
        "coupon_type": "discount",
        "scope": "wallet_subscription",
        "discount_value": 20,
        "threshold_amount": 0,
        "code": "UC-a1b2c3d4-e5f6-7890-abcd-ef1234567890",
        "status": "available",
        "claimed_at": 1736352000,
        "valid_from": 1704672000,
        "valid_to": 1736208000,
        "is_expired": false
      }
    ]
  }
}
```

#### 3.2.3 优惠券使用预览（可选）
**接口**: `POST /api/user/payment/coupon/preview`

**请求参数**:
```json
{
  "user_coupon_id": 789,                    // 用户优惠券 ID
  "order_amount": 10000,                    // 订单金额（分）
  "scene": "wallet"                         // 场景：wallet/subscription
}
```

**业务逻辑**:
- 校验优惠券状态、时间、作用域
- 计算优惠金额（不实际扣减）
- 返回最终应付金额

**成功响应（可用）**:
```json
{
  "success": true,
  "data": {
    "is_applicable": true,
    "original_amount": 10000,               // 原价（分）
    "discount_amount": 2000,                // 优惠金额（分）
    "final_amount": 8000                    // 最终应付（分）
  }
}
```

**成功响应（不可用）**:
```json
{
  "success": true,
  "data": {
    "is_applicable": false,
    "reason": "COUPON_THRESHOLD_NOT_MET",   // 不可用原因
    "message": "订单金额未达满减阈值（需满 ¥100.00）"
  }
}
```

#### 3.2.4 支付接口扩展
**现有支付接口增加参数**: `POST /api/user/orders/pay`, `POST /api/user/wallet/recharge`, `POST /api/user/subscriptions/pay`

**新增请求参数**:
```json
{
  // ... 原有参数 ...
  "user_coupon_id": 789                     // 可选：使用的用户优惠券 ID
}
```

**业务流程**:
1. 服务端复校验优惠券（状态、时间、作用域、阈值）
2. 计算折扣后价格
3. 创建订单/充值记录（保存 `original_amount`, `discount_amount`, `final_amount`）
4. 支付成功后核销优惠券（调用 `UseCoupon` 流程）
5. 写入 `user_bills` 账单（包含优惠券相关字段）

**响应扩展**:
```json
{
  "success": true,
  "data": {
    "order_id": 12345,
    "original_amount": 10000,
    "discount_amount": 2000,
    "final_amount": 8000,
    "coupon_used": {
      "user_coupon_id": 789,
      "coupon_name": "新年折扣券"
    }
  }
}
```

#### 3.2.5 兑换码使用接口扩展
**接口**: `POST /api/user/redemptions/use`（已有接口，补充优惠券绑定逻辑）

**补充业务流程**:
1. 校验兑换码有效性、未使用
2. **新增**: 查询 `coupon_redemption_bindings` 绑定记录
3. **新增**: 若绑定了专属用户（`bound_user_id`），校验是否匹配当前用户
4. **新增**: 若绑定了优惠券，执行以下逻辑：
   - 锁定绑定记录：`status = 'reserved'` → `'locked'`
   - 检查用户是否已有该优惠券实例
     - 若无：创建 `user_coupons` 记录（`status = 'locked'`）
     - 若有：将状态从 `'available'` 改为 `'locked'`
5. 执行兑换业务（充值/订阅）
6. **新增**: 兑换成功后，将用户优惠券状态改为 `'used'`
7. 标记兑换码为已使用
8. 写入审计日志

**错误响应示例**:
```json
// 不是专属用户
{
  "success": false,
  "error": "REDEMPTION_USER_MISMATCH",
  "message": "该兑换码不属于您，无法使用"
}

// 绑定已锁定
{
  "success": false,
  "error": "COUPON_REDEMPTION_LOCKED",
  "message": "该兑换码绑定的优惠券已失效"
}
```

---

## 四、核心流程

### 4.1 领券（事务 + 乐观锁）

**流程步骤**：
1. **查询并加锁**：查询优惠券模板并加载 `version`（乐观锁版本号）
2. **校验资格**：校验时间、库存、状态、用户领取限制（同一用户同一优惠券只能领取一次）
3. **乐观锁更新库存**：条件更新 `UPDATE coupons SET used_count=used_count+1, version=version+1 WHERE id=? AND version=? AND used_count < total_count`
4. **生成用户券实例**：`INSERT user_coupons (coupon_id, user_id, code='UC-'+UUID, status='available', claimed_at, created_at, updated_at)`
5. **审计日志**：记录领券操作到 `audit_logs`
6. **事务提交**：所有操作在一个事务中完成，确保原子性

**详细代码实现**：
```go
// 伪代码示例
func ClaimCoupon(userId int64, claimCode string) error {
    tx := DB.Begin()
    defer tx.Rollback()

    // 1. 查询优惠券并加载版本号
    var coupon Coupon
    err := tx.Where("code = ? AND status = ?", claimCode, "active").First(&coupon).Error
    if err != nil {
        return errors.New("COUPON_NOT_FOUND")
    }

    // 2. 校验时间、库存
    now := time.Now().Unix()
    if now < coupon.ValidFrom || now > coupon.ValidTo {
        return errors.New("COUPON_EXPIRED")
    }

    // 3. 校验用户领取限制（同一用户同一优惠券只能领取一次）
    var count int64
    tx.Model(&UserCoupon{}).
        Where("user_id = ? AND coupon_id = ?", userId, coupon.Id).
        Count(&count)
    if count > 0 {
        return errors.New("COUPON_ALREADY_CLAIMED")
    }

    // 4. 乐观锁更新库存（并发安全）
    result := tx.Model(&Coupon{}).
        Where("id = ? AND version = ? AND used_count < total_count",
              coupon.Id, coupon.Version).
        Updates(map[string]interface{}{
            "used_count": gorm.Expr("used_count + 1"),
            "version":    gorm.Expr("version + 1"),
        })

    if result.RowsAffected == 0 {
        return errors.New("COUPON_STOCK_INSUFFICIENT")
    }

    // 5. 插入用户券实例
    userCoupon := &UserCoupon{
        CouponId:   coupon.Id,
        UserId:     userId,
        Code:       GenerateUserCouponCode(), // UC- + UUID
        Status:     "available",
        ClaimedAt:  now,
        CreatedAt:  now,
        UpdatedAt:  now,
    }
    err = tx.Create(userCoupon).Error
    if err != nil {
        return err
    }

    // 6. 审计日志
    WriteAuditLog(tx, "coupon", "claim", userId, coupon.Id, map[string]interface{}{
        "coupon_code": claimCode,
        "user_coupon_id": userCoupon.Id,
    })

    tx.Commit()
    return nil
}
```

**乐观锁机制说明**：
- 每次更新 `coupons.used_count` 时检查 `version` 字段是否匹配
- 更新成功后 `version + 1`
- 若版本不匹配（并发更新），`RowsAffected = 0`，返回库存不足
- **预期并发量级**：1000 QPS，乐观锁性能优于悲观锁（避免锁等待）
- **超发防护**：`WHERE used_count < total_count` 双重保障

### 4.2 核销（支付成功内事务/幂等）

**流程步骤**：
1. **获取并加锁**：获取用户优惠券并加行锁（`FOR UPDATE`），校验 `status='available'` 且归属当前用户
2. **幂等性检查**：若已 `used` 且 `order_id` 相同，直接返回成功；否则报已使用
3. **基础校验**：校验优惠券有效期、作用域（`scope` 与场景匹配）、币种（当前固定 CNY）
4. **阈值与计算**：校验满减阈值（订单原价 >= `threshold_amount`），计算优惠金额：
   - 折扣券：`discount = orderAmount * discountValue / 100`
   - 满减券：若满足阈值，`discount = discountValue`；否则不可用
   - 立减券：`discount = min(discountValue, orderAmount)`
   - 最终应付：`finalAmount = max(0, orderAmount - discountAmount)`
5. **原子更新状态**：条件更新 `UPDATE user_coupons SET status='used', used_at=now, order_id=?, discount_amount=?, final_amount=? WHERE id=? AND status='available'`
6. **写入账单**：在 `user_bills` 中记录完整的优惠券使用信息（`coupon_id`, `user_coupon_id`, `original_amount`, `discount_amount`, `final_amount`, `refund_type='none'`）
7. **审计日志**：记录核销操作到 `audit_logs`
8. **事务提交**：确保核销操作的原子性和幂等性

**详细代码实现**：
```go
// 伪代码示例
func UseCoupon(userId int64, userCouponId int64, orderId int64, orderAmount int64, scene string) (*CouponUsageResult, error) {
    tx := DB.Begin()
    defer tx.Rollback()

    // 1. 获取用户优惠券并加行锁（FOR UPDATE）
    var userCoupon UserCoupon
    err := tx.Set("gorm:query_option", "FOR UPDATE").
        Where("id = ? AND user_id = ?", userCouponId, userId).
        First(&userCoupon).Error
    if err != nil {
        return nil, errors.New("COUPON_NOT_FOUND")
    }

    // 2. 幂等性检查：如果已使用且订单号相同，直接返回成功
    if userCoupon.Status == "used" {
        if userCoupon.OrderId != nil && *userCoupon.OrderId == orderId {
            // 幂等返回，已成功核销过
            return &CouponUsageResult{
                DiscountAmount: userCoupon.DiscountAmount,
                FinalAmount:    userCoupon.FinalAmount,
            }, nil
        }
        return nil, errors.New("COUPON_ALREADY_USED")
    }

    // 3. 状态校验：必须为 available
    if userCoupon.Status != "available" {
        return nil, errors.New("COUPON_STATUS_INVALID")
    }

    // 4. 获取优惠券模板信息
    var coupon Coupon
    err = tx.Where("id = ?", userCoupon.CouponId).First(&coupon).Error
    if err != nil {
        return nil, errors.New("COUPON_TEMPLATE_NOT_FOUND")
    }

    // 5. 时间校验
    now := time.Now().Unix()
    if now < coupon.ValidFrom || now > coupon.ValidTo {
        return nil, errors.New("COUPON_EXPIRED")
    }

    // 6. 作用域校验（scope 与场景匹配）
    if !isScopeMatch(coupon.Scope, scene) {
        return nil, errors.New("COUPON_SCOPE_MISMATCH")
    }

    // 7. 币种校验（当前固定 CNY）
    if coupon.Currency != "CNY" {
        return nil, errors.New("COUPON_CURRENCY_MISMATCH")
    }

    // 8. 计算优惠金额（基于订单原价 orderAmount）
    var discountAmount int64
    switch coupon.Type {
    case "discount": // 折扣券（百分比）
        // orderAmount 是订单原价（实际支付价格，未扣券前）
        discountAmount = orderAmount * coupon.DiscountValue / 100
    case "full_reduction": // 满减券
        if orderAmount >= coupon.ThresholdAmount {
            discountAmount = coupon.DiscountValue
        } else {
            return nil, errors.New("COUPON_THRESHOLD_NOT_MET")
        }
    case "instant_reduction": // 立减券
        discountAmount = min(coupon.DiscountValue, orderAmount)
    default:
        return nil, errors.New("COUPON_TYPE_INVALID")
    }

    // 9. 计算最终应付金额（不允许为负）
    finalAmount := max(0, orderAmount - discountAmount)

    // 10. 原子更新用户优惠券状态为 used（条件更新防止并发）
    result := tx.Model(&UserCoupon{}).
        Where("id = ? AND status = 'available'", userCouponId).
        Updates(map[string]interface{}{
            "status":          "used",
            "used_at":         now,
            "order_id":        orderId,
            "discount_amount": discountAmount,
            "final_amount":    finalAmount,
        })

    if result.Error != nil {
        return nil, result.Error
    }

    if result.RowsAffected == 0 {
        return nil, errors.New("COUPON_CONCURRENT_CONFLICT")
    }

    // 11. 写入 user_bills 账单记录（包含优惠券相关字段）
    bill := &UserBill{
        UserId:         userId,
        OrderId:        orderId,
        CouponId:       &coupon.Id,              // 优惠券模板ID
        UserCouponId:   &userCoupon.Id,          // 用户优惠券ID
        OriginalAmount: orderAmount,             // 原价（分）
        DiscountAmount: discountAmount,          // 优惠金额（分）
        FinalAmount:    finalAmount,             // 实付金额（分）
        RefundType:     "none",                  // 退款类型：none（未退款）
        CreatedAt:      now,
        UpdatedAt:      now,
    }
    err = tx.Create(bill).Error
    if err != nil {
        return nil, err
    }

    // 12. 写入审计日志
    WriteAuditLog(tx, "coupon", "use", userId, coupon.Id, map[string]interface{}{
        "user_coupon_id":  userCoupon.Id,
        "order_id":        orderId,
        "discount_amount": discountAmount,
        "final_amount":    finalAmount,
        "scene":           scene,
    })

    tx.Commit()

    return &CouponUsageResult{
        DiscountAmount: discountAmount,
        FinalAmount:    finalAmount,
    }, nil
}

func isScopeMatch(scope string, scene string) bool {
    switch scope {
    case "wallet":
        return scene == "wallet"
    case "subscription":
        return scene == "subscription"
    case "wallet_subscription":
        return scene == "wallet" || scene == "subscription"
    default:
        return false
    }
}

func min(a, b int64) int64 {
    if a < b {
        return a
    }
    return b
}

func max(a, b int64) int64 {
    if a > b {
        return a
    }
    return b
}
```

**核销关键要点**：
- **阈值判断**：基于订单原价 `orderAmount`（实际支付价格，未扣券前）
- **幂等性**：通过 `order_id` 判断，同订单重复请求返回成功
- **原子性**：使用条件更新 `WHERE status = 'available'` 防止并发核销
- **账单记录**：完整记录 `coupon_id`, `user_coupon_id`, `original_amount`, `discount_amount`, `final_amount`
- **退款政策**：优惠券核销后不可恢复，`refund_type` 初始为 `none`；退款时仅更新账单状态，不恢复券

### 4.3 兑换码绑定券流程（事务 + 专属用户校验）

#### 4.3.1 创建兑换码并绑定优惠券（管理端）

**流程步骤**：
1. **校验优惠券**：查询优惠券模板，校验其存在且状态为 `active`
2. **创建兑换码**：生成唯一兑换码记录（`redemptions` 表）
3. **创建绑定记录**：在 `coupon_redemption_bindings` 表中创建绑定关系
   - `status = 'reserved'`（已绑定未锁定）
   - `user_id = bound_user_id`（可为 NULL，表示不限用户）
   - `coupon_id` 和 `redemption_id` 建立关联
4. **审计日志**：记录绑定创建操作
5. **事务提交**：确保兑换码创建和绑定关系的原子性

**详细代码实现**：
```go
// 伪代码示例：管理员创建兑换码并绑定优惠券
func CreateRedemptionWithCoupon(adminId int64, redemptionName string, couponId int64, boundUserId *int64, payload interface{}) (*Redemption, error) {
    tx := DB.Begin()
    defer tx.Rollback()

    // 1. 校验优惠券存在且可绑定
    var coupon Coupon
    err := tx.Where("id = ? AND status = ?", couponId, "active").First(&coupon).Error
    if err != nil {
        return nil, errors.New("COUPON_NOT_FOUND")
    }

    // 2. 创建兑换码
    redemption := &Redemption{
        UserId:      adminId,
        Key:         GenerateRedemptionKey(), // 生成唯一兑换码
        Status:      RedemptionStatusEnabled,
        Name:        redemptionName,
        Type:        "subscription", // 或 "quota"
        Payload:     SerializePayload(payload),
        CreatedTime: time.Now().Unix(),
    }
    err = tx.Create(redemption).Error
    if err != nil {
        return nil, err
    }

    // 3. 创建绑定记录
    binding := &CouponRedemptionBinding{
        CouponId:     couponId,
        RedemptionId: redemption.Id,
        UserId:       boundUserId, // 可为 NULL（不限用户）或指定用户ID
        Status:       "reserved",  // 已绑定未锁定
        CreatedAt:    time.Now().Unix(),
        UpdatedAt:    time.Now().Unix(),
    }
    err = tx.Create(binding).Error
    if err != nil {
        return nil, err
    }

    // 4. 审计日志
    WriteAuditLog(tx, "coupon_binding", "create", adminId, couponId, map[string]interface{}{
        "redemption_id": redemption.Id,
        "bound_user_id": boundUserId,
    })

    tx.Commit()
    return redemption, nil
}

#### 4.3.2 用户兑换绑定优惠券的兑换码（用户端）

**流程步骤**：
1. **获取兑换码并加锁**：获取兑换码记录并加行锁（`FOR UPDATE`），校验状态和有效期
2. **获取绑定记录并加锁**：查询 `coupon_redemption_bindings` 并加行锁
3. **专属用户校验**：若 `binding.user_id` 不为空，校验是否匹配当前用户（不匹配则拒绝）
4. **绑定状态校验**：若 `status='locked'`，表示已兑换或失效，拒绝操作
5. **锁定绑定记录**：原子更新 `status='reserved'` → `'locked'`，记录 `locked_at` 时间
6. **处理用户优惠券实例**：
   - 若用户无此券实例：创建新记录，`status='locked'`
   - 若用户已有此券且 `status='available'`：更新为 `status='locked'`
   - 若用户已有此券但状态不是 `available`：拒绝操作
7. **执行兑换业务**：执行充值或订阅等兑换操作
8. **成功后核销优惠券**：将用户优惠券状态从 `'locked'` 改为 `'used'`，记录 `used_at` 时间
9. **标记兑换码已使用**：更新兑换码状态为已使用
10. **审计日志**：记录完整的兑换和券使用操作
11. **事务提交/回滚**：成功则提交，失败则回滚所有操作（券状态和绑定状态恢复）

**详细代码实现**：
```go
// 伪代码示例：用户兑换绑定优惠券的兑换码
func RedeemWithBoundCoupon(userId int64, redemptionKey string) error {
    tx := DB.Begin()
    defer tx.Rollback()

    // 1. 获取兑换码并加行锁
    var redemption Redemption
    keyCol := "`key`"
    if common.UsingPostgreSQL {
        keyCol = `"key"`
    }
    err := tx.Set("gorm:query_option", "FOR UPDATE").
        Where(keyCol+" = ?", redemptionKey).
        First(&redemption).Error
    if err != nil {
        return errors.New("REDEMPTION_NOT_FOUND")
    }

    // 2. 校验兑换码状态和有效期
    if redemption.Status != RedemptionStatusEnabled {
        return errors.New("REDEMPTION_USED")
    }
    if redemption.ExpiredTime != 0 && redemption.ExpiredTime < common.GetTimestamp() {
        return errors.New("REDEMPTION_EXPIRED")
    }

    // 3. 获取绑定记录并加行锁
    var binding CouponRedemptionBinding
    err = tx.Set("gorm:query_option", "FOR UPDATE").
        Where("redemption_id = ?", redemption.Id).
        First(&binding).Error
    if err != nil {
        return errors.New("COUPON_BINDING_NOT_FOUND")
    }

    // 4. 校验专属用户（如果绑定了特定用户）
    if binding.UserId != nil && *binding.UserId != userId {
        return errors.New("REDEMPTION_USER_MISMATCH")
    }

    // 5. 校验绑定状态（已锁定表示已兑换或失效）
    if binding.Status == "locked" {
        return errors.New("COUPON_REDEMPTION_LOCKED")
    }

    // 6. 锁定绑定记录（reserved → locked）
    now := time.Now().Unix()
    result := tx.Model(&CouponRedemptionBinding{}).
        Where("id = ? AND status = 'reserved'", binding.Id).
        Updates(map[string]interface{}{
            "status":    "locked",
            "locked_at": now,
        })
    if result.RowsAffected == 0 {
        return errors.New("COUPON_CONCURRENT_CONFLICT")
    }

    // 7. 取得或创建用户优惠券实例
    var userCoupon *UserCoupon
    err = tx.Where("user_id = ? AND coupon_id = ?", userId, binding.CouponId).
        First(&userCoupon).Error

    if errors.Is(err, gorm.ErrRecordNotFound) {
        // 7a. 用户没有此券实例，创建一条 locked 状态
        userCoupon = &UserCoupon{
            CouponId:  binding.CouponId,
            UserId:    userId,
            Code:      GenerateUserCouponCode(), // UC- + UUID
            Status:    "locked", // 兑换中锁定
            ClaimedAt: now,
            CreatedAt: now,
            UpdatedAt: now,
        }
        err = tx.Create(userCoupon).Error
        if err != nil {
            return err
        }
    } else if err != nil {
        return err
    } else {
        // 7b. 用户已有此券实例，将状态改为 locked
        if userCoupon.Status != "available" {
            return errors.New("COUPON_STATUS_INVALID")
        }
        result := tx.Model(&UserCoupon{}).
            Where("id = ? AND status = 'available'", userCoupon.Id).
            Update("status", "locked")
        if result.RowsAffected == 0 {
            return errors.New("COUPON_CONCURRENT_CONFLICT")
        }
    }

    // 8. 执行兑换业务（充值/订阅）
    err = ExecuteRedemption(tx, &redemption, userId)
    if err != nil {
        return err // 事务回滚，券状态和绑定状态恢复
    }

    // 9. 兑换成功，将用户券状态改为 used
    result = tx.Model(&UserCoupon{}).
        Where("id = ? AND status = 'locked'", userCoupon.Id).
        Updates(map[string]interface{}{
            "status":  "used",
            "used_at": now,
        })
    if result.RowsAffected == 0 {
        return errors.New("COUPON_CONCURRENT_CONFLICT")
    }

    // 10. 标记兑换码为已使用
    redemption.RedeemedTime = now
    redemption.Status = RedemptionStatusUsed
    redemption.UsedUserId = userId
    err = tx.Save(&redemption).Error
    if err != nil {
        return err
    }

    // 11. 审计日志
    WriteAuditLog(tx, "coupon_redemption", "redeem", userId, binding.CouponId, map[string]interface{}{
        "redemption_id":   redemption.Id,
        "user_coupon_id":  userCoupon.Id,
        "binding_id":      binding.Id,
    })

    tx.Commit()
    return nil
}
```

**兑换码绑定券关键要点**：
- **专属用户校验**：`binding.user_id` 不为空时，仅该用户可兑换
- **绑定状态机**：`reserved`（已绑定可兑换）→ `locked`（已兑换或失效）
- **券实例管理**：用户无券时创建 `locked` 状态实例；已有券时将 `available` 改为 `locked`
- **事务原子性**：兑换失败时回滚，券状态和绑定状态恢复
- **幂等性保护**：绑定已 `locked` 时拒绝重复兑换
- **审计完整性**：记录绑定创建、兑换操作、用户券状态变更

---

## 五、价格计算与校验细节
- 阈值判断基于订单原价（未扣券前）；折扣/立减后计算 `payable`，不允许为负。
- 币种：当前固定 CNY，若未来多币需在券创建时指定并与订单匹配。
- 单笔单券：接口层限制仅允许传 1 张券；服务层即便传多张也只取第一张并记录告警。

---

## 六、退款处理流程

### 6.1 退款策略

**核心原则**：
- 优惠券一旦核销（状态 `used`），**不可恢复**
- 退款金额基于**使用优惠券后的实际支付金额**（`final_amount`）
- 退款操作仅更新账单状态，不影响优惠券状态

### 6.2 退款类型

1. **人工退款（Manual Refund）**
   - 管理员手动操作
   - 通过管理后台发起
   - `refund_type = 'manual'`

2. **自动退款（Auto Refund）**
   - 系统自动触发（如订单取消、支付失败回退等）
   - `refund_type = 'auto'`

3. **无退款（None）**
   - 正常完成的订单
   - `refund_type = 'none'`

### 6.3 退款流程示例

```go
// 伪代码示例：处理订单退款
func ProcessRefund(orderId int64, refundReason string, refundType string) error {
    tx := DB.Begin()
    defer tx.Rollback()

    // 1. 获取订单账单记录
    var bill UserBill
    err := tx.Where("order_id = ?", orderId).First(&bill).Error
    if err != nil {
        return errors.New("BILL_NOT_FOUND")
    }

    // 2. 校验退款状态（防止重复退款）
    if bill.RefundType != "none" {
        return errors.New("BILL_ALREADY_REFUNDED")
    }

    // 3. 计算退款金额
    // 退款金额 = 实际支付金额（final_amount）
    // 注意：不是原价（original_amount），因为优惠券不恢复
    refundAmount := bill.FinalAmount

    // 4. 更新账单退款状态
    result := tx.Model(&UserBill{}).
        Where("id = ? AND refund_type = 'none'", bill.Id).
        Updates(map[string]interface{}{
            "refund_type": refundType, // 'manual' 或 'auto'
            "updated_at":  time.Now().Unix(),
        })
    if result.RowsAffected == 0 {
        return errors.New("BILL_CONCURRENT_CONFLICT")
    }

    // 5. 如果使用了优惠券，记录优惠券不恢复的说明
    var refundMetadata map[string]interface{}
    if bill.CouponId != nil && bill.UserCouponId != nil {
        refundMetadata = map[string]interface{}{
            "coupon_id":        *bill.CouponId,
            "user_coupon_id":   *bill.UserCouponId,
            "discount_amount":  bill.DiscountAmount,
            "original_amount":  bill.OriginalAmount,
            "refund_amount":    refundAmount,
            "coupon_restored":  false, // 优惠券不恢复
            "refund_reason":    refundReason,
        }
    } else {
        refundMetadata = map[string]interface{}{
            "original_amount": bill.OriginalAmount,
            "refund_amount":   refundAmount,
            "refund_reason":   refundReason,
        }
    }

    // 6. 执行实际退款操作（退回余额或第三方支付退款）
    err = ExecuteRefundPayment(tx, bill.UserId, refundAmount, orderId)
    if err != nil {
        return err
    }

    // 7. 写入审计日志
    WriteAuditLog(tx, "order", "refund", bill.UserId, orderId, refundMetadata)

    tx.Commit()
    return nil
}
```

### 6.4 退款账单记录示例

**使用优惠券的订单退款记录**：

| 字段 | 值 | 说明 |
|-----|-----|------|
| `order_id` | 12345 | 订单ID |
| `coupon_id` | 100 | 使用的优惠券模板ID |
| `user_coupon_id` | 500 | 使用的用户优惠券ID |
| `original_amount` | 10000 | 原价（100元，分） |
| `discount_amount` | 2000 | 优惠金额（20元，分） |
| `final_amount` | 8000 | 实付金额（80元，分） |
| `refund_type` | `manual` | 退款类型：人工退款 |
| `updated_at` | 1736088000 | 退款时间 |

**退款说明**：
- 用户实际支付了 **80元**（使用20元优惠券）
- 退款金额为 **80元**（`final_amount`）
- 优惠券**不恢复**，保持 `used` 状态
- 用户损失了20元优惠券的价值

### 6.5 退款场景分类

| 场景 | 退款金额 | 优惠券状态 | 说明 |
|-----|---------|-----------|------|
| 支付成功后退款 | `final_amount` | `used`（不恢复） | 正常退款流程 |
| 支付失败回滚 | 无需退款 | `available`（回滚） | 事务回滚，券状态恢复 |
| 订单取消（未支付） | 无需退款 | `available`（回滚） | 取消预扣，券状态恢复 |
| 部分退款 | 按比例计算 | `used`（不恢复） | 特殊场景，需业务定义 |

### 6.6 退款通知

退款完成后，系统应通知用户：

```
退款通知：
- 订单号：#12345
- 原价：¥100.00
- 优惠金额：¥20.00（优惠券不恢复）
- 实际支付：¥80.00
- 退款金额：¥80.00
- 退款方式：原路退回
- 退款原因：[具体原因]
```

---

## 七、并发与性能
- 库存扣减与领券插入同事务，依赖行锁/条件更新保证不超发。
- 核销使用条件更新防止重复使用；订单号+user_coupon_id 可作为幂等键。
- 绑定表 `unique(redemption_id)` 保证“一码一券”，索引覆盖主查询路径。
- 列表查询索引：`coupons(status,valid_to)`，`user_coupons(user_id,status)`，`coupon_redemption_bindings(redemption_id)`。

---

## 七、代码改造点（现有仓库）

- **常量/错误码**
  - `common/constants.go`：新增常量定义
    ```go
    // 优惠券类型
    const (
        CouponTypeDiscount        = "discount"         // 折扣券（百分比）
        CouponTypeFullReduction   = "full_reduction"   // 满减券
        CouponTypeInstantReduction = "instant_reduction" // 立减券
    )

    // 优惠券作用域
    const (
        CouponScopeWallet       = "wallet"             // 余额充值
        CouponScopeSubscription = "subscription"       // 订阅
        CouponScopeWalletSubscription = "wallet_subscription" // 充值+订阅均可
    )

    // 用户优惠券状态
    const (
        UserCouponStatusAvailable = "available" // 可用
        UserCouponStatusLocked    = "locked"    // 已锁定（核销进行中/兑换绑定）
        UserCouponStatusUsed      = "used"      // 已核销
        UserCouponStatusExpired   = "expired"   // 已过期
        UserCouponStatusInvalid   = "invalid"   // 作废
    )

    // 绑定表状态
    const (
        BindingStatusReserved = "reserved" // 已绑定未锁定
        BindingStatusLocked   = "locked"   // 已锁定（兑换时使用/已失效）
    )

    // 退款类型
    const (
        RefundTypeNone   = "none"   // 未退款
        RefundTypeManual = "manual" // 人工退款
        RefundTypeAuto   = "auto"   // 自动退款
    )
    ```

  - `types/error.go` / `common/subscription_messages.go`：完整错误码列表
    ```go
    // 优惠券相关错误码
    const (
        // 通用错误
        ErrorCodeCouponNotFound           = "COUPON_NOT_FOUND"           // 优惠券不存在
        ErrorCodeCouponExpired            = "COUPON_EXPIRED"             // 优惠券已过期
        ErrorCodeCouponInactive           = "COUPON_INACTIVE"            // 优惠券未激活

        // 领取相关
        ErrorCodeCouponAlreadyClaimed     = "COUPON_ALREADY_CLAIMED"     // 用户已领取该优惠券
        ErrorCodeCouponStockInsufficient  = "COUPON_STOCK_INSUFFICIENT"  // 优惠券库存不足
        ErrorCodeCouponClaimLimitReached  = "COUPON_CLAIM_LIMIT_REACHED" // 用户领取次数已达上限

        // 核销相关
        ErrorCodeCouponAlreadyUsed        = "COUPON_ALREADY_USED"        // 优惠券已使用
        ErrorCodeCouponStatusInvalid      = "COUPON_STATUS_INVALID"      // 优惠券状态无效
        ErrorCodeCouponScopeMismatch      = "COUPON_SCOPE_MISMATCH"      // 优惠券作用域不匹配
        ErrorCodeCouponThresholdNotMet    = "COUPON_THRESHOLD_NOT_MET"   // 未达满减阈值
        ErrorCodeCouponCurrencyMismatch   = "COUPON_CURRENCY_MISMATCH"   // 币种不匹配
        ErrorCodeCouponTypeInvalid        = "COUPON_TYPE_INVALID"        // 优惠券类型无效
        ErrorCodeCouponTemplateNotFound   = "COUPON_TEMPLATE_NOT_FOUND"  // 优惠券模板不存在

        // 并发相关
        ErrorCodeCouponConcurrentConflict = "COUPON_CONCURRENT_CONFLICT" // 并发冲突

        // 绑定相关
        ErrorCodeCouponUserMismatch       = "COUPON_USER_MISMATCH"       // 优惠券不属于该用户
        ErrorCodeCouponRedemptionLocked   = "COUPON_REDEMPTION_LOCKED"   // 兑换码绑定已锁定
        ErrorCodeRedemptionUserMismatch   = "REDEMPTION_USER_MISMATCH"   // 兑换码不属于该用户
    )

    // 错误消息常量（中文）
    const (
        MsgCouponNotFound          = "优惠券不存在"
        MsgCouponExpired           = "优惠券已过期"
        MsgCouponAlreadyClaimed    = "您已领取过该优惠券"
        MsgCouponStockInsufficient = "优惠券库存不足"
        MsgCouponAlreadyUsed       = "优惠券已使用"
        MsgCouponScopeMismatch     = "优惠券不适用于当前场景"
        MsgCouponThresholdNotMet   = "订单金额未达满减阈值"
        MsgCouponCurrencyMismatch  = "优惠券币种与订单不匹配"
        // ... 其他消息
    )
    ```

- **模型层**  
  - `model/coupon.go`：扩展字段/校验（type/scope/threshold/currency），限制 code 唯一。  
  - 新增 `model/user_coupon.go`（CRUD、核销、列表、校验）。  
  - 新增 `model/coupon_redemption_binding.go`（创建、锁定、解绑、按 redemption 查询）。  
  - `model/redemption.go`：添加 `bound_user_id` 字段；兑换流程增加专属用户校验与绑定锁定。

- **DTO/请求响应**
  - `dto/coupon_*.go`：详细 DTO 定义
    ```go
    // 优惠券创建/更新请求
    type CouponCreateRequest struct {
        Code              string  `json:"code" binding:"required"`              // 领取码
        Name              string  `json:"name" binding:"required"`              // 名称
        Description       *string `json:"description"`                          // 描述
        Type              string  `json:"type" binding:"required"`              // 类型: discount/full_reduction/instant_reduction
        Scope             string  `json:"scope" binding:"required"`             // 作用域: wallet/subscription/wallet_subscription
        DiscountValue     int64   `json:"discount_value" binding:"required"`    // 折扣值（百分比 1-100 或金额分）
        ThresholdAmount   int64   `json:"threshold_amount"`                     // 满减阈值（分）
        Currency          string  `json:"currency" binding:"required"`          // 币种（CNY）
        TotalCount        int64   `json:"total_count" binding:"required,min=1"` // 总发行量
        PerUserLimit      int64   `json:"per_user_limit" binding:"required,min=1"` // 每用户限领数量
        ValidFrom         int64   `json:"valid_from" binding:"required"`        // 生效时间（Unix 时间戳）
        ValidTo           int64   `json:"valid_to" binding:"required"`          // 过期时间（Unix 时间戳）
    }

    // 优惠券响应
    type CouponResponse struct {
        Id                int64   `json:"id"`
        Code              string  `json:"code"`
        Name              string  `json:"name"`
        Description       *string `json:"description"`
        Type              string  `json:"type"`
        Scope             string  `json:"scope"`
        DiscountValue     int64   `json:"discount_value"`
        ThresholdAmount   int64   `json:"threshold_amount"`
        Currency          string  `json:"currency"`
        TotalCount        int64   `json:"total_count"`
        UsedCount         int64   `json:"used_count"`
        RemainingCount    int64   `json:"remaining_count"` // 剩余库存
        PerUserLimit      int64   `json:"per_user_limit"`
        ValidFrom         int64   `json:"valid_from"`
        ValidTo           int64   `json:"valid_to"`
        Status            string  `json:"status"`
        CreatedBy         int64   `json:"created_by"`
        CreatedAt         int64   `json:"created_at"`
        UpdatedAt         int64   `json:"updated_at"`
    }

    // 用户领券请求
    type CouponClaimRequest struct {
        Code string `json:"code" binding:"required"` // 领取码（coupons.code）
    }

    // 用户领券响应
    type CouponClaimResponse struct {
        UserCouponId   int64  `json:"user_coupon_id"`   // 用户优惠券ID
        UserCouponCode string `json:"user_coupon_code"` // 用户优惠券核销码（UC-UUID）
        CouponName     string `json:"coupon_name"`      // 优惠券名称
        DiscountValue  int64  `json:"discount_value"`   // 折扣值
        ValidTo        int64  `json:"valid_to"`         // 过期时间
    }

    // 用户优惠券列表请求
    type UserCouponListRequest struct {
        Status   *string `json:"status" form:"status"`     // 状态筛选: available/used/expired
        Page     int     `json:"page" form:"page"`         // 页码（从1开始）
        PageSize int     `json:"page_size" form:"page_size"` // 每页数量
    }

    // 用户优惠券响应
    type UserCouponResponse struct {
        Id             int64   `json:"id"`
        CouponId       int64   `json:"coupon_id"`       // 优惠券模板ID
        CouponName     string  `json:"coupon_name"`     // 优惠券名称
        CouponType     string  `json:"coupon_type"`     // 类型
        Scope          string  `json:"scope"`           // 作用域
        DiscountValue  int64   `json:"discount_value"`  // 折扣值
        ThresholdAmount int64  `json:"threshold_amount"` // 满减阈值
        Code           string  `json:"code"`            // 用户核销码（UC-UUID）
        Status         string  `json:"status"`          // 状态
        ClaimedAt      int64   `json:"claimed_at"`      // 领取时间
        UsedAt         *int64  `json:"used_at"`         // 使用时间
        OrderId        *int64  `json:"order_id"`        // 关联订单ID
        ValidFrom      int64   `json:"valid_from"`      // 生效时间
        ValidTo        int64   `json:"valid_to"`        // 过期时间
        IsExpired      bool    `json:"is_expired"`      // 是否已过期
    }

    // 优惠券核销预览请求
    type CouponPreviewRequest struct {
        UserCouponId int64  `json:"user_coupon_id" binding:"required"` // 用户优惠券ID
        OrderAmount  int64  `json:"order_amount" binding:"required,min=1"` // 订单金额（分）
        Scene        string `json:"scene" binding:"required"` // 场景: wallet/subscription
    }

    // 优惠券核销预览响应
    type CouponPreviewResponse struct {
        IsApplicable    bool   `json:"is_applicable"`    // 是否可用
        DiscountAmount  int64  `json:"discount_amount"`  // 优惠金额（分）
        FinalAmount     int64  `json:"final_amount"`     // 最终应付金额（分）
        ReasonIfNotApplicable *string `json:"reason"` // 不可用原因
    }
    ```

  - 订单/充值/订阅 DTO 扩展：在现有 DTO 中增加 `user_coupon_id` 字段
    ```go
    // 订单创建请求扩展
    type OrderCreateRequest struct {
        // ... 现有字段 ...
        UserCouponId *int64 `json:"user_coupon_id"` // 可选：使用的用户优惠券ID
    }

    // 订单响应扩展
    type OrderResponse struct {
        // ... 现有字段 ...
        CouponId       *int64 `json:"coupon_id"`        // 使用的优惠券模板ID
        UserCouponId   *int64 `json:"user_coupon_id"`   // 使用的用户优惠券ID
        OriginalAmount int64  `json:"original_amount"`  // 原价（分）
        DiscountAmount int64  `json:"discount_amount"`  // 优惠金额（分）
        FinalAmount    int64  `json:"final_amount"`     // 实付金额（分）
    }
    ```

  - 兑换相关 DTO 扩展：增加 `bound_user_id`、可选 `coupon_id`
    ```go
    // 兑换码创建请求扩展
    type RedemptionCreateRequest struct {
        // ... 现有字段 ...
        BoundUserId *int64 `json:"bound_user_id"` // 可选：绑定专属用户ID
        CouponId    *int64 `json:"coupon_id"`     // 可选：绑定优惠券ID
    }

    // 兑换码响应扩展
    type RedemptionResponse struct {
        // ... 现有字段 ...
        BoundUserId *int64 `json:"bound_user_id"` // 绑定的专属用户ID
        BoundCoupon *CouponResponse `json:"bound_coupon"` // 绑定的优惠券信息
        BindingStatus *string `json:"binding_status"` // 绑定状态: reserved/locked
    }
    ```

- **Service/Controller**  
  - 新增领券接口；用户券列表接口。  
  - 下单/充值/订阅流程增加券校验 + 价格计算 + 核销。  
  - 兑换流程：校验专属用户 + 绑定锁定 + 创建/锁定 user_coupon + 核销或置失效。  
  - 审计：领券/核销/绑定/兑换写 `audit_logs`；账单写优惠额。

- **迁移脚本**  
  - `data/DB_DDL/subscription_migration_postgresql.sql` / `_mysql.sql`：新增表/列/索引及兼容迁移。

---

## 八、验收与测试要点
- 并发领券 100 并发，库存不超发，已领取=总数。  
- 过期/不在 scope/未达阈值时，校验失败。  
- 已使用券再次提交，返回 `COUPON_ALREADY_USED` 并保持账单幂等。  
- 兑换码绑定用户与券：非专属用户、已锁定状态均拒绝。  
- 账单和审计日志包含 `coupon_id/user_coupon_id/discount_cents`。  
- 回滚场景：支付失败/兑换失败，券状态回退（available/locked），绑定回退。

---

## 九、后续可选优化
- 定时任务：过期 user_coupon 批量标记 `expired`。  
- 黑名单/风控：单用户领取频率、IP 限制。  
- 多币种：未来支持时在券创建和订单校验补充汇率校验。

