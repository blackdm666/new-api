package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateUserSettingGatesUnpricedModelsByRole(t *testing.T) {
	for _, testCase := range []struct {
		name string
		role int
		want bool
	}{
		{name: "regular user", role: common.RoleCommonUser, want: false},
		{name: "administrator", role: common.RoleAdminUser, want: true},
		{name: "root", role: common.RoleRootUser, want: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			db := setupManageUserTestDB(t)
			user := model.User{
				Username: fmt.Sprintf("setting-role-%d", testCase.role),
				Password: "password",
				Role:     testCase.role,
				Status:   common.UserStatusEnabled,
				Group:    "default",
			}
			require.NoError(t, db.Create(&user).Error)

			gin.SetMode(gin.TestMode)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPut, "/api/user/setting", strings.NewReader(
				`{"notify_type":"email","quota_warning_threshold":1,"accept_unset_model_ratio_model":true}`,
			))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Set("id", user.Id)
			UpdateUserSetting(c)

			assert.Equal(t, http.StatusOK, recorder.Code)
			assert.Contains(t, recorder.Body.String(), `"success":true`)
			var got model.User
			require.NoError(t, db.First(&got, user.Id).Error)
			assert.Equal(t, testCase.want, got.GetSetting().AcceptUnsetRatioModel)
		})
	}
}
