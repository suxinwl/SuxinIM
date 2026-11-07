package hrm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/framework/util/gconv"
	"github.com/suxinwl/GoSuxin/framework/util/guid"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
)

func scopeFixtures(t *testing.T) context.Context {
	t.Helper()
	ctx := setup(t)
	for _, statement := range []string{
		`INSERT INTO gf_auth_dept(id,name,pid,status,deletetime) VALUES(11,'Child',10,0,NULL),(12,'Grandchild',11,0,NULL),(20,'Other',0,0,NULL),(30,'Disabled',10,1,NULL),(31,'Under disabled',30,0,NULL),(40,'Deleted',10,0,'2026-01-01'),(41,'Under deleted',40,0,NULL)`,
		`INSERT INTO gf_admin(id,name,username,nickname,dept_id,status) VALUES(2,'Self','0002','Self',11,0),(3,'Manager','0003','Manager',10,0),(4,'Stale','0004','Stale',20,0),(5,'Unbound','0005','Unbound',10,0),(6,'Pending','0006','Pending',11,1),(7,'Writer','0007','Writer',0,0),(8,'Blocked role','0008','Blocked role',10,0)`,
		`INSERT INTO gf_auth_rule(id,permission) VALUES(4,'biz-suite.hrm.self'),(5,'biz-suite.hrm.department')`,
		`INSERT INTO gf_auth_role(id,rules,btns,status) VALUES(2,'','4',0),(3,'','5',0),(4,'','2',0),(5,'','5',1)`,
		`INSERT INTO gf_auth_role_access VALUES(2,2),(3,3),(4,3),(5,3),(6,2),(7,4),(8,5)`,
	} {
		if _, err := core.DB().Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	for _, employee := range []Employee{
		{ID: "self", UserID: 2, DeptID: 11, Name: "Self", EmployeeNo: "0002"},
		{ID: "manager", UserID: 3, DeptID: 10, Name: "Manager", EmployeeNo: "0003"},
		{ID: "stale", UserID: 4, DeptID: 10, Name: "Stale", EmployeeNo: "0004"},
		{ID: "child", DeptID: 12, Name: "Grandchild employee", EmployeeNo: "0012"},
		{ID: "other", DeptID: 20, Name: "Other branch", EmployeeNo: "0020"},
		{ID: "disabled-dept", DeptID: 30, Name: "Disabled department"},
		{ID: "under-disabled", DeptID: 31, Name: "Under disabled department"},
		{ID: "under-deleted", DeptID: 41, Name: "Under deleted department"},
		{ID: "pending", UserID: 6, DeptID: 11, Name: "Pending", EmployeeNo: "0006"},
	} {
		if _, err := UpsertEmployee(ctx, employee); err != nil {
			t.Fatal(err)
		}
		if _, err := UpsertClock(ctx, Clock{ID: "clock-" + employee.ID, EmployeeID: employee.ID, ClockType: "in", ClockTime: "2026-01-05 09:00:00"}); err != nil {
			t.Fatal(err)
		}
		if _, err := UpsertLeave(ctx, Leave{ID: "leave-" + employee.ID, EmployeeID: employee.ID, StartTime: "2026-01-05 09:00:00", EndTime: "2026-01-05 10:00:00", Duration: 1, Reason: "Only visible to permitted readers", LeaveType: "事假"}); err != nil {
			t.Fatal(err)
		}
		for _, save := range []func(context.Context, Out) (string, error){UpsertOut, UpsertTrip} {
			if _, err := save(ctx, Out{ID: "absence-" + employee.ID, EmployeeID: employee.ID, StartTime: "2026-01-05 09:00:00", EndTime: "2026-01-05 10:00:00", Duration: 1}); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, row := range []g.Map{
		{"id": "mixed", "name": "Mixed", "employee_ids": `["self","other"]`, "dept_ids": `[11,20]`},
		{"id": "outside", "name": "Other only", "employee_ids": `["other"]`, "dept_ids": `[20]`},
	} {
		row["shifts"], row["rule_json"], row["status"], row["updated_at"], row["created_at"] = `[{"start":"09:00","end":"18:00","weekdays":[1,2,3,4,5]}]`, `{}`, 1, now(), now()
		if _, err := core.Model("biz_hrm_attendance_group").Data(row).Insert(); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := core.Model("biz_hrm_attendance_holiday").Data(g.Map{"id": "holiday", "holiday_date": "2026-01-01", "name": "公共假日", "is_workday": 0, "remark": "", "created_at": now(), "updated_at": now()}).Insert(); err != nil {
		t.Fatal(err)
	}
	return ctx
}

func TestHRMScopeUsesBoundIdentityAndActivePrimaryDepartmentTree(t *testing.T) {
	ctx := scopeFixtures(t)
	for _, test := range []struct {
		uid  int64
		all  bool
		want string
	}{
		{1, true, ""}, {2, false, "self"}, {3, false, "child,manager,pending,self,stale"},
		{4, false, "stale"}, {5, false, ""}, {7, true, ""},
	} {
		scope, err := loadHRMScope(context.WithValue(ctx, "uid", test.uid))
		if err != nil || scope.All != test.all || strings.Join(scope.EmployeeIDs, ",") != test.want {
			t.Fatalf("uid %d scope=%+v err=%v", test.uid, scope, err)
		}
	}
	for _, uid := range []int64{0, 6, 8, 99} {
		if _, err := loadHRMScope(context.WithValue(ctx, "uid", uid)); err == nil {
			t.Fatalf("unavailable identity/permission %d accepted", uid)
		}
	}
	// A soft-deleted primary department removes department expansion immediately.
	if _, err := core.HostModel("auth_dept").Where("id", 10).Update(g.Map{"deletetime": "2026-01-01"}); err != nil {
		t.Fatal(err)
	}
	scope, err := loadHRMScope(context.WithValue(ctx, "uid", int64(3)))
	if err != nil || strings.Join(scope.EmployeeIDs, ",") != "manager" {
		t.Fatalf("deleted primary department expanded: %+v %v", scope, err)
	}
}

func TestHRMScopedHTTPReadsCannotEscapeThroughFiltersCountsOrDetails(t *testing.T) {
	ctx := scopeFixtures(t)
	server := g.Server(guid.S())
	server.SetAddr("127.0.0.1:0")
	server.SetDumpRouterMap(false)
	server.Group("/", func(group *ghttp.RouterGroup) {
		group.Middleware(func(r *ghttp.Request) {
			r.SetCtx(context.WithValue(r.Context(), "uid", gconv.Int64(r.Header.Get("X-Test-UID"))))
			r.Middleware.Next()
		}, core.Middleware)
		Bind(ctx, group)
	})
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Shutdown() })
	call := func(uid int, method, path string) (http.Header, map[string]any, int) {
		t.Helper()
		req, _ := http.NewRequest(method, fmt.Sprintf("http://127.0.0.1:%d/api/admin/hrm/%s", server.GetListenedPort(), path), nil)
		req.Header.Set("X-Test-UID", fmt.Sprint(uid))
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		raw, _ := io.ReadAll(response.Body)
		body := map[string]any{}
		if response.StatusCode == 200 {
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Fatalf("%s: %v", path, err)
			}
		}
		return response.Header, body, response.StatusCode
	}
	ids := func(value any, field string) string {
		t.Helper()
		result := []string{}
		for _, item := range value.([]any) {
			result = append(result, item.(map[string]any)[field].(string))
		}
		sort.Strings(result)
		return strings.Join(result, ",")
	}
	for _, user := range []struct{ uid, count int }{{1, 9}, {2, 1}, {3, 5}, {4, 1}, {5, 0}, {7, 9}} {
		for _, path := range []string{"employees", "attendance/clocks", "attendance/leaves", "attendance/outs", "attendance/trips"} {
			headers, body, code := call(user.uid, "GET", path+"?page_size=1")
			if code != 200 || body["code"] != float64(0) || headers.Get("X-Total-Count") != fmt.Sprint(user.count) {
				t.Fatalf("uid %d %s escaped count or failed: status=%d total=%s body=%v", user.uid, path, code, headers.Get("X-Total-Count"), body)
			}
			wantRows := 1
			if user.count == 0 {
				wantRows = 0
			}
			if len(body["data"].([]any)) != wantRows {
				t.Fatalf("uid %d wrong page size", user.uid)
			}
		}
		_, body, _ := call(user.uid, "GET", "summary")
		for _, key := range []string{"employees", "clocks", "leaves", "outs", "trips"} {
			if body["data"].(map[string]any)[key] != float64(user.count) {
				t.Fatalf("uid %d leaked summary %s: %v", user.uid, key, body)
			}
		}
		_, body, _ = call(user.uid, "GET", "attendance/month?month=2026-01")
		if body["code"] != float64(0) || len(body["data"].(map[string]any)["items"].([]any)) != user.count {
			t.Fatalf("uid %d wrong monthly scope: %v", user.uid, body)
		}
		_, body, _ = call(user.uid, "GET", "attendance/clocks/options")
		if len(body["data"].(map[string]any)["employees"].([]any)) != user.count {
			t.Fatalf("uid %d leaked employee options", user.uid)
		}
	}
	for _, path := range []string{"attendance/clocks", "attendance/leaves", "attendance/outs", "attendance/trips"} {
		headers, body, _ := call(2, "GET", path+"?employee_id=other")
		if headers.Get("X-Total-Count") != "0" || len(body["data"].([]any)) != 0 {
			t.Fatalf("explicit foreign employee escaped scope: %s", path)
		}
		if _, _, code := call(2, "POST", path); code != 403 {
			t.Fatalf("scoped role was allowed to modify %s", path)
		}
	}
	_, body, _ := call(2, "GET", "attendance/month?month=2026-01&employee_id=other")
	if len(body["data"].(map[string]any)["items"].([]any)) != 0 {
		t.Fatal("foreign monthly employee escaped scope")
	}
	for _, path := range []string{"attendance/leaves/leave-other", "attendance/outs/absence-other", "attendance/trips/absence-other"} {
		_, body, _ = call(2, "GET", path)
		if body["code"] == float64(0) || (body["data"] != nil && body["data"] != false) {
			t.Fatalf("foreign detail escaped scope: %v", body)
		}
	}
	_, body, _ = call(2, "GET", "attendance/leaves/leave-self")
	if body["code"] != float64(0) || body["data"].(map[string]any)["employee_id"] != "self" {
		t.Fatalf("own detail inaccessible: %v", body)
	}
	for _, path := range []string{"employees", "attendance/groups", "attendance/holidays"} {
		if _, _, code := call(3, "POST", path); code != 403 {
			t.Fatalf("department permission was allowed to modify %s", path)
		}
	}
	_, body, _ = call(2, "GET", "options")
	options := body["data"].(map[string]any)
	if ids(options["employees"], "id") != "self" || len(options["users"].([]any)) != 1 || options["users"].([]any)[0].(map[string]any)["id"] != float64(2) {
		t.Fatalf("scoped identity options leaked: %v", options)
	}
	for _, department := range options["departments"].([]any) {
		id := department.(map[string]any)["id"].(float64)
		if id != 10 && id != 11 {
			t.Fatal("unrelated department option leaked")
		}
	}
	_, body, _ = call(1, "GET", "options")
	users := body["data"].(map[string]any)["users"].([]any)
	foundDisabled := false
	for _, user := range users {
		item := user.(map[string]any)
		if item["id"] == float64(6) && item["status"] == float64(1) {
			foundDisabled = true
		}
	}
	if !foundDisabled {
		t.Fatal("disabled bound account absent from mapping editor")
	}
	_, body, _ = call(2, "GET", "employees")
	item := body["data"].([]any)[0].(map[string]any)
	if item["host_username"] != "0002" || item["host_display_name"] != "Self" || item["host_department_name"] != "Child" || item["host_department_path"] != "Dev / Child" || item["mapping_status"] != "mapped" {
		t.Fatalf("host mapping presentation wrong: %v", item)
	}
	_, body, _ = call(1, "GET", "employees?search=0006")
	if body["data"].([]any)[0].(map[string]any)["mapping_status"] != "disabled" {
		t.Fatal("disabled bound account mapping status wrong")
	}
	_, body, _ = call(4, "GET", "employees")
	if body["data"].([]any)[0].(map[string]any)["mapping_status"] != "conflict" {
		t.Fatal("stale primary department not surfaced")
	}
	headers, body, _ := call(2, "GET", "attendance/groups")
	groups := body["data"].([]any)
	if headers.Get("X-Total-Count") != "1" || len(groups) != 1 || groups[0].(map[string]any)["employee_ids"] != `["self"]` || groups[0].(map[string]any)["dept_ids"] != `[11]` {
		t.Fatalf("group scope leaked memberships: %v", body)
	}
	for _, path := range []string{"options", "attendance/groups", "attendance/holidays"} {
		_, body, _ = call(5, "GET", path)
		if path == "options" {
			for _, field := range []string{"users", "employees", "departments"} {
				if len(body["data"].(map[string]any)[field].([]any)) != 0 {
					t.Fatalf("unbound account saw %s", field)
				}
			}
		} else if len(body["data"].([]any)) != 0 {
			t.Fatalf("unbound account saw %s", path)
		}
	}
	for _, uid := range []int{0, 6, 8} {
		_, _, code := call(uid, "GET", "employees")
		if code != 401 && code != 403 {
			t.Fatalf("inactive identity/role accepted: uid%d status%d", uid, code)
		}
	}
}

func TestEmployeeMappingAcceptsDisabledButRejectsDeletedHostAccount(t *testing.T) {
	ctx := scopeFixtures(t)
	// Contact updates must preserve the new disabled account, not fail until it
	// is activated. Soft-deleted accounts remain invalid mapping targets.
	if _, err := UpsertEmployee(ctx, Employee{ID: "pending", Name: "Pending updated", UserID: 6, DeptID: 11}); err != nil {
		t.Fatal("disabled mapped account rejected", err)
	}
	if _, err := core.HostModel("admin").Where("id", 6).Update(g.Map{"deletetime": "2026-01-01"}); err != nil {
		t.Fatal(err)
	}
	if _, err := UpsertEmployee(ctx, Employee{ID: "pending", Name: "Pending updated", UserID: 6, DeptID: 11}); err == nil {
		t.Fatal("deleted mapped account accepted")
	}
}
