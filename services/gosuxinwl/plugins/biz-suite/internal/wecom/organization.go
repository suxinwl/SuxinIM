package wecom

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/framework/util/guid"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/organization"
	"github.com/suxinwl/GoSuxin/utility/gf"
)

func bindOrganization(api *ghttp.RouterGroup, c *Client) {
	prune := func(r *ghttp.Request, preview bool) {
		if !auth(r) {
			return
		}
		if !core.Has(r.Context(), "biz-suite.wecom.manage") {
			r.Response.WriteStatus(http.StatusForbidden)
			return
		}
		if !c.claim() {
			fail(r, ErrSyncRunning)
			return
		}
		defer c.release()
		exclusions, err := core.ExplicitExclusionsForRules(r.Context(), c.Settings().exclusionRules())
		if err != nil {
			fail(r, err)
			return
		}
		result, err := organization.Prune(r.Context(), exclusions, preview)
		if err != nil {
			fail(r, err)
			return
		}
		r.Response.Header().Set("Cache-Control", "no-store")
		r.Response.WriteJson(gf.Success().SetData(result))
	}
	api.GET("/organization/prune", func(r *ghttp.Request) { prune(r, true) })
	api.POST("/organization/prune", func(r *ghttp.Request) { prune(r, false) })
	api.GET("/organization/sync", func(r *ghttp.Request) {
		if !auth(r) {
			return
		}
		result, err := organization.LastReport(r.Context())
		if err != nil {
			fail(r, err)
			return
		}
		r.Response.WriteJson(gf.Success().SetData(result))
	})
	api.POST("/organization/sync", func(r *ghttp.Request) {
		if !auth(r) {
			return
		}
		if !c.claim() {
			fail(r, ErrSyncRunning)
			return
		}
		defer c.release()
		result, err := c.syncOrganization(r.Context())
		if err != nil {
			fail(r, err)
			return
		}
		r.Response.WriteJson(gf.Success().SetData(result))
	})
}
func (c *Client) syncOrganization(ctx context.Context) (organization.Report, error) {
	f, err := c.filter(ctx)
	if err != nil {
		return organization.Report{}, err
	}
	in := organization.Input{AllowedDepartments: map[int64]bool{}, AllowedUsers: map[string]bool{}}
	for id := range f.parents {
		in.AllowedDepartments[id] = f.departmentAllowed(id)
	}
	rows, err := core.Model("wecom_user").Ctx(ctx).Fields("wecom_userid,department_ids").All()
	if err != nil {
		return organization.Report{}, err
	}
	for _, row := range rows {
		var depts []int64
		if json.Unmarshal([]byte(row["department_ids"].String()), &depts) != nil {
			continue
		}
		uid := row["wecom_userid"].String()
		in.AllowedUsers[uid] = f.allows(uid, depts)
	}
	id := guid.S()
	if _, err = core.Model("wecom_sync_run").Ctx(ctx).Data(g.Map{"id": id, "sync_type": "organization", "status": "running", "started_at": now(), "error_message": ""}).Insert(); err != nil {
		return organization.Report{}, dbError("organization run log")
	}
	report, syncErr := organization.Sync(ctx, in)
	status, message, failed := "success", "", report.ConflictCount
	if failed > 0 {
		status, message = "partial", "部分组织或账号未匹配，请查看组织与账号同步冲突列表"
	}
	if syncErr != nil {
		status, message, failed = "failed", "组织与账号同步失败，数据库变更已回滚", 1
	}
	finish, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if _, err = core.Model("wecom_sync_run").Ctx(finish).Where("id", id).Update(g.Map{"status": status, "finished_at": now(), "success_count": report.EmployeesLinked, "failure_count": failed, "error_message": message}); err != nil && syncErr == nil {
		syncErr = dbError("organization completion log")
	}
	return report, syncErr
}

func departmentMappings(ctx context.Context) ([]g.Map, error) {
	exclusions, err := core.LoadExplicitExclusions(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := core.Model("wecom_department").Ctx(ctx).FieldsEx("raw_json").OrderAsc("wecom_id").All()
	if err != nil {
		return nil, err
	}
	hosts, err := core.HostModel("auth_dept").Ctx(ctx).Fields("id,name,pid,status").WhereNull("deletetime").All()
	if err != nil {
		return nil, err
	}
	wc, local := map[int64]gdb.Record{}, map[int64]gdb.Record{}
	for _, row := range rows {
		wc[row["wecom_id"].Int64()] = row
	}
	for _, row := range hosts {
		local[row["id"].Int64()] = row
	}
	path := func(index map[int64]gdb.Record, id int64, parent string) string {
		parts := []string{}
		seen := map[int64]bool{}
		for id > 0 && !seen[id] {
			seen[id] = true
			row := index[id]
			if len(row) == 0 {
				break
			}
			parts = append([]string{row["name"].String()}, parts...)
			id = row[parent].Int64()
		}
		return strings.Join(parts, " / ")
	}
	out := []g.Map{}
	for _, row := range rows {
		if exclusions.Departments[row["wecom_id"].Int64()] {
			continue
		}
		entry := row.Map()
		id, hostID := row["wecom_id"].Int64(), row["host_dept_id"].Int64()
		entry["path"] = path(wc, id, "parent_id")
		entry["parent_name"] = wc[row["parent_id"].Int64()]["name"].String()
		entry["host_department_name"] = local[hostID]["name"].String()
		entry["host_department_path"] = path(local, hostID, "pid")
		entry["mapping_status"], entry["mapping_message"] = "unmapped", "等待同步组织架构"
		if hostID > 0 {
			entry["mapping_status"], entry["mapping_message"] = "mapped", "已关联本地部门"
			if len(local[hostID]) == 0 || local[hostID]["status"].Int() != 0 {
				entry["mapping_status"], entry["mapping_message"] = "conflict", "本地部门已停用或删除"
			}
		}
		out = append(out, entry)
	}
	return out, nil
}
