package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// ===================== Admin 套餐管理 API =====================

// GetAllSubscriptionPlans 获取套餐列表（支持筛选/分页）
// GET /api/admin/subscription-plans
func GetAllSubscriptionPlans(c *gin.Context) {
	var req dto.SubscriptionPlanListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "请求参数错误: " + err.Error(),
		})
		return
	}

	// 设置默认分页
	if req.Page == 0 {
		req.Page = 1
	}
	if req.PageSize == 0 {
		req.PageSize = 20
	}

	svc := service.GetSubscriptionPlanService()
	plans, total, err := svc.ListPlans(&req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// 转换为 DTO
	var planDTOs []dto.SubscriptionPlanResponse
	for _, plan := range plans {
		planDTO := convertPlanToDTO(plan)
		planDTOs = append(planDTOs, planDTO)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    planDTOs,
		"total":   total,
	})
}

// GetSubscriptionPlan 获取套餐详情
// GET /api/admin/subscription-plans/:id
func GetSubscriptionPlan(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的套餐 ID",
		})
		return
	}

	svc := service.GetSubscriptionPlanService()
	plan, err := svc.GetPlan(id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	planDTO := convertPlanToDTO(plan)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    planDTO,
	})
}

// CreateSubscriptionPlan 创建套餐
// POST /api/admin/subscription-plans
func CreateSubscriptionPlan(c *gin.Context) {
	var req dto.SubscriptionPlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "请求参数错误: " + err.Error(),
		})
		return
	}

	// 参数验证（binding 已经验证了必填字段）
	if req.Name == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "套餐名称不能为空",
		})
		return
	}

	svc := service.GetSubscriptionPlanService()
	createdPlan, err := svc.CreatePlan(&req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"id": createdPlan.Id,
		},
	})
}

// UpdateSubscriptionPlan 更新套餐
// PUT /api/admin/subscription-plans/:id
func UpdateSubscriptionPlan(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的套餐 ID",
		})
		return
	}

	var req dto.SubscriptionPlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "请求参数错误: " + err.Error(),
		})
		return
	}

	svc := service.GetSubscriptionPlanService()
	_, err = svc.UpdatePlan(id, &req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

// PublishSubscriptionPlan 上架套餐
// POST /api/admin/subscription-plans/:id/publish
func PublishSubscriptionPlan(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的套餐 ID",
		})
		return
	}

	svc := service.GetSubscriptionPlanService()
	err = svc.PublishPlan(id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

// UnpublishSubscriptionPlan 下架套餐
// POST /api/admin/subscription-plans/:id/unpublish
func UnpublishSubscriptionPlan(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的套餐 ID",
		})
		return
	}

	svc := service.GetSubscriptionPlanService()
	err = svc.UnpublishPlan(id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

// DeleteSubscriptionPlan 删除套餐
// DELETE /api/admin/subscription-plans/:id
func DeleteSubscriptionPlan(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的套餐 ID",
		})
		return
	}

	svc := service.GetSubscriptionPlanService()
	err = svc.DeletePlan(id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

// convertPlanToDTO 将套餐模型转换为 DTO
func convertPlanToDTO(plan *model.SubscriptionPlan) dto.SubscriptionPlanResponse {
	planDTO := dto.SubscriptionPlanResponse{
		ID:                  plan.Id,
		Name:                plan.Name,
		Status:              plan.Status,
		PriceCents:          plan.PriceCents,
		Currency:            plan.Currency,
		BillingCycle:        plan.BillingCycle,
		BillingCycleValue:   plan.BillingCycleValue,
		AllowWalletFallback: plan.AllowWalletFallback,
		CreatedAt:           plan.CreatedAt,
		UpdatedAt:           plan.UpdatedAt,
	}

	// 处理可选的指针字段
	if plan.Description != nil {
		planDTO.Description = *plan.Description
	}
	if plan.SKU != nil {
		planDTO.SKU = *plan.SKU
	}
	if plan.StartAt != nil {
		planDTO.StartAt = *plan.StartAt
	}
	if plan.EndAt != nil {
		planDTO.EndAt = *plan.EndAt
	}
	if plan.ModelWhitelist != nil {
		planDTO.ModelWhitelist = *plan.ModelWhitelist
	}
	if plan.ChannelGroups != nil {
		planDTO.ChannelGroups = *plan.ChannelGroups
	}

	// 获取限额配置（Admin 应该看到所有限额，包括 disabled 的）
	for _, limit := range plan.Limits {
		planDTO.Limits = append(planDTO.Limits, dto.SubscriptionPlanLimitResponse{
			ID:             limit.Id,
			PlanID:         limit.PlanId,
			Period:         limit.Period,
			Quota:          limit.Quota,
			Unit:           limit.Unit,
			WindowStrategy: limit.WindowStrategy,
			Enabled:        limit.Enabled,
			CreatedAt:      limit.CreatedAt,
			UpdatedAt:      limit.UpdatedAt,
		})
	}

	return planDTO
}

// convertPlanToDTOForUser 将套餐模型转换为用户端 DTO（过滤 disabled 限额）
func convertPlanToDTOForUser(plan *model.SubscriptionPlan) dto.SubscriptionPlanResponse {
	planDTO := dto.SubscriptionPlanResponse{
		ID:                  plan.Id,
		Name:                plan.Name,
		Status:              plan.Status,
		PriceCents:          plan.PriceCents,
		Currency:            plan.Currency,
		BillingCycle:        plan.BillingCycle,
		BillingCycleValue:   plan.BillingCycleValue,
		AllowWalletFallback: plan.AllowWalletFallback,
		CreatedAt:           plan.CreatedAt,
		UpdatedAt:           plan.UpdatedAt,
	}

	// 处理可选的指针字段
	if plan.Description != nil {
		planDTO.Description = *plan.Description
	}
	if plan.SKU != nil {
		planDTO.SKU = *plan.SKU
	}
	if plan.StartAt != nil {
		planDTO.StartAt = *plan.StartAt
	}
	if plan.EndAt != nil {
		planDTO.EndAt = *plan.EndAt
	}
	if plan.ModelWhitelist != nil {
		planDTO.ModelWhitelist = *plan.ModelWhitelist
	}
	if plan.ChannelGroups != nil {
		planDTO.ChannelGroups = *plan.ChannelGroups
	}

	// 只返回已启用的限额配置（用户端不应看到 disabled 的限额）
	for _, limit := range plan.Limits {
		if !limit.Enabled {
			continue // 跳过 disabled 的限额
		}
		planDTO.Limits = append(planDTO.Limits, dto.SubscriptionPlanLimitResponse{
			ID:             limit.Id,
			PlanID:         limit.PlanId,
			Period:         limit.Period,
			Quota:          limit.Quota,
			Unit:           limit.Unit,
			WindowStrategy: limit.WindowStrategy,
			Enabled:        limit.Enabled,
			CreatedAt:      limit.CreatedAt,
			UpdatedAt:      limit.UpdatedAt,
		})
	}

	return planDTO
}
