package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

// SubscriptionPlan 订阅套餐
type SubscriptionPlan struct {
	Id                  int64   `json:"id" gorm:"primaryKey;autoIncrement"`
	SKU                 *string `json:"sku" gorm:"type:varchar(64);uniqueIndex"`
	Name                string  `json:"name" gorm:"type:varchar(128);not null"`
	Description         *string `json:"description" gorm:"type:text"`
	PriceCents          int64   `json:"price_cents" gorm:"not null"`
	Currency            string  `json:"currency" gorm:"type:varchar(8);default:'USD'"`
	BillingCycle        string  `json:"billing_cycle" gorm:"type:varchar(32);not null"`
	BillingCycleValue   int     `json:"billing_cycle_value" gorm:"default:1"`
	AllowWalletFallback bool    `json:"allow_wallet_fallback" gorm:"default:true"`
	Status              string  `json:"status" gorm:"type:varchar(32);default:'draft';index"`
	StartAt             *int64  `json:"start_at" gorm:"bigint"`
	EndAt               *int64  `json:"end_at" gorm:"bigint"`
	ModelWhitelist      *string `json:"model_whitelist" gorm:"type:text"`
	ChannelGroups       *string `json:"channel_groups" gorm:"type:text"`
	Extra               *string `json:"extra" gorm:"type:text"`
	CreatedAt           int64   `json:"created_at" gorm:"bigint;autoCreateTime"`
	UpdatedAt           int64   `json:"updated_at" gorm:"bigint;autoUpdateTime"`

	// 关联
	Limits []SubscriptionPlanLimit `json:"limits" gorm:"foreignKey:PlanId;references:Id"`
}

func (SubscriptionPlan) TableName() string {
	return "subscription_plans"
}

// SubscriptionPlanLimit 套餐限额配置
type SubscriptionPlanLimit struct {
	Id             int64  `json:"id" gorm:"primaryKey;autoIncrement"`
	PlanId         int64  `json:"plan_id" gorm:"not null;index;uniqueIndex:idx_plan_period,priority:1"`
	Period         string `json:"period" gorm:"type:varchar(32);not null;uniqueIndex:idx_plan_period,priority:2"`
	Quota          int64  `json:"quota" gorm:"not null"`
	Unit           string `json:"unit" gorm:"type:varchar(32);default:'quota'"`
	Enabled        bool   `json:"enabled" gorm:"default:true"`
	WindowStrategy string `json:"window_strategy" gorm:"type:varchar(32);default:'rolling'"`
	CreatedAt      int64  `json:"created_at" gorm:"bigint;autoCreateTime"`
	UpdatedAt      int64  `json:"updated_at" gorm:"bigint;autoUpdateTime"`
}

func (SubscriptionPlanLimit) TableName() string {
	return "subscription_plan_limits"
}

// GetPlanLimitsByPlanId 根据套餐 ID 获取限额配置列表
func GetPlanLimitsByPlanId(planId int64) ([]SubscriptionPlanLimit, error) {
	if planId == 0 {
		return nil, errors.New("套餐 ID 不能为空")
	}

	var limits []SubscriptionPlanLimit
	err := DB.Where("plan_id = ?", planId).Find(&limits).Error
	if err != nil {
		return nil, err
	}

	return limits, nil
}

// GetPlanLimitsByPlanIdWithTx 在事务中根据套餐 ID 获取限额配置列表
func GetPlanLimitsByPlanIdWithTx(tx *gorm.DB, planId int64) ([]SubscriptionPlanLimit, error) {
	if planId == 0 {
		return nil, errors.New("套餐 ID 不能为空")
	}

	var limits []SubscriptionPlanLimit
	err := tx.Where("plan_id = ?", planId).Find(&limits).Error
	if err != nil {
		return nil, err
	}

	return limits, nil
}

// ===================== 套餐 CRUD 方法 =====================

// CreateSubscriptionPlan 创建套餐（含限额配置）
func CreateSubscriptionPlan(plan *SubscriptionPlan) error {
	if err := validateSubscriptionPlan(plan); err != nil {
		return err
	}

	// 检查名称唯一性
	var count int64
	if err := DB.Model(&SubscriptionPlan{}).Where("name = ?", plan.Name).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return errors.New("套餐名称已存在")
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		// 创建套餐（避免 GORM 自动保存关联的 Limits，后续统一手动插入）
		if err := tx.Omit("Limits").Create(plan).Error; err != nil {
			return err
		}

		// 更新限额的 PlanId
		for i := range plan.Limits {
			plan.Limits[i].PlanId = plan.Id
		}

		// 批量创建限额（显式写入 enabled，避免 false 被默认值覆盖）
		if len(plan.Limits) > 0 {
			now := common.GetTimestamp()
			for _, limit := range plan.Limits {
				if err := tx.Exec(
					"INSERT INTO subscription_plan_limits (plan_id, period, quota, unit, enabled, window_strategy, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
					plan.Id,
					limit.Period,
					limit.Quota,
					limit.Unit,
					limit.Enabled,
					limit.WindowStrategy,
					now,
					now,
				).Error; err != nil {
					return err
				}
			}
		}

		return nil
	})
}

// UpdateSubscriptionPlan 更新套餐（含限额配置）
func UpdateSubscriptionPlan(plan *SubscriptionPlan) error {
	if plan.Id == 0 {
		return errors.New("套餐 ID 不能为空")
	}

	if err := validateSubscriptionPlan(plan); err != nil {
		return err
	}

	// 检查名称唯一性（排除自己）
	var count int64
	if err := DB.Model(&SubscriptionPlan{}).Where("name = ? AND id != ?", plan.Name, plan.Id).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return errors.New("套餐名称已存在")
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		// 更新套餐基本信息（避免自动保存 Limits 关联）
		if err := tx.Model(&SubscriptionPlan{}).Where("id = ?", plan.Id).Updates(map[string]interface{}{
			"sku":                   plan.SKU,
			"name":                  plan.Name,
			"description":           plan.Description,
			"price_cents":           plan.PriceCents,
			"currency":              plan.Currency,
			"billing_cycle":         plan.BillingCycle,
			"billing_cycle_value":   plan.BillingCycleValue,
			"allow_wallet_fallback": plan.AllowWalletFallback,
			"status":                plan.Status,
			"start_at":              plan.StartAt,
			"end_at":                plan.EndAt,
			"model_whitelist":       plan.ModelWhitelist,
			"channel_groups":        plan.ChannelGroups,
			"extra":                 plan.Extra,
		}).Error; err != nil {
			return err
		}

		// 删除旧的限额配置
		if err := tx.Where("plan_id = ?", plan.Id).Delete(&SubscriptionPlanLimit{}).Error; err != nil {
			return err
		}

		// 更新限额的 PlanId
		for i := range plan.Limits {
			plan.Limits[i].PlanId = plan.Id
			plan.Limits[i].Id = 0 // 重置 ID 以便创建新记录
		}

		// 批量创建新限额（显式写入 enabled，避免 false 被默认值覆盖）
		if len(plan.Limits) > 0 {
			now := common.GetTimestamp()
			for _, limit := range plan.Limits {
				if err := tx.Exec(
					"INSERT INTO subscription_plan_limits (plan_id, period, quota, unit, enabled, window_strategy, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
					plan.Id,
					limit.Period,
					limit.Quota,
					limit.Unit,
					limit.Enabled,
					limit.WindowStrategy,
					now,
					now,
				).Error; err != nil {
					return err
				}
			}
		}

		return nil
	})
}

// GetSubscriptionPlanById 根据 ID 获取套餐（含限额）
func GetSubscriptionPlanById(id int64) (*SubscriptionPlan, error) {
	if id == 0 {
		return nil, errors.New("套餐 ID 不能为空")
	}

	var plan SubscriptionPlan
	err := DB.Preload("Limits").First(&plan, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New(common.MsgPlanNotFound)
		}
		return nil, err
	}

	return &plan, nil
}

// GetSubscriptionPlanByIdWithTx 根据 ID 获取套餐（含限额，支持外部事务/连接复用）
func GetSubscriptionPlanByIdWithTx(tx *gorm.DB, id int64) (*SubscriptionPlan, error) {
	if id == 0 {
		return nil, errors.New("套餐 ID 不能为空")
	}
	if tx == nil {
		tx = DB
	}

	var plan SubscriptionPlan
	err := tx.Preload("Limits").First(&plan, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New(common.MsgPlanNotFound)
		}
		return nil, err
	}

	return &plan, nil
}

// GetSubscriptionPlanBySKU 根据 SKU 获取套餐
func GetSubscriptionPlanBySKU(sku string) (*SubscriptionPlan, error) {
	if sku == "" {
		return nil, errors.New("SKU 不能为空")
	}

	var plan SubscriptionPlan
	err := DB.Preload("Limits").First(&plan, "sku = ?", sku).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New(common.MsgPlanNotFound)
		}
		return nil, err
	}

	return &plan, nil
}

// GetAllSubscriptionPlans 获取所有套餐（分页）
func GetAllSubscriptionPlans(startIdx int, num int, status string) (plans []*SubscriptionPlan, total int64, err error) {
	query := DB.Model(&SubscriptionPlan{})

	if status != "" {
		query = query.Where("status = ?", status)
	}

	// 获取总数
	err = query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	// 获取分页数据
	err = query.Preload("Limits").Order("id desc").Limit(num).Offset(startIdx).Find(&plans).Error
	if err != nil {
		return nil, 0, err
	}

	return plans, total, nil
}

// GetSubscriptionPlansWithFilters 获取套餐列表（支持多条件筛选）
func GetSubscriptionPlansWithFilters(startIdx int, num int, status string, billingCycle string, minPrice *int64, maxPrice *int64) (plans []*SubscriptionPlan, total int64, err error) {
	query := DB.Model(&SubscriptionPlan{})

	// 状态筛选
	if status != "" {
		query = query.Where("status = ?", status)
	}

	// 计费周期筛选
	if billingCycle != "" {
		query = query.Where("billing_cycle = ?", billingCycle)
	}

	// 最低价格筛选
	if minPrice != nil {
		query = query.Where("price_cents >= ?", *minPrice)
	}

	// 最高价格筛选
	if maxPrice != nil {
		query = query.Where("price_cents <= ?", *maxPrice)
	}

	// 获取总数
	err = query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	// 获取分页数据
	err = query.Preload("Limits").Order("id desc").Limit(num).Offset(startIdx).Find(&plans).Error
	if err != nil {
		return nil, 0, err
	}

	return plans, total, nil
}

// GetActiveSubscriptionPlans 获取所有已发布的套餐
func GetActiveSubscriptionPlans() ([]*SubscriptionPlan, error) {
	var plans []*SubscriptionPlan
	now := common.GetTimestamp()

	err := DB.Preload("Limits", "enabled = ?", true).
		Where("status = ?", common.PlanStatusActive).
		Where("(start_at IS NULL OR start_at <= ?)", now).
		Where("(end_at IS NULL OR end_at >= ?)", now).
		Order("id asc").
		Find(&plans).Error

	if err != nil {
		return nil, err
	}

	return plans, nil
}

// SearchSubscriptionPlans 搜索套餐
func SearchSubscriptionPlans(keyword string, startIdx int, num int) (plans []*SubscriptionPlan, total int64, err error) {
	query := DB.Model(&SubscriptionPlan{})

	if keyword != "" {
		likeKeyword := "%" + keyword + "%"
		query = query.Where("name LIKE ? OR sku LIKE ? OR description LIKE ?", likeKeyword, likeKeyword, likeKeyword)
	}

	// 获取总数
	err = query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	// 获取分页数据
	err = query.Preload("Limits").Order("id desc").Limit(num).Offset(startIdx).Find(&plans).Error
	if err != nil {
		return nil, 0, err
	}

	return plans, total, nil
}

// DeleteSubscriptionPlan 删除套餐（软删除：改为归档状态）
func DeleteSubscriptionPlan(id int64) error {
	if id == 0 {
		return errors.New("套餐 ID 不能为空")
	}

	// 检查套餐是否存在
	var plan SubscriptionPlan
	if err := DB.First(&plan, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New(common.MsgPlanNotFound)
		}
		return err
	}

	// 检查是否有关联的订阅
	var subscriptionCount int64
	if err := DB.Model(&Subscription{}).Where("plan_id = ?", id).Count(&subscriptionCount).Error; err != nil {
		return err
	}
	if subscriptionCount > 0 {
		return errors.New("该套餐已有订阅关联，无法删除")
	}

	// 检查是否有关联的订单
	var orderCount int64
	if err := DB.Model(&SubscriptionOrder{}).Where("plan_id = ?", id).Count(&orderCount).Error; err != nil {
		return err
	}
	if orderCount > 0 {
		return errors.New("该套餐已有订单关联，无法删除")
	}

	// 软删除：将状态改为 archived（保留限额配置和套餐数据）
	return DB.Model(&SubscriptionPlan{}).Where("id = ?", id).
		Update("status", common.PlanStatusArchived).Error
}

// ===================== 状态管理方法 =====================

// PublishSubscriptionPlan 发布套餐
func PublishSubscriptionPlan(id int64) error {
	if id == 0 {
		return errors.New("套餐 ID 不能为空")
	}

	plan, err := GetSubscriptionPlanById(id)
	if err != nil {
		return err
	}

	if plan.Status == common.PlanStatusActive {
		return errors.New("套餐已发布")
	}

	// 验证套餐是否可发布
	if plan.PriceCents <= 0 {
		return errors.New(common.MsgPlanPriceInvalid)
	}

	if len(plan.Limits) == 0 {
		return errors.New("套餐至少需要一个限额配置")
	}

	return DB.Model(&SubscriptionPlan{}).Where("id = ?", id).
		Update("status", common.PlanStatusActive).Error
}

// UnpublishSubscriptionPlan 下架套餐
func UnpublishSubscriptionPlan(id int64) error {
	if id == 0 {
		return errors.New("套餐 ID 不能为空")
	}

	// 检查套餐是否存在
	var plan SubscriptionPlan
	if err := DB.First(&plan, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New(common.MsgPlanNotFound)
		}
		return err
	}

	return DB.Model(&SubscriptionPlan{}).Where("id = ?", id).
		Update("status", common.PlanStatusArchived).Error
}

// ArchiveSubscriptionPlan 归档套餐
func ArchiveSubscriptionPlan(id int64) error {
	return UnpublishSubscriptionPlan(id)
}

// ===================== 验证方法 =====================

// validateSubscriptionPlan 验证套餐数据
func validateSubscriptionPlan(plan *SubscriptionPlan) error {
	if strings.TrimSpace(plan.Name) == "" {
		return errors.New("套餐名称不能为空")
	}

	// 价格必须 > 0
	if plan.PriceCents <= 0 {
		return errors.New(common.MsgPlanPriceInvalid)
	}

	// 计费周期仅允许 monthly/yearly/custom，不包含限额周期（five_hours/day/week/month）
	validBillingCycles := map[string]bool{
		common.BillingCycleMonthly: true,
		common.BillingCycleYearly:  true,
		common.BillingCycleCustom:  true,
	}
	if !validBillingCycles[plan.BillingCycle] {
		return errors.New("无效的计费周期，仅支持 monthly/yearly/custom")
	}

	// 修复：custom 计费周期必须指定有效的天数（BillingCycleValue > 0）
	if plan.BillingCycle == common.BillingCycleCustom && plan.BillingCycleValue <= 0 {
		return errors.New("自定义计费周期必须指定有效天数（billing_cycle_value > 0）")
	}

	validStatuses := map[string]bool{
		common.PlanStatusDraft:    true,
		common.PlanStatusActive:   true,
		common.PlanStatusArchived: true,
	}
	if plan.Status != "" && !validStatuses[plan.Status] {
		return errors.New("无效的套餐状态")
	}

	// 验证模型白名单格式
	if plan.ModelWhitelist != nil && *plan.ModelWhitelist != "" {
		if _, err := plan.GetModelWhitelistAsSlice(); err != nil {
			return errors.New("模型白名单格式无效")
		}
	}

	// 验证渠道分组格式
	if plan.ChannelGroups != nil && *plan.ChannelGroups != "" {
		if _, err := plan.GetChannelGroupsAsSlice(); err != nil {
			return errors.New("渠道分组格式无效")
		}
	}

	// 验证限额配置
	periodSeen := make(map[string]bool)
	for _, limit := range plan.Limits {
		if err := validateSubscriptionPlanLimit(&limit); err != nil {
			return err
		}
		if periodSeen[limit.Period] {
			return errors.New(common.MsgPlanPeriodDuplicate)
		}
		periodSeen[limit.Period] = true
	}

	return nil
}

// validateSubscriptionPlanLimit 验证限额配置
func validateSubscriptionPlanLimit(limit *SubscriptionPlanLimit) error {
	validPeriods := map[string]bool{
		common.LimitPeriodFiveHours: true,
		common.LimitPeriodDay:       true,
		common.LimitPeriodWeek:      true,
		common.LimitPeriodMonth:     true,
	}
	if !validPeriods[limit.Period] {
		return fmt.Errorf("无效的限额周期: %s", limit.Period)
	}

	if limit.Quota <= 0 {
		return errors.New("限额必须大于 0")
	}

	validStrategies := map[string]bool{
		common.WindowStrategyRolling: true,
		common.WindowStrategyFixed:   true,
		"natural":                    true, // natural 是 fixed 的别名
	}
	if limit.WindowStrategy != "" && !validStrategies[limit.WindowStrategy] {
		return fmt.Errorf("无效的窗口策略: %s，支持 rolling/fixed/natural", limit.WindowStrategy)
	}

	return nil
}

// ===================== 辅助方法 =====================

// GetModelWhitelistAsSlice 获取模型白名单切片
func (p *SubscriptionPlan) GetModelWhitelistAsSlice() ([]string, error) {
	if p.ModelWhitelist == nil || *p.ModelWhitelist == "" {
		return []string{}, nil
	}

	var models []string
	err := json.Unmarshal([]byte(*p.ModelWhitelist), &models)
	if err != nil {
		// 尝试以逗号分隔的字符串解析
		models = strings.Split(*p.ModelWhitelist, ",")
		for i := range models {
			models[i] = strings.TrimSpace(models[i])
		}
	}

	return models, nil
}

// SetModelWhitelistFromSlice 设置模型白名单
func (p *SubscriptionPlan) SetModelWhitelistFromSlice(models []string) error {
	if len(models) == 0 {
		p.ModelWhitelist = nil
		return nil
	}

	data, err := json.Marshal(models)
	if err != nil {
		return err
	}
	str := string(data)
	p.ModelWhitelist = &str
	return nil
}

// GetChannelGroupsAsSlice 获取渠道分组切片
func (p *SubscriptionPlan) GetChannelGroupsAsSlice() ([]string, error) {
	if p.ChannelGroups == nil || *p.ChannelGroups == "" {
		return []string{}, nil
	}

	var groups []string
	err := json.Unmarshal([]byte(*p.ChannelGroups), &groups)
	if err != nil {
		// 尝试以逗号分隔的字符串解析
		groups = strings.Split(*p.ChannelGroups, ",")
		for i := range groups {
			groups[i] = strings.TrimSpace(groups[i])
		}
	}

	return groups, nil
}

// SetChannelGroupsFromSlice 设置渠道分组
func (p *SubscriptionPlan) SetChannelGroupsFromSlice(groups []string) error {
	if len(groups) == 0 {
		p.ChannelGroups = nil
		return nil
	}

	data, err := json.Marshal(groups)
	if err != nil {
		return err
	}
	str := string(data)
	p.ChannelGroups = &str
	return nil
}

// IsAvailable 检查套餐是否可用
func (p *SubscriptionPlan) IsAvailable() bool {
	if p.Status != common.PlanStatusActive {
		return false
	}

	now := common.GetTimestamp()
	if p.StartAt != nil && *p.StartAt > now {
		return false
	}
	if p.EndAt != nil && *p.EndAt < now {
		return false
	}

	return true
}

// GetEnabledLimits 获取启用的限额配置
func (p *SubscriptionPlan) GetEnabledLimits() []SubscriptionPlanLimit {
	var enabled []SubscriptionPlanLimit
	for _, limit := range p.Limits {
		if limit.Enabled {
			enabled = append(enabled, limit)
		}
	}
	return enabled
}
