package service

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
)

func ReturnPreConsumedQuota(c *gin.Context, relayInfo *relaycommon.RelayInfo) {
	if relayInfo == nil {
		return
	}

	if relayInfo.BillingSource == BillingSourceSubscription && relayInfo.SubscriptionContextId != "" {
		contextId := relayInfo.SubscriptionContextId
		gopool.Go(func() {
			if err := GetSubscriptionUsageService().RollbackPreConsume(contextId); err != nil {
				common.SysLog("error rollback subscription pre-consume: " + err.Error())
			}
		})
		// subscription 计费不预扣用户余额，仅回滚 token 预扣
		if relayInfo.FinalPreConsumedQuota != 0 && !relayInfo.IsPlayground {
			tokenRefund := relayInfo.FinalPreConsumedQuota
			gopool.Go(func() {
				if err := model.IncreaseTokenQuota(relayInfo.TokenId, relayInfo.TokenKey, tokenRefund); err != nil {
					common.SysLog("error return pre-consumed token quota: " + err.Error())
				}
			})
		}
		return
	}

	if relayInfo.FinalPreConsumedQuota != 0 {
		logger.LogInfo(c, fmt.Sprintf("用户 %d 请求失败, 返还预扣费额度 %s", relayInfo.UserId, logger.FormatQuota(relayInfo.FinalPreConsumedQuota)))
		gopool.Go(func() {
			relayInfoCopy := *relayInfo

			err := PostConsumeQuota(&relayInfoCopy, -relayInfoCopy.FinalPreConsumedQuota, 0, false)
			if err != nil {
				common.SysLog("error return pre-consumed quota: " + err.Error())
			}
		})
	}
}

// PreConsumeQuota checks if the user has enough quota to pre-consume.
// It returns the pre-consumed quota if successful, or an error if not.
func PreConsumeQuota(c *gin.Context, preConsumedQuota int, relayInfo *relaycommon.RelayInfo) *types.NewAPIError {
	if relayInfo == nil {
		return types.NewError(errors.New("relayInfo 不能为空"), types.ErrorCodeInvalidRequestParams, types.ErrOptionWithSkipRetry())
	}

	useWallet := true
	var billingCtx *BillingContext

	if common.IsSubscriptionEnabled() {
		billingSvc := GetSubscriptionBillingService()
		ctx, err := billingSvc.SelectCandidate(int64(relayInfo.UserId), relayInfo.OriginModelName, "", relayInfo)
		if err != nil {
			return types.NewError(err, types.ErrorCodeSubscriptionOperationFailed, types.ErrOptionWithSkipRetry())
		}

		if ctx != nil {
			switch ctx.Source {
			case BillingSourceSkipped:
				relayInfo.BillingSkipReason = ctx.SkipReason
				relayInfo.BillingSource = BillingSourceWallet
			case BillingSourceWallet:
				relayInfo.BillingSource = BillingSourceWallet
			default:
				result, err := billingSvc.TryBilling(ctx, int64(preConsumedQuota), relayInfo)
				if err != nil {
					if apiErr, ok := err.(*types.NewAPIError); ok {
						return apiErr
					}
					return types.NewError(err, types.ErrorCodeSubscriptionOperationFailed, types.ErrOptionWithSkipRetry())
				}
				relayInfo.BillingSource = result.Source
				if result.Source == BillingSourceSubscription && result.Context != nil {
					useWallet = false
					billingCtx = result.Context
					relayInfo.SubscriptionId = result.SubscriptionId
					if result.Context.PreConsumeContext != nil {
						relayInfo.SubscriptionContextId = result.Context.PreConsumeContext.ContextId
					}
					relayInfo.SubscriptionPreConsumedQuota = int64(preConsumedQuota)
					relayInfo.SubscriptionChannelGroup = result.Context.ChannelGroup
					if group := firstGroup(result.Context.ChannelGroup); group != "" {
						common.SetContextKey(c, constant.ContextKeyUsingGroup, group)
						relayInfo.UsingGroup = group
					}
				}
			}
		}
	}

	if !useWallet {
		// subscription 路径：不扣用户余额，但仍需校验/预扣 token 额度
		if preConsumedQuota > 0 && !relayInfo.IsPlayground {
			if err := PreConsumeTokenQuota(relayInfo, preConsumedQuota); err != nil {
				if billingCtx != nil && billingCtx.PreConsumeContext != nil {
					_ = GetSubscriptionUsageService().RollbackPreConsume(billingCtx.PreConsumeContext.ContextId)
				}
				return types.NewErrorWithStatusCode(err, types.ErrorCodePreConsumeTokenQuotaFailed, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
			}
		}
		relayInfo.FinalPreConsumedQuota = preConsumedQuota
		return nil
	}

	userQuota, err := model.GetUserQuota(relayInfo.UserId, false)
	if err != nil {
		return types.NewError(err, types.ErrorCodeQueryDataError, types.ErrOptionWithSkipRetry())
	}
	if userQuota <= 0 {
		return types.NewErrorWithStatusCode(fmt.Errorf("用户额度不足, 剩余额度: %s", logger.FormatQuota(userQuota)), types.ErrorCodeInsufficientUserQuota, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
	}
	if userQuota-preConsumedQuota < 0 {
		return types.NewErrorWithStatusCode(fmt.Errorf("预扣费额度失败, 用户剩余额度: %s, 需要预扣费额度: %s", logger.FormatQuota(userQuota), logger.FormatQuota(preConsumedQuota)), types.ErrorCodeInsufficientUserQuota, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
	}

	trustQuota := common.GetTrustQuota()

	relayInfo.UserQuota = userQuota
	if userQuota > trustQuota {
		// 用户额度充足，判断令牌额度是否充足
		if !relayInfo.TokenUnlimited {
			// 非无限令牌，判断令牌额度是否充足
			tokenQuota := c.GetInt("token_quota")
			if tokenQuota > trustQuota {
				// 令牌额度充足，信任令牌
				preConsumedQuota = 0
				logger.LogInfo(c, fmt.Sprintf("用户 %d 剩余额度 %s 且令牌 %d 额度 %d 充足, 信任且不需要预扣费", relayInfo.UserId, logger.FormatQuota(userQuota), relayInfo.TokenId, tokenQuota))
			}
		} else {
			// in this case, we do not pre-consume quota
			// because the user has enough quota
			preConsumedQuota = 0
			logger.LogInfo(c, fmt.Sprintf("用户 %d 额度充足且为无限额度令牌, 信任且不需要预扣费", relayInfo.UserId))
		}
	}

	if preConsumedQuota > 0 {
		err := PreConsumeTokenQuota(relayInfo, preConsumedQuota)
		if err != nil {
			return types.NewErrorWithStatusCode(err, types.ErrorCodePreConsumeTokenQuotaFailed, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
		}
		err = model.DecreaseUserQuota(relayInfo.UserId, preConsumedQuota)
		if err != nil {
			return types.NewError(err, types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
		}
		logger.LogInfo(c, fmt.Sprintf("用户 %d 预扣费 %s, 预扣费后剩余额度: %s", relayInfo.UserId, logger.FormatQuota(preConsumedQuota), logger.FormatQuota(userQuota-preConsumedQuota)))
	}
	relayInfo.FinalPreConsumedQuota = preConsumedQuota
	if relayInfo.BillingSource == "" {
		relayInfo.BillingSource = BillingSourceWallet
	}
	return nil
}

func firstGroup(groupStr string) string {
	if groupStr == "" {
		return ""
	}
	parts := strings.Split(groupStr, ",")
	for _, part := range parts {
		group := strings.TrimSpace(part)
		if group != "" {
			return group
		}
	}
	return ""
}
