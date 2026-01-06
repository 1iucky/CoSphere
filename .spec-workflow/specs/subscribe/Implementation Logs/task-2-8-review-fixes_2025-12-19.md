# Task 2.8 Review 问题修复总结

**修复日期**: 2025-12-19
**修复人**: AI Assistant
**关联任务**: Task 2.8 二次 Review 问题修复

---

## 修复的问题清单

### 1. ✅ 退款链路原子性问题

**问题描述**:
`ProcessRefundWithCoupon` 使用独立事务而不是外部事务,导致退款与订单状态更新不在同一事务中,存在数据不一致风险。

**修复方案**:
- 新增 `ProcessRefundWithCouponWithTx` 方法接受外部事务
- 在订单退款事务中调用此方法,保证原子性
- 修改位置:
  - `service/coupon_service.go:1296-1372` - 新增 WithTx 方法
  - `service/subscription_order_service.go:965-1002` - 集成到订单退款流程

### 2. ✅ 订阅优先级策略不一致

**问题描述**:
使用 `GetNextAvailablePriorityWithTx` 简单递增优先级,而不是使用 `InitializeSubscriptionPriorityWithTx` 按 end_at 动态插入。

**修复方案**:
- 改用 `InitializeSubscriptionPriorityWithTx` 方法
- 确保优先级按 end_at 正确排序
- 修改位置: `service/subscription_order_service.go:646-667`

### 3. ✅ 事务内读取limits未使用tx

**问题描述**:
`initializeUsageWindowsInTx` 函数内调用 `GetPlanLimitsByPlanId` 未传递事务参数,导致读取不一致。

**修复方案**:
- 改用 `GetPlanLimitsByPlanIdWithTx` 方法
- 传递事务参数确保读取一致性
- 修改位置: `service/subscription_order_service.go:666`

### 4. ✅ CreateOrder幂等查询非原子

**问题描述**:
幂等性检查 `GetUserPendingOrderByPlan` 没有行锁,并发请求可能插入多个待支付订单。

**修复方案**:
1. 将 CreateOrder 包装在事务中
2. 使用 `GetUserPendingOrderByPlanWithTx` + FOR UPDATE 锁定查询
3. 若不存在 pending 订单，额外锁定 `users` 行并二次检查，避免并发下创建多个 pending 订单
4. 提取订单创建逻辑到 `createOrderInTx` 方法
5. 新增 Model 层方法 `GetUserPendingOrderByPlanWithTx` 支持行锁

**修改位置**:
- `service/subscription_order_service.go:86-155` - CreateOrder 事务包装
- `service/subscription_order_service.go:157-274` - 新增 createOrderInTx
- `model/subscription_order.go:372-403` - 新增 GetUserPendingOrderByPlanWithTx

### 5. ✅ 统一接口参数一致性

**问题描述**:
PreviewOrder 使用 `user_coupon_id`,CreateOrder 使用 `coupon_code`,参数不一致。

**修复方案**:
- 统一使用 `coupon_code` 作为请求参数
- PreviewOrder 添加 coupon_code → userCouponId 的映射和验证逻辑
- PreviewOrder 响应中的 `coupon_code` 字段返回“实际应用的用户券码”（而非优惠券名称）
- 验证包括:
  - 查询用户优惠券
  - 验证归属权
  - 验证状态
- 修改位置: `controller/subscription_user.go:499-546`

### 6. ✅ 同步规格文档

**文档更新内容**:

#### API 参数变更

**原设计**:
```json
// PreviewOrder 请求
{
  "plan_id": 123,
  "user_coupon_id": 456  // 使用用户优惠券ID
}
```

**实际实现**:
```json
// PreviewOrder 请求
{
  "plan_id": 123,
  "coupon_code": "UC-xxx-xxx"  // 使用优惠券码
}
```

**变更原因**:
1. 与 CreateOrder 保持一致
2. 前端不需要预先知道 userCouponId
3. 简化用户体验,用户只需输入优惠券码

#### 幂等性策略增强

**原设计**: 仅按 `user_id + plan_id` 检查待支付订单

**实际实现**: 增加 `coupon` 和 `payment_channel` 参数匹配检查

**变更原因**:
- 允许用户更换优惠券或支付方式
- 提供更明确的错误提示
- 提高系统灵活性

#### 事务与并发控制

**增强点**:
1. CreateOrder 全链路事务包装
2. 幂等查询使用 FOR UPDATE 行锁
3. 退款链路事务原子性保证
4. 订阅优先级初始化事务一致性

---

## 编译验证

```bash
$ go build -o /tmp/api-test
✅ 编译成功,无错误
```

---

## 影响范围

### 前端影响
- ✅ PreviewOrder API 请求参数需更新为 `coupon_code`
- ✅ 与 CreateOrder 保持一致,降低前端复杂度

### 后端影响
- ✅ Service 层增加事务方法
- ✅ Model 层增加事务查询方法
- ✅ Controller 层统一参数处理

### 数据库影响
- ✅ 无 Schema 变更
- ✅ 无数据迁移需求

---

## 待办事项

### 测试相关
- [ ] 补充单元测试覆盖所有修复点
- [ ] CreateOrder 幂等性并发测试
- [ ] 退款链路事务回滚测试
- [ ] 优惠券验证逻辑测试

### 文档同步
- [x] 更新 API 文档中的 PreviewOrder 接口说明
- [x] 更新幂等性策略文档
- [x] 更新事务管理文档

### 监控告警
- [ ] 添加 CreateOrder 并发冲突监控
- [ ] 添加优惠券验证失败告警
- [ ] 添加事务回滚监控

---

## 技术细节

### 1. 事务传播模式

```go
// CreateOrder - 创建新事务
func (s *SubscriptionOrderService) CreateOrder(...) (*CreateOrderResult, error) {
    err := model.DB.Transaction(func(tx *gorm.DB) error {
        // 幂等查询 + 行锁
        existingOrder, err := model.GetUserPendingOrderByPlanWithTx(tx, userId, planId, true)
        if err != nil {
            return err
        }

        // 参数匹配检查...

        // 创建订单
        return s.createOrderInTx(tx, ...)
    })
}

// createOrderInTx - 使用外部事务
func (s *SubscriptionOrderService) createOrderInTx(tx *gorm.DB, ...) error {
    // 使用传入的事务执行所有操作
}
```

### 2. 行锁使用

```go
// Model 层支持 FOR UPDATE
func GetUserPendingOrderByPlanWithTx(tx *gorm.DB, userId, planId int64, forUpdate bool) (*SubscriptionOrder, error) {
    query := tx.Where(...).Where(...).Order("created_at desc")

    if forUpdate {
        query = query.Set("gorm:query_option", "FOR UPDATE")
    }

    err := query.First(&order).Error
    // ...
}
```

### 3. 优惠券验证链路

```go
// Controller 层映射
var userCouponId *int64 = nil
if req.CouponCode != nil && *req.CouponCode != "" {
    // 1. 根据 code 查询用户优惠券
    userCoupon, err := model.GetUserCouponByCode(*req.CouponCode)
    if err != nil {
        return errors.New("优惠券无效")
    }

    // 2. 验证归属
    if userCoupon.UserId != userId {
        return errors.New("优惠券不属于当前用户")
    }

    // 3. 验证状态
    if userCoupon.Status != common.UserCouponStatusAvailable {
        return errors.New("优惠券状态无效")
    }

    userCouponId = &userCoupon.Id
}

// 传递给 Service 层
svc.PreviewOrder(userId, planId, userCouponId)
```

---

## 风险评估

| 风险项 | 等级 | 缓解措施 |
|--------|------|---------|
| 并发竞争 | ✅ 低 | 使用 FOR UPDATE 行锁 |
| 事务回滚 | ✅ 低 | GORM Transaction 自动回滚 |
| API 不兼容 | ⚠️ 中 | 需要前端配合更新 PreviewOrder 参数 |
| 优惠券验证 | ✅ 低 | 完整的归属和状态校验 |

---

## 总结

本次修复解决了 Task 2.8 review 发现的 **6 个关键问题**:

1. ✅ 退款链路原子性 - 使用外部事务保证一致性
2. ✅ 订阅优先级策略 - 使用动态插入保证 end_at 排序
3. ✅ 事务读取一致性 - 使用 WithTx 方法保证隔离性
4. ✅ CreateOrder 并发安全 - 使用 FOR UPDATE 行锁
5. ✅ API 参数一致性 - 统一使用 coupon_code
6. ✅ 规格文档同步 - 更新文档反映实际实现

**代码变化统计**:
- 新增方法: 3 个
- 修改方法: 5 个
- 新增代码行: ~200 行
- 编译状态: ✅ 通过

**后续行动**:
1. 通知前端团队更新 PreviewOrder API 调用
2. 补充单元测试和集成测试
3. 部署到测试环境进行验证
4. 监控生产环境运行情况
