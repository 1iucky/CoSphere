package service

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"

	"gorm.io/gorm"
)

// SubscriptionPriorityService 订阅优先级服务
// 负责管理用户多订阅的优先级排序、校验冲突、更新缓存
//
// 优先级语义：
// - priority 是正整数（可以不连续，如 1, 3, 10, 100），数值越小优先级越高
// - 默认按 end_at 升序排列（越早到期越优先）
// - end_at 相同时按 created_at 升序排列
// - 用户可通过拖拽调整顺序，设置任意正整数（只要不与其他订阅冲突）
// - 新订阅激活时自动按 end_at 在现有 priority 序列中动态插入
// - NormalizePriorities() 可将 priority 整理为连续序列，非强制
type SubscriptionPriorityService struct {
	mu sync.RWMutex
}

var (
	subscriptionPriorityService     *SubscriptionPriorityService
	subscriptionPriorityServiceOnce sync.Once
)

// GetSubscriptionPriorityService 获取订阅优先级服务单例
func GetSubscriptionPriorityService() *SubscriptionPriorityService {
	subscriptionPriorityServiceOnce.Do(func() {
		subscriptionPriorityService = &SubscriptionPriorityService{}
	})
	return subscriptionPriorityService
}

// ===================== 优先级查询 =====================

// GetSortedSubscriptions 获取用户排序后的订阅列表
// 排序规则：priority 升序 -> end_at 升序 -> created_at 升序
func (s *SubscriptionPriorityService) GetSortedSubscriptions(userId int64) ([]*model.Subscription, error) {
	if userId == 0 {
		return nil, errors.New("用户 ID 不能为空")
	}

	subscriptions, err := model.GetActiveSubscriptionsByUser(userId)
	if err != nil {
		return nil, fmt.Errorf("获取用户订阅失败: %w", err)
	}

	s.sortSubscriptions(subscriptions)
	return subscriptions, nil
}

// GetSortedActiveSubscriptions 获取用户所有有效订阅（按优先级排序）
// 仅返回状态为 active 且未过期的订阅
func (s *SubscriptionPriorityService) GetSortedActiveSubscriptions(userId int64) ([]*model.Subscription, error) {
	if userId == 0 {
		return nil, errors.New("用户 ID 不能为空")
	}

	now := common.GetTimestamp()
	subscriptions, err := model.GetActiveSubscriptionsByUser(userId)
	if err != nil {
		return nil, fmt.Errorf("获取用户订阅失败: %w", err)
	}

	// 过滤掉已过期的订阅
	var activeSubscriptions []*model.Subscription
	for _, sub := range subscriptions {
		if sub.Status == common.SubscriptionStatusActive && sub.EndAt > now {
			activeSubscriptions = append(activeSubscriptions, sub)
		}
	}

	s.sortSubscriptions(activeSubscriptions)
	return activeSubscriptions, nil
}

// getSortedSubscriptionsWithTx 在事务中获取用户排序后的订阅列表
// 排序规则：priority 升序 -> end_at 升序 -> created_at 升序
func (s *SubscriptionPriorityService) getSortedSubscriptionsWithTx(tx *gorm.DB, userId int64) ([]*model.Subscription, error) {
	if userId == 0 {
		return nil, errors.New("用户 ID 不能为空")
	}

	subscriptions, err := model.GetActiveSubscriptionsByUserWithTx(tx, userId)
	if err != nil {
		return nil, fmt.Errorf("获取用户订阅失败: %w", err)
	}

	s.sortSubscriptions(subscriptions)
	return subscriptions, nil
}

// sortSubscriptions 排序订阅列表
// 规则：priority 升序 -> end_at 升序 -> created_at 升序
func (s *SubscriptionPriorityService) sortSubscriptions(subs []*model.Subscription) {
	sort.Slice(subs, func(i, j int) bool {
		// 1. 首先按 priority 升序
		if subs[i].Priority != subs[j].Priority {
			return subs[i].Priority < subs[j].Priority
		}
		// 2. priority 相同时，按 end_at 升序（越早到期越优先）
		if subs[i].EndAt != subs[j].EndAt {
			return subs[i].EndAt < subs[j].EndAt
		}
		// 3. end_at 也相同时，按 created_at 升序
		return subs[i].CreatedAt < subs[j].CreatedAt
	})
}

// GetHighestPrioritySubscription 获取用户优先级最高的订阅
func (s *SubscriptionPriorityService) GetHighestPrioritySubscription(userId int64) (*model.Subscription, error) {
	subscriptions, err := s.GetSortedActiveSubscriptions(userId)
	if err != nil {
		return nil, err
	}

	if len(subscriptions) == 0 {
		return nil, nil
	}

	return subscriptions[0], nil
}

// ===================== 优先级更新 =====================

// UpdateUserPriority 更新单个订阅的优先级
// 用户可以自定义调整订阅的优先级顺序
// 业务规则：发生优先级冲突时自动规范化，而非返回错误
func (s *SubscriptionPriorityService) UpdateUserPriority(userId int64, subscriptionId int64, newPriority int) error {
	if userId == 0 {
		return errors.New("用户 ID 不能为空")
	}
	if subscriptionId == 0 {
		return errors.New("订阅 ID 不能为空")
	}
	if newPriority < 1 {
		return errors.New("优先级必须为正整数（从1开始）")
	}

	// 验证订阅存在且属于该用户
	sub, err := model.GetSubscriptionById(subscriptionId)
	if err != nil {
		return fmt.Errorf("订阅不存在: %w", err)
	}
	if sub.UserId != userId {
		return errors.New("无权修改此订阅")
	}

	// 检查优先级冲突
	if err := s.ValidatePriorityConflict(userId, subscriptionId, newPriority); err != nil {
		// 业务规则 Q5：发生优先级冲突时自动规范化
		// 先更新目标订阅的优先级，再调用 NormalizePriorities 重新整理所有优先级
		if updateErr := model.UpdateSubscriptionPriority(subscriptionId, newPriority); updateErr != nil {
			return fmt.Errorf("更新优先级失败: %w", updateErr)
		}
		// 自动规范化：将所有优先级整理为连续序列 1,2,3...
		if normalizeErr := s.NormalizePriorities(userId); normalizeErr != nil {
			return fmt.Errorf("自动规范化优先级失败: %w", normalizeErr)
		}
		// 刷新用户订阅缓存
		s.refreshUserSubscriptionCache(userId)
		return nil
	}

	// 无冲突，直接更新优先级
	if err := model.UpdateSubscriptionPriority(subscriptionId, newPriority); err != nil {
		return fmt.Errorf("更新优先级失败: %w", err)
	}

	// 刷新用户订阅缓存
	s.refreshUserSubscriptionCache(userId)

	return nil
}

// UpdateUserPriorityWithTx 在事务中更新单个订阅的优先级
// 业务规则：发生优先级冲突时自动规范化，而非返回错误
func (s *SubscriptionPriorityService) UpdateUserPriorityWithTx(tx *gorm.DB, userId int64, subscriptionId int64, newPriority int) error {
	if userId == 0 {
		return errors.New("用户 ID 不能为空")
	}
	if subscriptionId == 0 {
		return errors.New("订阅 ID 不能为空")
	}
	if newPriority < 1 {
		return errors.New("优先级必须为正整数（从1开始）")
	}

	// 验证订阅存在且属于该用户
	sub, err := model.GetSubscriptionByIdWithTx(tx, subscriptionId, false)
	if err != nil {
		return fmt.Errorf("订阅不存在: %w", err)
	}
	if sub.UserId != userId {
		return errors.New("无权修改此订阅")
	}

	// 检查优先级冲突（在事务中）
	if err := s.validatePriorityConflictWithTx(tx, userId, subscriptionId, newPriority); err != nil {
		// 业务规则 Q5：发生优先级冲突时自动规范化
		// 先更新目标订阅的优先级，再调用 NormalizePrioritiesWithTx 重新整理所有优先级
		if updateErr := model.UpdateSubscriptionPriorityWithTx(tx, subscriptionId, newPriority); updateErr != nil {
			return fmt.Errorf("更新优先级失败: %w", updateErr)
		}
		// 自动规范化：将所有优先级整理为连续序列 1,2,3...
		if normalizeErr := s.NormalizePrioritiesWithTx(tx, userId); normalizeErr != nil {
			return fmt.Errorf("自动规范化优先级失败: %w", normalizeErr)
		}
		return nil
	}

	// 无冲突，直接更新优先级
	if err := model.UpdateSubscriptionPriorityWithTx(tx, subscriptionId, newPriority); err != nil {
		return fmt.Errorf("更新优先级失败: %w", err)
	}

	return nil
}

// BatchUpdatePriorities 批量更新订阅优先级
// priorityMap: subscriptionId -> newPriority
// 校验：值必须为正整数、map内无重复、与未更新订阅无冲突
func (s *SubscriptionPriorityService) BatchUpdatePriorities(userId int64, priorityMap map[int64]int) error {
	if userId == 0 {
		return errors.New("用户 ID 不能为空")
	}
	if len(priorityMap) == 0 {
		return nil
	}

	// 验证所有优先级值必须为正整数
	for subId, priority := range priorityMap {
		if priority < 1 {
			return fmt.Errorf("订阅 %d 的优先级必须为正整数（从1开始）", subId)
		}
	}

	// 检查 map 内是否有重复优先级
	prioritySet := make(map[int]int64) // priority -> subscriptionId
	for subId, priority := range priorityMap {
		if existingSubId, exists := prioritySet[priority]; exists {
			return types.NewErrorWithStatusCode(
				fmt.Errorf("优先级 %d 重复分配给订阅 %d 和 %d", priority, existingSubId, subId),
				types.ErrorCodeSubscriptionPriorityConflict,
				http.StatusConflict,
			)
		}
		prioritySet[priority] = subId
	}

	// 在事务中批量更新，同时校验与未更新订阅的冲突
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		// 获取用户所有有效订阅
		allSubs, err := model.GetActiveSubscriptionsByUserWithTx(tx, userId)
		if err != nil {
			return fmt.Errorf("获取用户订阅失败: %w", err)
		}

		// 构建订阅 ID 集合，用于验证
		subIdSet := make(map[int64]bool)
		for _, sub := range allSubs {
			subIdSet[sub.Id] = true
		}

		// 验证所有要更新的订阅都属于该用户
		for subId := range priorityMap {
			if !subIdSet[subId] {
				return fmt.Errorf("订阅 %d 不存在或不属于当前用户", subId)
			}
		}

		// 检查与未更新订阅的优先级冲突
		for _, sub := range allSubs {
			if _, isUpdating := priorityMap[sub.Id]; isUpdating {
				continue // 跳过正在更新的订阅
			}
			// 检查未更新订阅的 priority 是否与新 priorityMap 冲突
			if conflictSubId, exists := prioritySet[sub.Priority]; exists {
				return types.NewErrorWithStatusCode(
					fmt.Errorf("订阅 %d 的新优先级 %d 与现有订阅 %d 冲突", conflictSubId, sub.Priority, sub.Id),
					types.ErrorCodeSubscriptionPriorityConflict,
					http.StatusConflict,
				)
			}
		}

		// 执行更新
		for subId, priority := range priorityMap {
			if err := model.UpdateSubscriptionPriorityWithTx(tx, subId, priority); err != nil {
				return fmt.Errorf("更新订阅 %d 优先级失败: %w", subId, err)
			}
		}
		return nil
	})

	if err != nil {
		return err
	}

	// 刷新用户订阅缓存
	s.refreshUserSubscriptionCache(userId)

	return nil
}

// ReorderSubscriptions 重新排序订阅（拖拽场景）
// orderedIds: 按期望优先级顺序排列的订阅ID列表（索引0的优先级最高）
// 要求：orderedIds 必须包含用户所有活跃订阅，且无重复
func (s *SubscriptionPriorityService) ReorderSubscriptions(userId int64, orderedIds []int64) error {
	if userId == 0 {
		return errors.New("用户 ID 不能为空")
	}
	if len(orderedIds) == 0 {
		return nil
	}

	// 检查 orderedIds 是否有重复
	idSet := make(map[int64]bool)
	for _, id := range orderedIds {
		if idSet[id] {
			return types.NewErrorWithStatusCode(
				fmt.Errorf("订阅 ID %d 重复", id),
				types.ErrorCodeSubscriptionPriorityConflict,
				http.StatusConflict,
			)
		}
		idSet[id] = true
	}

	// 获取用户所有活跃订阅，验证 orderedIds 是否完整覆盖
	activeSubs, err := s.GetSortedActiveSubscriptions(userId)
	if err != nil {
		return err
	}

	if len(orderedIds) != len(activeSubs) {
		return types.NewErrorWithStatusCode(
			fmt.Errorf("orderedIds 数量(%d)与活跃订阅数量(%d)不匹配", len(orderedIds), len(activeSubs)),
			types.ErrorCodeSubscriptionPriorityConflict,
			http.StatusConflict,
		)
	}

	// 验证 orderedIds 中的每个 ID 都是用户的活跃订阅
	activeIdSet := make(map[int64]bool)
	for _, sub := range activeSubs {
		activeIdSet[sub.Id] = true
	}
	for _, id := range orderedIds {
		if !activeIdSet[id] {
			return fmt.Errorf("订阅 %d 不是当前用户的活跃订阅", id)
		}
	}

	// 构建优先级映射（索引+1作为优先级，确保从1开始）
	priorityMap := make(map[int64]int)
	for i, subId := range orderedIds {
		priorityMap[subId] = i + 1
	}

	// 直接在事务中更新，无需再次校验冲突（因为是完整重排）
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		for subId, priority := range priorityMap {
			if err := model.UpdateSubscriptionPriorityWithTx(tx, subId, priority); err != nil {
				return fmt.Errorf("更新订阅 %d 优先级失败: %w", subId, err)
			}
		}
		return nil
	})

	if err != nil {
		return err
	}

	// 刷新用户订阅缓存
	s.refreshUserSubscriptionCache(userId)

	return nil
}

// ===================== 优先级冲突校验 =====================

// ValidatePriorityConflict 校验优先级冲突
// 检查同一用户的其他订阅是否已使用该优先级
func (s *SubscriptionPriorityService) ValidatePriorityConflict(userId int64, excludeSubId int64, priority int) error {
	if userId == 0 {
		return errors.New("用户 ID 不能为空")
	}

	// 获取用户所有有效订阅
	subscriptions, err := model.GetActiveSubscriptionsByUser(userId)
	if err != nil {
		return fmt.Errorf("获取用户订阅失败: %w", err)
	}

	// 检查是否存在相同优先级的订阅（排除当前订阅）
	for _, sub := range subscriptions {
		if sub.Id != excludeSubId && sub.Priority == priority {
			return types.NewErrorWithStatusCode(
				fmt.Errorf("%s: 订阅 %d 已使用优先级 %d", common.MsgSubscriptionPriorityConflict, sub.Id, priority),
				types.ErrorCodeSubscriptionPriorityConflict,
				http.StatusConflict,
			)
		}
	}

	return nil
}

// validatePriorityConflictWithTx 在事务中校验优先级冲突
func (s *SubscriptionPriorityService) validatePriorityConflictWithTx(tx *gorm.DB, userId int64, excludeSubId int64, priority int) error {
	if userId == 0 {
		return errors.New("用户 ID 不能为空")
	}

	// 获取用户所有有效订阅
	subscriptions, err := model.GetActiveSubscriptionsByUserWithTx(tx, userId)
	if err != nil {
		return fmt.Errorf("获取用户订阅失败: %w", err)
	}

	// 检查是否存在相同优先级的订阅（排除当前订阅）
	for _, sub := range subscriptions {
		if sub.Id != excludeSubId && sub.Priority == priority {
			return types.NewErrorWithStatusCode(
				fmt.Errorf("%s: 订阅 %d 已使用优先级 %d", common.MsgSubscriptionPriorityConflict, sub.Id, priority),
				types.ErrorCodeSubscriptionPriorityConflict,
				http.StatusConflict,
			)
		}
	}

	return nil
}

// HasPriorityConflict 检查是否存在优先级冲突（不返回错误，仅返回布尔值）
func (s *SubscriptionPriorityService) HasPriorityConflict(userId int64, excludeSubId int64, priority int) bool {
	return s.ValidatePriorityConflict(userId, excludeSubId, priority) != nil
}

// ===================== 动态插入（新订阅初始化） =====================

// InitializeSubscriptionPriority 初始化新订阅的优先级
// 按 end_at 动态插入到优先级队列中的正确位置
// 规则：找到第一个 end_at > newSub.EndAt 的订阅，插入到它前面；如果没有，则插入到末尾
func (s *SubscriptionPriorityService) InitializeSubscriptionPriority(sub *model.Subscription) error {
	if sub == nil {
		return errors.New("订阅不能为空")
	}

	return model.DB.Transaction(func(tx *gorm.DB) error {
		return s.InitializeSubscriptionPriorityWithTx(tx, sub)
	})
}

// InitializeSubscriptionPriorityWithTx 在事务中初始化新订阅的优先级
// 实现动态插入：按 end_at 找到正确位置，将后续订阅的 priority 都 +1
// 注意：使用 SELECT FOR UPDATE 行锁防止并发冲突
func (s *SubscriptionPriorityService) InitializeSubscriptionPriorityWithTx(tx *gorm.DB, sub *model.Subscription) error {
	if sub == nil {
		return errors.New("订阅不能为空")
	}

	// 获取用户所有有效订阅（排除当前订阅）
	// 使用 FOR UPDATE 锁定行，防止并发修改导致优先级冲突
	allSubs, err := model.GetActiveSubscriptionsByUserWithTxForUpdate(tx, sub.UserId)
	if err != nil {
		return fmt.Errorf("获取用户订阅失败: %w", err)
	}

	// 过滤掉当前订阅（如果已存在）
	var existingSubs []*model.Subscription
	for _, s := range allSubs {
		if s.Id != sub.Id {
			existingSubs = append(existingSubs, s)
		}
	}

	// 如果没有其他订阅，新订阅 priority = 1
	if len(existingSubs) == 0 {
		sub.Priority = 1
		return nil
	}

	// 按优先级 -> end_at -> created_at 排序现有订阅
	s.sortSubscriptions(existingSubs)

	// 找到插入位置：第一个 end_at > sub.EndAt 的位置
	// 如果 end_at 相同，比较 created_at
	insertPos := len(existingSubs) // 默认插入到末尾
	for i, existing := range existingSubs {
		if existing.EndAt > sub.EndAt {
			insertPos = i
			break
		}
		if existing.EndAt == sub.EndAt && existing.CreatedAt > sub.CreatedAt {
			insertPos = i
			break
		}
	}

	// 新订阅的 priority
	if insertPos == 0 {
		sub.Priority = 1
	} else {
		sub.Priority = existingSubs[insertPos-1].Priority + 1
	}

	// 将插入位置及之后的订阅 priority 都 +1
	// 注意：这里只是为插入腾出空间，priority 值本身允许不连续
	for i := insertPos; i < len(existingSubs); i++ {
		newPriority := existingSubs[i].Priority + 1
		if err := model.UpdateSubscriptionPriorityWithTx(tx, existingSubs[i].Id, newPriority); err != nil {
			return fmt.Errorf("更新订阅 %d 优先级失败: %w", existingSubs[i].Id, err)
		}
	}

	// 将新订阅的 priority 写回数据库
	// 注意：调用此函数时，订阅应该已经被创建（有 ID）
	if sub.Id != 0 {
		if err := model.UpdateSubscriptionPriorityWithTx(tx, sub.Id, sub.Priority); err != nil {
			return fmt.Errorf("更新新订阅 %d 优先级失败: %w", sub.Id, err)
		}
	}
	// 如果 sub.Id == 0，说明订阅还未持久化，只设置内存中的 Priority 字段
	// 调用者需要在 Create 时包含此 Priority 值

	return nil
}

// ===================== 缓存管理 =====================

// refreshUserSubscriptionCache 刷新用户订阅缓存
// 按设计要求：优先级调整后写入 subscription:active:<user_id> 并对齐 TTL
// 统一使用 SubscriptionCacheService.CacheActiveSubscriptions 方法
func (s *SubscriptionPriorityService) refreshUserSubscriptionCache(userId int64) {
	// 检查 Redis 是否启用且已初始化
	if !common.RedisEnabled || common.RDB == nil {
		return
	}

	// 获取用户最新的活跃订阅列表
	subscriptions, err := model.GetActiveSubscriptionsByUser(userId)
	if err != nil {
		common.SysError(fmt.Sprintf("刷新用户订阅缓存失败 - 获取订阅失败 [%d]: %v", userId, err))
		// 获取失败时，删除旧缓存以避免脏数据
		s.deleteUserSubscriptionCache(userId)
		return
	}

	// 统一使用 SubscriptionCacheService 写入缓存
	cacheSvc := GetSubscriptionCacheService()
	if err := cacheSvc.CacheActiveSubscriptions(userId, subscriptions); err != nil {
		common.SysError(fmt.Sprintf("刷新用户订阅缓存失败 - 写入缓存失败 [%d]: %v", userId, err))
	}
}

// deleteUserSubscriptionCache 删除用户订阅缓存
func (s *SubscriptionPriorityService) deleteUserSubscriptionCache(userId int64) {
	if !common.RedisEnabled || common.RDB == nil {
		return
	}

	cacheKey := fmt.Sprintf("subscription:active:%d", userId)
	_ = common.RedisDel(cacheKey)

	listCacheKey := fmt.Sprintf("subscription:list:%d", userId)
	_ = common.RedisDel(listCacheKey)
}

// InvalidateUserSubscriptionCache 使用户订阅缓存失效（公开方法）
func (s *SubscriptionPriorityService) InvalidateUserSubscriptionCache(userId int64) {
	s.refreshUserSubscriptionCache(userId)
}

// GetCachedActiveSubscriptions 从缓存获取用户活跃订阅（如果缓存存在）
// 返回值：subscriptions, fromCache, error
// 注意：会过滤掉已过期的订阅，确保返回的数据都是当前有效的
// 统一使用 SubscriptionCacheService.GetActiveSubscriptionsFromCache 方法
func (s *SubscriptionPriorityService) GetCachedActiveSubscriptions(userId int64) ([]CachedSubscription, bool, error) {
	if !common.RedisEnabled || common.RDB == nil {
		return nil, false, nil
	}

	// 统一使用 SubscriptionCacheService 读取缓存
	cacheSvc := GetSubscriptionCacheService()
	cachedSubs, hit := cacheSvc.GetActiveSubscriptionsFromCache(userId)
	if !hit {
		return nil, false, nil
	}

	return cachedSubs, true, nil
}

// ===================== 辅助方法 =====================

// GetPriorityRange 获取用户订阅的优先级范围
func (s *SubscriptionPriorityService) GetPriorityRange(userId int64) (minPriority int, maxPriority int, err error) {
	if userId == 0 {
		return 0, 0, errors.New("用户 ID 不能为空")
	}

	subscriptions, err := model.GetActiveSubscriptionsByUser(userId)
	if err != nil {
		return 0, 0, fmt.Errorf("获取用户订阅失败: %w", err)
	}

	if len(subscriptions) == 0 {
		return 0, 0, nil
	}

	minPriority = subscriptions[0].Priority
	maxPriority = subscriptions[0].Priority

	for _, sub := range subscriptions {
		if sub.Priority < minPriority {
			minPriority = sub.Priority
		}
		if sub.Priority > maxPriority {
			maxPriority = sub.Priority
		}
	}

	return minPriority, maxPriority, nil
}

// NormalizePriorities 规范化用户订阅的优先级
// 将优先级重新排序为连续的整数序列（1, 2, 3, ...）
// 保持原有的相对顺序不变
func (s *SubscriptionPriorityService) NormalizePriorities(userId int64) error {
	if userId == 0 {
		return errors.New("用户 ID 不能为空")
	}

	// 获取排序后的订阅（保持原有顺序）
	subscriptions, err := s.GetSortedSubscriptions(userId)
	if err != nil {
		return err
	}

	if len(subscriptions) == 0 {
		return nil
	}

	// 在事务中更新
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		for i, sub := range subscriptions {
			newPriority := i + 1
			if sub.Priority != newPriority {
				if err := model.UpdateSubscriptionPriorityWithTx(tx, sub.Id, newPriority); err != nil {
					return fmt.Errorf("更新订阅 %d 优先级失败: %w", sub.Id, err)
				}
			}
		}
		return nil
	})

	if err != nil {
		return err
	}

	// 刷新缓存
	s.refreshUserSubscriptionCache(userId)

	return nil
}

// NormalizePrioritiesWithTx 在事务中规范化用户订阅的优先级
// 将优先级重新排序为连续的整数序列（1, 2, 3, ...）
// 保持原有的相对顺序不变
func (s *SubscriptionPriorityService) NormalizePrioritiesWithTx(tx *gorm.DB, userId int64) error {
	if userId == 0 {
		return errors.New("用户 ID 不能为空")
	}

	// 获取排序后的订阅（保持原有顺序）
	subscriptions, err := s.getSortedSubscriptionsWithTx(tx, userId)
	if err != nil {
		return err
	}

	if len(subscriptions) == 0 {
		return nil
	}

	// 在事务中更新
	for i, sub := range subscriptions {
		newPriority := i + 1
		if sub.Priority != newPriority {
			if err := model.UpdateSubscriptionPriorityWithTx(tx, sub.Id, newPriority); err != nil {
				return fmt.Errorf("更新订阅 %d 优先级失败: %w", sub.Id, err)
			}
		}
	}

	return nil
}

// InsertPositionResult 插入位置计算结果
type InsertPositionResult struct {
	Position       int // 逻辑位置（第几个，从1开始）
	Priority       int // 实际将被分配的 priority 值
	TotalCount     int // 插入后总订阅数
	AffectedCount  int // 受影响的订阅数（需要 priority +1 的数量）
}

// CalculateInsertPosition 计算新订阅应该插入的位置（仅用于预览，不执行更新）
// 返回值：InsertPositionResult 包含逻辑位置和实际 priority 值
// 注意：Position 是逻辑序号（第1、2、3...），Priority 是实际数据库值（可能不连续）
func (s *SubscriptionPriorityService) CalculateInsertPosition(userId int64, newEndAt int64, newCreatedAt int64) (*InsertPositionResult, error) {
	if userId == 0 {
		return nil, errors.New("用户 ID 不能为空")
	}

	subscriptions, err := s.GetSortedActiveSubscriptions(userId)
	if err != nil {
		return nil, err
	}

	result := &InsertPositionResult{
		TotalCount: len(subscriptions) + 1,
	}

	if len(subscriptions) == 0 {
		result.Position = 1
		result.Priority = 1
		result.AffectedCount = 0
		return result, nil
	}

	// 找到插入位置（按 end_at -> created_at 排序后的逻辑位置）
	insertPos := len(subscriptions) // 默认插入到末尾
	for i, sub := range subscriptions {
		if sub.EndAt > newEndAt {
			insertPos = i
			break
		}
		if sub.EndAt == newEndAt && sub.CreatedAt > newCreatedAt {
			insertPos = i
			break
		}
	}

	result.Position = insertPos + 1
	result.AffectedCount = len(subscriptions) - insertPos

	// 计算实际 priority 值（基于前一个订阅的 priority + 1）
	if insertPos == 0 {
		result.Priority = 1
	} else {
		result.Priority = subscriptions[insertPos-1].Priority + 1
	}

	return result, nil
}
