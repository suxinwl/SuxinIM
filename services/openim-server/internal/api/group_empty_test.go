package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/openimsdk/protocol/group"
	"github.com/openimsdk/protocol/sdkws"
	"google.golang.org/grpc"
)

type joinedGroupFixture struct {
	group.GroupClient
	groups []*sdkws.GroupInfo
}

func (f joinedGroupFixture) GetJoinedGroupList(context.Context, *group.GetJoinedGroupListReq, ...grpc.CallOption) (*group.GetJoinedGroupListResp, error) {
	return &group.GetJoinedGroupListResp{Groups: f.groups, Total: uint32(len(f.groups))}, nil
}

func TestJoinedGroupHTTPResponseKeepsArrayContractForNewEmployees(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, groups := range [][]*sdkws.GroupInfo{nil, {{GroupID: "fixture-group"}}} {
		api := NewGroupApi(joinedGroupFixture{groups: groups})
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/group/get_joined_group_list", strings.NewReader(`{"fromUserID":"emp_fixture","pagination":{"pageNumber":1,"showNumber":100}}`))
		c.Request.Header.Set("Content-Type", "application/json")
		api.GetJoinedGroupList(c)
		var response struct {
			ErrCode int `json:"errCode"`
			Data    struct {
				Groups []map[string]any `json:"groups"`
				Total  int              `json:"total"`
			} `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil || w.Code != 200 || response.ErrCode != 0 || response.Data.Groups == nil || len(response.Data.Groups) != len(groups) || response.Data.Total != len(groups) {
			t.Fatal("joined groups must serialize as an array", w.Code, err, w.Body.String())
		}
	}
}
