package wecom

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/framework/util/guid"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
)

func exceptionEmployee(t *testing.T, id, uid, name, no string) {
	t.Helper()
	var userid, number any
	if uid != "" {
		userid = uid
	}
	if no != "" {
		number = no
	}
	if _, err := core.Model("biz_hrm_employee").Data(g.Map{"id": id, "wecom_userid": userid, "name": name, "employee_no": number, "created_at": now(), "updated_at": now()}).Insert(); err != nil {
		t.Fatal(err)
	}
}

func exceptionMember(t *testing.T, uid, employeeID, name, no string) {
	t.Helper()
	raw := g.Map{"userid": uid, "private": "PRIVATE-RAW-DIRECTORY"}
	if no != "" {
		raw["extattr"] = g.Map{"attrs": []g.Map{{"name": "工号", "type": 0, "text": g.Map{"value": no}}}}
	}
	if _, err := core.Model("wecom_user").Data(g.Map{"id": uid, "wecom_userid": uid, "employee_id": employeeID, "name": name, "department_ids": "[]", "raw_json": mustJSON(raw), "synced_at": now()}).Insert(); err != nil {
		t.Fatal(err)
	}
}

func TestExceptionIdentitiesUseOnlyUniqueExactBindings(t *testing.T) {
	ctx := setupDB(t)
	exceptionEmployee(t, "e-a", "u-a", "李明", "007319")
	exceptionEmployee(t, "e-b", "u-b", "李明", "003240")
	exceptionMember(t, "u-a", "e-b", "旧名字", "stale-no")
	exceptionMember(t, "u-b", "e-b", "李明", "003240")
	exceptionEmployee(t, "e-bound", "", "唯一绑定", "0088")
	exceptionMember(t, "bound", "e-bound", "", "")
	exceptionEmployee(t, "e-ambiguous", "", "不得借用的姓名", "0099")
	exceptionMember(t, "ambiguous-a", "e-ambiguous", "甲", "0100")
	exceptionMember(t, "ambiguous-b", "e-ambiguous", "乙", "0101")
	exceptionMember(t, "wrong-binding", "e-a", "本人", "")
	exceptionMember(t, "7319", "", "7319", "")
	exceptionMember(t, "raw-only", "", "只在通讯录", "0007")
	exceptionEmployee(t, "placeholder", "placeholder-user", "placeholder", "")
	exceptionMember(t, "placeholder-user", "placeholder", "真实姓名", "0008")
	got, err := loadExceptionIdentities(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for uid, want := range map[string]exceptionIdentity{
		"u-a": {"李明", "007319"}, "u-b": {"李明", "003240"},
		"bound": {"唯一绑定", "0088"}, "ambiguous-a": {"甲", "0100"}, "ambiguous-b": {"乙", "0101"},
		"wrong-binding": {"本人", ""}, "7319": {"", ""}, "raw-only": {"只在通讯录", "0007"},
		"placeholder-user": {"真实姓名", "0008"},
	} {
		if got[uid] != want {
			t.Errorf("%s: got %+v want %+v", uid, got[uid], want)
		}
	}
}

func TestExceptionEndpointAddsIdentityToEveryCategoryWithoutChangingErrors(t *testing.T) {
	ctx := setupDB(t)
	exceptionEmployee(t, "e-contact", "u-contact", "测试员工甲", "7319")
	exceptionMember(t, "u-contact", "e-contact", "旧显示名称", "旧工号")
	exceptionEmployee(t, "e-approval", "u-approval", "王芳", "3240")
	exceptionMember(t, "u-clock", "", "徐小会", "3299")
	if _, err := core.Model("wecom_user").Where("wecom_userid", "u-contact").Update(g.Map{"processing_error": "CONTACT-EXCEPTION"}); err != nil {
		t.Fatal(err)
	}
	if _, err := core.Model("wecom_checkin_raw").Data(g.Map{"id": "clock", "wecom_userid": "u-clock", "checkin_time": "2026-10-29 08:00:00", "raw_json": "PRIVATE-RAW-CLOCK", "external_ref": "clock", "processing_error": "CHECKIN-EXCEPTION", "created_at": now()}).Insert(); err != nil {
		t.Fatal(err)
	}
	if _, err := core.Model("wecom_approval_raw").Data(g.Map{"id": "approval", "sp_no": "approval", "wecom_userid": "u-approval", "raw_json": "PRIVATE-RAW-APPROVAL", "detail_json": "PRIVATE-DETAIL-APPROVAL", "processing_error": "APPROVAL-EXCEPTION", "synced_at": now()}).Insert(); err != nil {
		t.Fatal(err)
	}
	s := g.Server(guid.S())
	s.SetAddr("127.0.0.1:0")
	s.SetDumpRouterMap(false)
	s.Group("/", func(group *ghttp.RouterGroup) {
		group.Middleware(func(r *ghttp.Request) {
			if r.Header.Get("X-Test-UID") == "1" {
				r.SetCtx(context.WithValue(r.Context(), "uid", int64(1)))
			}
			r.Middleware.Next()
		})
		Bind(ctx, group, &Client{})
	})
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	defer s.Shutdown()
	url := fmt.Sprintf("http://127.0.0.1:%d/api/admin/wecom/exceptions", s.GetListenedPort())
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatal("anonymous exceptions access allowed")
	}
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("X-Test-UID", "1")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil || resp.StatusCode != http.StatusOK || strings.Contains(string(body), "PRIVATE-") {
		t.Fatal("endpoint failed or leaked raw profile", err)
	}
	var result struct {
		Code int `json:"code"`
		Data struct {
			Contacts  []g.Map        `json:"contacts"`
			Checkins  []g.Map        `json:"checkins"`
			Approvals []g.Map        `json:"approvals"`
			Totals    map[string]int `json:"totals"`
			Limit     int            `json:"limit"`
		} `json:"data"`
	}
	if err = json.Unmarshal(body, &result); err != nil || result.Code != 0 {
		t.Fatal("invalid response", err)
	}
	for category, want := range map[string][]string{
		"contacts":  {"测试员工甲", "7319", "CONTACT-EXCEPTION"},
		"checkins":  {"徐小会", "3299", "CHECKIN-EXCEPTION"},
		"approvals": {"王芳", "3240", "APPROVAL-EXCEPTION"},
	} {
		rows := map[string][]g.Map{"contacts": result.Data.Contacts, "checkins": result.Data.Checkins, "approvals": result.Data.Approvals}[category]
		if len(rows) != 1 || rows[0]["employee_name"] != want[0] || rows[0]["employee_no"] != want[1] || rows[0]["processing_error"] != want[2] {
			t.Fatalf("%s identity or original error changed: %+v", category, rows)
		}
		if result.Data.Totals[category] != 1 || result.Data.Limit != 100 {
			t.Fatal("incorrect exception totals", result.Data.Totals, result.Data.Limit)
		}
	}
	for _, table := range []string{"wecom_user", "wecom_checkin_raw", "wecom_approval_raw"} {
		n, err := core.Model(table).Where("processing_error != ?", "").Count()
		if err != nil || n != 1 {
			t.Fatal("read endpoint changed exception data", table, n, err)
		}
	}
}

func TestExceptionPriorityKeepsConfiguredFailuresVisibleAboveUnmappedBacklog(t *testing.T) {
	ctx := setupDB(t)
	if _, e := core.Model("wecom_approval_template").Data(g.Map{"id": "configured", "template_id": "configured", "approval_type": "leave", "enabled": 1, "mapping_json": "{}", "created_at": now(), "updated_at": now()}).Insert(); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 115; i++ {
		id, template, problem, stamp := fmt.Sprintf("unmapped-%03d", i), "unmapped", "approval template is not enabled or mapped", "2026-10-29 12:00:00"
		if i == 113 {
			id, template, problem, stamp = "known-other", "configured", "unknown upstream failure", "2026-08-01 08:00:00"
		}
		if i == 114 {
			id, template, problem, stamp = "known-normalization", "configured", "normalized approval failed HRM validation or persistence", "2026-08-01 07:00:00"
		}
		if _, e := core.Model("wecom_approval_raw").Data(g.Map{"id": id, "sp_no": id, "template_id": template, "raw_json": "PRIVATE-RAW", "detail_json": "PRIVATE-DETAIL", "processing_error": problem, "synced_at": stamp}).Insert(); e != nil {
			t.Fatal(e)
		}
	}
	for pass := 0; pass < 2; pass++ {
		result, e := exceptionRecords(ctx)
		if e != nil {
			t.Fatal(e)
		}
		rows := result["approvals"].([]g.Map)
		if len(rows) != 100 || rows[0]["id"] != "known-normalization" || rows[1]["id"] != "known-other" || rows[2]["id"] != "unmapped-000" {
			t.Fatal("important failures hidden or ordering unstable", rows[:3])
		}
		if result["totals"].(g.Map)["approvals"] != 115 || result["approval_counts"].(g.Map)["configured"] != 2 || result["approval_counts"].(g.Map)["unmapped"] != 113 {
			t.Fatal("wrong full counts", result["totals"], result["approval_counts"])
		}
		for _, row := range rows {
			if _, exists := row["raw_json"]; exists {
				t.Fatal("raw data leaked")
			}
			if _, exists := row["detail_json"]; exists {
				t.Fatal("detail data leaked")
			}
		}
	}
	n, e := core.Model("wecom_approval_raw").Where("processing_error != ?", "").Count()
	if e != nil || n != 115 {
		t.Fatal("exception read changed original evidence", n, e)
	}
}
