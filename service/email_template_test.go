package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPreviewInvoiceEmailTemplatesUseScenarioSpecificSamples(t *testing.T) {
	tests := []struct {
		key             string
		subjectContains string
		bodyContains    []string
	}{
		{
			key:             constant.EmailTemplateKeyInvoiceRequestAdmin,
			subjectContains: "新发票申请 #1024",
			bodyContains:    []string{"新的发票申请", "alice 提交了一份新的发票申请", "/admin-invoices/1024", "前往开票"},
		},
		{
			key:             constant.EmailTemplateKeyInvoiceIssuedUser,
			subjectContains: "发票申请 #1024 已开具",
			bodyContains:    []string{"发票已开具", "电子发票已随本邮件发送", "/invoices/1024", "前往控制台下载"},
		},
		{
			key:             constant.EmailTemplateKeyInvoiceRejectedUser,
			subjectContains: "发票申请 #1024 已被驳回",
			bodyContains:    []string{"发票申请已驳回", "驳回原因", "税号与公司名称不匹配", "已退还税费", "已支付的税费 ¥21.04 已退回账户余额", "/invoices/1024", "查看申请详情"},
		},
		{
			key:             constant.EmailTemplateKeyInvoiceExpiredUser,
			subjectContains: "发票申请 #1024 已过期",
			bodyContains:    []string{"发票申请已过期", "系统已自动关闭", "已退还税费", "已支付的税费 ¥21.04 已退回账户余额", "/invoices/1024", "查看申请详情"},
		},
		{
			key:             constant.EmailTemplateKeyInvoiceExpiryAdmin,
			subjectContains: "发票申请 #1024 将在 24 小时内过期",
			bodyContains:    []string{"发票申请即将过期", "预计过期时间", "/admin-invoices/1024", "立即处理"},
		},
	}

	for _, test := range tests {
		t.Run(test.key, func(t *testing.T) {
			subject, body, err := PreviewEmailTemplate(test.key, "", "")
			require.NoError(t, err)
			assert.Contains(t, subject, test.subjectContains)
			assert.NotContains(t, subject, "{{")
			assert.NotContains(t, body, "{{")
			assert.Contains(t, body, "示例科技有限公司")
			for _, expected := range test.bodyContains {
				assert.Contains(t, body, expected)
			}
		})
	}
}

func TestPreviewAffiliateEmailTemplatesUseLocalizedConfiguredRates(t *testing.T) {
	tests := []struct {
		name            string
		key             string
		lang            string
		subjectContains string
		bodyContains    []string
	}{
		{
			name:            "upgrade-zh",
			key:             constant.EmailTemplateKeyAffiliateUpgradeAdmin,
			lang:            "zh-CN",
			subjectContains: "推广员升级提醒",
			bodyContains:    []string{"alice", "高级推广", "10%", "¥2,000.00", "/admin-affiliates"},
		},
		{
			name:            "commission-en",
			key:             constant.EmailTemplateKeyAffiliateCommissionUser,
			lang:            "en",
			subjectContains: "Commission approved",
			bodyContains:    []string{"Invited user", "al***", "5.00", "Approved", "Processed at", "/referral"},
		},
		{
			name:            "upgrade-user-zh",
			key:             constant.EmailTemplateKeyAffiliateUpgradeUser,
			lang:            "zh-CN",
			subjectContains: "推广等级升级成功",
			bodyContains:    []string{"alice", "高级推广", "10%", "/referral"},
		},
		{
			name:            "payout-user-en",
			key:             constant.EmailTemplateKeyAffiliatePayoutUser,
			lang:            "en",
			subjectContains: "Commission payout approved",
			bodyContains:    []string{"#1024", "¥100.00", "Approved, awaiting payout", "/referral?tab=payouts"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			subject, body, err := PreviewEmailTemplateForLang(test.key, test.lang, "", "")
			require.NoError(t, err)
			assert.Contains(t, subject, test.subjectContains)
			assert.NotContains(t, subject, "{{")
			assert.NotContains(t, body, "{{")
			for _, expected := range test.bodyContains {
				assert.Contains(t, body, expected)
			}
		})
	}
}

func TestAffiliateCommissionResultEmailShowsOnlyMaskedInviteeAndCommission(t *testing.T) {
	tests := []struct {
		name         string
		record       *model.AffiliateCommission
		inviteeName  string
		lang         string
		wantMode     string
		bodyContains []string
		bodyExcludes []string
	}{
		{
			name: "approved-zh",
			record: &model.AffiliateCommission{
				TradeNo: "ORDER-SECRET-APPROVED", TopUpAmountCents: 123456, CommissionCents: 6173,
				Status: model.AffiliateCommissionStatusApproved, ApprovedTime: 1792000000,
			},
			inviteeName:  "alice-buyer",
			lang:         "zh-CN",
			wantMode:     "approved",
			bodyContains: []string{"受邀用户", "al***", "61.73", "已通过", "处理时间"},
			bodyExcludes: []string{"alice-buyer", "ORDER-SECRET-APPROVED", "1234.56", "充值金额", "充值订单"},
		},
		{
			name: "rejected-en-keeps-reason",
			record: &model.AffiliateCommission{
				TradeNo: "ORDER-SECRET-REJECTED", TopUpAmountCents: 50000, CommissionCents: 2500,
				Status: model.AffiliateCommissionStatusRejected, RejectReason: "risk review rejected", UpdatedTime: 1792000000,
			},
			inviteeName:  "bob",
			lang:         "en",
			wantMode:     "rejected",
			bodyContains: []string{"Invited user", "bo***", "25.00", "Rejected", "risk review rejected", "Processed at"},
			bodyExcludes: []string{"ORDER-SECRET-REJECTED", "500.00", "Top-up amount"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mode, subject, body := buildAffiliateCommissionResultEmail(test.record, test.inviteeName, test.lang)
			assert.Equal(t, test.wantMode, mode)
			assert.NotEmpty(t, subject)
			assert.NotContains(t, body, "{{")
			for _, expected := range test.bodyContains {
				assert.Contains(t, body, expected)
			}
			for _, hidden := range test.bodyExcludes {
				assert.NotContains(t, body, hidden)
			}
		})
	}
}

func TestAffiliateCommissionResultEmailBlanksRetiredTemplateVariables(t *testing.T) {
	key := constant.EmailTemplateBodyLangKey(constant.EmailTemplateKeyAffiliateCommissionUser, "zh-CN")
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = map[string]string{}
	}
	previous, existed := common.OptionMap[key]
	common.OptionMap[key] = "order=[{{order_number}}] topup=[{{topup_amount}}] invitee=[{{invitee_username}}]"
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		defer common.OptionMapRWMutex.Unlock()
		if existed {
			common.OptionMap[key] = previous
			return
		}
		delete(common.OptionMap, key)
	})

	record := &model.AffiliateCommission{
		TradeNo: "ORDER-SECRET-CUSTOM", TopUpAmountCents: 10000, CommissionCents: 500,
		Status: model.AffiliateCommissionStatusApproved, ApprovedTime: 1792000000,
	}
	_, _, body := buildAffiliateCommissionResultEmail(record, "carol", "zh-CN")
	require.Equal(t, "order=[] topup=[] invitee=[ca***]", body)
}
