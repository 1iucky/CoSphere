package service

import (
	"errors"
	"fmt"
	"sort"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

var (
	ErrAllGroupsFailed    = errors.New("all configured groups failed")
	ErrNoAvailableGroup   = errors.New("no available fallback group")
	randomChannelSelector = model.GetRandomSatisfiedChannel
)

func setGroupSelectionContext(c *gin.Context, group string, autoSmart bool) {
	if group == "" {
		return
	}
	common.SetContextKey(c, constant.ContextKeySelectedGroup, group)
	common.SetContextKey(c, constant.ContextKeyUsingGroup, group)
	if autoSmart {
		common.SetContextKey(c, constant.ContextKeyAutoSmartGroupUsed, true)
	} else {
		common.SetContextKey(c, constant.ContextKeyAutoSmartGroupUsed, false)
	}
}

func selectChannelFromSingleGroup(c *gin.Context, group string, modelName string, retry int) (*model.Channel, string, error) {
	channel, selectGroup, err := CacheGetRandomSatisfiedChannel(c, group, modelName, retry)
	if err == nil && channel != nil {
		setGroupSelectionContext(c, selectGroup, false)
	}
	return channel, selectGroup, err
}

func CacheGetRandomSatisfiedChannel(c *gin.Context, group string, modelName string, retry int) (*model.Channel, string, error) {
	var channel *model.Channel
	var err error
	selectGroup := group
	userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	if group == "auto" {
		if len(setting.GetAutoGroups()) == 0 {
			return nil, selectGroup, errors.New("auto groups is not enabled")
		}
		for _, autoGroup := range GetUserAutoGroup(userGroup) {
			logger.LogDebug(c, "Auto selecting group:", autoGroup)
			channel, _ = randomChannelSelector(autoGroup, modelName, retry)
			if channel == nil {
				continue
			} else {
				c.Set("auto_group", autoGroup)
				selectGroup = autoGroup
				logger.LogDebug(c, "Auto selected group:", autoGroup)
				break
			}
		}
	} else {
		channel, err = randomChannelSelector(group, modelName, retry)
		if err != nil {
			return nil, group, err
		}
	}
	return channel, selectGroup, nil
}

// SelectChannelWithPriority 按分组优先级选择渠道
func SelectChannelWithPriority(c *gin.Context, token *model.Token, modelName string, retry int) (*model.Channel, string, error) {
	priorities, err := token.GetGroupPriorities()
	if err != nil {
		logger.LogError(c, "Failed to parse group priorities: "+err.Error())
		fallbackGroup := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
		if fallbackGroup == "" {
			fallbackGroup = token.Group
		}
		return selectChannelFromSingleGroup(c, fallbackGroup, modelName, retry)
	}

	baseGroup := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	if baseGroup == "" {
		baseGroup = token.Group
	}
	if baseGroup == "" {
		baseGroup = common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	}

	if len(priorities) == 0 {
		return selectChannelFromSingleGroup(c, baseGroup, modelName, retry)
	}

	for _, p := range priorities {
		logger.LogDebug(c, fmt.Sprintf("Trying group: %s (priority: %d)", p.Group, p.Priority))

		channel, selectGroup, err := CacheGetRandomSatisfiedChannel(c, p.Group, modelName, retry)
		if err != nil {
			logger.LogDebug(c, fmt.Sprintf("Group %s failed: %s", p.Group, err.Error()))
			continue
		}

		if channel != nil {
			logger.LogInfo(c, fmt.Sprintf("Selected channel from group: %s", selectGroup))
			setGroupSelectionContext(c, selectGroup, false)
			return channel, selectGroup, nil
		}
	}

	logger.LogWarn(c, "All configured groups failed")

	if token.AutoSmartGroup {
		logger.LogInfo(c, "Auto smart group enabled, trying fallback groups by ratio")
		return selectChannelByRatio(c, modelName, retry, priorities)
	}

	return nil, "", ErrAllGroupsFailed
}

// selectChannelByRatio 按费率从低到高选择分组
func selectChannelByRatio(c *gin.Context, modelName string, retry int, excludePriorities []model.GroupPriority) (*model.Channel, string, error) {
	userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	usableGroups := GetUserUsableGroups(userGroup)
	ratioMap := ratio_setting.GetGroupRatioCopy()

	excludeMap := make(map[string]bool)
	for _, p := range excludePriorities {
		excludeMap[p.Group] = true
	}

	type groupWithRatio struct {
		group string
		ratio float64
	}
	candidates := make([]groupWithRatio, 0)

	for group := range usableGroups {
		if excludeMap[group] {
			continue
		}
		ratio, ok := ratioMap[group]
		if !ok {
			continue
		}
		candidates = append(candidates, groupWithRatio{
			group: group,
			ratio: ratio,
		})
	}

	if len(candidates) == 0 {
		return nil, "", ErrNoAvailableGroup
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].ratio < candidates[j].ratio
	})

	for _, item := range candidates {
		logger.LogDebug(c, fmt.Sprintf("Auto smart group trying: %s (ratio: %.2f)", item.group, item.ratio))

		channel, selectGroup, err := CacheGetRandomSatisfiedChannel(c, item.group, modelName, retry)
		if err != nil {
			logger.LogDebug(c, fmt.Sprintf("Auto smart group %s failed: %s", item.group, err.Error()))
			continue
		}

		if channel != nil {
			logger.LogInfo(c, fmt.Sprintf("Auto smart group selected: %s", selectGroup))
			setGroupSelectionContext(c, selectGroup, true)
			return channel, selectGroup, nil
		}
	}

	return nil, "", ErrAllGroupsFailed
}
