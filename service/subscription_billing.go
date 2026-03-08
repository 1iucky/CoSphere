package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
)

// ===================== Service 层 - 订阅计费服务 =====================
// 负责在请求流中决定扣费来源，实现"订阅优先，余额兜底"的计费策略

// SubscriptionBillingService 订阅计费服务
type SubscriptionBillingService struct {
	priorityService *SubscriptionPriorityService
	usageService    *SubscriptionUsageService
	cacheService    *SubscriptionCacheService
}

var (
	billingServiceInstance *SubscriptionBillingService
	billingServiceOnce     sync.Once
)

// GetSubscriptionBillingService 获取计费服务单例
func GetSubscriptionBillingService() *SubscriptionBillingService {
	billingServiceOnce.Do(func() {
		billingServiceInstance = &SubscriptionBillingService{
			priorityService: GetSubscriptionPriorityService(),
			usageService:    GetSubscriptionUsageService(),
			cacheService:    GetSubscriptionCacheService(),
		}
	})
	return billingServiceInstance
}

// ===================== 计费来源常量 =====================

const (
	// BillingSourceSubscription 订阅扣费
	BillingSourceSubscription = "subscription"
	// BillingSourceWallet 钱包余额扣费
	BillingSourceWallet = "wallet"
	// BillingSourceFallback 自动兜底扣费
	BillingSourceFallback = "fallback"
	// BillingSourceSkipped 跳过订阅扣费（订阅优先未启用）
	BillingSourceSkipped = "skipped"
)

// ===================== 计费上下文 =====================

// BillingContext 计费上下文（贯穿整个请求生命周期）
type BillingContext struct {
	UserId            int64                    `json:"user_id"`
	TokenId           int                      `json:"token_id"`
	ModelName         string                   `json:"model_name"`
	ChannelGroup      string                   `json:"channel_group"`
	Source            string                   `json:"source"`              // 计费来源
	SubscriptionId    int64                    `json:"subscription_id"`     // 使用的订阅 ID（如果有）
	PreConsumeContext *PreConsumeContext       `json:"pre_consume_context"` // 预扣上下文
	FallbackReason    string                   `json:"fallback_reason"`     // 兜底原因
	SkipReason        string                   `json:"skip_reason"`         // 跳过原因
	AutoWalletEnabled bool                     `json:"auto_wallet_enabled"` // 是否启用自动兜底
	Metadata          map[string]interface{}   `json:"metadata"`            // 额外元数据
	Candidates        []*CandidateSubscription `json:"-"`                   // 候选订阅（内部使用，避免重复筛选）
}

// CandidateSubscription 候选订阅（用于筛选）
type CandidateSubscription struct {
	Subscription *model.Subscription        `json:"subscription"`
	Plan         *model.SubscriptionPlan    `json:"plan"`
	Limits       []model.SubscriptionPlanLimit `json:"limits"`
	Priority     int `json:"priority"`
}

// BillingResult 计费结果
type BillingResult struct {
	Success        bool            `json:"success"`
	Source         string          `json:"source"`
	SubscriptionId int64           `json:"subscription_id,omitempty"`
	Context        *BillingContext `json:"context"`
	ErrorMessage   string          `json:"error_message,omitempty"`
}

// ===================== 核心计费方法 =====================

// SelectCandidateSubscriptions 筛选可用订阅
// 过滤条件：status=active && 模型匹配
// 渠道分组匹配仅在显式传入 channelGroup 时生效（与 token 分组无关）
// 检查 token.subscription_preferred（未启用订阅优先则跳过订阅计费）
func (s *SubscriptionBillingService) SelectCandidateSubscriptions(
	userId int64,
	modelName string,
	channelGroup string,
	tokenSubscriptionPreferred bool,
) ([]*CandidateSubscription, error) {
	if userId == 0 {
		return nil, errors.New("用户 ID 不能为空")
	}

	// 1. 如果 token 未启用订阅优先，直接返回空
	if !tokenSubscriptionPreferred {
		return nil, nil
	}

	// 2. 获取用户所有有效订阅（优先从缓存读取，按优先级排序）
	subscriptions, err := s.cacheService.GetActiveSubscriptions(userId)
	if err != nil {
		return nil, fmt.Errorf("获取用户订阅失败: %w", err)
	}

	if len(subscriptions) == 0 {
		return nil, nil
	}

	// 3. 筛选匹配的订阅
	var candidates []*CandidateSubscription
	for _, sub := range subscriptions {
		// 检查订阅状态
		if sub.Status != common.SubscriptionStatusActive {
			continue
		}

		// 检查是否已过期
		if sub.IsExpired() {
			continue
		}

		// 检查模型是否在白名单中
		canUseModel, err := sub.CanUseModel(modelName)
		if err != nil {
			continue
		}
		if !canUseModel {
			continue
		}

		// 检查渠道分组是否匹配（仅在显式传入 channelGroup 时生效）
		if channelGroup != "" && !s.matchChannelGroup(sub, channelGroup) {
			continue
		}

		// 获取套餐限额配置（优先使用订阅快照，确保套餐变更不影响历史订阅）
		var limits []model.SubscriptionPlanLimit
		if snapshot, err := sub.GetPlanSnapshotFromMetadata(); err == nil && snapshot != nil {
			limits = convertSnapshotLimits(snapshot.Limits)
		}
		if len(limits) == 0 {
			if sub.Plan != nil {
				limits = sub.Plan.GetEnabledLimits()
				if len(limits) == 0 {
					if dbLimits, err := model.GetPlanLimitsByPlanId(sub.PlanId); err == nil {
						limits = filterEnabledPlanLimits(dbLimits)
					}
				}
			} else {
				// 从数据库获取
				if dbLimits, err := model.GetPlanLimitsByPlanId(sub.PlanId); err == nil {
					limits = filterEnabledPlanLimits(dbLimits)
				}
			}
		}

		if len(limits) == 0 {
			continue
		}

		candidates = append(candidates, &CandidateSubscription{
			Subscription: sub,
			Plan:         sub.Plan,
			Limits:       limits,
			Priority:     sub.Priority,
		})
	}

	return candidates, nil
}

// matchChannelGroup 检查订阅是否匹配渠道分组
// 支持多分组匹配：渠道分组用逗号分隔，订阅绑定多个分组时，任一匹配即可
func (s *SubscriptionBillingService) matchChannelGroup(sub *model.Subscription, channelGroup string) bool {
	// 如果订阅没有绑定渠道分组，则匹配所有
	if sub.BindChannelGroup == nil || *sub.BindChannelGroup == "" {
		return true
	}

	// 如果请求没有指定分组，则匹配所有
	if channelGroup == "" {
		return true
	}

	// 解析订阅绑定的渠道分组（支持多分组，用逗号分隔）
	subGroups := s.parseChannelGroups(*sub.BindChannelGroup)

	// 解析请求的渠道分组（支持多分组，用逗号分隔）
	reqGroups := s.parseChannelGroups(channelGroup)

	// 检查是否有交集：任一订阅分组与任一请求分组匹配即可
	for _, subGroup := range subGroups {
		for _, reqGroup := range reqGroups {
			if subGroup == reqGroup {
				return true
			}
		}
	}

	return false
}

// parseChannelGroups 解析渠道分组字符串，返回分组数组
func (s *SubscriptionBillingService) parseChannelGroups(groupStr string) []string {
	if groupStr == "" {
		return []string{}
	}

	// TODO: 使用更标准的解析方式，支持空格处理
	// 目前简单按逗号分隔
	groups := strings.Split(groupStr, ",")
	for i, group := range groups {
		groups[i] = strings.TrimSpace(group)
	}

	return groups
}

func filterEnabledPlanLimits(limits []model.SubscriptionPlanLimit) []model.SubscriptionPlanLimit {
	if len(limits) == 0 {
		return nil
	}

	enabled := make([]model.SubscriptionPlanLimit, 0, len(limits))
	for _, limit := range limits {
		if limit.Enabled {
			enabled = append(enabled, limit)
		}
	}
	return enabled
}

func convertSnapshotLimits(limits []model.PlanLimitSnapshotData) []model.SubscriptionPlanLimit {
	if len(limits) == 0 {
		return nil
	}

	converted := make([]model.SubscriptionPlanLimit, 0, len(limits))
	for _, limit := range limits {
		converted = append(converted, model.SubscriptionPlanLimit{
			Period:         limit.Period,
			Quota:          limit.Quota,
			Unit:           limit.Unit,
			Enabled:        true,
			WindowStrategy: limit.WindowStrategy,
		})
	}
	return converted
}

// TryDeductFromSubscriptions 按优先级遍历订阅尝试预扣
// 返回成功的订阅 ID 和预扣上下文
func (s *SubscriptionBillingService) TryDeductFromSubscriptions(
	candidates []*CandidateSubscription,
	amount int64,
) (*BillingContext, error) {
	if len(candidates) == 0 {
		return nil, errors.New("没有可用的订阅")
	}

	if amount <= 0 {
		return nil, errors.New("预扣额度必须为正数")
	}

	// 遍历候选订阅，按优先级顺序

	// 按优先级遍历（candidates 已按优先级排序）
	for _, candidate := range candidates {
		sub := candidate.Subscription

		// 构建周期配置映射（支持每周期不同策略）
		periodConfigs := make(map[string]PeriodConfig)
		for _, limit := range candidate.Limits {
			if limit.Enabled {
				strategy := limit.WindowStrategy
				if strategy == "" {
					strategy = common.WindowStrategyRolling
				}
				periodConfigs[limit.Period] = PeriodConfig{
					LimitQuota: limit.Quota,
					Strategy:   strategy,
				}
			}
		}

		if len(periodConfigs) == 0 {
			// 无启用限额，跳过此订阅
			continue
		}

		// 尛试预扣（使用支持每周期不同策略的方法）
		preConsumeCtx, err := s.usageService.TryPreConsumeWithStrategies(sub.Id, amount, periodConfigs)
		if err != nil {
			// 额度不足则尝试下一个订阅，系统异常则直接返回
			if apiErr, ok := err.(*types.NewAPIError); ok {
				if apiErr.GetErrorCode() == types.ErrorCodeUsageQuotaExceeded {
					// 额度不足，尝试下一个订阅
					// details 应该反映"决策订阅"的配置，在 TryBilling 中统一构造
					continue
				}
				return nil, apiErr
			}
			return nil, err
		}

		bindGroup := ""
		if sub.BindChannelGroup != nil {
			bindGroup = strings.TrimSpace(*sub.BindChannelGroup)
		}

		// 预扣成功
		return &BillingContext{
			UserId:            sub.UserId,
			Source:            BillingSourceSubscription,
			SubscriptionId:    sub.Id,
			ChannelGroup:      bindGroup,
			PreConsumeContext: preConsumeCtx,
			AutoWalletEnabled: sub.AutoWalletFallback,
		}, nil
	}

	// 所有订阅都预扣失败
	// 简化错误返回：不在此处构造 details
	// details 应该反映"决策订阅"的配置，由 TryBilling 统一构造
	errOpts := []types.NewAPIErrorOptions{
		types.ErrOptionWithHint(common.MsgSubscriptionLimitReachedHint),
	}
	return nil, types.NewErrorWithStatusCode(
		errors.New(common.MsgSubscriptionLimitReached),
		types.ErrorCodeSubscriptionLimitReached,
		http.StatusTooManyRequests,
		errOpts...,
	)
}

// ===================== 自动兜底处理 =====================

// CheckAutoWalletFallback 检查是否启用自动兜底
// 优先级：订阅级 > 用户级 > 系统级
// 当 subscriptionId > 0 且订阅存在时，使用订阅级设置
// 否则按"用户设置 > 系统默认"继承
func (s *SubscriptionBillingService) CheckAutoWalletFallback(userId int64, subscriptionId int64) (bool, error) {
	// 1. 优先检查订阅级别设置（当有 subscriptionId 时）
	if subscriptionId > 0 {
		sub, err := model.GetSubscriptionById(subscriptionId)
		if err == nil && sub != nil {
			// 订阅存在，使用订阅级别的自动兜底设置
			return sub.AutoWalletFallback, nil
		}
		// 订阅不存在或查询失败，继续检查用户级和系统级
	}

	// 2. 检查用户级别设置
	userSettings, err := model.GetUserSetting(int(userId), false)
	if err == nil {
		// 检查用户是否有显式设置自动兜底
		if userSettings.AutoWalletFallbackExplicit != nil {
			// 用户有显式设置，使用用户设置
			return userSettings.AutoWalletFallback, nil
		}
	}

	// 3. 用户未显式设置时，返回系统默认值
	return common.GetAutoWalletFallbackDefault(), nil
}

// FallbackToWallet 失败时切换到余额扣费
// 返回更新后的计费上下文
func (s *SubscriptionBillingService) FallbackToWallet(ctx *BillingContext, reason string) *BillingContext {
	if ctx == nil {
		ctx = &BillingContext{}
	}

	ctx.Source = BillingSourceFallback
	ctx.FallbackReason = reason

	return ctx
}

// RecordFallbackEvent 记录 fallback 事件（审计日志）
func (s *SubscriptionBillingService) RecordFallbackEvent(ctx *BillingContext, relayInfo *relaycommon.RelayInfo) error {
	if ctx == nil {
		return errors.New("计费上下文不能为空")
	}

	metadata := map[string]interface{}{
		"source":           ctx.Source,
		"subscription_id":  ctx.SubscriptionId,
		"fallback_reason":  ctx.FallbackReason,
		"model_name":       ctx.ModelName,
		"channel_group":    ctx.ChannelGroup,
		"token_id":         ctx.TokenId,
	}

	if relayInfo != nil {
		metadata["user_id"] = relayInfo.UserId
		metadata["origin_model"] = relayInfo.OriginModelName
	}

	metadataJSON, _ := json.Marshal(metadata)

	auditLog := &model.AuditLog{
		OperatorId: &ctx.UserId,
		ObjectType: "subscription_billing",
		ObjectId:   &ctx.SubscriptionId,
		Action:     "fallback",
		Metadata:   string(metadataJSON),
		CreatedAt:  common.GetTimestamp(),
	}

	return model.CreateAuditLog(auditLog)
}

// ===================== 订阅优先关闭跳过逻辑 =====================

// ShouldSkipSubscriptionBilling 检查是否应跳过订阅扣费
// 当 token 未启用订阅优先（subscription_preferred = false）时，跳过订阅计费
func (s *SubscriptionBillingService) ShouldSkipSubscriptionBilling(tokenSubscriptionPreferred bool) bool {
	// 如果 token 未启用订阅优先，则跳过订阅计费
	return !tokenSubscriptionPreferred
}

// ReturnSkipReason 返回跳过订阅扣费的原因
func (s *SubscriptionBillingService) ReturnSkipReason(tokenId int, reason string) string {
	if reason != "" {
		return reason
	}
	return "subscription_preferred_disabled"
}

// ===================== 计费入口方法 =====================

// SelectCandidate 选择计费候选（PreConsumeQuota 调用的入口）
// 实现设计文档中的 subscription_billing.SelectCandidate 逻辑
func (s *SubscriptionBillingService) SelectCandidate(
	userId int64,
	modelName string,
	channelGroup string,
	relayInfo *relaycommon.RelayInfo,
) (*BillingContext, error) {
	ctx := &BillingContext{
		UserId:       userId,
		ModelName:    modelName,
		ChannelGroup: channelGroup,
		Metadata:     make(map[string]interface{}),
	}

	// 从 relayInfo 获取 token 的订阅偏好设置
	tokenSubscriptionPreferred := false // 默认不启用订阅优先
	if relayInfo != nil {
		ctx.TokenId = relayInfo.TokenId
		tokenSubscriptionPreferred = relayInfo.TokenSubscriptionPreferred
	}

	// 1. 检查是否应跳过订阅计费（token 未启用订阅优先）
	if s.ShouldSkipSubscriptionBilling(tokenSubscriptionPreferred) {
		ctx.Source = BillingSourceSkipped
		ctx.SkipReason = s.ReturnSkipReason(ctx.TokenId, "subscription_preferred_disabled")
		return ctx, nil
	}

	// 2. 筛选候选订阅
	candidates, err := s.SelectCandidateSubscriptions(userId, modelName, channelGroup, tokenSubscriptionPreferred)
	if err != nil {
		return ctx, err
	}

	if len(candidates) == 0 {
		// 没有可用订阅，使用钱包
		ctx.Source = BillingSourceWallet
		return ctx, nil
	}

	// 3. 存储候选订阅到上下文（避免 TryBilling 重复筛选）
	ctx.Candidates = candidates
	ctx.Metadata["candidate_count"] = len(candidates)

	return ctx, nil
}

// TryBilling 尝试计费（在 PreConsumeQuota 中调用）
func (s *SubscriptionBillingService) TryBilling(
	ctx *BillingContext,
	amount int64,
	relayInfo *relaycommon.RelayInfo,
) (*BillingResult, error) {
	result := &BillingResult{
		Success: false,
		Context: ctx,
	}

	if ctx == nil {
		result.ErrorMessage = "计费上下文不能为空"
		return result, errors.New(result.ErrorMessage)
	}

	// 如果已经跳过或使用钱包，直接返回
	if ctx.Source == BillingSourceSkipped || ctx.Source == BillingSourceWallet {
		result.Success = true
		result.Source = ctx.Source
		return result, nil
	}

	// 优先使用上下文中的候选订阅（避免重复筛选）
	candidates := ctx.Candidates
	if len(candidates) == 0 {
		// 如果上下文中没有候选订阅，回退到重新筛选（兼容旧调用方式）
		tokenSubscriptionPreferred := false
		if relayInfo != nil {
			tokenSubscriptionPreferred = relayInfo.TokenSubscriptionPreferred
		}
		var err error
		candidates, err = s.SelectCandidateSubscriptions(ctx.UserId, ctx.ModelName, ctx.ChannelGroup, tokenSubscriptionPreferred)
		if err != nil {
			result.ErrorMessage = err.Error()
			return result, err
		}
	}

	if len(candidates) == 0 {
		ctx.Source = BillingSourceWallet
		result.Success = true
		result.Source = BillingSourceWallet
		return result, nil
	}

	// 尝试从订阅预扣
	billingCtx, err := s.TryDeductFromSubscriptions(candidates, amount)
	if err != nil {
		// 判断是否为订阅额度不足错误（仅此情况才允许兜底）
		isLimitReachedError := false
		if apiErr, ok := err.(*types.NewAPIError); ok {
			isLimitReachedError = apiErr.GetErrorCode() == types.ErrorCodeSubscriptionLimitReached
		}

		// 只有订阅额度不足才检查自动兜底，系统/DB 错误直接返回
		if isLimitReachedError {
			autoWallet := false
			var decisionSub *model.Subscription
			if len(candidates) > 0 && candidates[0].Subscription != nil {
				// 使用最高优先级订阅作为决策订阅
				decisionSub = candidates[0].Subscription
				autoWallet = decisionSub.AutoWalletFallback
			} else {
				autoWallet, _ = s.CheckAutoWalletFallback(ctx.UserId, 0)
			}
			if autoWallet {
				// 启用兜底，切换到钱包
				ctx = s.FallbackToWallet(ctx, err.Error())
				_ = s.RecordFallbackEvent(ctx, relayInfo)
				result.Success = true
				result.Source = BillingSourceFallback
				result.Context = ctx
				return result, nil
			}

			// 未启用兜底：构造 details，反映"决策订阅"的配置
			// 只有 auto_wallet_fallback=false 时才会返回此错误
			if decisionSub != nil {
				// 找到决策订阅的首个启用的限额配置
				var enabledLimit *model.SubscriptionPlanLimit
				for _, limit := range candidates[0].Limits {
					if limit.Enabled {
						enabledLimit = &limit
						break
					}
				}

				// 构造 details：所有字段都来自决策订阅，保持语义一致性
				details := map[string]interface{}{
					"subscription_id":      decisionSub.Id,
					"requested":            amount,
					"auto_wallet_fallback": decisionSub.AutoWalletFallback, // 应为 false（因为未启用兜底）
				}

				if enabledLimit != nil {
					// 有启用限额：填充限额详情
					details["period"] = enabledLimit.Period
					details["limit_quota"] = enabledLimit.Quota
					details["used_quota"] = int64(0) // TryPreConsumeWithStrategies 未返回 used_quota
				} else {
					// 无启用限额：使用占位字段
					details["period"] = "none" // 明确标识为"无启用限额"
					details["limit_quota"] = int64(0)
					details["used_quota"] = int64(0)
				}

				// 构造带 details 的错误
				if apiErr, ok := err.(*types.NewAPIError); ok && apiErr.Details == nil {
					apiErr.Details = details
					err = apiErr
				}
			}
		}

		// 未启用兜底或非额度不足错误，返回错误
		result.ErrorMessage = err.Error()
		return result, err
	}

	// 预扣成功
	if billingCtx != nil {
		billingCtx.ModelName = ctx.ModelName
		billingCtx.TokenId = ctx.TokenId
		if billingCtx.ChannelGroup == "" {
			billingCtx.ChannelGroup = ctx.ChannelGroup
		}
		if billingCtx.Metadata == nil {
			billingCtx.Metadata = ctx.Metadata
		}
	}
	result.Success = true
	result.Source = BillingSourceSubscription
	result.SubscriptionId = billingCtx.SubscriptionId
	result.Context = billingCtx
	return result, nil
}

// PostBilling 计费后处理（在 PostConsumeQuota 中调用）
// 根据真实使用量修正 usage
func (s *SubscriptionBillingService) PostBilling(
	ctx *BillingContext,
	preConsumedAmount int64,
	actualAmount int64,
	relayInfo *relaycommon.RelayInfo,
) error {
	if ctx == nil {
		return errors.New("计费上下文不能为空")
	}

	// 如果不是订阅计费，无需处理
	if ctx.Source != BillingSourceSubscription && ctx.Source != BillingSourceFallback {
		return nil
	}

	// 如果没有预扣上下文，无需处理
	if ctx.PreConsumeContext == nil {
		return nil
	}

	// 计算差额
	diff := actualAmount - preConsumedAmount
	if diff == 0 {
		// 无差额，预扣即为最终使用量，无需调整
		return nil
	}

	// 统一调整：AdjustUsage 会根据 actualAmount 自动计算差额并处理补扣或返还
	// diff > 0: 补扣（实际消耗大于预扣）
	// diff < 0: 返还（实际消耗小于预扣）
	return s.usageService.AdjustUsage(ctx.PreConsumeContext.ContextId, actualAmount)
}

// ===================== 性能监控 =====================

// BillingPerformanceStats 计费性能统计
type BillingPerformanceStats struct {
	TotalCount        int64              `json:"total_count"`          // 总请求数
	TotalDurationMs   float64            `json:"total_duration_ms"`    // 总耗时（毫秒）
	AvgDurationMs     float64            `json:"avg_duration_ms"`      // 平均耗时（毫秒）
	MaxDurationMs     float64            `json:"max_duration_ms"`      // 最大耗时（毫秒）
	MinDurationMs     float64            `json:"min_duration_ms"`      // 最小耗时（毫秒）
	Over5msCount      int64              `json:"over_5ms_count"`       // 超过 5ms 的请求数
	Over10msCount     int64              `json:"over_10ms_count"`      // 超过 10ms 的请求数
	BySource          map[string]int64   `json:"by_source"`            // 按来源统计
	BySourceDuration  map[string]float64 `json:"by_source_duration"`   // 按来源累计耗时
}

var (
	billingPerfStats     BillingPerformanceStats
	billingPerfStatsLock sync.RWMutex
)

func init() {
	billingPerfStats = BillingPerformanceStats{
		MinDurationMs:    -1, // -1 表示未初始化
		BySource:         make(map[string]int64),
		BySourceDuration: make(map[string]float64),
	}
}

// RecordBillingDuration 记录计费耗时
// 用于收集性能统计数据，便于监控和优化
func (s *SubscriptionBillingService) RecordBillingDuration(durationMs float64, source string) {
	billingPerfStatsLock.Lock()
	defer billingPerfStatsLock.Unlock()

	billingPerfStats.TotalCount++
	billingPerfStats.TotalDurationMs += durationMs

	// 更新最大/最小值
	if durationMs > billingPerfStats.MaxDurationMs {
		billingPerfStats.MaxDurationMs = durationMs
	}
	if billingPerfStats.MinDurationMs < 0 || durationMs < billingPerfStats.MinDurationMs {
		billingPerfStats.MinDurationMs = durationMs
	}

	// 统计超时请求
	if durationMs > 5 {
		billingPerfStats.Over5msCount++
	}
	if durationMs > 10 {
		billingPerfStats.Over10msCount++
	}

	// 按来源统计
	billingPerfStats.BySource[source]++
	billingPerfStats.BySourceDuration[source] += durationMs
}

// GetBillingPerformanceStats 获取计费性能统计
func (s *SubscriptionBillingService) GetBillingPerformanceStats() BillingPerformanceStats {
	billingPerfStatsLock.RLock()
	defer billingPerfStatsLock.RUnlock()

	stats := BillingPerformanceStats{
		TotalCount:       billingPerfStats.TotalCount,
		TotalDurationMs:  billingPerfStats.TotalDurationMs,
		MaxDurationMs:    billingPerfStats.MaxDurationMs,
		MinDurationMs:    billingPerfStats.MinDurationMs,
		Over5msCount:     billingPerfStats.Over5msCount,
		Over10msCount:    billingPerfStats.Over10msCount,
		BySource:         make(map[string]int64),
		BySourceDuration: make(map[string]float64),
	}

	// 计算平均耗时
	if stats.TotalCount > 0 {
		stats.AvgDurationMs = stats.TotalDurationMs / float64(stats.TotalCount)
	}

	// 复制 map 数据
	for k, v := range billingPerfStats.BySource {
		stats.BySource[k] = v
	}
	for k, v := range billingPerfStats.BySourceDuration {
		stats.BySourceDuration[k] = v
	}

	return stats
}

// ResetBillingPerformanceStats 重置计费性能统计（用于测试或定期重置）
func (s *SubscriptionBillingService) ResetBillingPerformanceStats() {
	billingPerfStatsLock.Lock()
	defer billingPerfStatsLock.Unlock()

	billingPerfStats = BillingPerformanceStats{
		MinDurationMs:    -1,
		BySource:         make(map[string]int64),
		BySourceDuration: make(map[string]float64),
	}
}
