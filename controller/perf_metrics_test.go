package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPerfMetricsVisibleGroupsHidesPricedGroupsUsersCannotSelect(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	savedGroupRatio := ratio_setting.GroupRatio2JSONString()
	savedUsableGroups := setting.UserUsableGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(savedGroupRatio))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(savedUsableGroups))
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"GPT-Pro":1,"GPT-Pro不降智":1}`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","GPT-Pro":"Pro","not-priced":"x"}`))
	require.NoError(t, db.Create(&model.User{
		Id:       1201,
		Username: "perf-hidden-group-member",
		Password: "password",
		Group:    "GPT-Pro不降智",
		Status:   common.UserStatusEnabled,
	}).Error)

	anonymous, _ := gin.CreateTestContext(httptest.NewRecorder())
	anonymous.Request = httptest.NewRequest(http.MethodGet, "/api/perf-metrics?model=gpt-6-sol", nil)
	assert.ElementsMatch(t, []string{"auto", "default", "GPT-Pro"}, perfMetricsVisibleGroups(anonymous))

	member, _ := gin.CreateTestContext(httptest.NewRecorder())
	member.Request = httptest.NewRequest(http.MethodGet, "/api/perf-metrics?model=gpt-6-sol", nil)
	member.Set("id", 1201)
	assert.ElementsMatch(t, []string{"auto", "default", "GPT-Pro", "GPT-Pro不降智"}, perfMetricsVisibleGroups(member))
}
