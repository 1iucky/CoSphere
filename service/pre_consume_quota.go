package service

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
)

// 订阅判定性能监控阈值
const (
	// 订阅判定目标耗时阈值 (P99 ≤5ms)
	subscriptionBillingThresholdMs = 5
	// 性能警告阈值 (>10ms 记录警告)
	subscriptionBillingWarningMs = 10
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

	// 性能监控：记录订阅判定开始时间
	var subscriptionBillingStart time.Time

	// 使用灰度判断逻辑，支持按用户ID灰度
	subscriptionEnabled, grayscaleReason := common.IsSubscriptionEnabledForUser(relayInfo.UserId)
	if subscriptionEnabled {
		subscriptionBillingStart = time.Now()

		// 记录灰度命中原因，便于调试
		if grayscaleReason != "global_enabled" {
			logger.LogInfo(c, fmt.Sprintf("用户 %d 命中订阅灰度: %s", relayInfo.UserId, grayscaleReason))
		}
		billingSvc := GetSubscriptionBillingService()
		ctx, err := billingSvc.SelectCandidate(int64(relayInfo.UserId), relayInfo.OriginModelName, "", relayInfo)
		if err != nil {
			// 性能监控：记录失败时的耗时
			logSubscriptionBillingDuration(c, subscriptionBillingStart, relayInfo.UserId, "select_candidate_error")
			return types.NewError(err, types.ErrorCodeSubscriptionOperationFailed, types.ErrOptionWithSkipRetry())
		}

		if ctx != nil {
			switch ctx.Source {
			case BillingSourceSkipped:
				relayInfo.BillingSkipReason = ctx.SkipReason
				relayInfo.BillingSource = BillingSourceSkipped
				// 性能监控：记录跳过时的耗时
				logSubscriptionBillingDuration(c, subscriptionBillingStart, relayInfo.UserId, "skipped")
			case BillingSourceWallet:
				relayInfo.BillingSource = BillingSourceWallet
				// 性能监控：记录使用钱包时的耗时
				logSubscriptionBillingDuration(c, subscriptionBillingStart, relayInfo.UserId, "wallet")
			default:
				// 当预扣额度 <= 0 时（如 freeModel），跳过订阅扣费，使用钱包路径
				if preConsumedQuota <= 0 {
					relayInfo.BillingSource = BillingSourceWallet
					logSubscriptionBillingDuration(c, subscriptionBillingStart, relayInfo.UserId, "wallet_free_model")
					break
				}
				result, err := billingSvc.TryBilling(ctx, int64(preConsumedQuota), relayInfo)
				if err != nil {
					// 性能监控：记录 TryBilling 失败时的耗时
					logSubscriptionBillingDuration(c, subscriptionBillingStart, relayInfo.UserId, "try_billing_error")
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
					// 性能监控：记录订阅扣费成功时的耗时
					logSubscriptionBillingDuration(c, subscriptionBillingStart, relayInfo.UserId, "subscription")
				} else if result.Source == BillingSourceFallback {
					// 性能监控：记录 fallback 时的耗时
					logSubscriptionBillingDuration(c, subscriptionBillingStart, relayInfo.UserId, "fallback")
				} else {
					// 性能监控：记录使用钱包时的耗时
					logSubscriptionBillingDuration(c, subscriptionBillingStart, relayInfo.UserId, "wallet_after_try")
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

		// 将 billing 信息存入 context，用于 IOCopyBytesGracefully 和 StreamScannerHandler 统一设置响应头
		c.Set(string(constant.ContextKeyBillingSource), relayInfo.BillingSource)
		if relayInfo.BillingSkipReason != "" {
			c.Set(string(constant.ContextKeyBillingSkipReason), relayInfo.BillingSkipReason)
		}

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

	// 将 billing 信息存入 context，用于 IOCopyBytesGracefully 和 StreamScannerHandler 统一设置响应头
	c.Set(string(constant.ContextKeyBillingSource), relayInfo.BillingSource)
	if relayInfo.BillingSkipReason != "" {
		c.Set(string(constant.ContextKeyBillingSkipReason), relayInfo.BillingSkipReason)
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

// ===================== 性能监控 =====================

// logSubscriptionBillingDuration 记录订阅判定耗时
// 当耗时超过阈值时记录警告日志，便于性能分析和优化
// 参数:
//   - c: gin.Context 用于日志上下文
//   - start: 开始时间
//   - userId: 用户 ID
//   - source: 计费来源（subscription/wallet/fallback/skipped/error）
func logSubscriptionBillingDuration(c *gin.Context, start time.Time, userId int, source string) {
	if start.IsZero() {
		return
	}

	duration := time.Since(start)
	durationMs := float64(duration.Microseconds()) / 1000.0

	// 记录性能指标到缓存服务（用于统计）
	GetSubscriptionBillingService().RecordBillingDuration(durationMs, source)

	// 超过警告阈值（>10ms）时记录警告
	if durationMs > subscriptionBillingWarningMs {
		logger.LogWarn(c, fmt.Sprintf(
			"[性能警告] 订阅判定耗时 %.2fms (阈值: %dms), user_id=%d, source=%s",
			durationMs, subscriptionBillingWarningMs, userId, source,
		))
		return
	}

	// 超过目标阈值（>5ms）时记录调试信息
	if durationMs > subscriptionBillingThresholdMs {
		logger.LogDebug(c, fmt.Sprintf(
			"[性能监控] 订阅判定耗时 %.2fms (目标: %dms), user_id=%d, source=%s",
			durationMs, subscriptionBillingThresholdMs, userId, source,
		))
	}
}
