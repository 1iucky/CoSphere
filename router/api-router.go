package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"

	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
)

func SetApiRouter(router *gin.Engine) {
	apiRouter := router.Group("/api")
	apiRouter.Use(gzip.Gzip(gzip.DefaultCompression))
	apiRouter.Use(middleware.GlobalAPIRateLimit())
	{
		apiRouter.GET("/setup", controller.GetSetup)
		apiRouter.POST("/setup", controller.PostSetup)
		apiRouter.GET("/status", controller.GetStatus)
		apiRouter.GET("/uptime/status", controller.GetUptimeKumaStatus)
		apiRouter.GET("/models", middleware.UserAuth(), controller.DashboardListModels)
		apiRouter.GET("/status/test", middleware.AdminAuth(), controller.TestStatus)
		apiRouter.GET("/notice", controller.GetNotice)
		apiRouter.GET("/user-agreement", controller.GetUserAgreement)
		apiRouter.GET("/privacy-policy", controller.GetPrivacyPolicy)
		apiRouter.GET("/about", controller.GetAbout)
		//apiRouter.GET("/midjourney", controller.GetMidjourney)
		apiRouter.GET("/home_page_content", controller.GetHomePageContent)
		apiRouter.GET("/pricing", middleware.TryUserAuth(), controller.GetPricing)
		apiRouter.GET("/verification", middleware.EmailVerificationRateLimit(), middleware.TurnstileCheck(), controller.SendEmailVerification)
		apiRouter.GET("/reset_password", middleware.CriticalRateLimit(), middleware.TurnstileCheck(), controller.SendPasswordResetEmail)
		apiRouter.POST("/user/reset", middleware.CriticalRateLimit(), controller.ResetPassword)
		apiRouter.GET("/oauth/github", middleware.CriticalRateLimit(), controller.GitHubOAuth)
		apiRouter.GET("/oauth/google", middleware.CriticalRateLimit(), controller.GoogleOAuth)
		apiRouter.GET("/oauth/discord", middleware.CriticalRateLimit(), controller.DiscordOAuth)
		apiRouter.GET("/oauth/oidc", middleware.CriticalRateLimit(), controller.OidcAuth)
		apiRouter.GET("/oauth/linuxdo", middleware.CriticalRateLimit(), controller.LinuxdoOAuth)
		apiRouter.GET("/oauth/state", middleware.CriticalRateLimit(), controller.GenerateOAuthCode)
		apiRouter.GET("/oauth/wechat", middleware.CriticalRateLimit(), controller.WeChatAuth)
		apiRouter.GET("/oauth/wechat/bind", middleware.CriticalRateLimit(), controller.WeChatBind)
		apiRouter.GET("/oauth/email/bind", middleware.CriticalRateLimit(), controller.EmailBind)
		apiRouter.GET("/oauth/telegram/login", middleware.CriticalRateLimit(), controller.TelegramLogin)
		apiRouter.GET("/oauth/telegram/bind", middleware.CriticalRateLimit(), controller.TelegramBind)
		apiRouter.GET("/ratio_config", middleware.CriticalRateLimit(), controller.GetRatioConfig)

		apiRouter.POST("/stripe/webhook", controller.StripeWebhook)
		apiRouter.POST("/creem/webhook", controller.CreemWebhook)

		// Universal secure verification routes
		apiRouter.POST("/verify", middleware.UserAuth(), middleware.CriticalRateLimit(), controller.UniversalVerify)
		apiRouter.GET("/verify/status", middleware.UserAuth(), controller.GetVerificationStatus)

		userRoute := apiRouter.Group("/user")
		{
			userRoute.POST("/register", middleware.CriticalRateLimit(), middleware.TurnstileCheck(), controller.Register)
			userRoute.POST("/login", middleware.CriticalRateLimit(), middleware.TurnstileCheck(), controller.Login)
			userRoute.POST("/login/2fa", middleware.CriticalRateLimit(), controller.Verify2FALogin)
			userRoute.POST("/passkey/login/begin", middleware.CriticalRateLimit(), controller.PasskeyLoginBegin)
			userRoute.POST("/passkey/login/finish", middleware.CriticalRateLimit(), controller.PasskeyLoginFinish)
			//userRoute.POST("/tokenlog", middleware.CriticalRateLimit(), controller.TokenLog)
			userRoute.GET("/logout", controller.Logout)
			userRoute.GET("/epay/notify", controller.EpayNotify)
			userRoute.GET("/groups", controller.GetUserGroups)

			selfRoute := userRoute.Group("/")
			selfRoute.Use(middleware.UserAuth())
			{
				selfRoute.GET("/self/groups", controller.GetUserGroups)
				selfRoute.GET("/self", controller.GetSelf)
				selfRoute.GET("/models", controller.GetUserModels)
				selfRoute.PUT("/self", controller.UpdateSelf)
				selfRoute.DELETE("/self", controller.DeleteSelf)
				selfRoute.GET("/token", controller.GenerateAccessToken)
				selfRoute.GET("/passkey", controller.PasskeyStatus)
				selfRoute.POST("/passkey/register/begin", controller.PasskeyRegisterBegin)
				selfRoute.POST("/passkey/register/finish", controller.PasskeyRegisterFinish)
				selfRoute.POST("/passkey/verify/begin", controller.PasskeyVerifyBegin)
				selfRoute.POST("/passkey/verify/finish", controller.PasskeyVerifyFinish)
				selfRoute.DELETE("/passkey", controller.PasskeyDelete)
				selfRoute.GET("/aff", controller.GetAffCode)
				selfRoute.GET("/topup/info", controller.GetTopUpInfo)
				selfRoute.GET("/topup/self", controller.GetUserTopUps)
				selfRoute.POST("/topup", middleware.CriticalRateLimit(), controller.TopUp)
				selfRoute.POST("/pay", middleware.CriticalRateLimit(), controller.RequestEpay)
				selfRoute.POST("/amount", controller.RequestAmount)
				selfRoute.POST("/stripe/pay", middleware.CriticalRateLimit(), controller.RequestStripePay)
				selfRoute.POST("/stripe/amount", controller.RequestStripeAmount)
				selfRoute.POST("/creem/pay", middleware.CriticalRateLimit(), controller.RequestCreemPay)
				selfRoute.POST("/aff_transfer", controller.TransferAffQuota)
				selfRoute.PUT("/setting", controller.UpdateUserSetting)

				// 2FA routes
				selfRoute.GET("/2fa/status", controller.Get2FAStatus)
				selfRoute.POST("/2fa/setup", controller.Setup2FA)
				selfRoute.POST("/2fa/enable", controller.Enable2FA)
				selfRoute.POST("/2fa/disable", controller.Disable2FA)
				selfRoute.POST("/2fa/backup_codes", controller.RegenerateBackupCodes)
			}

			adminRoute := userRoute.Group("/")
			adminRoute.Use(middleware.AdminAuth())
			{
				adminRoute.GET("/", controller.GetAllUsers)
				adminRoute.GET("/topup", controller.GetAllTopUps)
				adminRoute.POST("/topup/complete", controller.AdminCompleteTopUp)
				adminRoute.GET("/search", controller.SearchUsers)
				adminRoute.GET("/:id", controller.GetUser)
				adminRoute.POST("/", controller.CreateUser)
				adminRoute.POST("/manage", controller.ManageUser)
				adminRoute.PUT("/", controller.UpdateUser)
				adminRoute.DELETE("/:id", controller.DeleteUser)
				adminRoute.DELETE("/:id/reset_passkey", controller.AdminResetPasskey)

				// Admin 2FA routes
				adminRoute.GET("/2fa/stats", controller.Admin2FAStats)
				adminRoute.DELETE("/:id/2fa", controller.AdminDisable2FA)
			}
		}
		optionRoute := apiRouter.Group("/option")
		optionRoute.Use(middleware.RootAuth())
		{
			optionRoute.GET("/", controller.GetOptions)
			optionRoute.PUT("/", controller.UpdateOption)
			optionRoute.POST("/rest_model_ratio", controller.ResetModelRatio)
			optionRoute.POST("/migrate_console_setting", controller.MigrateConsoleSetting) // 用于迁移检测的旧键，下个版本会删除
		}
		ratioSyncRoute := apiRouter.Group("/ratio_sync")
		ratioSyncRoute.Use(middleware.RootAuth())
		{
			ratioSyncRoute.GET("/channels", controller.GetSyncableChannels)
			ratioSyncRoute.POST("/fetch", controller.FetchUpstreamRatios)
		}
		channelRoute := apiRouter.Group("/channel")
		channelRoute.Use(middleware.AdminAuth())
		{
			channelRoute.GET("/", controller.GetAllChannels)
			channelRoute.GET("/search", controller.SearchChannels)
			channelRoute.GET("/models", controller.ChannelListModels)
			channelRoute.GET("/models_enabled", controller.EnabledListModels)
			channelRoute.GET("/:id", controller.GetChannel)
			channelRoute.POST("/:id/key", middleware.RootAuth(), middleware.CriticalRateLimit(), middleware.DisableCache(), middleware.SecureVerificationRequired(), controller.GetChannelKey)
			channelRoute.GET("/test", controller.TestAllChannels)
			channelRoute.GET("/test/:id", controller.TestChannel)
			channelRoute.GET("/update_balance", controller.UpdateAllChannelsBalance)
			channelRoute.GET("/update_balance/:id", controller.UpdateChannelBalance)
			channelRoute.POST("/", controller.AddChannel)
			channelRoute.PUT("/", controller.UpdateChannel)
			channelRoute.DELETE("/disabled", controller.DeleteDisabledChannel)
			channelRoute.POST("/tag/disabled", controller.DisableTagChannels)
			channelRoute.POST("/tag/enabled", controller.EnableTagChannels)
			channelRoute.PUT("/tag", controller.EditTagChannels)
			channelRoute.DELETE("/:id", controller.DeleteChannel)
			channelRoute.POST("/batch", controller.DeleteChannelBatch)
			channelRoute.POST("/fix", controller.FixChannelsAbilities)
			channelRoute.GET("/fetch_models/:id", controller.FetchUpstreamModels)
			channelRoute.POST("/fetch_models", controller.FetchModels)
			channelRoute.POST("/batch/tag", controller.BatchSetChannelTag)
			channelRoute.GET("/tag/models", controller.GetTagModels)
			channelRoute.POST("/copy/:id", controller.CopyChannel)
			channelRoute.POST("/multi_key/manage", controller.ManageMultiKeys)
		}
		tokenRoute := apiRouter.Group("/token")
		tokenRoute.Use(middleware.UserAuth())
		{
			tokenRoute.GET("/", controller.GetAllTokens)
			tokenRoute.GET("/search", controller.SearchTokens)
			tokenRoute.GET("/:id", controller.GetToken)
			tokenRoute.POST("/", controller.AddToken)
			tokenRoute.PUT("/", controller.UpdateToken)
			tokenRoute.DELETE("/:id", controller.DeleteToken)
			tokenRoute.POST("/batch", controller.DeleteTokenBatch)
			tokenRoute.POST("/:id/reset_rate_limit", controller.ResetTokenRateLimit)
		}

		usageRoute := apiRouter.Group("/usage")
		usageRoute.Use(middleware.CriticalRateLimit())
		{
			tokenUsageRoute := usageRoute.Group("/token")
			tokenUsageRoute.Use(middleware.TokenAuth())
			{
				tokenUsageRoute.GET("/", controller.GetTokenUsage)
			}
		}

		redemptionRoute := apiRouter.Group("/redemption")
		redemptionRoute.Use(middleware.AdminAuth())
		{
			redemptionRoute.GET("/", controller.GetAllRedemptions)
			redemptionRoute.GET("/search", controller.SearchRedemptions)
			redemptionRoute.GET("/:id", controller.GetRedemption)
			redemptionRoute.POST("/", controller.AddRedemption)
			redemptionRoute.PUT("/", controller.UpdateRedemption)
			redemptionRoute.DELETE("/invalid", controller.DeleteInvalidRedemption)
			redemptionRoute.DELETE("/:id", controller.DeleteRedemption)
		}
		logRoute := apiRouter.Group("/log")
		logRoute.GET("/", middleware.AdminAuth(), controller.GetAllLogs)
		logRoute.DELETE("/", middleware.AdminAuth(), controller.DeleteHistoryLogs)
		logRoute.GET("/stat", middleware.AdminAuth(), controller.GetLogsStat)
		logRoute.GET("/self/stat", middleware.UserAuth(), controller.GetLogsSelfStat)
		logRoute.GET("/search", middleware.AdminAuth(), controller.SearchAllLogs)
		logRoute.GET("/self", middleware.UserAuth(), controller.GetUserLogs)
		logRoute.GET("/self/search", middleware.UserAuth(), controller.SearchUserLogs)

		dataRoute := apiRouter.Group("/data")
		dataRoute.GET("/", middleware.AdminAuth(), controller.GetAllQuotaDates)
		dataRoute.GET("/self", middleware.UserAuth(), controller.GetUserQuotaDates)

		logRoute.Use(middleware.CORS())
		{
			logRoute.GET("/token", middleware.UserAuth(), middleware.TokenQueryRateLimit(), controller.GetLogByKey)
			logRoute.GET("/token/usage", middleware.TokenQueryRateLimit(), controller.GetTokenUsageOverview)
		}
		groupRoute := apiRouter.Group("/group")
		groupRoute.Use(middleware.AdminAuth())
		{
			groupRoute.GET("/", controller.GetGroups)
		}

		prefillGroupRoute := apiRouter.Group("/prefill_group")
		prefillGroupRoute.Use(middleware.AdminAuth())
		{
			prefillGroupRoute.GET("", controller.GetPrefillGroups)
			prefillGroupRoute.POST("", controller.CreatePrefillGroup)
			prefillGroupRoute.PUT("", controller.UpdatePrefillGroup)
			prefillGroupRoute.DELETE("/:id", controller.DeletePrefillGroup)
		}

		mjRoute := apiRouter.Group("/mj")
		mjRoute.GET("/self", middleware.UserAuth(), controller.GetUserMidjourney)
		mjRoute.GET("/", middleware.AdminAuth(), controller.GetAllMidjourney)

		taskRoute := apiRouter.Group("/task")
		{
			taskRoute.GET("/self", middleware.UserAuth(), controller.GetUserTask)
			taskRoute.GET("/", middleware.AdminAuth(), controller.GetAllTask)
		}

		vendorRoute := apiRouter.Group("/vendors")
		vendorRoute.Use(middleware.AdminAuth())
		{
			vendorRoute.GET("/", controller.GetAllVendors)
			vendorRoute.GET("/search", controller.SearchVendors)
			vendorRoute.GET("/:id", controller.GetVendorMeta)
			vendorRoute.POST("/", controller.CreateVendorMeta)
			vendorRoute.PUT("/", controller.UpdateVendorMeta)
			vendorRoute.DELETE("/:id", controller.DeleteVendorMeta)
		}

		modelsRoute := apiRouter.Group("/models")
		modelsRoute.Use(middleware.AdminAuth())
		{
			modelsRoute.GET("/sync_upstream/preview", controller.SyncUpstreamPreview)
			modelsRoute.POST("/sync_upstream", controller.SyncUpstreamModels)
			modelsRoute.GET("/missing", controller.GetMissingModels)
			modelsRoute.GET("/", controller.GetAllModelsMeta)
			modelsRoute.GET("/search", controller.SearchModelsMeta)
			modelsRoute.GET("/:id", controller.GetModelMeta)
			modelsRoute.POST("/", controller.CreateModelMeta)
			modelsRoute.PUT("/", controller.UpdateModelMeta)
			modelsRoute.DELETE("/:id", controller.DeleteModelMeta)
		}

		// ===================== 订阅系统路由 =====================

		// Admin 套餐管理路由
		subscriptionPlanRoute := apiRouter.Group("/admin/subscription-plans")
		subscriptionPlanRoute.Use(middleware.AdminAuth())
		{
			subscriptionPlanRoute.GET("/", controller.GetAllSubscriptionPlans)
			subscriptionPlanRoute.GET("/:id", controller.GetSubscriptionPlan)
			subscriptionPlanRoute.POST("/", controller.CreateSubscriptionPlan)
			subscriptionPlanRoute.PUT("/:id", controller.UpdateSubscriptionPlan)
			subscriptionPlanRoute.POST("/:id/publish", controller.PublishSubscriptionPlan)
			subscriptionPlanRoute.POST("/:id/unpublish", controller.UnpublishSubscriptionPlan)
			subscriptionPlanRoute.DELETE("/:id", controller.DeleteSubscriptionPlan)
		}

		// Admin 订阅管理路由
		subscriptionRoute := apiRouter.Group("/admin/subscriptions")
		subscriptionRoute.Use(middleware.AdminAuth())
		{
			subscriptionRoute.GET("/", controller.GetAllSubscriptions)
			subscriptionRoute.GET("/:id", controller.GetSubscriptionAdmin)
			subscriptionRoute.POST("/:id/cancel", controller.CancelSubscriptionAdmin)
			subscriptionRoute.POST("/:id/refund", controller.RefundSubscriptionAdmin)
			subscriptionRoute.PUT("/:id/priority", controller.UpdateSubscriptionPriorityAdmin)
			subscriptionRoute.POST("/:id/activate", controller.ActivateSubscriptionAdmin)
			subscriptionRoute.POST("/:id/expire", controller.ExpireSubscriptionAdmin)
		}

		// Admin 订单管理路由
		subscriptionOrderRoute := apiRouter.Group("/admin/subscription-orders")
		subscriptionOrderRoute.Use(middleware.AdminAuth())
		{
			subscriptionOrderRoute.GET("/", controller.GetAllSubscriptionOrders)
			subscriptionOrderRoute.GET("/:id", controller.GetSubscriptionOrder)
			subscriptionOrderRoute.POST("/:id/refund", controller.RefundSubscriptionOrder)
			subscriptionOrderRoute.POST("/:id/cancel", controller.CancelSubscriptionOrderAdmin)
		}

		// Admin 优惠券管理路由
		subscriptionCouponRoute := apiRouter.Group("/admin/subscription-coupons")
		subscriptionCouponRoute.Use(middleware.AdminAuth())
		{
			subscriptionCouponRoute.GET("/", controller.GetAllSubscriptionCoupons)
			subscriptionCouponRoute.GET("/:id", controller.GetSubscriptionCoupon)
			subscriptionCouponRoute.POST("/", controller.CreateSubscriptionCoupon)
			subscriptionCouponRoute.PUT("/:id", controller.UpdateSubscriptionCoupon)
			subscriptionCouponRoute.DELETE("/:id", controller.DeleteSubscriptionCoupon)
			subscriptionCouponRoute.POST("/:id/bind-redemption", controller.BindCouponToRedemption)
			subscriptionCouponRoute.DELETE("/:id/unbind", controller.UnbindCouponFromRedemption)
			subscriptionCouponRoute.GET("/:id/bindings", controller.GetCouponBindings)
		}

		// 公开套餐查询路由（无需登录）
		apiRouter.GET("/subscription-plans", controller.GetAvailablePlans)
		apiRouter.GET("/subscription-plans/:id", controller.GetAvailablePlanDetail)

		// 用户套餐查询路由（兼容规格 /api/user/subscription-plans）
		userPlanRoute := apiRouter.Group("/user/subscription-plans")
		userPlanRoute.Use(middleware.UserAuth())
		{
			userPlanRoute.GET("", controller.GetAvailablePlans)
			userPlanRoute.GET("/:id", controller.GetAvailablePlanDetail)
		}

		// 用户订阅管理路由（兼容规格 /api/user/subscriptions/*）
		userSubscriptionsRoute := apiRouter.Group("/user/subscriptions")
		userSubscriptionsRoute.Use(middleware.UserAuth())
		{
			// 订阅列表和详情
			userSubscriptionsRoute.GET("", controller.GetUserSubscriptions)
			userSubscriptionsRoute.GET("/active", controller.GetUserActiveSubscriptions)
			userSubscriptionsRoute.GET("/:id", controller.GetUserSubscriptionDetail)
			userSubscriptionsRoute.GET("/:id/usage", controller.GetUserSubscriptionUsage)
			userSubscriptionsRoute.GET("/:id/history", controller.GetUserSubscriptionHistory)
			userSubscriptionsRoute.PUT("/:id/settings", controller.UpdateUserSubscriptionSettings)
			userSubscriptionsRoute.POST("/:id/cancel", controller.CancelUserSubscription)
			// 自动兜底开关
			userSubscriptionsRoute.PUT("/:id/auto-wallet", controller.UpdateSubscriptionAutoWallet)
			// 批量更新优先级
			userSubscriptionsRoute.PUT("/priorities", controller.BatchUpdateSubscriptionPriorities)
			userSubscriptionsRoute.POST("/reorder", controller.ReorderUserSubscriptionPriorities)
			// 订单相关（兼容规格 /api/user/subscriptions/orders）
			userSubscriptionsRoute.POST("/orders", controller.CreateOrder)
			userSubscriptionsRoute.POST("/orders/preview", controller.PreviewOrder)
			userSubscriptionsRoute.POST("/orders/purchase", controller.PurchaseSubscription)
			userSubscriptionsRoute.GET("/orders", controller.GetUserOrders)
			userSubscriptionsRoute.GET("/orders/:id", controller.GetUserOrderDetail)
			userSubscriptionsRoute.POST("/orders/:id/pay", controller.PayOrder)
			userSubscriptionsRoute.POST("/orders/:id/cancel", controller.CancelUserOrder)
			// 第三方支付入口（复用现有支付能力）
			userSubscriptionsRoute.POST("/orders/:id/pay/epay", controller.SubscriptionOrderEpay)
			userSubscriptionsRoute.POST("/orders/:id/pay/stripe", controller.SubscriptionOrderStripe)
		}

		// 旧路由（保持向后兼容）
		userSubscriptionRoute := apiRouter.Group("/subscription/self")
		userSubscriptionRoute.Use(middleware.UserAuth())
		{
			userSubscriptionRoute.GET("/", controller.GetUserSubscriptions)
			userSubscriptionRoute.GET("/active", controller.GetUserActiveSubscriptions)
			userSubscriptionRoute.GET("/:id", controller.GetUserSubscriptionDetail)
			userSubscriptionRoute.GET("/:id/usage", controller.GetUserSubscriptionUsage)
			userSubscriptionRoute.GET("/:id/history", controller.GetUserSubscriptionHistory)
			userSubscriptionRoute.PUT("/:id/settings", controller.UpdateUserSubscriptionSettings)
			userSubscriptionRoute.POST("/:id/cancel", controller.CancelUserSubscription)
			userSubscriptionRoute.POST("/reorder", controller.ReorderUserSubscriptionPriorities)
		}

		// 用户设置路由
		userSettingsRoute := apiRouter.Group("/user/settings")
		userSettingsRoute.Use(middleware.UserAuth())
		{
			userSettingsRoute.GET("/auto-wallet-fallback", controller.GetUserAutoWalletFallback)
			userSettingsRoute.PUT("/auto-wallet-fallback", controller.UpdateUserAutoWalletFallback)
		}

		// 用户优惠券路由
		userCouponRoute := apiRouter.Group("/user/coupons")
		userCouponRoute.Use(middleware.UserAuth())
		{
			userCouponRoute.POST("/claim", controller.ClaimCoupon)
			userCouponRoute.GET("/", controller.GetUserCoupons)
			userCouponRoute.GET("/available", controller.GetAvailableCoupons)
			userCouponRoute.GET("/:id", controller.GetUserCouponDetail)
		}

		// 用户支付优惠券预览路由
		userPaymentRoute := apiRouter.Group("/user/payment")
		userPaymentRoute.Use(middleware.UserAuth())
		{
			userPaymentRoute.POST("/coupon/preview", controller.PreviewCouponUsage)
		}

		// 用户兑换订阅路由
		userRedemptionRoute := apiRouter.Group("/user/redemptions")
		userRedemptionRoute.Use(middleware.UserAuth())
		{
			userRedemptionRoute.POST("/use", controller.UseSubscriptionRedemption)
			userRedemptionRoute.POST("/preview", controller.PreviewSubscriptionRedemption)
		}

		// 用户订单管理路由
		userOrderRoute := apiRouter.Group("/subscription-orders")
		userOrderRoute.Use(middleware.UserAuth())
		{
			userOrderRoute.GET("/self", controller.GetUserOrders)
			userOrderRoute.GET("/self/:id", controller.GetUserOrderDetail)
			userOrderRoute.POST("/preview", controller.PreviewOrder)
			userOrderRoute.POST("/", controller.CreateOrder)
			userOrderRoute.POST("/purchase", controller.PurchaseSubscription) // 一键购买（创建+支付）
			userOrderRoute.POST("/:id/pay", controller.PayOrder)
			userOrderRoute.POST("/:id/cancel", controller.CancelUserOrder)
		}
	}
}
