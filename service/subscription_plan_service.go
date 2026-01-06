package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
)

// SubscriptionPlanService 套餐管理服务
type SubscriptionPlanService struct{}

var planService = &SubscriptionPlanService{}

// GetSubscriptionPlanService 获取套餐服务实例
func GetSubscriptionPlanService() *SubscriptionPlanService {
	return planService
}

// ===================== 套餐管理方法 =====================

// CreatePlan 创建套餐（含业务验证）
func (s *SubscriptionPlanService) CreatePlan(req *dto.SubscriptionPlanRequest) (*model.SubscriptionPlan, error) {
	// 1. 构建套餐模型
	plan := s.buildPlanFromRequest(req)

	// 2. 业务验证
	if err := s.validatePlanBusiness(plan); err != nil {
		return nil, err
	}

	// 3. 创建套餐（Model 层已包含事务）
	if err := model.CreateSubscriptionPlan(plan); err != nil {
		return nil, fmt.Errorf("创建套餐失败: %w", err)
	}

	// 4. 触发事件通知
	s.notifyPlanChanged(plan.Id, "created")

	return plan, nil
}

// UpdatePlan 更新套餐（含业务验证）
func (s *SubscriptionPlanService) UpdatePlan(id int64, req *dto.SubscriptionPlanRequest) (*model.SubscriptionPlan, error) {
	// 1. 检查套餐是否存在
	existing, err := model.GetSubscriptionPlanById(id)
	if err != nil {
		return nil, fmt.Errorf("套餐不存在: %w", err)
	}

	// 2. 构建更新后的套餐模型
	plan := s.buildPlanFromRequest(req)
	plan.Id = id
	plan.CreatedAt = existing.CreatedAt // 保留创建时间

	// 3. 业务验证
	if err := s.validatePlanBusiness(plan); err != nil {
		return nil, err
	}

	// 4. 更新套餐（Model 层已包含事务）
	if err := model.UpdateSubscriptionPlan(plan); err != nil {
		return nil, fmt.Errorf("更新套餐失败: %w", err)
	}

	// 5. 触发事件通知
	s.notifyPlanChanged(plan.Id, "updated")

	return plan, nil
}

// GetPlan 获取套餐详情
func (s *SubscriptionPlanService) GetPlan(id int64) (*model.SubscriptionPlan, error) {
	plan, err := model.GetSubscriptionPlanById(id)
	if err != nil {
		return nil, fmt.Errorf("获取套餐失败: %w", err)
	}
	return plan, nil
}

// GetPlanBySKU 根据 SKU 获取套餐
func (s *SubscriptionPlanService) GetPlanBySKU(sku string) (*model.SubscriptionPlan, error) {
	plan, err := model.GetSubscriptionPlanBySKU(sku)
	if err != nil {
		return nil, fmt.Errorf("获取套餐失败: %w", err)
	}
	return plan, nil
}

// ListPlans 列表查询（带筛选和分页）
func (s *SubscriptionPlanService) ListPlans(req *dto.SubscriptionPlanListRequest) (plans []*model.SubscriptionPlan, total int64, err error) {
	// 设置默认分页参数
	page := req.Page
	if page < 1 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}

	startIdx := (page - 1) * pageSize

	// 使用支持多条件筛选的查询方法
	plans, total, err = model.GetSubscriptionPlansWithFilters(
		startIdx,
		pageSize,
		req.Status,
		req.BillingCycle,
		req.MinPrice,
		req.MaxPrice,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("查询套餐列表失败: %w", err)
	}

	return plans, total, nil
}

// SearchPlans 搜索套餐
func (s *SubscriptionPlanService) SearchPlans(keyword string, page int, pageSize int) (plans []*model.SubscriptionPlan, total int64, err error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}

	startIdx := (page - 1) * pageSize
	plans, total, err = model.SearchSubscriptionPlans(keyword, startIdx, pageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("搜索套餐失败: %w", err)
	}

	return plans, total, nil
}

// GetActiveEnsPlans 获取所有可用套餐（用户端）
func (s *SubscriptionPlanService) GetActivePlans() ([]*model.SubscriptionPlan, error) {
	plans, err := model.GetActiveSubscriptionPlans()
	if err != nil {
		return nil, fmt.Errorf("获取可用套餐失败: %w", err)
	}
	return plans, nil
}

// PublishPlan 发布套餐
func (s *SubscriptionPlanService) PublishPlan(id int64) error {
	// 1. 发布套餐（Model 层已包含验证）
	if err := model.PublishSubscriptionPlan(id); err != nil {
		return fmt.Errorf("发布套餐失败: %w", err)
	}

	// 2. 触发事件通知
	s.notifyPlanChanged(id, "published")

	return nil
}

// UnpublishPlan 下架套餐
func (s *SubscriptionPlanService) UnpublishPlan(id int64) error {
	// 1. 下架套餐
	if err := model.UnpublishSubscriptionPlan(id); err != nil {
		return fmt.Errorf("下架套餐失败: %w", err)
	}

	// 2. 触发事件通知
	s.notifyPlanChanged(id, "unpublished")

	return nil
}

// ArchivePlan 归档套餐
func (s *SubscriptionPlanService) ArchivePlan(id int64) error {
	return s.UnpublishPlan(id)
}

// DeletePlan 删除套餐
func (s *SubscriptionPlanService) DeletePlan(id int64) error {
	// 1. 删除套餐（Model 层已包含关联检查）
	if err := model.DeleteSubscriptionPlan(id); err != nil {
		return fmt.Errorf("删除套餐失败: %w", err)
	}

	// 2. 触发事件通知
	s.notifyPlanChanged(id, "deleted")

	return nil
}

// ===================== 业务验证方法 =====================

// validatePlanBusiness 业务层验证
func (s *SubscriptionPlanService) validatePlanBusiness(plan *model.SubscriptionPlan) error {
	// 1. 验证模型白名单
	if plan.ModelWhitelist != nil && *plan.ModelWhitelist != "" {
		models, err := plan.GetModelWhitelistAsSlice()
		if err != nil {
			return fmt.Errorf("模型白名单格式错误: %w", err)
		}
		if err := s.ValidateModelWhitelist(models); err != nil {
			return err
		}
	}

	// 2. 验证渠道分组
	if plan.ChannelGroups != nil && *plan.ChannelGroups != "" {
		groups, err := plan.GetChannelGroupsAsSlice()
		if err != nil {
			return fmt.Errorf("渠道分组格式错误: %w", err)
		}
		if err := s.ValidateChannelGroups(groups); err != nil {
			return err
		}
	}

	// 3. 验证价格区间 - 价格必须大于 0
	if plan.PriceCents <= 0 {
		return types.NewErrorWithStatusCode(
			errors.New("套餐价格必须大于 0"),
			types.ErrorCodePlanPriceInvalid,
			http.StatusBadRequest,
		)
	}

	// 4. 验证时间范围
	if plan.StartAt != nil && plan.EndAt != nil && *plan.StartAt >= *plan.EndAt {
		return types.NewErrorWithStatusCode(
			errors.New("套餐结束时间必须晚于开始时间"),
			types.ErrorCodePlanTimeConflict,
			http.StatusBadRequest,
		)
	}

	return nil
}

// ValidateModelWhitelist 校验模型列表有效性（严格模式：不存在或未启用则报错）
func (s *SubscriptionPlanService) ValidateModelWhitelist(models []string) error {
	if len(models) == 0 {
		return nil
	}

	// 获取系统中所有可用的模型（包含 status 字段）
	allModels, err := model.GetAllModels(0, 10000)
	if err != nil {
		// 获取模型列表失败是严重错误，直接返回
		return fmt.Errorf("获取系统模型列表失败: %w", err)
	}

	// 构建模型名称映射（仅包含已启用的模型，status = 1）
	enabledModelMap := make(map[string]bool)
	disabledModelMap := make(map[string]bool)
	for _, m := range allModels {
		if m.Status == common.ModelStatusEnabled {
			enabledModelMap[m.ModelName] = true
		} else {
			disabledModelMap[m.ModelName] = true
		}
	}

	// 检查每个模型是否存在且已启用
	var invalidModels []string
	var disabledModels []string
	for _, modelName := range models {
		modelName = strings.TrimSpace(modelName)
		if modelName == "" {
			continue
		}
		if disabledModelMap[modelName] {
			// 模型存在但已禁用
			disabledModels = append(disabledModels, modelName)
		} else if !enabledModelMap[modelName] {
			// 模型不存在
			invalidModels = append(invalidModels, modelName)
		}
	}

	// 优先报告禁用的模型（比不存在更严重）
	if len(disabledModels) > 0 {
		return types.NewErrorWithStatusCode(
			fmt.Errorf("以下模型已被禁用，无法添加到套餐: %v", disabledModels),
			types.ErrorCodePlanModelInvalid,
			http.StatusBadRequest,
		)
	}

	if len(invalidModels) > 0 {
		// 严格模式：不存在的模型返回错误
		return types.NewErrorWithStatusCode(
			fmt.Errorf("以下模型在系统中不存在: %v", invalidModels),
			types.ErrorCodePlanModelInvalid,
			http.StatusBadRequest,
		)
	}

	return nil
}

// ValidateChannelGroups 校验渠道分组有效性（严格模式：分组必须来自已启用的渠道）
func (s *SubscriptionPlanService) ValidateChannelGroups(groups []string) error {
	if len(groups) == 0 {
		return nil
	}

	// 获取系统中所有渠道（包含 status 字段）
	channels, err := model.GetAllChannels(0, 10000, false, false)
	if err != nil {
		// 获取渠道列表失败是严重错误，直接返回
		return fmt.Errorf("获取渠道列表失败: %w", err)
	}

	// 构建分组映射（仅包含已启用渠道的分组，status = 1）
	enabledGroupMap := make(map[string]bool)
	disabledGroupMap := make(map[string]bool)
	for _, ch := range channels {
		channelGroups := ch.GetGroups()
		for _, g := range channelGroups {
			if ch.Status == common.ChannelStatusEnabled {
				enabledGroupMap[g] = true
			} else {
				// 只有当分组不在已启用渠道中时，才标记为禁用
				if !enabledGroupMap[g] {
					disabledGroupMap[g] = true
				}
			}
		}
	}

	// 检查每个分组是否存在且来自已启用的渠道
	var invalidGroups []string
	var disabledGroups []string
	for _, group := range groups {
		group = strings.TrimSpace(group)
		if group == "" {
			continue
		}
		if enabledGroupMap[group] {
			// 分组来自已启用的渠道，有效
			continue
		}
		if disabledGroupMap[group] {
			// 分组只存在于已禁用的渠道
			disabledGroups = append(disabledGroups, group)
		} else {
			// 分组不存在
			invalidGroups = append(invalidGroups, group)
		}
	}

	// 优先报告仅存在于禁用渠道的分组
	if len(disabledGroups) > 0 {
		return types.NewErrorWithStatusCode(
			fmt.Errorf("以下渠道分组仅存在于已禁用的渠道中: %v", disabledGroups),
			types.ErrorCodePlanChannelInvalid,
			http.StatusBadRequest,
		)
	}

	if len(invalidGroups) > 0 {
		// 严格模式：不存在的渠道分组返回错误
		return types.NewErrorWithStatusCode(
			fmt.Errorf("以下渠道分组在系统中不存在: %v", invalidGroups),
			types.ErrorCodePlanChannelInvalid,
			http.StatusBadRequest,
		)
	}

	return nil
}

// ===================== 辅助方法 =====================

// buildPlanFromRequest 从 DTO 构建套餐模型
func (s *SubscriptionPlanService) buildPlanFromRequest(req *dto.SubscriptionPlanRequest) *model.SubscriptionPlan {
	plan := &model.SubscriptionPlan{
		Name:         req.Name,
		PriceCents:   req.PriceCents,
		BillingCycle: req.BillingCycle,
		Status:       req.Status,
	}

	// 修复：为 monthly/yearly 设置默认值，避免 0 天订阅
	if req.BillingCycle == common.BillingCycleMonthly {
		if req.BillingCycleValue <= 0 {
			plan.BillingCycleValue = 30 // 默认 30 天
		} else {
			plan.BillingCycleValue = req.BillingCycleValue
		}
	} else if req.BillingCycle == common.BillingCycleYearly {
		if req.BillingCycleValue <= 0 {
			plan.BillingCycleValue = 365 // 默认 365 天
		} else {
			plan.BillingCycleValue = req.BillingCycleValue
		}
	} else {
		// custom 周期必须明确提供值（model 层会验证）
		plan.BillingCycleValue = req.BillingCycleValue
	}

	// 可选字段
	if req.SKU != nil {
		plan.SKU = req.SKU
	}
	if req.Description != "" {
		plan.Description = &req.Description
	}
	if req.Currency != "" {
		plan.Currency = req.Currency
	} else {
		plan.Currency = "USD" // 默认美元
	}
	if req.AllowWalletFallback != nil {
		plan.AllowWalletFallback = *req.AllowWalletFallback
	} else {
		// 从系统配置读取默认值（而非硬编码）
		plan.AllowWalletFallback = common.GetAutoWalletFallbackDefault()
	}
	if req.Status == "" {
		plan.Status = common.PlanStatusDraft // 默认草稿状态
	}
	if req.StartAt != nil {
		plan.StartAt = req.StartAt
	}
	if req.EndAt != nil {
		plan.EndAt = req.EndAt
	}
	if req.ModelWhitelist != "" {
		plan.ModelWhitelist = &req.ModelWhitelist
	}
	if req.ChannelGroups != "" {
		plan.ChannelGroups = &req.ChannelGroups
	}
	if req.Extra != "" {
		plan.Extra = &req.Extra
	}

	// 构建限额配置
	if len(req.Limits) > 0 {
		plan.Limits = make([]model.SubscriptionPlanLimit, len(req.Limits))
		for i, limitReq := range req.Limits {
			limit := model.SubscriptionPlanLimit{
				Period:         limitReq.Period,
				Quota:          limitReq.Quota,
				Unit:           limitReq.Unit,
				WindowStrategy: limitReq.WindowStrategy,
			}
			if limitReq.Enabled != nil {
				limit.Enabled = *limitReq.Enabled
			} else {
				limit.Enabled = true // 默认启用
			}
			if limit.Unit == "" {
				limit.Unit = "quota" // 默认单位
			}
			if limit.WindowStrategy == "" {
				// 按周期类型设置默认窗口策略（按需求文档建议）
				limit.WindowStrategy = s.getDefaultWindowStrategy(limit.Period)
			}
			plan.Limits[i] = limit
		}
	}

	return plan
}

// getDefaultWindowStrategy 根据周期类型返回推荐的默认窗口策略
// 参考：requirements.md - 策略选择建议
func (s *SubscriptionPlanService) getDefaultWindowStrategy(period string) string {
	switch period {
	case common.LimitPeriodFiveHours:
		// five_hours: 推荐 rolling，滚动窗口，首次使用触发
		return common.WindowStrategyRolling
	case common.LimitPeriodDay:
		// day: 推荐 fixed，自然日，便于用户理解
		return common.WindowStrategyFixed
	case common.LimitPeriodWeek, common.LimitPeriodMonth:
		// week/month: 可根据业务选择，默认 rolling
		return common.WindowStrategyRolling
	default:
		return common.WindowStrategyRolling
	}
}

// notifyPlanChanged 触发套餐变更事件通知
func (s *SubscriptionPlanService) notifyPlanChanged(planId int64, action string) {
	// 1. 触发 SUB_PLAN_CHANGED 事件（按设计文档要求）
	eventType := "SUB_PLAN_CHANGED"
	subject := fmt.Sprintf("套餐 #%d 已%s", planId, s.getActionText(action))
	content := fmt.Sprintf("套餐 ID: %d\n操作: %s\n时间: %d", planId, s.getActionText(action), common.GetTimestamp())

	// 通知管理员
	NotifyRootUser(eventType, subject, content)

	// 2. 刷新套餐相关缓存
	s.refreshPlanCache(planId, action)

	// 记录系统日志
	common.SysLog(fmt.Sprintf("套餐变更通知: %s (ID: %d, event: %s)", s.getActionText(action), planId, eventType))
}

// 套餐缓存 Key 前缀
const (
	planCacheKeyPrefix       = "subscription:plan:"       // 单个套餐缓存 key
	planListCacheKey         = "subscription:plans:list"  // 套餐列表缓存 key
	planActiveCacheKey       = "subscription:plans:active" // 可用套餐列表缓存 key
)

// refreshPlanCache 刷新套餐相关缓存
func (s *SubscriptionPlanService) refreshPlanCache(planId int64, action string) {
	// 如果 Redis 未启用，则跳过缓存刷新
	if !common.RedisEnabled || common.RDB == nil {
		common.SysLog(fmt.Sprintf("Redis 未启用，跳过套餐缓存刷新: planId=%d, action=%s", planId, action))
		return
	}

	// 根据操作类型刷新不同缓存
	switch action {
	case "created", "updated", "published":
		// 套餐创建/更新/发布时，刷新可用套餐列表缓存
		// 1. 删除可用套餐列表缓存（下次请求时会重新加载）
		if err := common.RedisDel(planActiveCacheKey); err != nil {
			common.SysError(fmt.Sprintf("删除可用套餐列表缓存失败: %v", err))
		}
		// 2. 删除套餐列表缓存
		if err := common.RedisDel(planListCacheKey); err != nil {
			common.SysError(fmt.Sprintf("删除套餐列表缓存失败: %v", err))
		}
		// 3. 删除单个套餐缓存（updated/published 都会改变套餐状态，需要刷新）
		if action == "updated" || action == "published" {
			planKey := fmt.Sprintf("%s%d", planCacheKeyPrefix, planId)
			if err := common.RedisDel(planKey); err != nil {
				common.SysError(fmt.Sprintf("删除单个套餐缓存失败: %v", err))
			}
		}
		common.SysLog(fmt.Sprintf("刷新套餐缓存完成: planId=%d, action=%s", planId, action))

	case "unpublished", "deleted":
		// 套餐下架/删除时，刷新可用套餐列表缓存
		// 1. 删除可用套餐列表缓存
		if err := common.RedisDel(planActiveCacheKey); err != nil {
			common.SysError(fmt.Sprintf("删除可用套餐列表缓存失败: %v", err))
		}
		// 2. 删除套餐列表缓存
		if err := common.RedisDel(planListCacheKey); err != nil {
			common.SysError(fmt.Sprintf("删除套餐列表缓存失败: %v", err))
		}
		// 3. 删除单个套餐缓存
		planKey := fmt.Sprintf("%s%d", planCacheKeyPrefix, planId)
		if err := common.RedisDel(planKey); err != nil {
			common.SysError(fmt.Sprintf("删除单个套餐缓存失败: %v", err))
		}
		common.SysLog(fmt.Sprintf("删除套餐缓存完成: planId=%d, action=%s", planId, action))
	}
}

// getActionText 获取操作文本
func (s *SubscriptionPlanService) getActionText(action string) string {
	actionMap := map[string]string{
		"created":     "创建",
		"updated":     "更新",
		"published":   "发布",
		"unpublished": "下架",
		"deleted":     "删除",
	}
	if text, ok := actionMap[action]; ok {
		return text
	}
	return action
}

// ConvertPlanToResponse 将套餐模型转换为响应 DTO
func ConvertPlanToResponse(plan *model.SubscriptionPlan) *dto.SubscriptionPlanResponse {
	if plan == nil {
		return nil
	}

	resp := &dto.SubscriptionPlanResponse{
		ID:                  plan.Id,
		Name:                plan.Name,
		PriceCents:          plan.PriceCents,
		Currency:            plan.Currency,
		BillingCycle:        plan.BillingCycle,
		BillingCycleValue:   plan.BillingCycleValue,
		AllowWalletFallback: plan.AllowWalletFallback,
		Status:              plan.Status,
		CreatedAt:           plan.CreatedAt,
		UpdatedAt:           plan.UpdatedAt,
	}

	// 可选字段
	if plan.SKU != nil {
		resp.SKU = *plan.SKU
	}
	if plan.Description != nil {
		resp.Description = *plan.Description
	}
	if plan.StartAt != nil {
		resp.StartAt = *plan.StartAt
	}
	if plan.EndAt != nil {
		resp.EndAt = *plan.EndAt
	}
	if plan.ModelWhitelist != nil {
		resp.ModelWhitelist = *plan.ModelWhitelist
	}
	if plan.ChannelGroups != nil {
		resp.ChannelGroups = *plan.ChannelGroups
	}
	if plan.Extra != nil {
		resp.Extra = *plan.Extra
	}

	// 转换限额配置
	if len(plan.Limits) > 0 {
		resp.Limits = make([]dto.SubscriptionPlanLimitResponse, len(plan.Limits))
		for i, limit := range plan.Limits {
			resp.Limits[i] = dto.SubscriptionPlanLimitResponse{
				ID:             limit.Id,
				PlanID:         limit.PlanId,
				Period:         limit.Period,
				Quota:          limit.Quota,
				Unit:           limit.Unit,
				Enabled:        limit.Enabled,
				WindowStrategy: limit.WindowStrategy,
				CreatedAt:      limit.CreatedAt,
				UpdatedAt:      limit.UpdatedAt,
			}
		}
	}

	return resp
}

// ConvertPlansToResponses 批量转换套餐列表
func ConvertPlansToResponses(plans []*model.SubscriptionPlan) []dto.SubscriptionPlanResponse {
	if len(plans) == 0 {
		return []dto.SubscriptionPlanResponse{}
	}

	responses := make([]dto.SubscriptionPlanResponse, len(plans))
	for i, plan := range plans {
		if resp := ConvertPlanToResponse(plan); resp != nil {
			responses[i] = *resp
		}
	}

	return responses
}

// ===================== 高级查询方法 =====================

// GetPlansByIds 批量获取套餐
func (s *SubscriptionPlanService) GetPlansByIds(ids []int64) ([]*model.SubscriptionPlan, error) {
	if len(ids) == 0 {
		return []*model.SubscriptionPlan{}, nil
	}

	var plans []*model.SubscriptionPlan
	for _, id := range ids {
		plan, err := model.GetSubscriptionPlanById(id)
		if err != nil {
			common.SysLog(fmt.Sprintf("获取套餐 ID %d 失败: %v", id, err))
			continue
		}
		plans = append(plans, plan)
	}

	return plans, nil
}

// GetPlanSnapshot 获取套餐快照（用于订单）
func (s *SubscriptionPlanService) GetPlanSnapshot(planId int64) (string, error) {
	plan, err := model.GetSubscriptionPlanById(planId)
	if err != nil {
		return "", fmt.Errorf("获取套餐失败: %w", err)
	}

	// 序列化为 JSON 快照
	snapshot, err := json.Marshal(plan)
	if err != nil {
		return "", fmt.Errorf("序列化套餐快照失败: %w", err)
	}

	return string(snapshot), nil
}
