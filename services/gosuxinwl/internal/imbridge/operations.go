package imbridge

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
)

// Fixed operations and fields are the only bridge surface available to an
// administrator browser. Upstream credentials and URLs cannot enter this API.
var managementFields = map[string]string{
	"groups/search":           "keyword pageNumber showNumber page pageSize",
	"groups/members":          "groupID pageNumber showNumber",
	"groups/create":           "groupName ownerUserID memberUserIDs groupType notification",
	"groups/dismiss":          "groupID",
	"groups/members/add":      "groupID invitedUserIDs",
	"groups/members/remove":   "groupID kickedUserIDs",
	"groups/members/mute":     "groupID userID mutedSeconds",
	"groups/members/role":     "groupID userID roleLevel",
	"groups/members/nickname": "groupID userID nickname",
	"groups/owner":            "groupID oldOwnerUserID newOwnerUserID",
	"groups/info":             "groupID groupName notification introduction",
	"groups/mute":             "groupID muted",
	"groups/notice":           "groupID notice",
	"messages/search":         "sendID recvID groupID keyword startTime endTime pageNumber showNumber",
	"messages/revoke":         "conversationID seq userID",
	"messages/notification":   "sendID recvID groupID content sendAll",
	"users/online":            "userIDs",
	"users/kick":              "userID platformID",
	"friends/search":          "userID pageNumber showNumber",
	"friends/add":             "ownerUserID friendUserID",
	"friends/remove":          "ownerUserID friendUserID",
	"rtc/status":              "",
	"rtc/records/search":      "userID startTime endTime pageNumber showNumber",
	"rtc/records/delete":      "recordIDs",
	"config/get":              "",
	"config/update":           "webTitle appName logoURL privacyURL userAgreementURL allowAddFriend discoverEnabled",
	"versions/search":         "platform pageNumber showNumber",
	"versions/create":         "platform version downloadURL force description build size sha256",
	"versions/update":         "id platform version downloadURL force description build size sha256",
	"versions/delete":         "id",
	"discovery/list":          "",
	"discovery/save":          "items",
	"storage/capacity":        "",
}

func ManagementOperations() []string {
	result := make([]string, 0, len(managementFields))
	for operation := range managementFields {
		result = append(result, operation)
	}
	sort.Strings(result)
	return result
}
func decodeManagement(r *http.Request, operation string) (map[string]any, error) {
	fields, ok := managementFields[operation]
	if !ok {
		return nil, ErrInvalid
	}
	r.Body = http.MaxBytesReader(nil, r.Body, 64<<10)
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()
	var body map[string]any
	if decoder.Decode(&body) != nil || body == nil || decoder.Decode(new(any)) != io.EOF {
		return nil, ErrInvalid
	}
	allowed := " " + fields + " "
	for field, value := range body {
		if !strings.Contains(allowed, " "+field+" ") {
			return nil, ErrInvalid
		}
		if text, ok := value.(string); ok && len(text) > 16384 {
			return nil, ErrInvalid
		}
		if items, ok := value.([]any); ok && len(items) > 100 {
			return nil, ErrInvalid
		}
		if field == "showNumber" {
			n, ok := value.(json.Number)
			if !ok {
				return nil, ErrInvalid
			}
			size, err := n.Int64()
			if err != nil || size < 1 || size > 100 {
				return nil, ErrInvalid
			}
		}
		if field == "pageNumber" {
			n, ok := value.(json.Number)
			if !ok {
				return nil, ErrInvalid
			}
			page, err := n.Int64()
			if err != nil || page < 1 {
				return nil, ErrInvalid
			}
		}
	}
	if id, _ := body["groupID"].(string); strings.HasPrefix(id, "dept_") && operation != "groups/members" && operation != "messages/search" {
		return nil, errors.New("部门群由HRM统一维护")
	}
	return body, nil
}
func bindOperations(group *ghttp.RouterGroup, with func(func(context.Context, *Service, *ghttp.Request) (any, error)) func(*ghttp.Request)) {
	base := "/api/admin/im/manager/openim/"
	group.GET(base+"capabilities", with(func(_ context.Context, _ *Service, r *ghttp.Request) (any, error) {
		pair, err := SelectEndpoints(r.Request)
		return map[string]any{"operations": ManagementOperations(), "version": "3.8", "endpoints": map[string]string{"hrm": pair.HRM, "im": pair.IM}}, err
	}))
	for _, name := range ManagementOperations() {
		operation := name
		group.POST(base+"operations/"+operation, with(func(ctx context.Context, service *Service, r *ghttp.Request) (any, error) {
			body, err := decodeManagement(r.Request, operation)
			if err != nil {
				return nil, err
			}
			return service.Remote.Manage(ctx, operation, body)
		}))
	}
}
