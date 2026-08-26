package service

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
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
			bodyContains:    []string{"ORDER-202608-001", "5.00", "Approved", "/referral"},
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
