package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"

	// Import oauth package to register providers via init()
	_ "github.com/QuantumNous/new-api/oauth"

	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
)

func SetApiRouter(router *gin.Engine) {
	apiRouter := router.Group("/api")
	apiRouter.Use(middleware.RouteTag("api"))
	apiRouter.Use(gzip.Gzip(gzip.DefaultCompression))
	apiRouter.Use(middleware.BodyStorageCleanup()) // 清理请求体存储
	apiRouter.Use(middleware.GlobalAPIRateLimit())
	anonymousRequestBodyLimit := middleware.AnonymousRequestBodyLimit()
	{
		apiRouter.GET("/setup", controller.GetSetup)
		apiRouter.POST("/setup", anonymousRequestBodyLimit, controller.PostSetup)
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
		apiRouter.GET("/pricing", middleware.HeaderNavModuleAuth("pricing"), controller.GetPricing)
		perfMetricsRoute := apiRouter.Group("/perf-metrics")
		perfMetricsRoute.Use(middleware.HeaderNavModulePublicOrUserAuth("pricing"))
		{
			perfMetricsRoute.GET("/summary", controller.GetPerfMetricsSummary)
			perfMetricsRoute.GET("", controller.GetPerfMetrics)
		}
		apiRouter.GET("/rankings", middleware.HeaderNavModuleAuth("rankings"), controller.GetRankings)
		apiRouter.GET("/verification", middleware.EmailVerificationRateLimit(), middleware.TurnstileCheck(), controller.SendEmailVerification)
		apiRouter.POST("/verification", middleware.EmailVerificationRateLimit(), anonymousRequestBodyLimit, middleware.TurnstileCheckFromBody(), controller.SendEmailVerification)
		apiRouter.GET("/reset_password", middleware.CriticalRateLimit(), middleware.TurnstileCheck(), controller.SendPasswordResetEmail)
		apiRouter.POST("/reset_password", middleware.CriticalRateLimit(), anonymousRequestBodyLimit, middleware.TurnstileCheckFromBody(), controller.SendPasswordResetEmail)
		apiRouter.POST("/user/reset", middleware.CriticalRateLimit(), anonymousRequestBodyLimit, controller.ResetPassword)
		// OAuth routes - specific routes must come before :provider wildcard
		apiRouter.POST("/oauth/state", middleware.CriticalRateLimit(), middleware.DisableCache(), middleware.TryUserAuth(), anonymousRequestBodyLimit, middleware.OAuthStateTurnstileCheck(), controller.GenerateOAuthCode)
		apiRouter.POST("/oauth/email/bind", middleware.UserAuth(), middleware.CriticalRateLimit(), controller.EmailBind)
		// Non-standard OAuth (WeChat, Telegram) - keep original routes
		apiRouter.GET("/oauth/wechat", middleware.CriticalRateLimit(), middleware.DisableCache(), middleware.TurnstileCheck(), controller.WeChatAuth)
		apiRouter.POST("/oauth/wechat/bind", middleware.UserAuth(), middleware.CriticalRateLimit(), controller.WeChatBind)
		apiRouter.GET("/oauth/telegram/login", middleware.CriticalRateLimit(), middleware.DisableCache(), middleware.TurnstileCheck(), controller.TelegramLogin)
		apiRouter.POST("/oauth/telegram/bind/start", middleware.UserAuth(), middleware.CriticalRateLimit(), middleware.DisableCache(), controller.TelegramBindStart)
		apiRouter.GET("/oauth/telegram/bind/:flow_token", middleware.CriticalRateLimit(), middleware.DisableCache(), controller.TelegramBind)
		// Standard OAuth providers (GitHub, Discord, OIDC, LinuxDO) - unified route
		apiRouter.GET("/oauth/:provider", middleware.CriticalRateLimit(), middleware.DisableCache(), middleware.TryUserAuth(), controller.HandleOAuth)
		apiRouter.GET("/ratio_config", middleware.CriticalRateLimit(), controller.GetRatioConfig)

		apiRouter.POST("/stripe/webhook", anonymousRequestBodyLimit, controller.StripeWebhook)
		apiRouter.POST("/creem/webhook", anonymousRequestBodyLimit, controller.CreemWebhook)
		apiRouter.POST("/waffo/webhook", anonymousRequestBodyLimit, controller.WaffoWebhook)
		// :env separates test vs prod URLs so the operator can register each
		// in Pancake's matching webhook slot; handler enforces env match.
		apiRouter.POST("/waffo-pancake/webhook/:env", anonymousRequestBodyLimit, controller.WaffoPancakeWebhook)

		// Universal secure verification routes
		apiRouter.POST("/verify", middleware.UserAuth(), middleware.CriticalRateLimit(), middleware.DisableCache(), controller.UniversalVerify)

		userRoute := apiRouter.Group("/user")
		{
			userRoute.POST("/auth/refresh", middleware.SessionCookieOriginGuard(), middleware.CriticalRateLimit(), middleware.DisableCache(), controller.RefreshAuth)
			userRoute.POST("/auth/logout", middleware.SessionCookieOriginGuard(), middleware.CriticalRateLimit(), middleware.DisableCache(), controller.AuthLogout)
			userRoute.POST("/register", middleware.CriticalRateLimit(), anonymousRequestBodyLimit, middleware.TurnstileCheck(), controller.Register)
			userRoute.POST("/login", middleware.CriticalRateLimit(), middleware.DisableCache(), anonymousRequestBodyLimit, middleware.TurnstileCheck(), controller.Login)
			userRoute.POST("/login/2fa", middleware.CriticalRateLimit(), middleware.DisableCache(), anonymousRequestBodyLimit, controller.Verify2FALogin)
			userRoute.POST("/passkey/login/begin", middleware.CriticalRateLimit(), middleware.DisableCache(), anonymousRequestBodyLimit, controller.PasskeyLoginBegin)
			userRoute.POST("/passkey/login/finish", middleware.CriticalRateLimit(), middleware.DisableCache(), anonymousRequestBodyLimit, controller.PasskeyLoginFinish)
			//userRoute.POST("/tokenlog", middleware.CriticalRateLimit(), controller.TokenLog)
			userRoute.POST("/epay/notify", anonymousRequestBodyLimit, controller.EpayNotify)
			userRoute.GET("/epay/notify", controller.EpayNotify)
			userRoute.POST("/antom/notify", anonymousRequestBodyLimit, controller.AntomNotify)
			userRoute.GET("/groups", controller.GetUserGroups)

			selfRoute := userRoute.Group("/")
			selfRoute.Use(middleware.UserAuth())
			{
				selfRoute.GET("/sessions", middleware.DisableCache(), controller.GetLoginSessions)
				selfRoute.DELETE("/sessions/:sid", middleware.DisableCache(), controller.DeleteLoginSession)
				selfRoute.POST("/sessions/revoke-others", middleware.DisableCache(), controller.RevokeOtherLoginSessions)
				selfRoute.GET("/self/groups", controller.GetUserGroups)
				selfRoute.GET("/self", controller.GetSelf)
				selfRoute.GET("/models", controller.GetUserModels)
				selfRoute.PUT("/self", middleware.CriticalRateLimit(), middleware.DisableCache(), controller.UpdateSelf)
				selfRoute.DELETE("/self", controller.DeleteSelf)
				selfRoute.GET("/token", middleware.CriticalRateLimit(), middleware.UserCriticalRateLimit("access-token"), middleware.DisableCache(), controller.GenerateAccessToken)
				selfRoute.GET("/passkey", controller.PasskeyStatus)
				selfRoute.POST("/passkey/register/begin", middleware.DisableCache(), controller.PasskeyRegisterBegin)
				selfRoute.POST("/passkey/register/finish", middleware.DisableCache(), controller.PasskeyRegisterFinish)
				selfRoute.POST("/passkey/verify/begin", middleware.DisableCache(), controller.PasskeyVerifyBegin)
				selfRoute.POST("/passkey/verify/finish", middleware.DisableCache(), controller.PasskeyVerifyFinish)
				selfRoute.DELETE("/passkey", middleware.DisableCache(), controller.PasskeyDelete)
				selfRoute.GET("/aff", controller.GetAffCode)
				selfRoute.GET("/aff/summary", controller.GetAffiliateSummary)
				selfRoute.GET("/aff/commissions", controller.GetAffiliateCommissions)
				selfRoute.GET("/aff/invitees", controller.GetUserInvitees)
				selfRoute.GET("/aff/invitee_stats", controller.GetAffiliateInviteeStats)
				selfRoute.GET("/aff/transfers", controller.GetAffiliateTransfers)
				selfRoute.POST("/aff/transfers", middleware.UserCriticalRateLimit("affiliate-transfer"), controller.TransferAffiliateCommission)
				selfRoute.GET("/aff/payouts/summary", controller.GetAffiliatePayoutSummary)
				selfRoute.GET("/aff/payouts", controller.GetAffiliatePayouts)
				selfRoute.POST("/aff/payouts", middleware.UserCriticalRateLimit("affiliate-payout"), controller.CreateAffiliatePayout)
				selfRoute.POST("/aff/payouts/:id/cancel", middleware.UserCriticalRateLimit("affiliate-payout-cancel"), controller.CancelAffiliatePayout)
				selfRoute.GET("/topup/info", controller.GetTopUpInfo)
				selfRoute.GET("/topup/self", controller.GetUserTopUps)
				selfRoute.POST("/topup", middleware.CriticalRateLimit(), controller.TopUp)
				selfRoute.POST("/pay", middleware.CriticalRateLimit(), controller.RequestEpay)
				selfRoute.POST("/amount", controller.RequestAmount)
				selfRoute.POST("/antom/pay", middleware.CriticalRateLimit(), controller.RequestAntomPay)
				selfRoute.POST("/antom/query", middleware.CriticalRateLimit(), controller.RequestAntomQuery)
				selfRoute.POST("/stripe/pay", middleware.CriticalRateLimit(), controller.RequestStripePay)
				selfRoute.POST("/stripe/amount", controller.RequestStripeAmount)
				selfRoute.POST("/creem/pay", middleware.CriticalRateLimit(), controller.RequestCreemPay)
				selfRoute.POST("/waffo/amount", controller.RequestWaffoAmount)
				selfRoute.POST("/waffo/pay", middleware.CriticalRateLimit(), controller.RequestWaffoPay)
				selfRoute.POST("/waffo-pancake/amount", controller.RequestWaffoPancakeAmount)
				selfRoute.POST("/waffo-pancake/pay", middleware.CriticalRateLimit(), controller.RequestWaffoPancakePay)
				selfRoute.POST("/aff_transfer", middleware.UserCriticalRateLimit("aff-transfer"), controller.TransferAffQuota)
				selfRoute.PUT("/setting", controller.UpdateUserSetting)

				// 2FA routes
				selfRoute.GET("/2fa/status", controller.Get2FAStatus)
				selfRoute.POST("/2fa/setup", middleware.DisableCache(), controller.Setup2FA)
				selfRoute.POST("/2fa/enable", middleware.DisableCache(), controller.Enable2FA)
				selfRoute.POST("/2fa/disable", middleware.DisableCache(), controller.Disable2FA)
				selfRoute.POST("/2fa/backup_codes", middleware.DisableCache(), controller.RegenerateBackupCodes)

				// Check-in routes
				selfRoute.GET("/checkin", controller.GetCheckinStatus)
				selfRoute.POST("/checkin", middleware.TurnstileCheck(), controller.DoCheckin)

				// Custom OAuth bindings
				selfRoute.GET("/oauth/bindings", controller.GetUserOAuthBindings)
				selfRoute.DELETE("/oauth/bindings/:provider_id", controller.UnbindCustomOAuth)
			}

			adminRoute := userRoute.Group("/")
			adminRoute.Use(middleware.AdminAuth())
			{
				adminRoute.GET("/", controller.GetAllUsers)
				adminRoute.GET("/topup", controller.GetAllTopUps)
				adminRoute.POST("/topup/complete", controller.AdminCompleteTopUp)
				adminRoute.GET("/search", controller.SearchUsers)
				adminRoute.GET("/inviter-options", controller.GetUserInviterOptions)
				adminRoute.GET("/:id/oauth/bindings", controller.GetUserOAuthBindingsByAdmin)
				adminRoute.DELETE("/:id/oauth/bindings/:provider_id", controller.UnbindCustomOAuthByAdmin)
				adminRoute.DELETE("/:id/bindings/:binding_type", controller.AdminClearUserBinding)
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

		affiliateAdminRoute := apiRouter.Group("/affiliate/admin")
		affiliateAdminRoute.Use(middleware.DisableCache(), middleware.AdminAuth())
		{
			affiliateAdminRoute.GET("/summary", controller.GetAdminAffiliateSummary)
			affiliateAdminRoute.GET("/invitees", controller.GetAdminAffiliateInviteRecords)
			affiliateAdminRoute.GET("/commissions", controller.GetAdminAffiliateCommissions)
			affiliateAdminRoute.GET("/transfers", controller.GetAdminAffiliateTransfers)
			affiliateAdminRoute.POST("/commissions/:id/approve", middleware.CriticalRateLimit(), controller.ApproveAffiliateCommission)
			affiliateAdminRoute.POST("/commissions/:id/reject", middleware.CriticalRateLimit(), controller.RejectAffiliateCommission)
			affiliateAdminRoute.GET("/payouts/summary", controller.GetAdminAffiliatePayoutSummary)
			affiliateAdminRoute.GET("/payouts", controller.GetAdminAffiliatePayouts)
			affiliateAdminRoute.GET("/payout-provider", controller.GetAdminAffiliateAlipayPayoutStatus)
			affiliateAdminRoute.POST("/payouts/:id/approve", middleware.CriticalRateLimit(), controller.ApproveAffiliatePayout)
			affiliateAdminRoute.POST("/payouts/:id/reject", middleware.CriticalRateLimit(), controller.RejectAffiliatePayout)
			affiliateAdminRoute.POST("/payouts/:id/paid", middleware.CriticalRateLimit(), controller.MarkAffiliatePayoutPaid)
			affiliateAdminRoute.POST("/payouts/:id/alipay", middleware.CriticalRateLimit(), controller.PayAffiliatePayoutWithAlipay)
			affiliateAdminRoute.POST("/payouts/:id/alipay/status", middleware.CriticalRateLimit(), controller.RefreshAffiliatePayoutAlipayStatus)
			affiliateAdminRoute.GET("/upgrade-candidates", controller.GetAdminAffiliateUpgradeCandidates)
			affiliateAdminRoute.POST("/upgrade-candidates/:id/approve", middleware.CriticalRateLimit(), controller.ApproveAdminAffiliateUpgrade)
			affiliateAdminRoute.GET("/notification-failures", controller.GetFailedAffiliateUpgradeNotices)
			affiliateAdminRoute.POST("/notification-failures/:id/retry", middleware.CriticalRateLimit(), controller.RetryAffiliateUpgradeNotice)
		}
		affiliateRootRoute := apiRouter.Group("/affiliate/root")
		affiliateRootRoute.Use(middleware.DisableCache(), middleware.RootAuth())
		{
			affiliateRootRoute.PUT("/settings", controller.UpdateAffiliateSettings)
			affiliateRootRoute.GET("/payout-settings", controller.GetAffiliateAlipayPayoutSettings)
			affiliateRootRoute.PUT("/payout-settings", middleware.CriticalRateLimit(), controller.UpdateAffiliateAlipayPayoutSettings)
			affiliateRootRoute.POST("/payout-settings/test", middleware.CriticalRateLimit(), controller.TestAffiliateAlipayPayoutSettings)
		}

		invoiceRoute := apiRouter.Group("/invoice")
		invoiceRoute.Use(middleware.DisableCache(), middleware.UserAuth())
		{
			invoiceRoute.GET("/config", controller.GetInvoiceConfig)
			invoiceRoute.GET("/eligible_orders", controller.ListEligibleInvoiceOrders)
			invoiceRoute.POST("/requests", middleware.UserCriticalRateLimit("invoice-create"), controller.CreateInvoiceRequest)
			invoiceRoute.GET("/requests", controller.ListUserInvoiceRequests)
			invoiceRoute.GET("/requests/:id", controller.GetUserInvoiceRequest)
			invoiceRoute.POST("/requests/:id/withdraw", middleware.UserCriticalRateLimit("invoice-withdraw"), controller.WithdrawInvoiceRequest)
			invoiceRoute.GET("/requests/:id/files/:file_id", controller.DownloadInvoiceFile)
		}

		invoiceAdminRoute := apiRouter.Group("/invoice/admin")
		invoiceAdminRoute.Use(middleware.DisableCache(), middleware.AdminAuth())
		{
			invoiceAdminRoute.GET("/requests", controller.ListAdminInvoiceRequests)
			invoiceAdminRoute.GET("/requests/:id", controller.GetAdminInvoiceRequest)
			invoiceAdminRoute.GET("/requests/:id/user-profile", controller.GetInvoiceUserProfile)
			invoiceAdminRoute.POST("/requests/:id/files", middleware.CriticalRateLimit(), controller.UploadInvoiceFile)
			invoiceAdminRoute.DELETE("/requests/:id/files/:file_id", controller.DeleteInvoiceFile)
			invoiceAdminRoute.POST("/requests/:id/notifications/issued/resend", middleware.CriticalRateLimit(), controller.ResendIssuedInvoiceNotification)
			invoiceAdminRoute.PUT("/requests/:id/status", middleware.CriticalRateLimit(), controller.UpdateInvoiceRequestStatus)
			invoiceAdminRoute.DELETE("/requests/:id", middleware.CriticalRateLimit(), controller.PurgeInvoiceRequest)
			invoiceAdminRoute.GET("/maintenance", controller.GetInvoiceMaintenance)
			invoiceAdminRoute.POST("/maintenance/cleanups/:cleanup_id/retry", middleware.CriticalRateLimit(), controller.RetryInvoiceFileCleanup)
			invoiceAdminRoute.POST("/maintenance/notifications/:delivery_id/retry", middleware.CriticalRateLimit(), controller.RetryInvoiceNotification)
			invoiceAdminRoute.POST("/maintenance/reconcile", middleware.CriticalRateLimit(), controller.ReconcileInvoiceStorage)
			invoiceAdminRoute.POST("/maintenance/orphans/cleanup", middleware.CriticalRateLimit(), controller.CleanupInvoiceOrphans)
		}

		// Subscription billing (plans, purchase, admin management)
		subscriptionRoute := apiRouter.Group("/subscription")
		subscriptionRoute.Use(middleware.UserAuth())
		{
			subscriptionRoute.GET("/plans", controller.GetSubscriptionPlans)
			subscriptionRoute.GET("/self", controller.GetSubscriptionSelf)
			subscriptionRoute.PUT("/self/preference", controller.UpdateSubscriptionPreference)
			subscriptionRoute.POST("/balance/pay", middleware.CriticalRateLimit(), controller.SubscriptionRequestBalancePay)
			subscriptionRoute.POST("/epay/pay", middleware.CriticalRateLimit(), controller.SubscriptionRequestEpay)
			subscriptionRoute.POST("/stripe/pay", middleware.CriticalRateLimit(), controller.SubscriptionRequestStripePay)
			subscriptionRoute.POST("/creem/pay", middleware.CriticalRateLimit(), controller.SubscriptionRequestCreemPay)
			subscriptionRoute.POST("/waffo-pancake/pay", middleware.CriticalRateLimit(), controller.SubscriptionRequestWaffoPancakePay)
		}
		subscriptionAdminRoute := apiRouter.Group("/subscription/admin")
		subscriptionAdminRoute.Use(middleware.AdminAuth())
		{
			subscriptionAdminRoute.GET("/plans", controller.AdminListSubscriptionPlans)
			subscriptionAdminRoute.POST("/plans", controller.AdminCreateSubscriptionPlan)
			subscriptionAdminRoute.PUT("/plans/:id", controller.AdminUpdateSubscriptionPlan)
			subscriptionAdminRoute.PATCH("/plans/:id", controller.AdminUpdateSubscriptionPlanStatus)
			subscriptionAdminRoute.POST("/bind", controller.AdminBindSubscription)
			subscriptionAdminRoute.POST("/plans/:id/subscriptions/reset", controller.AdminResetPlanSubscriptions)

			// User subscription management (admin)
			subscriptionAdminRoute.GET("/users/:id/subscriptions", controller.AdminListUserSubscriptions)
			subscriptionAdminRoute.POST("/users/:id/subscriptions", controller.AdminCreateUserSubscription)
			subscriptionAdminRoute.POST("/users/:id/subscriptions/reset", controller.AdminResetUserSubscriptionsByPlan)
			subscriptionAdminRoute.POST("/user_subscriptions/:id/invalidate", controller.AdminInvalidateUserSubscription)
			subscriptionAdminRoute.DELETE("/user_subscriptions/:id", controller.AdminDeleteUserSubscription)
		}

		// Subscription payment callbacks (no auth)
		apiRouter.POST("/subscription/epay/notify", anonymousRequestBodyLimit, controller.SubscriptionEpayNotify)
		apiRouter.GET("/subscription/epay/notify", controller.SubscriptionEpayNotify)
		apiRouter.GET("/subscription/epay/return", controller.SubscriptionEpayReturn)
		apiRouter.POST("/subscription/epay/return", anonymousRequestBodyLimit, controller.SubscriptionEpayReturn)
		apiRouter.GET("/marketing/c/:token", controller.MarketingClick)
		optionRoute := apiRouter.Group("/option")
		optionRoute.Use(middleware.DisableCache(), middleware.RootAuth())
		{
			optionRoute.GET("/", controller.GetOptions)
			// Compatibility for administration pages that were opened before the
			// option update endpoint changed from POST /api/option to PUT
			// /api/option/. Keep the same RootAuth middleware and handler so stale
			// tabs can save safely until they are refreshed.
			optionRoute.POST("", controller.UpdateOption)
			optionRoute.PUT("/", controller.UpdateOption)
			optionRoute.POST("/smtp-test", middleware.CriticalRateLimit(), controller.TestSMTPEmail)
			optionRoute.POST("/payment_compliance", controller.ConfirmPaymentCompliance)
			optionRoute.GET("/channel_affinity_cache", controller.GetChannelAffinityCacheStats)
			optionRoute.DELETE("/channel_affinity_cache", controller.ClearChannelAffinityCache)
			optionRoute.POST("/rest_model_ratio", controller.ResetModelRatio)
			optionRoute.GET("/waffo-pancake/catalog", controller.ListWaffoPancakeCatalog)
			optionRoute.POST("/waffo-pancake/pair", controller.CreateWaffoPancakePair)
			optionRoute.POST("/waffo-pancake/save", controller.SaveWaffoPancake)
			optionRoute.POST("/waffo-pancake/subscription-product", controller.CreateWaffoPancakeSubscriptionProduct)
			optionRoute.GET("/waffo-pancake/subscription-product-options", controller.ListWaffoPancakeSubscriptionProductOptions)
			optionRoute.GET("/email_templates", controller.ListEmailTemplates)
			optionRoute.POST("/email_templates/save", controller.SaveEmailTemplate)
			optionRoute.POST("/email_templates/preview", controller.PreviewEmailTemplate)
			optionRoute.POST("/email_templates/reset", controller.ResetEmailTemplate)
			optionRoute.GET("/email_deliveries", controller.ListEmailDeliveries)
			optionRoute.GET("/email_deliveries/stats", controller.GetEmailDeliveryStats)
			optionRoute.POST("/email_deliveries/:id/retry", middleware.CriticalRateLimit(), controller.RetryFailedEmailDelivery)
			optionRoute.POST("/email_deliveries/retry", middleware.CriticalRateLimit(), controller.RetryFailedEmailDeliveries)
			optionRoute.PUT("/invoice", controller.UpdateInvoiceSettings)
			optionRoute.POST("/invoice/storage-test", controller.TestInvoiceStorage)
		}

		marketingRoute := apiRouter.Group("/marketing")
		marketingRoute.Use(middleware.DisableCache(), middleware.RootAuth())
		{
			marketingRoute.GET("/overview", controller.MarketingOverview)
			marketingRoute.GET("/campaigns", controller.ListMarketingCampaigns)
			marketingRoute.POST("/campaigns", controller.CreateMarketingCampaign)
			marketingRoute.PUT("/campaigns/:id", controller.UpdateMarketingCampaign)
			marketingRoute.POST("/campaigns/preview", controller.PreviewMarketingCampaignAudience)
			marketingRoute.POST("/campaigns/:id/schedule", controller.ScheduleMarketingCampaign)
			marketingRoute.POST("/campaigns/:id/pause", controller.PauseMarketingCampaign)
			marketingRoute.POST("/campaigns/:id/resume", controller.ResumeMarketingCampaign)
			marketingRoute.POST("/campaigns/:id/cancel", controller.CancelMarketingCampaign)
			marketingRoute.POST("/campaigns/:id/clone", controller.CloneMarketingCampaign)
			marketingRoute.GET("/campaigns/:id/recipients", controller.ListMarketingRecipients)
			marketingRoute.POST("/test", controller.TestMarketingEmail)
			marketingRoute.POST("/preview", controller.PreviewMarketingEmail)
			marketingRoute.GET("/automations", controller.ListMarketingAutomations)
			marketingRoute.GET("/announcements/latest", controller.LatestMarketingAnnouncement)
			marketingRoute.PUT("/automations/:scene", controller.UpdateMarketingAutomation)
			marketingRoute.GET("/automations/:scene/preview", controller.PreviewMarketingAutomation)
			marketingRoute.GET("/suppressions", controller.ListMarketingSuppressions)
			marketingRoute.POST("/suppressions", controller.CreateMarketingSuppression)
			marketingRoute.DELETE("/suppressions/:id", controller.DeleteMarketingSuppression)
		}

		// Custom OAuth provider management (root only)
		customOAuthRoute := apiRouter.Group("/custom-oauth-provider")
		customOAuthRoute.Use(middleware.RootAuth())
		{
			customOAuthRoute.POST("/discovery", controller.FetchCustomOAuthDiscovery)
			customOAuthRoute.GET("/", controller.GetCustomOAuthProviders)
			customOAuthRoute.GET("/:id", controller.GetCustomOAuthProvider)
			customOAuthRoute.POST("/", controller.CreateCustomOAuthProvider)
			customOAuthRoute.PUT("/:id", controller.UpdateCustomOAuthProvider)
			customOAuthRoute.DELETE("/:id", controller.DeleteCustomOAuthProvider)
		}
		performanceRoute := apiRouter.Group("/performance")
		performanceRoute.Use(middleware.RootAuth())
		{
			performanceRoute.GET("/stats", controller.GetPerformanceStats)
			performanceRoute.DELETE("/disk_cache", controller.ClearDiskCache)
			performanceRoute.POST("/reset_stats", controller.ResetPerformanceStats)
			performanceRoute.POST("/gc", controller.ForceGC)
			performanceRoute.GET("/logs", controller.GetLogFiles)
			performanceRoute.DELETE("/logs", controller.CleanupLogFiles)
		}
		ratioSyncRoute := apiRouter.Group("/ratio_sync")
		ratioSyncRoute.Use(middleware.RootAuth())
		{
			ratioSyncRoute.GET("/channels", controller.GetSyncableChannels)
			ratioSyncRoute.POST("/fetch", controller.FetchUpstreamRatios)
		}
		registerChannelRoutes(apiRouter)
		registerAuthzRoutes(apiRouter)
		tokenRoute := apiRouter.Group("/token")
		tokenRoute.Use(middleware.UserAuth())
		{
			tokenRoute.GET("/", controller.GetAllTokens)
			tokenRoute.GET("/search", middleware.SearchRateLimit(), controller.SearchTokens)
			tokenRoute.GET("/auto-groups", controller.GetTokenAutoGroups)
			tokenRoute.GET("/:id", controller.GetToken)
			tokenRoute.POST("/:id/key", middleware.CriticalRateLimit(), middleware.DisableCache(), controller.GetTokenKey)
			tokenRoute.POST("/", controller.AddToken)
			tokenRoute.PUT("/", controller.UpdateToken)
			tokenRoute.DELETE("/:id", controller.DeleteToken)
			tokenRoute.POST("/batch", controller.DeleteTokenBatch)
			tokenRoute.POST("/batch/keys", middleware.CriticalRateLimit(), middleware.DisableCache(), controller.GetTokenKeysBatch)
		}

		usageRoute := apiRouter.Group("/usage")
		usageRoute.Use(middleware.CORS(), middleware.CriticalRateLimit())
		{
			tokenUsageRoute := usageRoute.Group("/token")
			tokenUsageRoute.Use(middleware.TokenAuthReadOnly())
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
		logRoute.GET("/stat", middleware.AdminAuth(), controller.GetLogsStat)
		logRoute.GET("/self/stat", middleware.UserAuth(), controller.GetLogsSelfStat)
		logRoute.GET("/channel_affinity_usage_cache", middleware.AdminAuth(), controller.GetChannelAffinityUsageCacheStats)
		logRoute.GET("/search", middleware.AdminAuth(), controller.SearchAllLogs)
		logRoute.GET("/self", middleware.UserAuth(), controller.GetUserLogs)
		logRoute.GET("/self/search", middleware.UserAuth(), middleware.SearchRateLimit(), controller.SearchUserLogs)

		systemTaskRoute := apiRouter.Group("/system-task")
		systemTaskRoute.Use(middleware.RootAuth())
		{
			systemTaskRoute.POST("/log-cleanup", controller.CreateLogCleanupSystemTask)
			systemTaskRoute.GET("/list", controller.ListSystemTasks)
			systemTaskRoute.GET("/current", controller.GetCurrentSystemTask)
			systemTaskRoute.GET("/:task_id", controller.GetSystemTask)
		}
		systemInfoRoute := apiRouter.Group("/system-info")
		systemInfoRoute.Use(middleware.RootAuth())
		{
			systemInfoRoute.GET("/instances", controller.ListSystemInstances)
			systemInfoRoute.DELETE("/stale-instances", controller.DeleteStaleSystemInstances)
			systemInfoRoute.DELETE("/instances/:node_name", controller.DeleteStaleSystemInstance)
		}

		dataRoute := apiRouter.Group("/data")
		dataRoute.GET("/", middleware.AdminAuth(), controller.GetAllQuotaDates)
		dataRoute.GET("/users", middleware.AdminAuth(), controller.GetQuotaDatesByUser)
		dataRoute.GET("/self", middleware.UserAuth(), controller.GetUserQuotaDates)
		dataRoute.GET("/flow", middleware.AdminAuth(), controller.GetAllFlowQuotaDates)
		dataRoute.GET("/flow/self", middleware.UserAuth(), controller.GetUserFlowQuotaDates)

		logRoute.Use(middleware.CORS(), middleware.CriticalRateLimit())
		{
			logRoute.GET("/token", middleware.TokenAuthReadOnly(), controller.GetLogByKey)
		}
		groupRoute := apiRouter.Group("/group")
		groupRoute.Use(middleware.AdminAuth())
		{
			groupRoute.GET("/", controller.GetGroups)
		}

		prefillGroupRoute := apiRouter.Group("/prefill_group")
		prefillGroupRoute.Use(middleware.AdminAuth())
		{
			prefillGroupRoute.GET("/", controller.GetPrefillGroups)
			prefillGroupRoute.POST("/", controller.CreatePrefillGroup)
			prefillGroupRoute.PUT("/", controller.UpdatePrefillGroup)
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

		// Deployments (model deployment management)
		deploymentsRoute := apiRouter.Group("/deployments")
		deploymentsRoute.Use(middleware.AdminAuth())
		{
			deploymentsRoute.GET("/settings", controller.GetModelDeploymentSettings)
			deploymentsRoute.POST("/settings/test-connection", controller.TestIoNetConnection)
			deploymentsRoute.GET("/", controller.GetAllDeployments)
			deploymentsRoute.GET("/search", controller.SearchDeployments)
			deploymentsRoute.POST("/test-connection", controller.TestIoNetConnection)
			deploymentsRoute.GET("/hardware-types", controller.GetHardwareTypes)
			deploymentsRoute.GET("/locations", controller.GetLocations)
			deploymentsRoute.GET("/available-replicas", controller.GetAvailableReplicas)
			deploymentsRoute.POST("/price-estimation", controller.GetPriceEstimation)
			deploymentsRoute.GET("/check-name", controller.CheckClusterNameAvailability)
			deploymentsRoute.POST("/", controller.CreateDeployment)

			deploymentsRoute.GET("/:id", controller.GetDeployment)
			deploymentsRoute.GET("/:id/logs", controller.GetDeploymentLogs)
			deploymentsRoute.GET("/:id/containers", controller.ListDeploymentContainers)
			deploymentsRoute.GET("/:id/containers/:container_id", controller.GetContainerDetails)
			deploymentsRoute.PUT("/:id", controller.UpdateDeployment)
			deploymentsRoute.PUT("/:id/name", controller.UpdateDeploymentName)
			deploymentsRoute.POST("/:id/extend", controller.ExtendDeployment)
			deploymentsRoute.DELETE("/:id", controller.DeleteDeployment)
		}
	}
}
