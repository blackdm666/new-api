package i18n

// Message keys of i18n/locales/*.yaml, for text the backend translates itself:
// errors returned to AI clients, emails and notifications. Web console messages
// are not listed here; the web console translates them (see common.Message).

// Errors returned to AI clients
const (
	MsgDatabaseError          = "common.database_error"
	MsgAuthUserBanned         = "auth.user_banned"
	MsgTokenGetInfoFailed     = "token.get_info_failed"
	MsgTokenInvalid           = "token.invalid"
	MsgTokenNotProvided       = "token.not_provided"
	MsgTokenStatusUnavailable = "token.status_unavailable"

	MsgAuthClientIPUnresolved     = "auth.client_ip_unresolved"
	MsgAuthTokenIPNotAllowed      = "auth.token_ip_not_allowed"
	MsgAuthTokenGroupForbidden    = "auth.token_group_forbidden"
	MsgAuthTokenGroupDeprecated   = "auth.token_group_deprecated"
	MsgAuthChannelSelectForbidden = "auth.channel_select_forbidden"

	MsgRateLimitReached                   = "rate_limit.reached"
	MsgRateLimitTotalReached              = "rate_limit.total_reached"
	MsgOmniReferenceVideoDurationExceeded = "video.omni.reference_duration_exceeded"

	MsgTTSResponseFormatUnsupported = "tts.response_format_unsupported"
	MsgTTSInputRequired             = "tts.input_required"
	MsgTTSInputTooLong              = "tts.input_too_long"
	MsgTTSVoiceRequired             = "tts.voice_required"
	MsgTTSVoiceUnsupported          = "tts.voice_unsupported"
	MsgTTSSpeedOutOfRange           = "tts.speed_out_of_range"
	MsgTTSStreamUnsupported         = "tts.stream_unsupported"
	MsgTTSChatEndpointUnsupported   = "tts.chat_endpoint_unsupported"
	MsgSTTChatEndpointUnsupported   = "stt.chat_endpoint_unsupported"
	MsgRealtimeEndpointUnsupported  = "realtime.endpoint_unsupported"
	MsgTTSNoAudio                   = "tts.no_audio"
	MsgSTTTranslationUnsupported    = "stt.translation_unsupported"
	MsgSTTResponseFormatUnsupported = "stt.response_format_unsupported"
	MsgSTTStreamUnsupported         = "stt.stream_unsupported"
	MsgSTTFileRequired              = "stt.file_required"
	MsgSTTFileTooLarge              = "stt.file_too_large"
	MsgSTTFileFormatUnsupported     = "stt.file_format_unsupported"

	MsgDistributorInvalidRequest               = "distributor.invalid_request"
	MsgDistributorInvalidChannelId             = "distributor.invalid_channel_id"
	MsgDistributorChannelDisabled              = "distributor.channel_disabled"
	MsgDistributorTokenNoModelAccess           = "distributor.token_no_model_access"
	MsgDistributorTokenModelForbidden          = "distributor.token_model_forbidden"
	MsgDistributorModelNameRequired            = "distributor.model_name_required"
	MsgDistributorInvalidPlayground            = "distributor.invalid_playground_request"
	MsgDistributorGroupAccessDenied            = "distributor.group_access_denied"
	MsgDistributorGetChannelFailed             = "distributor.get_channel_failed"
	MsgDistributorNoAvailableChannel           = "distributor.no_available_channel"
	MsgDistributorNoAvailableChannelTaskPlugin = "distributor.no_available_channel_task_plugin"
	MsgDistributorInvalidParseModel            = "distributor.invalid_request_parse_model"
	MsgDistributorModelNotInGroup              = "distributor.model_not_in_group"
	MsgDistributorModelNotInGroupSuggest       = "distributor.model_not_in_group_suggest"
	MsgDistributorVideoModelWrongEndpoint      = "distributor.video_model_wrong_endpoint"
	MsgDistributorImageModelWrongEndpoint      = "distributor.image_model_wrong_endpoint"
	MsgDistributorImagesEndpointNotSupported   = "distributor.images_endpoint_not_supported"

	MsgQuotaUserInsufficient             = "quota.user_insufficient"
	MsgQuotaPreConsumeInsufficient       = "quota.pre_consume_insufficient"
	MsgQuotaSubscriptionInsufficient     = "quota.subscription_insufficient"
	MsgQuotaTokenInsufficient            = "quota.token_insufficient"
	MsgRelayModelPriceNotConfigured      = "relay.model_price_not_configured"
	MsgRelayModelPriceNotConfiguredAdmin = "relay.model_price_not_configured_admin"
	MsgRelayTaskChannelDisabled          = "relay.task_channel_disabled"
	MsgRelayGroupSaturated               = "relay.group_saturated"
	MsgRelayGroupUpstreamSaturated       = "relay.group_upstream_saturated"
	MsgRelayRetryGetChannelFailed        = "relay.retry_get_channel_failed"
	MsgRelayRetryNoAvailableChannel      = "relay.retry_no_available_channel"
	MsgRelayAccessTokenUnsupported       = "relay.access_token_unsupported"
)

// Emails and the name of the token created at registration, in the
// recipient's language
const (
	MsgUserDefaultTokenName = "user.default_token_name"

	MsgEmailVerificationSubject  = "email.verification.subject"
	MsgEmailVerificationGreeting = "email.verification.greeting"
	MsgEmailVerificationCode     = "email.verification.code"
	MsgEmailVerificationValidity = "email.verification.validity"

	MsgEmailPasswordResetSubject      = "email.password_reset.subject"
	MsgEmailPasswordResetGreeting     = "email.password_reset.greeting"
	MsgEmailPasswordResetLink         = "email.password_reset.link"
	MsgEmailPasswordResetLinkFallback = "email.password_reset.link_fallback"
	MsgEmailPasswordResetValidity     = "email.password_reset.validity"

	MsgEmailBindingSubject     = "email.binding.subject"
	MsgEmailBindingNewIntro    = "email.binding.new_intro"
	MsgEmailBindingOldIntro    = "email.binding.old_intro"
	MsgEmailBindingCode        = "email.binding.code"
	MsgEmailBindingNewValidity = "email.binding.new_validity"
	MsgEmailBindingOldValidity = "email.binding.old_validity"

	MsgEmailSecurityNoticeSubject = "email.security_notice.subject"
	MsgEmailSecurityNoticeChanged = "email.security_notice.changed"
	MsgEmailSecurityNoticeAdvice  = "email.security_notice.advice"

	MsgSecurityEventPasswordUpdated      = "email.security_event.password_updated"
	MsgSecurityEventEmailChangeRequested = "email.security_event.email_change_requested"
	MsgSecurityEventEmailChanged         = "email.security_event.email_changed"
	MsgSecurityEventEmailConfirmed       = "email.security_event.email_confirmed"
	MsgSecurityEventLoginLinked          = "email.security_event.login_linked"
	MsgSecurityEventLoginUnlinked        = "email.security_event.login_unlinked"
	MsgSecurityEventWeChatLinked         = "email.security_event.wechat_linked"
)

// Low-quota notices, in the recipient's saved language
const (
	MsgNotifyQuotaLowTitle             = "notify.quota_low_title"
	MsgNotifySubscriptionQuotaLowTitle = "notify.subscription_quota_low_title"
	MsgNotifyQuotaLowBark              = "notify.quota_low_bark"
	MsgNotifyQuotaLowGotify            = "notify.quota_low_gotify"
	MsgNotifyQuotaLowEmail             = "notify.quota_low_email"
)

// Channel notices sent to administrators who receive them and to model update
// watchers
const (
	MsgChannelNotifyDisabledSubject = "channel.notify_disabled_subject"
	MsgChannelNotifyDisabledContent = "channel.notify_disabled_content"
	MsgChannelNotifyEnabledSubject  = "channel.notify_enabled_subject"
	MsgChannelNotifyEnabledContent  = "channel.notify_enabled_content"
	MsgChannelTestCompletedSubject  = "channel.test_completed_subject"
	MsgChannelTestCompletedContent  = "channel.test_completed_content"
	MsgChannelResponseTimeExceeded  = "channel.response_time_exceeded"
	// MsgChannelUpstreamBillingDisabled is the disable reason of a channel
	// whose Google Cloud project stopped billing.
	MsgChannelUpstreamBillingDisabled = "channel.upstream_billing_disabled"

	MsgChannelUpstreamUpdateNotifySubject   = "channel.upstream_update_notify_subject"
	MsgChannelUpstreamUpdateSummary         = "channel.upstream_update_summary"
	MsgChannelUpstreamUpdateChangedChannels = "channel.upstream_update_changed_channels"
	MsgChannelUpstreamUpdateMoreChannels    = "channel.upstream_update_more_channels"
	MsgChannelUpstreamUpdateAddedModels     = "channel.upstream_update_added_models"
	MsgChannelUpstreamUpdateRemovedModels   = "channel.upstream_update_removed_models"
	MsgChannelUpstreamUpdateFailedChannels  = "channel.upstream_update_failed_channels"
	MsgChannelUpstreamUpdateMoreOmitted     = "channel.upstream_update_more_omitted"
)

// 88API: keys of customized features that the backend still translates itself
// (web console messages of 88API handlers, and their emails).

// Task plugin messages
const (
	MsgTaskPluginUnknownMetaField = "task_plugin.unknown_meta_field"
)

// Common error messages
const (
	MsgInvalidParams = "common.invalid_params"
	MsgInvalidId     = "common.invalid_id"
	MsgUpdateSuccess = "common.update_success"
	MsgUpdateFailed  = "common.update_failed"
	MsgBatchTooMany  = "common.batch_too_many"
)

// Auth middleware messages
const (
	MsgAuthNotLoggedIn           = "auth.not_logged_in"
	MsgAuthAccessTokenInvalid    = "auth.access_token_invalid"
	MsgAuthUserInfoInvalid       = "auth.user_info_invalid"
	MsgAuthInsufficientPrivilege = "auth.insufficient_privilege"
)

// Token related messages
const (
	MsgTokenNameTooLong          = "token.name_too_long"
	MsgTokenQuotaNegative        = "token.quota_negative"
	MsgTokenQuotaExceedMax       = "token.quota_exceed_max"
	MsgTokenGenerateFailed       = "token.generate_failed"
	MsgTokenExpiredCannotEnable  = "token.expired_cannot_enable"
	MsgTokenExhaustedCannotEable = "token.exhausted_cannot_enable"
	MsgTokenAutoGroupsTooMany    = "token.auto_groups_too_many"
	MsgTokenAutoGroupsDuplicate  = "token.auto_groups_duplicate"
	MsgTokenAutoGroupsInvalid    = "token.auto_groups_invalid"
)

// Redemption related messages
const (
	MsgRedemptionNameLength        = "redemption.name_length"
	MsgRedemptionCountPositive     = "redemption.count_positive"
	MsgRedemptionCountMax          = "redemption.count_max"
	MsgRedemptionCreateFailed      = "redemption.create_failed"
	MsgRedemptionExpireTimeInvalid = "redemption.expire_time_invalid"
)

// User related messages
const (
	MsgUserPasswordLoginDisabled     = "user.password_login_disabled"
	MsgUserRegisterDisabled          = "user.register_disabled"
	MsgUserPasswordRegisterDisabled  = "user.password_register_disabled"
	MsgUserUsernameOrPasswordError   = "user.username_or_password_error"
	MsgUserExists                    = "user.exists"
	MsgUserNotExists                 = "user.not_exists"
	MsgUserEmailVerificationRequired = "user.email_verification_required"
	MsgUserVerificationCodeError     = "user.verification_code_error"
	MsgUserEmailAlreadyTaken         = "user.email_already_taken"
	MsgUserPasswordResetLinkInvalid  = "user.password_reset_link_invalid"
	MsgUserInputInvalid              = "user.input_invalid"
	MsgUserNoPermissionSameLevel     = "user.no_permission_same_level"
	MsgUserNoPermissionHigherLevel   = "user.no_permission_higher_level"
	MsgUserCannotCreateHigherLevel   = "user.cannot_create_higher_level"
	MsgUserCannotDeleteRootUser      = "user.cannot_delete_root_user"
	MsgUserCannotDisableRootUser     = "user.cannot_disable_root_user"
	MsgUserCannotDemoteRootUser      = "user.cannot_demote_root_user"
	MsgUserInviterInvalid            = "user.inviter_invalid"
	MsgUserInviterNotFound           = "user.inviter_not_found"
	MsgUserInviterSelf               = "user.inviter_self"
	MsgUserInviterCycle              = "user.inviter_cycle"
	MsgUserAlreadyAdmin              = "user.already_admin"
	MsgUserAlreadyCommon             = "user.already_common"
	MsgUserAdminCannotPromote        = "user.admin_cannot_promote"
	MsgUserTransferSuccess           = "user.transfer_success"
	MsgUserTransferFailed            = "user.transfer_failed"
	MsgUserTopUpProcessing           = "user.topup_processing"
	MsgUserRegisterFailed            = "user.register_failed"
	MsgUserDefaultTokenFailed        = "user.default_token_failed"
	MsgUserEmailEmpty                = "user.email_empty"
	MsgUserQuotaChangeZero           = "user.quota_change_zero"
)

// Quota related messages
const (
	MsgQuotaThresholdGtZero = "quota.threshold_gt_zero"
)

// Invoice related messages
const (
	MsgInvoiceNotFound                = "invoice.not_found"
	MsgInvoiceStatusInvalid           = "invoice.status_invalid"
	MsgInvoiceStatusTransition        = "invoice.status_transition"
	MsgInvoiceRejectionReasonRequired = "invoice.rejection_reason_required"
	MsgInvoiceRejectionReasonInvalid  = "invoice.rejection_reason_invalid"
	MsgInvoiceOrderEmpty              = "invoice.order_empty"
	MsgInvoiceOrderInvalid            = "invoice.order_invalid"
	MsgInvoiceOrderDuplicate          = "invoice.order_duplicate"
	MsgInvoiceAmountTooSmall          = "invoice.amount_too_small"
	MsgInvoiceTaxFeeInsufficient      = "invoice.tax_fee_insufficient"
	MsgInvoiceFileRequired            = "invoice.file_required"
	MsgInvoiceCompanyEmpty            = "invoice.company_empty"
	MsgInvoiceCompanyInvalid          = "invoice.company_invalid"
	MsgInvoiceTaxNumberEmpty          = "invoice.tax_number_empty"
	MsgInvoiceTaxNumberInvalid        = "invoice.tax_number_invalid"
	MsgInvoiceEmailEmpty              = "invoice.email_empty"
	MsgInvoiceEmailInvalid            = "invoice.email_invalid"
	MsgInvoiceOptionalFieldInvalid    = "invoice.optional_field_invalid"
	MsgInvoiceWithdrawForbidden       = "invoice.withdraw_forbidden"
	MsgInvoicePurgeForbidden          = "invoice.purge_forbidden"
	MsgInvoiceFileDisabled            = "invoice.file_disabled"
	MsgInvoiceFileLimit               = "invoice.file_limit"
	MsgInvoiceFileMissing             = "invoice.file_missing"
	MsgInvoiceFileEmpty               = "invoice.file_empty"
	MsgInvoiceFileTooLarge            = "invoice.file_too_large"
	MsgInvoiceFileNotFound            = "invoice.file_not_found"
	MsgInvoiceIssuedFileRequired      = "invoice.issued_file_required"
	MsgInvoiceFileContentMissing      = "invoice.file_content_missing"
	MsgInvoiceFileMutationRejected    = "invoice.file_mutation_rejected"
	MsgInvoiceFileMutationArchived    = "invoice.file_mutation_archived"
	MsgInvoiceFileMutationFinal       = "invoice.file_mutation_final"
	MsgInvoiceOperationFailed         = "invoice.operation_failed"
	MsgInvoiceLoadFailed              = "invoice.load_failed"
	MsgInvoiceSubmitFailed            = "invoice.submit_failed"
	MsgInvoiceUpdateFailed            = "invoice.update_failed"
	MsgInvoiceUploadFailed            = "invoice.upload_failed"
	MsgInvoiceDownloadFailed          = "invoice.download_failed"
	MsgInvoiceFileDeleteFailed        = "invoice.file_delete_failed"
	MsgInvoiceMaintenanceFailed       = "invoice.maintenance_failed"
	MsgInvoiceSettingsInvalid         = "invoice.settings_invalid"
	MsgInvoiceSettingsSaveFailed      = "invoice.settings_save_failed"
	MsgInvoiceStorageTestFailed       = "invoice.storage_test_failed"
	MsgInvoiceFileNameInvalid         = "invoice.file_name_invalid"
	MsgInvoiceFileTypeNotAllowed      = "invoice.file_type_not_allowed"
	MsgInvoiceFileTypeMismatch        = "invoice.file_type_mismatch"
	MsgInvoiceFileSVGForbidden        = "invoice.file_svg_forbidden"
	MsgInvoiceOrphanKeyRejected       = "invoice.orphan_key_rejected"
	MsgInvoiceWechatNotConfigured     = "invoice.wechat_not_configured"
	MsgInvoiceWechatTestFailed        = "invoice.wechat_test_failed"
	MsgInvoiceWechatAutoIssueOff      = "invoice.wechat_auto_issue_disabled"
	MsgInvoiceWechatGroupMismatch     = "invoice.wechat_group_mismatch"
)

// Payment related messages
const (
	MsgPaymentComplianceRequired = "payment.compliance_required"
)

// Channel related messages
const (
	MsgChannelNotExists = "channel.not_exists"
)

// Passkey related messages
const (
	MsgPasskeyRPIDInvalid             = "passkey.rp_id_invalid"
	MsgPasskeyRPIDUnavailable         = "passkey.rp_id_unavailable"
	MsgPasskeyRPIDRemovalConfirmation = "passkey.rp_id_removal_confirmation"
)

// Setting related messages
const (
	MsgSettingInvalidType      = "setting.invalid_type"
	MsgSettingWebhookEmpty     = "setting.webhook_empty"
	MsgSettingWebhookInvalid   = "setting.webhook_invalid"
	MsgSettingEmailInvalid     = "setting.email_invalid"
	MsgSettingBarkUrlEmpty     = "setting.bark_url_empty"
	MsgSettingBarkUrlInvalid   = "setting.bark_url_invalid"
	MsgSettingGotifyUrlEmpty   = "setting.gotify_url_empty"
	MsgSettingGotifyTokenEmpty = "setting.gotify_token_empty"
	MsgSettingGotifyUrlInvalid = "setting.gotify_url_invalid"
	MsgSettingUrlMustHttp      = "setting.url_must_http"
	MsgSettingSaved            = "setting.saved"
)

// OAuth related messages
const (
	MsgOAuthInvalidCode     = "oauth.invalid_code"
	MsgOAuthGetUserErr      = "oauth.get_user_error"
	MsgOAuthUnknownProvider = "oauth.unknown_provider"
	MsgOAuthStateInvalid    = "oauth.state_invalid"
	MsgOAuthNotEnabled      = "oauth.not_enabled"
	MsgOAuthUserDeleted     = "oauth.user_deleted"
	MsgOAuthUserBanned      = "oauth.user_banned"
	MsgOAuthBindSuccess     = "oauth.bind_success"
	MsgOAuthAlreadyBound    = "oauth.already_bound"
	MsgOAuthNotAutoLinked   = "oauth.not_auto_linked"
	MsgOAuthConnectFailed   = "oauth.connect_failed"
	MsgOAuthTokenFailed     = "oauth.token_failed"
	MsgOAuthUserInfoEmpty   = "oauth.user_info_empty"
	MsgOAuthTrustLevelLow   = "oauth.trust_level_low"
)

// Model layer error messages (for translation in controller)
const (
	MsgRedeemFailed          = "redeem.failed"
	MsgCreateDefaultTokenErr = "user.create_default_token_error"
	MsgUuidDuplicate         = "common.uuid_duplicate"
	MsgInvalidInput          = "common.invalid_input"
)

// Affiliate commission messages
const (
	MsgAffiliateCommissionNotFound             = "affiliate.commission_not_found"
	MsgAffiliateCommissionStatusInvalid        = "affiliate.commission_status_invalid"
	MsgAffiliateTopUpInvalid                   = "affiliate.topup_invalid"
	MsgAffiliateRejectReasonRequired           = "affiliate.reject_reason_required"
	MsgAffiliateUpgradeNotEligible             = "affiliate.upgrade_not_eligible"
	MsgAffiliatePayoutNotFound                 = "affiliate.payout_not_found"
	MsgAffiliatePayoutForbidden                = "affiliate.payout_forbidden"
	MsgAffiliatePayoutStatusInvalid            = "affiliate.payout_status_invalid"
	MsgAffiliatePayoutAmountTooSmall           = "affiliate.payout_amount_too_small"
	MsgAffiliatePayoutInsufficientBalance      = "affiliate.payout_insufficient_balance"
	MsgAffiliatePayoutAccountInvalid           = "affiliate.payout_account_invalid"
	MsgAffiliatePayoutRequestIdInvalid         = "affiliate.payout_request_id_invalid"
	MsgAffiliatePayoutRequestConflict          = "affiliate.payout_request_conflict"
	MsgAffiliatePayoutRejectionReasonRequired  = "affiliate.payout_rejection_reason_required"
	MsgAffiliatePayoutSettlementNotDue         = "affiliate.payout_settlement_not_due"
	MsgAffiliateLegacyRewardEnabled            = "affiliate.legacy_reward_enabled"
	MsgAffiliateAlipayNotConfigured            = "affiliate.alipay_not_configured"
	MsgAffiliateAlipayConfigInvalid            = "affiliate.alipay_config_invalid"
	MsgAffiliateAlipayPayoutFailed             = "affiliate.alipay_payout_failed"
	MsgAffiliateAlipayResponseSignatureInvalid = "affiliate.alipay_response_signature_invalid"
	MsgAffiliateAlipayClientIPNotTrusted       = "affiliate.alipay_client_ip_not_trusted"
)

// Invoice notification email texts, looked up by key when the email is built
const (
	MsgInvoiceEmailStatusPending          = "invoice.email.status.pending"
	MsgInvoiceEmailStatusIssued           = "invoice.email.status.issued"
	MsgInvoiceEmailStatusRejected         = "invoice.email.status.rejected"
	MsgInvoiceEmailStatusWithdrawn        = "invoice.email.status.withdrawn"
	MsgInvoiceEmailStatusExpired          = "invoice.email.status.expired"
	MsgInvoiceEmailStatusUnknown          = "invoice.email.status.unknown"
	MsgInvoiceEmailLabelRequestId         = "invoice.email.label.request_id"
	MsgInvoiceEmailLabelUsername          = "invoice.email.label.username"
	MsgInvoiceEmailLabelCompanyName       = "invoice.email.label.company_name"
	MsgInvoiceEmailLabelTaxNumber         = "invoice.email.label.tax_number"
	MsgInvoiceEmailLabelAmount            = "invoice.email.label.amount"
	MsgInvoiceEmailLabelTaxRate           = "invoice.email.label.tax_rate"
	MsgInvoiceEmailLabelTaxFee            = "invoice.email.label.tax_fee"
	MsgInvoiceEmailLabelStatus            = "invoice.email.label.status"
	MsgInvoiceEmailLabelCreatedAt         = "invoice.email.label.created_at"
	MsgInvoiceEmailLabelOrders            = "invoice.email.label.orders"
	MsgInvoiceEmailLabelExpiresAt         = "invoice.email.label.expires_at"
	MsgInvoiceEmailAdminCreatedSubject    = "invoice.email.admin.created.subject"
	MsgInvoiceEmailAdminCreatedHeading    = "invoice.email.admin.created.heading"
	MsgInvoiceEmailAdminCreatedIntro      = "invoice.email.admin.created.intro"
	MsgInvoiceEmailAdminCreatedAction     = "invoice.email.admin.created.action"
	MsgInvoiceEmailUserIssuedSubject      = "invoice.email.user.issued.subject"
	MsgInvoiceEmailUserIssuedHeading      = "invoice.email.user.issued.heading"
	MsgInvoiceEmailUserIssuedIntro        = "invoice.email.user.issued.intro"
	MsgInvoiceEmailUserIssuedAction       = "invoice.email.user.issued.action"
	MsgInvoiceEmailAdminExpirySubject     = "invoice.email.admin.expiry.subject"
	MsgInvoiceEmailAdminExpiryHeading     = "invoice.email.admin.expiry.heading"
	MsgInvoiceEmailAdminExpiryIntro       = "invoice.email.admin.expiry.intro"
	MsgInvoiceEmailAdminExpiryAction      = "invoice.email.admin.expiry.action"
	MsgInvoiceEmailLabelRejectionReason   = "invoice.email.label.rejection_reason"
	MsgInvoiceEmailLabelTaxFeeRefunded    = "invoice.email.label.tax_fee_refunded"
	MsgInvoiceEmailPreviewRejectionReason = "invoice.email.preview.rejection_reason"
	MsgInvoiceEmailUserExpiredSubject     = "invoice.email.user.expired.subject"
	MsgInvoiceEmailUserExpiredHeading     = "invoice.email.user.expired.heading"
	MsgInvoiceEmailUserExpiredIntro       = "invoice.email.user.expired.intro"
	MsgInvoiceEmailUserExpiredAction      = "invoice.email.user.expired.action"
	MsgInvoiceEmailUserRejectedSubject    = "invoice.email.user.rejected.subject"
	MsgInvoiceEmailUserRejectedHeading    = "invoice.email.user.rejected.heading"
	MsgInvoiceEmailUserRejectedIntro      = "invoice.email.user.rejected.intro"
	MsgInvoiceEmailUserRejectedAction     = "invoice.email.user.rejected.action"
	MsgInvoiceEmailUserRefundNote         = "invoice.email.user.refund_note"
)

// Affiliate commission email texts, looked up by key when the email is built
const (
	MsgAffiliateEmailLabelPromoter             = "affiliate.email.label.promoter"
	MsgAffiliateEmailLabelUsername             = "affiliate.email.label.username"
	MsgAffiliateEmailLabelUid                  = "affiliate.email.label.uid"
	MsgAffiliateEmailLabelCurrentGroup         = "affiliate.email.label.current_group"
	MsgAffiliateEmailLabelEffectiveInvitees    = "affiliate.email.label.effective_invitees"
	MsgAffiliateEmailLabelEffectiveTopupAmount = "affiliate.email.label.effective_topup_amount"
	MsgAffiliateEmailLabelThreshold            = "affiliate.email.label.threshold"
	MsgAffiliateEmailLabelNextGroup            = "affiliate.email.label.next_group"
	MsgAffiliateEmailLabelRate                 = "affiliate.email.label.rate"
	MsgAffiliateEmailLabelOrder                = "affiliate.email.label.order"
	MsgAffiliateEmailLabelTopupAmount          = "affiliate.email.label.topup_amount"
	MsgAffiliateEmailLabelCommission           = "affiliate.email.label.commission"
	MsgAffiliateEmailLabelStatus               = "affiliate.email.label.status"
	MsgAffiliateEmailLabelReason               = "affiliate.email.label.reason"
	MsgAffiliateEmailLabelInvitee              = "affiliate.email.label.invitee"
	MsgAffiliateEmailLabelProcessedTime        = "affiliate.email.label.processed_time"
	MsgAffiliateEmailLabelNewGroup             = "affiliate.email.label.new_group"
	MsgAffiliateEmailLabelPayoutId             = "affiliate.email.label.payout_id"
	MsgAffiliateEmailLabelAmount               = "affiliate.email.label.amount"
	MsgAffiliateEmailLabelAlipayRecipient      = "affiliate.email.label.alipay_recipient"
	MsgAffiliateEmailLabelAlipayAccount        = "affiliate.email.label.alipay_account"
	MsgAffiliateEmailLabelSettlementTime       = "affiliate.email.label.settlement_time"
	MsgAffiliateEmailTierDefault               = "affiliate.email.tier.default"
	MsgAffiliateEmailTierJunior                = "affiliate.email.tier.junior"
	MsgAffiliateEmailTierAdvanced              = "affiliate.email.tier.advanced"
	MsgAffiliateEmailTierGold                  = "affiliate.email.tier.gold"
	MsgAffiliateEmailStatusApproved            = "affiliate.email.status.approved"
	MsgAffiliateEmailStatusRejected            = "affiliate.email.status.rejected"
	MsgAffiliateEmailUpgradeSubject            = "affiliate.email.upgrade.subject"
	MsgAffiliateEmailUpgradeHeading            = "affiliate.email.upgrade.heading"
	MsgAffiliateEmailUpgradeIntro              = "affiliate.email.upgrade.intro"
	MsgAffiliateEmailUpgradeAction             = "affiliate.email.upgrade.action"
	MsgAffiliateEmailCommissionApprovedSubject = "affiliate.email.commission.approved.subject"
	MsgAffiliateEmailCommissionApprovedHeading = "affiliate.email.commission.approved.heading"
	MsgAffiliateEmailCommissionApprovedIntro   = "affiliate.email.commission.approved.intro"
	MsgAffiliateEmailCommissionRejectedSubject = "affiliate.email.commission.rejected.subject"
	MsgAffiliateEmailCommissionRejectedHeading = "affiliate.email.commission.rejected.heading"
	MsgAffiliateEmailCommissionRejectedIntro   = "affiliate.email.commission.rejected.intro"
	MsgAffiliateEmailCommissionAction          = "affiliate.email.commission.action"
	MsgAffiliateEmailUpgradeApprovedSubject    = "affiliate.email.upgrade_approved.subject"
	MsgAffiliateEmailUpgradeApprovedHeading    = "affiliate.email.upgrade_approved.heading"
	MsgAffiliateEmailUpgradeApprovedIntro      = "affiliate.email.upgrade_approved.intro"
	MsgAffiliateEmailUpgradeApprovedAction     = "affiliate.email.upgrade_approved.action"
	MsgAffiliateEmailPayoutApprovedSubject     = "affiliate.email.payout.approved.subject"
	MsgAffiliateEmailPayoutApprovedHeading     = "affiliate.email.payout.approved.heading"
	MsgAffiliateEmailPayoutApprovedIntro       = "affiliate.email.payout.approved.intro"
	MsgAffiliateEmailPayoutRejectedSubject     = "affiliate.email.payout.rejected.subject"
	MsgAffiliateEmailPayoutRejectedHeading     = "affiliate.email.payout.rejected.heading"
	MsgAffiliateEmailPayoutRejectedIntro       = "affiliate.email.payout.rejected.intro"
	MsgAffiliateEmailPayoutPaidSubject         = "affiliate.email.payout.paid.subject"
	MsgAffiliateEmailPayoutPaidHeading         = "affiliate.email.payout.paid.heading"
	MsgAffiliateEmailPayoutPaidIntro           = "affiliate.email.payout.paid.intro"
	MsgAffiliateEmailPayoutAction              = "affiliate.email.payout.action"
	MsgAffiliateEmailPayoutStatusApproved      = "affiliate.email.payout.status.approved"
	MsgAffiliateEmailPayoutStatusRejected      = "affiliate.email.payout.status.rejected"
	MsgAffiliateEmailPayoutStatusPaid          = "affiliate.email.payout.status.paid"
)

// Referral program application confirmation email texts (88api referral)
const (
	MsgReferralApplicationEmailSubject        = "referral_application.email.subject"
	MsgReferralApplicationEmailHeading        = "referral_application.email.heading"
	MsgReferralApplicationEmailIntro          = "referral_application.email.intro"
	MsgReferralApplicationEmailLabelReference = "referral_application.email.label.reference"
	MsgReferralApplicationEmailLabelStartRate = "referral_application.email.label.start_rate"
	MsgReferralApplicationEmailLabelNextGroup = "referral_application.email.label.next_group"
	MsgReferralApplicationEmailLabelUpgrade   = "referral_application.email.label.upgrade"
	MsgReferralApplicationEmailLabelPayout    = "referral_application.email.label.payout"
	MsgReferralApplicationEmailValueStartRate = "referral_application.email.value.start_rate"
	MsgReferralApplicationEmailValueNextGroup = "referral_application.email.value.next_group"
	MsgReferralApplicationEmailValueUpgrade   = "referral_application.email.value.upgrade"
	MsgReferralApplicationEmailValuePayout    = "referral_application.email.value.payout"
	MsgReferralApplicationEmailStepsTitle     = "referral_application.email.steps_title"
	MsgReferralApplicationEmailStepLink       = "referral_application.email.step.link"
	MsgReferralApplicationEmailStepKit        = "referral_application.email.step.kit"
	MsgReferralApplicationEmailStepKitLink    = "referral_application.email.step.kit_link"
	MsgReferralApplicationEmailStepShare      = "referral_application.email.step.share"
	MsgReferralApplicationEmailStepCommission = "referral_application.email.step.commission"
	MsgReferralApplicationEmailSupportNote    = "referral_application.email.support_note"
	MsgReferralApplicationEmailRulesNote      = "referral_application.email.rules_note"
	MsgReferralApplicationEmailSecurityNote   = "referral_application.email.security_note"
	MsgReferralApplicationEmailAction         = "referral_application.email.action"
)

// Built-in email template texts, looked up by key when the email is built
const (
	MsgCoreEmailVerificationSubject           = "core_email.verification.subject"
	MsgCoreEmailVerificationHeading           = "core_email.verification.heading"
	MsgCoreEmailVerificationIntro             = "core_email.verification.intro"
	MsgCoreEmailVerificationValidity          = "core_email.verification.validity"
	MsgCoreEmailVerificationSecurity          = "core_email.verification.security"
	MsgCoreEmailPasswordResetSubject          = "core_email.password_reset.subject"
	MsgCoreEmailPasswordResetHeading          = "core_email.password_reset.heading"
	MsgCoreEmailPasswordResetIntro            = "core_email.password_reset.intro"
	MsgCoreEmailPasswordResetAction           = "core_email.password_reset.action"
	MsgCoreEmailPasswordResetValidityLabel    = "core_email.password_reset.validity_label"
	MsgCoreEmailPasswordResetValidityValue    = "core_email.password_reset.validity_value"
	MsgCoreEmailPasswordResetLinkLabel        = "core_email.password_reset.link_label"
	MsgCoreEmailPasswordResetSecurity         = "core_email.password_reset.security"
	MsgCoreEmailSystemAlertIntro              = "core_email.system_alert.intro"
	MsgCoreEmailSystemAlertSecurity           = "core_email.system_alert.security"
	MsgCoreEmailQuotaWarningIntro             = "core_email.quota_warning.intro"
	MsgCoreEmailQuotaWarningPreviewSubject    = "core_email.quota_warning.preview_subject"
	MsgCoreEmailQuotaWarningPreviewBody       = "core_email.quota_warning.preview_body"
	MsgCoreEmailQuotaWarningAction            = "core_email.quota_warning.action"
	MsgCoreEmailChannelStatusIntro            = "core_email.channel_status.intro"
	MsgCoreEmailChannelStatusPreviewSubject   = "core_email.channel_status.preview_subject"
	MsgCoreEmailChannelStatusPreviewBody      = "core_email.channel_status.preview_body"
	MsgCoreEmailInspectionAlertIntro          = "core_email.inspection_alert.intro"
	MsgCoreEmailInspectionAlertPreviewSubject = "core_email.inspection_alert.preview_subject"
	MsgCoreEmailInspectionAlertPreviewBody    = "core_email.inspection_alert.preview_body"
)
