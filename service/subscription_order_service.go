package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// ===================== Service 层 - 订阅订单服务 =====================

// SubscriptionOrderService 订阅订单服务
type SubscriptionOrderService struct {
	// 依赖服务
	planService     *SubscriptionPlanService
	usageService    *SubscriptionUsageService
	couponService   *CouponService
	priorityService *SubscriptionPriorityService
}

var (
	orderServiceInstance *SubscriptionOrderService
	orderServiceOnce     sync.Once
)

// GetSubscriptionOrderService 获取订单服务单例
func GetSubscriptionOrderService() *SubscriptionOrderService {
	orderServiceOnce.Do(func() {
		orderServiceInstance = &SubscriptionOrderService{
			planService:     GetSubscriptionPlanService(),
			usageService:    GetSubscriptionUsageService(),
			couponService:   GetCouponService(),
			priorityService: GetSubscriptionPriorityService(),
		}
	})
	return orderServiceInstance
}

// ===================== 订单创建结果 =====================

// CreateOrderResult 创建订单结果
type CreateOrderResult struct {
	Order          *model.SubscriptionOrder `json:"order"`
	OriginalAmount int64                    `json:"original_amount"` // 原价（分）
	DiscountAmount int64                    `json:"discount_amount"` // 优惠金额（分）
	FinalAmount    int64                    `json:"final_amount"`    // 最终应付（分）
	CouponApplied  bool                     `json:"coupon_applied"`  // 是否应用优惠券
	IsIdempotent   bool                     `json:"is_idempotent"`   // 是否为幂等返回（返回已存在的待支付订单）
}

// PaymentResult 支付结果
type PaymentResult struct {
	Success        bool                     `json:"success"`
	Order          *model.SubscriptionOrder `json:"order"`
	Subscription   *model.Subscription      `json:"subscription"`
	BillId         int64                    `json:"bill_id"`
	PaymentChannel string                   `json:"payment_channel"`
	TradeNo        string                   `json:"trade_no"`
	ErrorMessage   string                   `json:"error_message,omitempty"`
	IsIdempotent   bool                     `json:"is_idempotent"` // 是否为幂等返回
}

// OrderPreviewResult 订单预览结果
type OrderPreviewResult struct {
	PlanId           int64  `json:"plan_id"`
	PlanName         string `json:"plan_name"`
	BillingCycle     string `json:"billing_cycle"`
	OriginalAmount   int64  `json:"original_amount"`   // 原价（分）
	DiscountAmount   int64  `json:"discount_amount"`   // 优惠金额（分）
	FinalAmount      int64  `json:"final_amount"`      // 最终应付（分）
	Currency         string `json:"currency"`          // 币种
	DurationDays     int    `json:"duration_days"`     // 订阅时长（天）
	CouponApplicable bool   `json:"coupon_applicable"` // 优惠券是否可用
	CouponName       string `json:"coupon_name"`       // 优惠券名称
}

// ===================== 订单创建方法 =====================

// CreateOrder 创建订阅订单
// 包含套餐快照、优惠券快照、价格计算
// 幂等性：如果用户已有同一套餐的未过期待支付订单，直接返回该订单
// 并发安全：通过事务内加锁查询保证幂等性检查的原子性
func (s *SubscriptionOrderService) CreateOrder(userId int64, planId int64, userCouponId *int64, paymentChannel string) (*CreateOrderResult, error) {
	if userId == 0 {
		return nil, errors.New("用户 ID 不能为空")
	}
	if planId == 0 {
		return nil, errors.New("套餐 ID 不能为空")
	}

	var result *CreateOrderResult

	// 使用事务保证幂等性检查和订单创建的原子性
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		// 幂等性检查：在事务内加锁查询用户对该套餐的未过期待支付订单
		// 使用 FOR UPDATE 锁定查询结果，防止并发创建重复订单
		existingOrder, err := model.GetUserPendingOrderByPlanWithTx(tx, userId, planId, true)
		if err != nil {
			return fmt.Errorf("检查已有订单失败: %w", err)
		}

		// 并发保护：如果当前还没有 pending 订单，则锁定用户行，避免并发下“双写”
		// 注意：FOR UPDATE 只能锁定已存在的订单行，订单不存在时需要额外的串行化手段
		if existingOrder == nil {
			var user model.User
			if err := tx.Set("gorm:query_option", "FOR UPDATE").Select("id").First(&user, "id = ?", userId).Error; err != nil {
				return fmt.Errorf("锁定用户失败: %w", err)
			}
			// 再次检查：等待锁的期间可能已有其他事务创建了 pending 订单
			existingOrder, err = model.GetUserPendingOrderByPlanWithTx(tx, userId, planId, true)
			if err != nil {
				return fmt.Errorf("检查已有订单失败: %w", err)
			}
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
				return fmt.Errorf("该套餐已有待支付订单(订单ID:%d)，使用的优惠券为 %s，与当前请求的优惠券 %s 不匹配。请先完成或取消现有订单",
					existingOrder.Id, oldCouponInfo, newCouponInfo)
			}

			if !channelMatches {
				// 支付渠道不匹配
				return fmt.Errorf("该套餐已有待支付订单(订单ID:%d)，支付渠道为 %s，与当前请求的支付渠道 %s 不匹配。请先完成或取消现有订单",
					existingOrder.Id, existingOrder.PaymentChannel, paymentChannel)
			}

			// 参数完全匹配，返回已存在的订单（真正的幂等）
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

		// 订单不存在，继续创建新订单
		return s.createOrderInTx(tx, userId, planId, userCouponId, paymentChannel, &result)
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

// createOrderInTx 在事务中创建新订单
func (s *SubscriptionOrderService) createOrderInTx(tx *gorm.DB, userId int64, planId int64, userCouponId *int64, paymentChannel string, result **CreateOrderResult) error {
	// 1. 获取套餐信息
	plan, err := model.GetSubscriptionPlanByIdWithTx(tx, planId)
	if err != nil {
		return fmt.Errorf("获取套餐失败: %w", err)
	}

	// 2. 校验套餐状态
	if plan.Status != common.PlanStatusActive {
		return errors.New(common.MsgPlanNotPublished)
	}

	// 3. 校验套餐有效期
	now := common.GetTimestamp()
	if plan.StartAt != nil && *plan.StartAt > now {
		return errors.New("套餐尚未生效")
	}
	if plan.EndAt != nil && *plan.EndAt < now {
		return errors.New("套餐已停售")
	}

	// 4. 计算价格
	originalAmount := plan.PriceCents
	var discountAmount int64 = 0
	var couponApplied bool = false
	var couponSnapshotJSON *string = nil
	var couponTemplateId *int64 = nil

	// 5. 处理优惠券
	if userCouponId != nil && *userCouponId > 0 {
		// 预览优惠券使用效果（包含套餐/币种校验）
		preview, err := s.couponService.PreviewCouponUsageWithPlanWithTx(tx, userId, *userCouponId, originalAmount, "subscription", &plan.Id, plan.Currency)
		if err != nil {
			return fmt.Errorf("优惠券校验失败: %w", err)
		}
		if preview.Valid {
			discountAmount = preview.DiscountAmount
			couponApplied = true

			// 获取优惠券详情用于快照（使用事务内读取保证一致性）
			userCoupon, err := model.GetUserCouponByIdWithTx(tx, *userCouponId, false)
			if err != nil {
				return fmt.Errorf("获取用户优惠券失败: %w", err)
			}
			coupon, err := model.GetCouponByIdWithTx(tx, userCoupon.CouponId, false)
			if err != nil {
				return fmt.Errorf("获取优惠券模板失败: %w", err)
			}
			couponTemplateId = &coupon.Id

			snapshotStr, err := model.CreateCouponSnapshot(coupon)
			if err != nil {
				return fmt.Errorf("创建优惠券快照失败: %w", err)
			}
			couponSnapshotJSON = &snapshotStr
		} else {
			// 优惠券不可用，返回错误信息
			return fmt.Errorf("优惠券不可用: %s", preview.ErrorMessage)
		}
	}

	finalAmount := originalAmount - discountAmount
	if finalAmount < 0 {
		finalAmount = 0
	}

	// 6. 创建套餐快照（使用统一的快照生成函数，确保包含完整信息）
	planSnapshotJSON, err := model.CreatePlanSnapshot(plan)
	if err != nil {
		return fmt.Errorf("创建套餐快照失败: %w", err)
	}

	// 7. 创建订单（使用传入的事务）
	order := &model.SubscriptionOrder{
		UserId:          userId,
		PlanId:          planId,
		Status:          common.OrderStatusPending,
		PriceCents:      originalAmount,
		DiscountCents:   discountAmount,
		FinalPriceCents: finalAmount,
		PaymentChannel:  paymentChannel,
		PlanSnapshot:    planSnapshotJSON, // 直接使用已生成的快照 JSON
	}

	// 设置优惠券快照
	if couponSnapshotJSON != nil {
		order.CouponSnapshot = couponSnapshotJSON
		order.UserCouponId = userCouponId
		order.CouponId = couponTemplateId
	}

	// 创建订单记录
	if err := model.CreateSubscriptionOrderWithTx(tx, order); err != nil {
		return fmt.Errorf("创建订单失败: %w", err)
	}

	// 写入审计日志
	if err := s.logOrderAction(tx, userId, order.Id, "create_order", map[string]interface{}{
		"plan_id":         planId,
		"original_amount": originalAmount,
		"discount_amount": discountAmount,
		"final_amount":    finalAmount,
		"coupon_applied":  couponApplied,
	}); err != nil {
		return err
	}

	// 设置返回结果
	*result = &CreateOrderResult{
		Order:          order,
		OriginalAmount: originalAmount,
		DiscountAmount: discountAmount,
		FinalAmount:    finalAmount,
		CouponApplied:  couponApplied,
	}

	return nil
}

// PreviewOrder 预览订单（不创建）
func (s *SubscriptionOrderService) PreviewOrder(userId int64, planId int64, userCouponId *int64) (*OrderPreviewResult, error) {
	if planId == 0 {
		return nil, errors.New("套餐 ID 不能为空")
	}

	// 1. 获取套餐信息
	plan, err := s.planService.GetPlan(planId)
	if err != nil {
		return nil, err
	}

	// 2. 校验套餐状态
	if plan.Status != common.PlanStatusActive {
		return nil, errors.New(common.MsgPlanNotPublished)
	}

	// 计算订阅时长（天）
	durationDays := s.calculateDurationDays(plan.BillingCycle, plan.BillingCycleValue)

	result := &OrderPreviewResult{
		PlanId:         plan.Id,
		PlanName:       plan.Name,
		BillingCycle:   plan.BillingCycle,
		OriginalAmount: plan.PriceCents,
		DiscountAmount: 0,
		FinalAmount:    plan.PriceCents,
		Currency:       plan.Currency,
		DurationDays:   durationDays,
	}

	// 3. 处理优惠券预览
	if userCouponId != nil && *userCouponId > 0 && userId > 0 {
		preview, err := s.couponService.PreviewCouponUsageWithPlan(userId, *userCouponId, plan.PriceCents, "subscription", &plan.Id, plan.Currency)
		if err == nil && preview.Valid {
			result.DiscountAmount = preview.DiscountAmount
			result.FinalAmount = preview.FinalAmount
			result.CouponApplicable = true

			// 获取优惠券名称
			userCoupon, _ := model.GetUserCouponById(*userCouponId)
			if userCoupon != nil {
				coupon, _ := model.GetCouponById(userCoupon.CouponId)
				if coupon != nil {
					result.CouponName = coupon.Name
				}
			}
		}
	}

	return result, nil
}

// ===================== 支付处理方法 =====================

// ProcessPayment 处理订单支付（用户入口）
// 仅允许 wallet/free，第三方支付通过回调进入 ProcessPaymentInternal
func (s *SubscriptionOrderService) ProcessPayment(userId int64, orderId int64, paymentChannel string, tradeNo string) (*PaymentResult, error) {
	return s.processPayment(userId, orderId, paymentChannel, tradeNo, false)
}

// ProcessPaymentInternal 处理订单支付（内部回调入口）
// 允许第三方支付与 redemption 渠道
func (s *SubscriptionOrderService) ProcessPaymentInternal(userId int64, orderId int64, paymentChannel string, tradeNo string) (*PaymentResult, error) {
	return s.processPayment(userId, orderId, paymentChannel, tradeNo, true)
}

// processPayment 处理订单支付核心逻辑
// allowThirdParty=true 时允许第三方渠道与 redemption
func (s *SubscriptionOrderService) processPayment(userId int64, orderId int64, paymentChannel string, tradeNo string, allowThirdParty bool) (*PaymentResult, error) {
	result := &PaymentResult{
		Success:        false,
		PaymentChannel: paymentChannel,
		TradeNo:        tradeNo,
	}

	if userId == 0 {
		result.ErrorMessage = "用户 ID 不能为空"
		return result, errors.New(result.ErrorMessage)
	}
	if orderId == 0 {
		result.ErrorMessage = "订单 ID 不能为空"
		return result, errors.New(result.ErrorMessage)
	}

	// 安全校验：外部接口只允许 wallet/free，内部可放开第三方与 redemption
	allowedChannels := map[string]bool{
		common.PaymentChannelWallet: true,
		common.PaymentChannelFree:   true,
	}
	if allowThirdParty {
		allowedChannels[common.PaymentChannelAlipay] = true
		allowedChannels[common.PaymentChannelWechat] = true
		allowedChannels[common.PaymentChannelStripe] = true
		allowedChannels[common.PaymentChannelPaypal] = true
		allowedChannels[common.PaymentChannelRedemption] = true
	}
	if !allowedChannels[paymentChannel] {
		result.ErrorMessage = "不支持的支付渠道，请使用正确的支付方式"
		return result, errors.New(result.ErrorMessage)
	}

	// 1. 获取订单（带行锁）
	var order *model.SubscriptionOrder
	var subscription *model.Subscription
	var billId int64
	var quotaAmount int64

	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var err error
		order, err = model.GetSubscriptionOrderByIdWithTx(tx, orderId, true)
		if err != nil {
			return err
		}

		// 2. 校验订单归属
		if order.UserId != userId {
			return errors.New("订单不属于该用户")
		}

		// 3. 幂等性检查：如果订单已支付，直接返回成功
		if order.Status == common.OrderStatusPaid {
			result.IsIdempotent = true
			result.Success = true
			result.Order = order
			// 返回真实的支付信息（忽略本次请求的 paymentChannel / tradeNo）
			result.PaymentChannel = order.PaymentChannel
			if order.TradeNo != nil {
				result.TradeNo = *order.TradeNo
			} else {
				result.TradeNo = ""
			}
			if order.BillId != nil {
				billId = *order.BillId
			}

			// 获取关联的订阅（paid 状态应当存在）
			if order.SubscriptionId == nil {
				return errors.New("订单已支付但缺少关联订阅")
			}
			subscription, err = model.GetSubscriptionByIdWithTx(tx, *order.SubscriptionId, false)
			if err != nil {
				return fmt.Errorf("获取关联订阅失败: %w", err)
			}
			result.Subscription = subscription
			return nil
		}

		// 4. 校验订单状态
		if order.Status != common.OrderStatusPending {
			return fmt.Errorf("订单状态无效: %s", order.Status)
		}

		// 5. 校验订单是否过期
		if order.IsExpired() {
			// 更新订单状态为过期
			_ = model.ExpireOrderWithTx(tx, orderId)
			return errors.New(common.MsgOrderExpired)
		}

		// 6. 安全校验：支付渠道必须与订单创建时一致
		// 例外：free 渠道可以用于任何 0 元订单（防止创建时渠道为 wallet 但实际是 0 元）
		if paymentChannel == common.PaymentChannelFree {
			if order.FinalPriceCents > 0 {
				return errors.New("非零元订单不能使用免费支付")
			}
		} else if paymentChannel != order.PaymentChannel {
			return fmt.Errorf("支付渠道不匹配，订单要求 %s，实际 %s", order.PaymentChannel, paymentChannel)
		}

		// 7. 根据支付渠道处理支付
		quotaAmount = 0
		switch paymentChannel {
		case common.PaymentChannelWallet:
			// 余额支付
			quotaAmount, err = s.processWalletPayment(tx, userId, order)
			if err != nil {
				return err
			}
		case common.PaymentChannelFree, common.PaymentChannelRedemption,
			common.PaymentChannelAlipay, common.PaymentChannelWechat,
			common.PaymentChannelStripe, common.PaymentChannelPaypal:
			// 免费订单（0元）- 已在上面校验过金额为0
			// 或第三方/兑换码支付（外部已完成支付），无需扣款
			quotaAmount = 0
		default:
			// 不应该到达这里，因为上面已经限制了允许的渠道
			return errors.New("内部错误：不支持的支付渠道")
		}

		// 7. 核销优惠券（如果有）- 使用事务内版本保证原子性
		if order.UserCouponId != nil && *order.UserCouponId > 0 {
			// 预防历史脏数据：确保优惠券币种与订单币种一致（优先使用快照，缺失则回查模板）
			planSnapshot, err := order.GetPlanSnapshotAsData()
			if err != nil {
				return fmt.Errorf("获取套餐快照失败: %w", err)
			}
			if order.CouponSnapshot != nil && *order.CouponSnapshot != "" {
				couponSnapshot, err := order.GetCouponSnapshotData()
				if err != nil {
					return fmt.Errorf("解析优惠券快照失败: %w", err)
				}
				if couponSnapshot != nil && couponSnapshot.Currency != "" &&
					strings.ToUpper(couponSnapshot.Currency) != strings.ToUpper(planSnapshot.Currency) {
					return errors.New("优惠券币种与订单不匹配")
				}
			} else {
				userCoupon, err := model.GetUserCouponByIdWithTx(tx, *order.UserCouponId, false)
				if err != nil {
					return fmt.Errorf("获取用户优惠券失败: %w", err)
				}
				coupon, err := model.GetCouponByIdWithTx(tx, userCoupon.CouponId, false)
				if err != nil {
					return fmt.Errorf("获取优惠券模板失败: %w", err)
				}
				if strings.ToUpper(coupon.Currency) != strings.ToUpper(planSnapshot.Currency) {
					return errors.New("优惠券币种与订单不匹配")
				}
			}

			_, err = s.couponService.UseCouponWithPlanTx(tx, userId, *order.UserCouponId, orderId, order.PriceCents, "subscription", &order.PlanId)
			if err != nil {
				return fmt.Errorf("优惠券核销失败: %w", err)
			}
		}

		// 8. 更新订单状态为已支付
		now := common.GetTimestamp()
		order.Status = common.OrderStatusPaid
		order.PaidAt = &now
		order.PaymentChannel = paymentChannel
		if tradeNo != "" {
			order.TradeNo = &tradeNo
		}
		if err := model.UpdateSubscriptionOrderWithTx(tx, order); err != nil {
			return fmt.Errorf("更新订单状态失败: %w", err)
		}

		// 9. 激活订阅
		subscription, err = s.activateSubscriptionInTx(tx, userId, order)
		if err != nil {
			return fmt.Errorf("激活订阅失败: %w", err)
		}

		// 10. 更新订单关联的订阅ID
		order.SubscriptionId = &subscription.Id
		if err := tx.Model(order).Update("subscription_id", subscription.Id).Error; err != nil {
			return fmt.Errorf("更新订单订阅ID失败: %w", err)
		}

		// 11. 初始化使用量窗口
		if err := s.initializeUsageWindowsInTx(tx, subscription); err != nil {
			return fmt.Errorf("初始化使用量窗口失败: %w", err)
		}

		// 12. 记录账单（传递 quotaAmount 用于正确记录余额变动）
		bill, err := s.recordBillInTx(tx, userId, order, subscription, paymentChannel, quotaAmount)
		if err != nil {
			return fmt.Errorf("记录账单失败: %w", err)
		}
		billId = bill.Id

		// 12.1. 更新订单的账单ID
		order.BillId = &billId
		if err := model.UpdateSubscriptionOrderWithTx(tx, order); err != nil {
			return fmt.Errorf("更新订单账单ID失败: %w", err)
		}

		// 13. 记录审计日志
		return s.logOrderAction(tx, userId, orderId, "pay_order", map[string]interface{}{
			"payment_channel": paymentChannel,
			"trade_no":        tradeNo,
			"final_amount":    order.FinalPriceCents,
			"subscription_id": subscription.Id,
			"bill_id":         billId,
		})
	})

	if err != nil {
		result.ErrorMessage = err.Error()
		return result, err
	}

	if paymentChannel == common.PaymentChannelWallet && quotaAmount > 0 {
		_ = model.AdjustUserQuotaCache(int(userId), -quotaAmount)
	}

	if subscription != nil {
		cacheSvc := GetSubscriptionCacheService()
		_ = cacheSvc.InvalidateOnStatusChange(userId, subscription.Id)
		GetSubscriptionPriorityService().InvalidateUserSubscriptionCache(userId)
	}

	result.Success = true
	result.Order = order
	result.Subscription = subscription
	result.BillId = billId

	return result, nil
}

// processWalletPayment 处理余额支付
// 返回扣减的 Quota 金额（已转换单位）
func (s *SubscriptionOrderService) processWalletPayment(tx *gorm.DB, userId int64, order *model.SubscriptionOrder) (int64, error) {
	if order.FinalPriceCents <= 0 {
		return 0, nil // 免费订单无需扣款
	}

	// 获取用户当前余额（带行锁）
	user, err := model.GetUserByIdWithTx(tx, int(userId), true)
	if err != nil {
		return 0, fmt.Errorf("获取用户信息失败: %w", err)
	}

	// 获取套餐快照以确定货币类型
	planSnapshot, err := order.GetPlanSnapshotAsData()
	if err != nil {
		return 0, fmt.Errorf("获取套餐快照失败: %w", err)
	}

	// 参数校验：防止除零错误
	if operation_setting.USDExchangeRate <= 0 {
		return 0, fmt.Errorf("汇率配置异常: USDExchangeRate=%f", operation_setting.USDExchangeRate)
	}
	if common.QuotaPerUnit <= 0 {
		return 0, fmt.Errorf("配额单位配置异常: QuotaPerUnit=%f", common.QuotaPerUnit)
	}

	// 单位换算说明：
	// - user.Quota: 系统内部额度单位，QuotaPerUnit（500,000）= $1 USD
	// - order.FinalPriceCents: 订单实付金额（单位：分）
	// - planSnapshot.Currency: 套餐货币类型（USD 或 CNY）
	// - operation_setting.USDExchangeRate: USD 到 CNY 汇率（如 7.3 表示 1 USD = 7.3 CNY）
	//
	// 换算逻辑：
	// 1. USD 套餐：FinalPriceCents (USD cents) → USD → Quota
	//    quotaNeeded = FinalPriceCents / 100 * QuotaPerUnit
	// 2. CNY 套餐：FinalPriceCents (CNY cents) → CNY → USD → Quota
	//    quotaNeeded = FinalPriceCents / 100 / USDExchangeRate * QuotaPerUnit
	dFinalPriceCents := decimal.NewFromInt(order.FinalPriceCents)
	dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)

	var dQuotaNeeded decimal.Decimal
	currency := strings.ToUpper(planSnapshot.Currency)

	switch currency {
	case "USD":
		// USD 套餐：直接换算
		dQuotaNeeded = dFinalPriceCents.Div(decimal.NewFromInt(100)).Mul(dQuotaPerUnit)
	case "CNY":
		// CNY 套餐：需要汇率转换
		dExchangeRate := decimal.NewFromFloat(operation_setting.USDExchangeRate)
		dQuotaNeeded = dFinalPriceCents.Div(decimal.NewFromInt(100)).Div(dExchangeRate).Mul(dQuotaPerUnit)
	default:
		return 0, fmt.Errorf("不支持的货币类型: %s", planSnapshot.Currency)
	}

	// 向上取整，确保不会少扣款
	quotaNeeded := dQuotaNeeded.Ceil().IntPart()

	// 校验余额是否充足
	if int64(user.Quota) < quotaNeeded {
		return 0, errors.New(common.MsgInsufficientBalance)
	}

	// 扣减余额：使用 DecreaseUserQuota 保证 Redis 缓存一致性
	// 注意：DecreaseUserQuota 参数是 int，需要确保 quotaNeeded 在 int 范围内
	if quotaNeeded > int64(^uint(0)>>1) {
		return 0, fmt.Errorf("扣款金额超出系统限制: %d", quotaNeeded)
	}

	// 在事务内直接更新（DecreaseUserQuota 不支持事务，所以这里仍使用直接更新）
	// 但需要注意这可能导致缓存不一致，后续需要改进
	newQuota := int64(user.Quota) - quotaNeeded
	if err := tx.Model(&model.User{}).Where("id = ?", userId).Update("quota", newQuota).Error; err != nil {
		return 0, fmt.Errorf("扣减余额失败: %w", err)
	}

	return quotaNeeded, nil
}

// activateSubscriptionInTx 在事务中激活订阅
func (s *SubscriptionOrderService) activateSubscriptionInTx(tx *gorm.DB, userId int64, order *model.SubscriptionOrder) (*model.Subscription, error) {
	// 获取套餐快照
	planSnapshot, err := order.GetPlanSnapshotAsData()
	if err != nil {
		return nil, fmt.Errorf("解析套餐快照失败: %w", err)
	}

	// 计算订阅时间（从 BillingCycle 和 BillingCycleValue 计算天数）
	now := common.GetTimestamp()
	durationDays := s.calculateDurationDays(planSnapshot.BillingCycle, planSnapshot.BillingCycleValue)
	durationSeconds := int64(durationDays) * 24 * 60 * 60
	endAt := now + durationSeconds

	// 获取用户自动兜底配置（遵循用户设置 > 系统默认）
	systemDefault := common.GetAutoWalletFallbackDefault()
	autoWalletFallback := systemDefault
	user, _ := model.GetUserById(int(userId), false)
	if user != nil {
		autoWalletFallback, _ = user.GetEffectiveAutoWalletFallback(systemDefault)
	}

	// 创建订阅
	subscription := &model.Subscription{
		UserId:             userId,
		PlanId:             order.PlanId,
		OrderId:            &order.Id,
		Status:             common.SubscriptionStatusActive,
		StartAt:            now,
		EndAt:              endAt,
		Priority:           0, // 初始化为0，后续由优先级服务动态分配
		AutoWalletFallback: autoWalletFallback,
		RedeemOption:       common.RedeemOptionStack,
		CreatedAt:          now, // 确保优先级初始化时 created_at 可用于 tie-break
		UpdatedAt:          now,
	}

	// 设置渠道分组（使用订单内 plan_snapshot，避免套餐后续变更影响历史订单）
	if len(planSnapshot.ChannelGroups) > 0 {
		channelGroup := strings.Join(planSnapshot.ChannelGroups, ",")
		subscription.BindChannelGroup = &channelGroup
	}

	// 设置模型白名单缓存（使用订单内 plan_snapshot）
	if len(planSnapshot.ModelWhitelist) > 0 {
		_ = subscription.SetModelWhitelistFromSlice(planSnapshot.ModelWhitelist)
	}

	// 将套餐快照写入订阅元数据，确保套餐变更不影响已购买订阅
	metadata := map[string]interface{}{}
	if subscription.Metadata != nil {
		if existing, err := subscription.GetMetadataAsMap(); err == nil {
			metadata = existing
		}
	}
	metadata["plan_snapshot"] = planSnapshot
	if err := subscription.SetMetadataFromMap(metadata); err != nil {
		return nil, fmt.Errorf("设置订阅快照失败: %w", err)
	}

	// 设置优惠券ID（如果有）
	if order.UserCouponId != nil {
		userCoupon, _ := model.GetUserCouponById(*order.UserCouponId)
		if userCoupon != nil {
			subscription.CouponId = &userCoupon.CouponId
		}
	}

	// 使用优先级服务按 end_at 动态插入，分配正确的优先级
	priorityService := GetSubscriptionPriorityService()
	if err := priorityService.InitializeSubscriptionPriorityWithTx(tx, subscription); err != nil {
		return nil, fmt.Errorf("初始化订阅优先级失败: %w", err)
	}

	// 创建订阅记录
	if err := model.CreateSubscriptionWithTx(tx, subscription); err != nil {
		return nil, err
	}

	return subscription, nil
}

// initializeUsageWindowsInTx 在事务中初始化使用量窗口
// 窗口策略说明：
// - rolling: 滚动窗口，窗口在首次使用时创建，不在此处预创建
// - fixed/natural: 固定窗口，使用自然时间边界，在订阅激活时预创建
func (s *SubscriptionOrderService) initializeUsageWindowsInTx(tx *gorm.DB, subscription *model.Subscription) error {
	var limitSnapshots []model.PlanLimitSnapshotData
	if snapshot, err := subscription.GetPlanSnapshotFromMetadata(); err != nil {
		return err
	} else if snapshot != nil && len(snapshot.Limits) > 0 {
		limitSnapshots = snapshot.Limits
	}

	if len(limitSnapshots) == 0 {
		// 获取套餐限额配置（使用事务查询保证一致性）
		limits, err := model.GetPlanLimitsByPlanIdWithTx(tx, subscription.PlanId)
		if err != nil {
			return err
		}

		// 为每个限额周期创建初始窗口
		for _, limit := range limits {
			if !limit.Enabled {
				continue
			}

			// 根据窗口策略决定是否预创建窗口
			// rolling 策略：窗口在首次使用时按需创建，此处跳过
			// fixed/natural 策略：使用自然时间边界预创建窗口
			if limit.WindowStrategy == common.WindowStrategyRolling {
				// 滚动窗口不预创建，由 GetOrCreateActiveUsageWindow 在首次使用时创建
				continue
			}

			// 计算固定窗口边界（使用自然时间边界）
			windowStart, windowEnd := model.CalculateFixedWindowBounds(limit.Period, subscription.StartAt)

			// 创建使用量记录
			usage := &model.SubscriptionUsage{
				SubscriptionId: subscription.Id,
				Period:         limit.Period,
				WindowStart:    windowStart,
				WindowEnd:      windowEnd,
				UsedQuota:      0,
				LimitQuota:     limit.Quota,
			}

			if err := model.CreateSubscriptionUsageWithTx(tx, usage); err != nil {
				return err
			}
		}

		return nil
	}

	// 使用快照限额初始化窗口（避免套餐变更影响订阅）
	for _, limit := range limitSnapshots {
		strategy := limit.WindowStrategy
		if strategy == "" {
			strategy = common.WindowStrategyRolling
		}
		if strategy == common.WindowStrategyRolling {
			continue
		}

		windowStart, windowEnd := model.CalculateFixedWindowBounds(limit.Period, subscription.StartAt)
		usage := &model.SubscriptionUsage{
			SubscriptionId: subscription.Id,
			Period:         limit.Period,
			WindowStart:    windowStart,
			WindowEnd:      windowEnd,
			UsedQuota:      0,
			LimitQuota:     limit.Quota,
		}

		if err := model.CreateSubscriptionUsageWithTx(tx, usage); err != nil {
			return err
		}
	}

	return nil
}

// recordBillInTx 在事务中记录账单
// quotaAmount 是已转换为 Quota 单位的扣款金额
func (s *SubscriptionOrderService) recordBillInTx(tx *gorm.DB, userId int64, order *model.SubscriptionOrder, subscription *model.Subscription, paymentChannel string, quotaAmount int64) (*model.UserBill, error) {
	// 获取用户当前余额（用于记录账单前后余额）
	user, err := model.GetUserByIdWithTx(tx, int(userId), false)
	if err != nil {
		return nil, err
	}

	// 计算账单前余额（支付后的余额 + 支付金额（Quota单位）= 支付前余额）
	balanceBefore := int64(user.Quota) + quotaAmount
	balanceAfter := int64(user.Quota)

	// 生成描述
	description := "订阅套餐"
	if planSnapshot, err := order.GetPlanSnapshotAsData(); err == nil && planSnapshot != nil && planSnapshot.Name != "" {
		description = fmt.Sprintf("订阅套餐：%s", planSnapshot.Name)
	}

	// 计算实际支付金额：优先使用 Metadata 中的 actual_paid_cents（Stripe 含税场景）
	finalAmount := order.FinalPriceCents
	var taxFeeCents int64 = 0
	if order.Metadata != nil && *order.Metadata != "" {
		var metadata map[string]interface{}
		if err := json.Unmarshal([]byte(*order.Metadata), &metadata); err == nil {
			if actualPaid, ok := metadata["actual_paid_cents"]; ok {
				switch v := actualPaid.(type) {
				case float64:
					finalAmount = int64(v)
				case int64:
					finalAmount = v
				}
			}
			if taxFee, ok := metadata["tax_fee_cents"]; ok {
				switch v := taxFee.(type) {
				case float64:
					taxFeeCents = int64(v)
				case int64:
					taxFeeCents = v
				}
			}
		}
	}

	// 如果有税费，更新描述
	if taxFeeCents > 0 {
		description = fmt.Sprintf("%s（含税费 %.2f）", description, float64(taxFeeCents)/100)
	}

	sourceType := common.BillSourceTypeSubscriptionOrder
	bill := &model.UserBill{
		UserId:         userId,
		BillType:       common.BillTypeSubscription,
		Amount:         -quotaAmount, // 支出为负数（Quota 单位）
		BalanceBefore:  balanceBefore,
		BalanceAfter:   balanceAfter,
		SourceType:     &sourceType,
		SourceId:       &order.Id,
		PaymentChannel: &paymentChannel,
		Description:    &description,
		OriginalAmount: order.PriceCents,    // 原价（人民币分）
		DiscountAmount: order.DiscountCents, // 优惠（人民币分）
		FinalAmount:    finalAmount,         // 实付（含税，人民币分/美分）
		CreatedAt:      common.GetTimestamp(),
	}

	// 设置优惠券信息
	if order.UserCouponId != nil {
		bill.UserCouponId = order.UserCouponId
		userCoupon, _ := model.GetUserCouponById(*order.UserCouponId)
		if userCoupon != nil {
			bill.CouponId = &userCoupon.CouponId
		}
	}

	if err := model.CreateUserBillWithTx(tx, bill); err != nil {
		return nil, err
	}

	return bill, nil
}

// ===================== 订单取消方法 =====================

// CancelOrder 取消订单
func (s *SubscriptionOrderService) CancelOrder(userId int64, orderId int64, reason string) error {
	if userId == 0 {
		return errors.New("用户 ID 不能为空")
	}
	if orderId == 0 {
		return errors.New("订单 ID 不能为空")
	}

	return model.DB.Transaction(func(tx *gorm.DB) error {
		// 获取订单（带行锁）
		order, err := model.GetSubscriptionOrderByIdWithTx(tx, orderId, true)
		if err != nil {
			return err
		}

		// 校验订单归属
		if order.UserId != userId {
			return errors.New("订单不属于该用户")
		}

		// 校验订单状态
		if order.Status != common.OrderStatusPending {
			return fmt.Errorf("订单状态无效，无法取消: %s", order.Status)
		}

		// 取消订单
		if err := model.CancelOrderWithTx(tx, orderId); err != nil {
			return err
		}

		// 记录审计日志
		return s.logOrderAction(tx, userId, orderId, "cancel_order", map[string]interface{}{
			"reason": reason,
		})
	})
}

// ===================== 退款处理方法 =====================

// RefundOrderRequest 退款请求
type RefundOrderRequest struct {
	OrderId      int64  `json:"order_id"`
	UserId       int64  `json:"user_id"`
	RefundAmount int64  `json:"refund_amount"` // 退款金额（分），0表示全额退款
	RefundType   string `json:"refund_type"`   // manual / auto
	RefundReason string `json:"refund_reason"`
	OperatorId   int64  `json:"operator_id"` // 操作员ID（管理员退款时）
}

// RefundOrderResult 退款结果
type RefundOrderResult struct {
	Success      bool   `json:"success"`
	RefundAmount int64  `json:"refund_amount"`
	BillId       int64  `json:"bill_id"`
	ErrorMessage string `json:"error_message,omitempty"`
}

// RefundOrder 退款订单
func (s *SubscriptionOrderService) RefundOrder(req *RefundOrderRequest) (*RefundOrderResult, error) {
	result := &RefundOrderResult{
		Success: false,
	}

	if req.OrderId == 0 {
		result.ErrorMessage = "订单 ID 不能为空"
		return result, errors.New(result.ErrorMessage)
	}
	if req.UserId == 0 {
		result.ErrorMessage = "用户 ID 不能为空"
		return result, errors.New(result.ErrorMessage)
	}
	if req.RefundType != "manual" && req.RefundType != "auto" {
		result.ErrorMessage = "无效的退款类型"
		return result, errors.New(result.ErrorMessage)
	}

	var quotaRefund int64
	var orderPaymentChannel string // 保存订单支付渠道，用于事务外的缓存刷新判断
	var subscriptionId int64
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		// 获取订单（带行锁）
		order, err := model.GetSubscriptionOrderByIdWithTx(tx, req.OrderId, true)
		if err != nil {
			return err
		}

		// 保存支付渠道
		orderPaymentChannel = order.PaymentChannel

		// 校验订单归属
		if order.UserId != req.UserId {
			return errors.New("订单不属于该用户")
		}

		// 校验订单状态
		if order.Status != common.OrderStatusPaid {
			return fmt.Errorf("订单状态无效，无法退款: %s", order.Status)
		}

		// 计算退款金额
		refundAmount := req.RefundAmount
		if refundAmount <= 0 || refundAmount > order.FinalPriceCents {
			refundAmount = order.FinalPriceCents // 全额退款
		}
		result.RefundAmount = refundAmount

		// 更新订单状态
		if err := model.RefundOrderWithTx(tx, req.OrderId); err != nil {
			return err
		}

		// 取消关联的订阅（如果有）
		if order.SubscriptionId != nil {
			subscriptionId = *order.SubscriptionId
			if err := model.CancelSubscriptionWithTx(tx, subscriptionId); err != nil {
				return fmt.Errorf("取消订阅失败: %w", err)
			}
		}

		// 退还余额（需要单位转换：订单金额 → Quota 单位）
		// 重要：退款金额的换算必须与扣款时保持一致，确保金额对等
		quotaRefund = 0
		if refundAmount > 0 && order.PaymentChannel == common.PaymentChannelWallet {
			// 获取套餐快照以确定货币类型
			planSnapshot, err := order.GetPlanSnapshotAsData()
			if err != nil {
				return fmt.Errorf("获取套餐快照失败: %w", err)
			}

			// 参数校验：防止除零错误
			if operation_setting.USDExchangeRate <= 0 {
				return fmt.Errorf("汇率配置异常: USDExchangeRate=%f", operation_setting.USDExchangeRate)
			}
			if common.QuotaPerUnit <= 0 {
				return fmt.Errorf("配额单位配置异常: QuotaPerUnit=%f", common.QuotaPerUnit)
			}

			// 单位换算说明：
			// - refundAmount: 退款金额（单位：分，币种取决于 planSnapshot.Currency）
			// - planSnapshot.Currency: 套餐货币类型（USD 或 CNY）
			// 换算逻辑：
			// 1. USD 套餐：refundAmount (USD cents) → USD → Quota
			//    quotaRefund = refundAmount / 100 * QuotaPerUnit
			// 2. CNY 套餐：refundAmount (CNY cents) → CNY → USD → Quota
			//    quotaRefund = refundAmount / 100 / USDExchangeRate * QuotaPerUnit
			dRefundCents := decimal.NewFromInt(refundAmount)
			dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)

			var dQuotaRefund decimal.Decimal
			currency := strings.ToUpper(planSnapshot.Currency)

			switch currency {
			case "USD":
				// USD 套餐：直接换算
				dQuotaRefund = dRefundCents.Div(decimal.NewFromInt(100)).Mul(dQuotaPerUnit)
			case "CNY":
				// CNY 套餐：需要汇率转换
				dExchangeRate := decimal.NewFromFloat(operation_setting.USDExchangeRate)
				dQuotaRefund = dRefundCents.Div(decimal.NewFromInt(100)).Div(dExchangeRate).Mul(dQuotaPerUnit)
			default:
				return fmt.Errorf("不支持的货币类型: %s", planSnapshot.Currency)
			}

			// 退款向下取整（与扣款时的向上取整相对应，确保系统不会多退）
			quotaRefund = dQuotaRefund.Floor().IntPart()

			if err := tx.Model(&model.User{}).Where("id = ?", req.UserId).
				Update("quota", gorm.Expr("quota + ?", quotaRefund)).Error; err != nil {
				return fmt.Errorf("退还余额失败: %w", err)
			}
		}

		// 获取当前余额（用于账单）
		user, _ := model.GetUserByIdWithTx(tx, int(req.UserId), false)
		balanceAfter := int64(0)
		if user != nil {
			balanceAfter = int64(user.Quota)
		}

		// 记录退款账单（包含完整的优惠券和金额信息）
		sourceType := common.BillSourceTypeSubscriptionOrder
		description := fmt.Sprintf("订单退款：%s", req.RefundReason)
		bill := &model.UserBill{
			UserId:         req.UserId,
			BillType:       common.BillTypeRefund,
			Amount:         quotaRefund, // 退款为正数（Quota 单位）
			BalanceBefore:  balanceAfter - quotaRefund,
			BalanceAfter:   balanceAfter,
			SourceType:     &sourceType,
			SourceId:       &req.OrderId,
			RefundType:     &req.RefundType,
			Description:    &description,
			OperatorId:     &req.OperatorId,
			OriginalAmount: order.PriceCents,    // 原价（人民币分）
			DiscountAmount: order.DiscountCents, // 优惠（人民币分）
			FinalAmount:    refundAmount,        // 退款金额（分，支持部分退款）
			CreatedAt:      common.GetTimestamp(),
		}

		// 设置优惠券信息
		if order.UserCouponId != nil {
			bill.UserCouponId = order.UserCouponId
			userCoupon, _ := model.GetUserCouponById(*order.UserCouponId)
			if userCoupon != nil {
				bill.CouponId = &userCoupon.CouponId
			}
		}

		if err := model.CreateUserBillWithTx(tx, bill); err != nil {
			return err
		}
		result.BillId = bill.Id

		// 处理优惠券（不恢复，只记录）- 在同一事务内执行，保证原子性
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
				FinalAmount:    refundAmount,
				RefundType:     req.RefundType,
				RefundReason:   req.RefundReason,
				OperatorId:     req.OperatorId,
			}, couponResult); err != nil {
				// 如果优惠券处理失败，整个退款事务回滚
				return fmt.Errorf("处理优惠券退款失败: %w", err)
			}
		}

		// 记录审计日志
		operatorId := req.OperatorId
		if operatorId == 0 {
			operatorId = req.UserId
		}
		return s.logOrderActionWithOperator(tx, operatorId, req.OrderId, "refund_order", map[string]interface{}{
			"refund_amount": refundAmount,
			"refund_type":   req.RefundType,
			"refund_reason": req.RefundReason,
			"bill_id":       result.BillId,
		})
	})

	if err != nil {
		result.ErrorMessage = err.Error()
		return result, err
	}

	if quotaRefund > 0 && orderPaymentChannel == common.PaymentChannelWallet {
		_ = model.AdjustUserQuotaCache(int(req.UserId), quotaRefund)
	}

	if subscriptionId > 0 {
		cacheSvc := GetSubscriptionCacheService()
		_ = cacheSvc.InvalidateOnStatusChange(req.UserId, subscriptionId)
		GetSubscriptionPriorityService().InvalidateUserSubscriptionCache(req.UserId)
	}

	result.Success = true
	return result, nil
}

// ===================== 查询方法 =====================

// GetOrderById 根据ID获取订单
func (s *SubscriptionOrderService) GetOrderById(orderId int64) (*model.SubscriptionOrder, error) {
	return model.GetSubscriptionOrderById(orderId)
}

// GetOrdersByUser 获取用户订单列表
func (s *SubscriptionOrderService) GetOrdersByUser(userId int64, status string, page int, pageSize int) ([]*model.SubscriptionOrder, int64, error) {
	if userId == 0 {
		return nil, 0, errors.New("用户 ID 不能为空")
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	offset := (page - 1) * pageSize
	return model.GetSubscriptionOrdersByUser(userId, offset, pageSize, status)
}

// GetOrderByTradeNo 根据交易号获取订单
func (s *SubscriptionOrderService) GetOrderByTradeNo(tradeNo string) (*model.SubscriptionOrder, error) {
	if tradeNo == "" {
		return nil, errors.New("交易号不能为空")
	}
	return model.GetSubscriptionOrderByTradeNo(tradeNo)
}

// ===================== 辅助方法 =====================

// logOrderAction 记录订单操作审计日志
func (s *SubscriptionOrderService) logOrderAction(tx *gorm.DB, userId int64, orderId int64, action string, metadata map[string]interface{}) error {
	return s.logOrderActionWithOperator(tx, userId, orderId, action, metadata)
}

// logOrderActionWithOperator 记录订单操作审计日志（指定操作者）
func (s *SubscriptionOrderService) logOrderActionWithOperator(tx *gorm.DB, operatorId int64, orderId int64, action string, metadata map[string]interface{}) error {
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		metadataJSON = []byte("{}")
	}

	auditLog := &model.AuditLog{
		OperatorId: &operatorId,
		ObjectType: "subscription_order",
		ObjectId:   &orderId,
		Action:     action,
		Metadata:   string(metadataJSON),
		CreatedAt:  common.GetTimestamp(),
	}

	return model.CreateAuditLogWithTx(tx, auditLog)
}

// calculateDurationDays 根据计费周期计算天数
func (s *SubscriptionOrderService) calculateDurationDays(billingCycle string, billingCycleValue int) int {
	switch billingCycle {
	case common.BillingCycleMonthly:
		// 月付：30天 * 周期值
		return 30 * billingCycleValue
	case common.BillingCycleYearly:
		// 年付：365天 * 周期值
		return 365 * billingCycleValue
	case common.BillingCycleCustom:
		// 自定义：直接使用周期值作为天数
		if billingCycleValue > 0 {
			return billingCycleValue
		}
		return 30 // 默认30天
	default:
		return 30 // 默认30天
	}
}

// ExpireOverdueOrders 过期超时未支付的订单（供定时任务调用）
func (s *SubscriptionOrderService) ExpireOverdueOrders(batchSize int) (int, error) {
	if batchSize <= 0 {
		batchSize = 100
	}

	// 获取过期订单
	orders, err := model.GetExpiredPendingOrders(batchSize)
	if err != nil {
		return 0, err
	}

	expiredCount := 0
	for _, order := range orders {
		err := model.DB.Transaction(func(tx *gorm.DB) error {
			if err := model.ExpireOrderWithTx(tx, order.Id); err != nil {
				return err
			}
			return s.logOrderAction(tx, order.UserId, order.Id, "expire_order", map[string]interface{}{
				"reason": "订单超时未支付",
			})
		})
		if err == nil {
			expiredCount++
		}
	}

	return expiredCount, nil
}
