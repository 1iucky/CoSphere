# API 规格更新说明

**更新日期**: 2025-12-19
**更新原因**: Task 2.8 Review 修复后的实际实现与原规格文档存在差异

---

## 1. 订单预览接口 (PreviewOrder)

### 1.1 接口信息

- **路径**: `POST /api/subscription-orders/preview`
- **权限**: 需要用户登录
- **用途**: 计算订单价格,支持优惠券预览

### 1.2 请求参数

**原规格文档** ❌:
```json
{
  "plan_id": 123,
  "user_coupon_id": 456  // 用户优惠券ID (int64)
}
```

**实际实现** ✅:
```json
{
  "plan_id": 123,
  "coupon_code": "UC-a1b2c3d4-e5f6-7890-abcd-ef1234567890"  // 优惠券码 (string, optional)
}
```

### 1.3 参数说明

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| plan_id | int64 | 是 | 套餐ID |
| coupon_code | string | 否 | 用户优惠券核销码,格式: UC-UUID |

### 1.4 响应格式

```json
{
  "success": true,
  "message": "",
  "data": {
    "plan_id": 123,
    "plan_name": "专业版月付",
    "price_cents": 9900,           // 原价(分)
    "discount_cents": 1000,        // 优惠金额(分)
    "final_price_cents": 8900,     // 最终应付(分)
    "currency": "CNY",
    "coupon_applied": true,        // 是否应用了优惠券
    "coupon_code": "UC-xxx"        // 应用的优惠券码
  }
}
```

### 1.5 变更原因

1. **一致性**: 与 CreateOrder 接口保持一致,都使用 `coupon_code`
2. **简化前端**: 前端不需要先获取 `user_coupon_id`,直接使用用户输入的优惠券码
3. **用户体验**: 用户只需要知道自己的优惠券码,不需要知道内部ID

### 1.6 后端处理逻辑

```go
// controller/subscription_user.go:499-546
var req struct {
    PlanID     int64   `json:"plan_id" binding:"required,min=1"`
    CouponCode *string `json:"coupon_code"`  // 接收优惠券码
}

// 处理优惠券: 根据 coupon_code 查询 userCouponId
var userCouponId *int64 = nil
if req.CouponCode != nil && *req.CouponCode != "" {
    // 1. 根据 code 查询用户优惠券
    userCoupon, err := model.GetUserCouponByCode(*req.CouponCode)
    if err != nil {
        return gin.H{"success": false, "message": "优惠券无效"}
    }

    // 2. 验证优惠券归属
    if userCoupon.UserId != int64(userId) {
        return gin.H{"success": false, "message": "该优惠券不属于当前用户"}
    }

    // 3. 验证优惠券状态
    if userCoupon.Status != common.UserCouponStatusAvailable {
        return gin.H{"success": false, "message": "优惠券状态无效"}
    }

    userCouponId = &userCoupon.Id
}

// 调用 Service 层
svc := service.GetSubscriptionOrderService()
result, err := svc.PreviewOrder(int64(userId), req.PlanID, userCouponId)
```

---

## 2. 创建订单接口 (CreateOrder)

### 2.1 接口信息

- **路径**: `POST /api/subscription-orders`
- **权限**: 需要用户登录

### 2.2 请求参数

```json
{
  "plan_id": 123,
  "coupon_code": "UC-xxx",  // 优惠券码 (string, optional)
  "payment_channel": "wallet"
}
```

**说明**: CreateOrder 已经使用 `coupon_code`,现在 PreviewOrder 与其保持一致。

---

## 3. 幂等性策略更新

### 3.1 CreateOrder 幂等性增强

**原策略** ❌:
- 仅按 `user_id + plan_id` 检查待支付订单
- 不考虑优惠券和支付渠道差异

**新策略** ✅:
- 按 `user_id + plan_id` 查询待支付订单
- 检查优惠券是否匹配 (`user_coupon_id`)
- 检查支付渠道是否匹配 (`payment_channel`)
- 参数不匹配时返回明确错误

### 3.2 实现代码

```go
// service/subscription_order_service.go:86-155
func (s *SubscriptionOrderService) CreateOrder(...) (*CreateOrderResult, error) {
    var result *CreateOrderResult

    // 使用事务保证幂等性检查和订单创建的原子性
    err := model.DB.Transaction(func(tx *gorm.DB) error {
        // 幂等性检查: 在事务内加锁查询用户对该套餐的未过期待支付订单
        // 使用 FOR UPDATE 锁定查询结果,防止并发创建重复订单
        existingOrder, err := model.GetUserPendingOrderByPlanWithTx(tx, userId, planId, true)
        if err != nil {
            return fmt.Errorf("检查已有订单失败: %w", err)
        }
        if existingOrder != nil {
            // 检查订单参数是否匹配
            couponMatches := (existingOrder.UserCouponId == nil && userCouponId == nil) ||
                (existingOrder.UserCouponId != nil && userCouponId != nil && *existingOrder.UserCouponId == *userCouponId)
            channelMatches := existingOrder.PaymentChannel == paymentChannel

            if !couponMatches {
                // 优惠券不匹配
                oldCouponInfo := "无"
                if existingOrder.UserCouponId != nil {
                    oldCouponInfo = fmt.Sprintf("ID:%d", *existingOrder.UserCouponId)
                }
                newCouponInfo := "无"
                if userCouponId != nil {
                    newCouponInfo = fmt.Sprintf("ID:%d", *userCouponId)
                }
                return fmt.Errorf("该套餐已有待支付订单(订单ID:%d),使用的优惠券为 %s,与当前请求的优惠券 %s 不匹配。请先完成或取消现有订单",
                    existingOrder.Id, oldCouponInfo, newCouponInfo)
            }

            if !channelMatches {
                // 支付渠道不匹配
                return fmt.Errorf("该套餐已有待支付订单(订单ID:%d),支付渠道为 %s,与当前请求的支付渠道 %s 不匹配。请先完成或取消现有订单",
                    existingOrder.Id, existingOrder.PaymentChannel, paymentChannel)
            }

            // 参数完全匹配,返回已存在的订单(真正的幂等)
            result = &CreateOrderResult{
                Order:          existingOrder,
                OriginalAmount: existingOrder.PriceCents,
                DiscountAmount: existingOrder.DiscountCents,
                FinalAmount:    existingOrder.FinalPriceCents,
                CouponApplied:  existingOrder.UserCouponId != nil && *existingOrder.UserCouponId > 0,
                IsIdempotent:   true,
            }
            return nil
        }

        // 订单不存在,继续创建新订单
        return s.createOrderInTx(tx, userId, planId, userCouponId, paymentChannel, &result)
    })

    if err != nil {
        return nil, err
    }

    return result, nil
}
```

### 3.3 并发安全保证

1. **订单行锁 (FOR UPDATE)**: 若已存在 pending 订单,通过 `subscription_orders` 行锁确保并发请求返回同一订单
2. **用户行锁 (FOR UPDATE)**: 若不存在 pending 订单,先锁定 `users` 行并二次检查,避免并发下创建多个 pending 订单
3. **事务原子性**: 查询/锁定/创建在同一事务中执行,任一步骤失败回滚
4. **参数匹配检查**: 支持 coupon/payment_channel 匹配校验,不匹配时返回明确提示

---

## 4. 事务管理更新

### 4.1 退款链路原子性

**问题**: 原实现优惠券退款处理使用独立事务

**修复**: 新增 `ProcessRefundWithCouponWithTx` 方法,接受外部事务

```go
// service/coupon_service.go:1296-1372
func (s *CouponService) ProcessRefundWithCouponWithTx(tx *gorm.DB, req *RefundWithCouponRequest, result *RefundWithCouponResult) error {
    // 1. 根据订单ID查找原订阅账单(加行锁)
    var originalBill model.UserBill
    err := tx.Set("gorm:query_option", "FOR UPDATE").
        Where("source_id = ? AND user_id = ? AND bill_type IN ?",
            req.OrderId, req.UserId,
            []string{common.BillTypeSubscription, common.BillTypeSubscriptionRenew}).
        First(&originalBill).Error

    // 2. 检查是否已退款(防止重复退款)
    // 3. 计算退款金额
    // 4. 更新 refund_type 和 metadata
    // ...
}
```

**调用方式**:
```go
// service/subscription_order_service.go:965-1002
err := model.DB.Transaction(func(tx *gorm.DB) error {
    // ... 其他退款逻辑 ...

    // 处理优惠券(不恢复,只记录)- 在同一事务内执行,保证原子性
    if order.UserCouponId != nil {
        couponResult := &RefundWithCouponResult{
            Success:        false,
            CouponRestored: false,
        }
        if err := s.couponService.ProcessRefundWithCouponWithTx(tx, &RefundWithCouponRequest{
            OrderId:        req.OrderId,
            UserId:         req.UserId,
            UserCouponId:   *order.UserCouponId,
            OriginalAmount: order.PriceCents,
            DiscountAmount: order.DiscountCents,
            FinalAmount:    order.FinalPriceCents,
            RefundType:     req.RefundType,
            RefundReason:   req.RefundReason,
            OperatorId:     req.OperatorId,
        }, couponResult); err != nil {
            // 如果优惠券处理失败,整个退款事务回滚
            return fmt.Errorf("处理优惠券退款失败: %w", err)
        }
    }

    return nil
})
```

---

## 5. Model 层新增方法

### 5.1 GetUserPendingOrderByPlanWithTx

**文件**: `model/subscription_order.go:372-403`

**功能**: 在事务中查询用户对某套餐的未过期待支付订单,支持行锁

```go
func GetUserPendingOrderByPlanWithTx(tx *gorm.DB, userId int64, planId int64, forUpdate bool) (*SubscriptionOrder, error) {
    if userId == 0 || planId == 0 {
        return nil, nil
    }

    // 计算 30 分钟前的时间戳(与 IsExpired 逻辑一致)
    expireThreshold := common.GetTimestamp() - 30*60

    query := tx.Where("user_id = ?", userId).
        Where("plan_id = ?", planId).
        Where("status = ?", common.OrderStatusPending).
        Where("created_at > ?", expireThreshold). // 只查询未过期的订单
        Order("created_at desc")

    // 如果需要行锁,添加 FOR UPDATE
    if forUpdate {
        query = query.Set("gorm:query_option", "FOR UPDATE")
    }

    var order SubscriptionOrder
    err := query.First(&order).Error

    if err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            return nil, nil
        }
        return nil, err
    }

    return &order, nil
}
```

---

## 6. 前端对接指南

### 6.1 PreviewOrder 接口调用示例

**JavaScript/TypeScript**:
```typescript
// 预览订单价格(不使用优惠券)
const previewWithoutCoupon = async (planId: number) => {
  const response = await fetch('/api/subscription-orders/preview', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ plan_id: planId })
  });
  return await response.json();
};

// 预览订单价格(使用优惠券)
const previewWithCoupon = async (planId: number, couponCode: string) => {
  const response = await fetch('/api/subscription-orders/preview', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      plan_id: planId,
      coupon_code: couponCode  // 用户输入的优惠券码
    })
  });
  return await response.json();
};
```

### 6.2 错误处理

**优惠券无效**:
```json
{
  "success": false,
  "message": "优惠券无效: 优惠券不存在"
}
```

**优惠券不属于当前用户**:
```json
{
  "success": false,
  "message": "该优惠券不属于当前用户"
}
```

**优惠券状态无效**:
```json
{
  "success": false,
  "message": "优惠券状态无效: used"
}
```

---

## 7. 迁移清单

### 7.1 前端需要更新的地方

- [ ] PreviewOrder API 调用参数从 `user_coupon_id` 改为 `coupon_code`
- [ ] 优惠券输入框改为接收优惠券码(格式: UC-UUID)
- [ ] 更新错误提示文案
- [ ] 更新 TypeScript 类型定义

### 7.2 测试清单

- [ ] PreviewOrder 不使用优惠券
- [ ] PreviewOrder 使用有效优惠券
- [ ] PreviewOrder 使用无效优惠券
- [ ] PreviewOrder 使用他人优惠券
- [ ] PreviewOrder 使用已用优惠券
- [ ] CreateOrder 幂等性测试(相同参数)
- [ ] CreateOrder 幂等性测试(不同优惠券)
- [ ] CreateOrder 幂等性测试(不同支付渠道)

---

## 8. 总结

本次 API 规格更新主要改进:

1. ✅ **统一接口参数**: PreviewOrder 和 CreateOrder 都使用 `coupon_code`
2. ✅ **增强幂等性**: CreateOrder 考虑优惠券和支付渠道参数
3. ✅ **事务原子性**: 退款链路使用外部事务保证一致性
4. ✅ **并发安全**: 使用 FOR UPDATE 行锁防止重复订单
5. ✅ **用户体验**: 简化前端逻辑,用户只需输入优惠券码

**影响范围**: 需要前端配合更新 PreviewOrder 接口调用方式

**向后兼容**: 不影响旧订单数据,仅影响新接口调用
