package constant

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmailTemplateSpecsExposeCoreBusinessTemplates(t *testing.T) {
	keys := make(map[string]bool)
	for _, spec := range EmailTemplateSpecs() {
		keys[spec.Key] = true
	}

	for _, key := range []string{
		EmailTemplateKeyAccountVerificationUser,
		EmailTemplateKeyPasswordResetUser,
		EmailTemplateKeyQuotaWarningUser,
		EmailTemplateKeyChannelStatusAdmin,
		EmailTemplateKeyInspectionAlertAdmin,
	} {
		assert.True(t, keys[key], "template catalog is missing %s", key)
	}
	assert.False(t, keys[EmailTemplateKeySystemAlertUser], "legacy shared alert template must stay hidden")
}

func TestActionEmailTemplatesCenterTheirPrimaryButton(t *testing.T) {
	for _, spec := range EmailTemplateSpecs() {
		if !strings.Contains(spec.DefaultBody, "{{action_url}}") {
			continue
		}
		assert.Contains(t, spec.DefaultBody, "text-align:center;", spec.Key)
	}
}

func TestDefaultEmailTemplatesExposeLinkedSiteBrand(t *testing.T) {
	for _, spec := range EmailTemplateSpecs() {
		assert.Contains(t, spec.DefaultBody, `href="{{server_address}}"`, spec.Key)
		assert.Contains(t, spec.DefaultBody, `>{{system_name}}</a>`, spec.Key)
		assert.Contains(t, spec.DefaultBody, "linear-gradient(135deg,#0891b2", spec.Key)
		assert.Contains(t, spec.DefaultBody, "background-color:#5b5ce2", spec.Key)

		hasServerAddress := false
		for _, variable := range spec.Variables {
			if variable.Name == "server_address" {
				hasServerAddress = true
				break
			}
		}
		assert.True(t, hasServerAddress, "%s must expose server_address", spec.Key)
	}
}

func TestLegacySystemAlertTemplateRemainsRenderable(t *testing.T) {
	spec, ok := FindEmailTemplateSpec(EmailTemplateKeySystemAlertUser)
	require.True(t, ok)
	assert.Equal(t, EmailTemplateKeySystemAlertUser, spec.Key)
	assert.NotEmpty(t, spec.DefaultBody)
}
