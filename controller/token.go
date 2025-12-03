package controller

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// TokenRequest 用于接收创建/更新令牌的请求
type TokenRequest struct {
	model.Token
	GroupPrioritiesArray []model.GroupPriority `json:"group_priorities_array,omitempty"` // 接收前端发送的数组格式
}

func collectGroupsFromPriorities(priorities []model.GroupPriority) []string {
	result := make([]string, 0, len(priorities))
	seen := make(map[string]struct{})
	for _, priority := range priorities {
		groupName := strings.TrimSpace(priority.Group)
		if groupName == "" {
			continue
		}
		if _, ok := seen[groupName]; ok {
			continue
		}
		seen[groupName] = struct{}{}
		result = append(result, groupName)
	}
	return result
}

func getRequestUserGroup(c *gin.Context) string {
	if group := common.GetContextKeyString(c, constant.ContextKeyUserGroup); group != "" {
		return group
	}
	return c.GetString(string(constant.ContextKeyUserGroup))
}

func ensureUserGroupsAccessible(c *gin.Context, groups []string) error {
	if len(groups) == 0 {
		return nil
	}
	userGroup := getRequestUserGroup(c)
	usableGroups := service.GetUserUsableGroups(userGroup)
	for _, group := range groups {
		if group == "" {
			continue
		}
		if _, ok := usableGroups[group]; !ok {
			return fmt.Errorf("无权访问分组: %s", group)
		}
	}
	return nil
}

func GetAllTokens(c *gin.Context) {
	userId := c.GetInt("id")
	pageInfo := common.GetPageQuery(c)
	tokens, err := model.GetAllUserTokens(userId, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	total, _ := model.CountUserTokens(userId)
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(tokens)
	common.ApiSuccess(c, pageInfo)
	return
}

func SearchTokens(c *gin.Context) {
	userId := c.GetInt("id")
	keyword := c.Query("keyword")
	token := c.Query("token")
	tokens, err := model.SearchUserTokens(userId, keyword, token)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    tokens,
	})
	return
}

func GetToken(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	userId := c.GetInt("id")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	token, err := model.GetTokenByIds(id, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    token,
	})
	return
}

func GetTokenStatus(c *gin.Context) {
	tokenId := c.GetInt("token_id")
	userId := c.GetInt("id")
	token, err := model.GetTokenByIds(tokenId, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	expiredAt := token.ExpiredTime
	if expiredAt == -1 {
		expiredAt = 0
	}
	c.JSON(http.StatusOK, gin.H{
		"object":          "credit_summary",
		"total_granted":   token.RemainQuota,
		"total_used":      0, // not supported currently
		"total_available": token.RemainQuota,
		"expires_at":      expiredAt * 1000,
	})
}

func GetTokenUsage(c *gin.Context) {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "No Authorization header",
		})
		return
	}

	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "Invalid Bearer token",
		})
		return
	}
	tokenKey := parts[1]

	token, err := model.GetTokenByKey(strings.TrimPrefix(tokenKey, "sk-"), false)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	expiredAt := token.ExpiredTime
	if expiredAt == -1 {
		expiredAt = 0
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    true,
		"message": "ok",
		"data": gin.H{
			"object":               "token_usage",
			"name":                 token.Name,
			"total_granted":        token.RemainQuota + token.UsedQuota,
			"total_used":           token.UsedQuota,
			"total_available":      token.RemainQuota,
			"unlimited_quota":      token.UnlimitedQuota,
			"model_limits":         token.GetModelLimitsMap(),
			"model_limits_enabled": token.ModelLimitsEnabled,
			"expires_at":           expiredAt,
		},
	})
}

func AddToken(c *gin.Context) {
	tokenReq := TokenRequest{}
	err := c.ShouldBindJSON(&tokenReq)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if len(tokenReq.Name) > 30 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "令牌名称过长",
		})
		return
	}
	key, err := common.GenerateKey()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "生成令牌失败",
		})
		common.SysLog("failed to generate token key: " + err.Error())
		return
	}
	trimmedGroup := strings.TrimSpace(tokenReq.Group)
	cleanToken := model.Token{
		UserId:             c.GetInt("id"),
		Name:               tokenReq.Name,
		Key:                key,
		CreatedTime:        common.GetTimestamp(),
		AccessedTime:       common.GetTimestamp(),
		ExpiredTime:        tokenReq.ExpiredTime,
		RemainQuota:        tokenReq.RemainQuota,
		UnlimitedQuota:     tokenReq.UnlimitedQuota,
		ModelLimitsEnabled: tokenReq.ModelLimitsEnabled,
		ModelLimits:        tokenReq.ModelLimits,
		AllowIps:           tokenReq.AllowIps,
		Group:              trimmedGroup,
		AutoSmartGroup:     tokenReq.AutoSmartGroup,
	}

	// 处理分组优先级
	if len(tokenReq.GroupPrioritiesArray) > 0 {
		if err := ensureUserGroupsAccessible(c, collectGroupsFromPriorities(tokenReq.GroupPrioritiesArray)); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
		err = cleanToken.SetGroupPriorities(tokenReq.GroupPrioritiesArray)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "分组优先级设置失败: " + err.Error(),
			})
			return
		}
	} else if trimmedGroup != "" {
		if err := ensureUserGroupsAccessible(c, []string{trimmedGroup}); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	}

	err = cleanToken.Insert()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}

func DeleteToken(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	userId := c.GetInt("id")
	err := model.DeleteTokenById(id, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}

func UpdateToken(c *gin.Context) {
	userId := c.GetInt("id")
	statusOnly := c.Query("status_only")
	tokenReq := TokenRequest{}
	err := c.ShouldBindJSON(&tokenReq)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if len(tokenReq.Name) > 30 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "令牌名称过长",
		})
		return
	}
	cleanToken, err := model.GetTokenByIds(tokenReq.Id, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if tokenReq.Status == common.TokenStatusEnabled {
		if cleanToken.Status == common.TokenStatusExpired && cleanToken.ExpiredTime <= common.GetTimestamp() && cleanToken.ExpiredTime != -1 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "令牌已过期，无法启用，请先修改令牌过期时间，或者设置为永不过期",
			})
			return
		}
		if cleanToken.Status == common.TokenStatusExhausted && cleanToken.RemainQuota <= 0 && !cleanToken.UnlimitedQuota {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "令牌可用额度已用尽，无法启用，请先修改令牌剩余额度，或者设置为无限额度",
			})
			return
		}
	}
	if statusOnly != "" {
		cleanToken.Status = tokenReq.Status
	} else {
		// If you add more fields, please also update token.Update()
		cleanToken.Name = tokenReq.Name
		cleanToken.ExpiredTime = tokenReq.ExpiredTime
		cleanToken.RemainQuota = tokenReq.RemainQuota
		cleanToken.UnlimitedQuota = tokenReq.UnlimitedQuota
		cleanToken.ModelLimitsEnabled = tokenReq.ModelLimitsEnabled
		cleanToken.ModelLimits = tokenReq.ModelLimits
		cleanToken.AllowIps = tokenReq.AllowIps
		cleanToken.Group = strings.TrimSpace(tokenReq.Group)
		cleanToken.AutoSmartGroup = tokenReq.AutoSmartGroup

		// 处理分组优先级
		if len(tokenReq.GroupPrioritiesArray) > 0 {
			if err := ensureUserGroupsAccessible(c, collectGroupsFromPriorities(tokenReq.GroupPrioritiesArray)); err != nil {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": err.Error(),
				})
				return
			}
			err = cleanToken.SetGroupPriorities(tokenReq.GroupPrioritiesArray)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": "分组优先级设置失败: " + err.Error(),
				})
				return
			}
		} else if tokenReq.GroupPriorities == "" {
			// 如果前端明确传递空字符串，清空优先级
			cleanToken.GroupPriorities = ""
			if cleanToken.Group != "" {
				if err := ensureUserGroupsAccessible(c, []string{cleanToken.Group}); err != nil {
					c.JSON(http.StatusOK, gin.H{
						"success": false,
						"message": err.Error(),
					})
					return
				}
			}
		}
	}
	err = cleanToken.Update()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    cleanToken,
	})
	return
}

type TokenBatch struct {
	Ids []int `json:"ids"`
}

func DeleteTokenBatch(c *gin.Context) {
	tokenBatch := TokenBatch{}
	if err := c.ShouldBindJSON(&tokenBatch); err != nil || len(tokenBatch.Ids) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "参数错误",
		})
		return
	}
	userId := c.GetInt("id")
	count, err := model.BatchDeleteTokens(tokenBatch.Ids, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    count,
	})
}
