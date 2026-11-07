package wecom

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
	"github.com/suxinwl/GoSuxin/utility/gf"
)

func Bind(ctx context.Context, group *ghttp.RouterGroup, client *Client) {
	group.Group("/api/admin/wecom", func(api *ghttp.RouterGroup) {
		bindOrganization(api, client)
		bindCheckinSnapshots(api, client)
		api.GET("/settings", func(r *ghttp.Request) {
			if !auth(r) {
				return
			}
			r.Response.WriteJson(gf.Success().SetData(client.Settings()))
		})
		api.POST("/settings", func(r *ghttp.Request) {
			if !auth(r) {
				return
			}
			var in struct {
				Settings
				CredentialUpdate
			}
			if r.Parse(&in) != nil {
				fail(r, errors.New("配置格式无效"))
				return
			}
			if err := client.SaveConfiguration(r.Context(), in.Settings, in.CredentialUpdate); err != nil {
				fail(r, err)
				return
			}
			r.Response.WriteJson(gf.Success().SetData(client.Settings()))
		})
		api.GET("/filter-options", func(r *ghttp.Request) {
			if !auth(r) {
				return
			}
			deps, err := core.Model("wecom_department").Ctx(r.Context()).Fields("wecom_id,parent_id,name").OrderAsc("wecom_id").All()
			if err != nil {
				fail(r, dbError("filter departments"))
				return
			}
			departmentNames := map[int64]string{}
			for _, d := range deps {
				departmentNames[d["wecom_id"].Int64()] = strings.TrimSpace(d["name"].String())
			}
			users, err := loadFilterUsers(r.Context(), departmentNames)
			if err != nil {
				fail(r, dbError("filter users"))
				return
			}
			r.Response.WriteJson(gf.Success().SetData(g.Map{"departments": deps, "users": users}))
		})
		api.GET("/departments/options", func(r *ghttp.Request) {
			if !auth(r) {
				return
			}
			rows, err := core.HostModel("auth_dept").Ctx(r.Context()).Fields("id,name,pid").OrderAsc("id").All()
			if err != nil {
				fail(r, dbError("host departments"))
				return
			}
			r.Response.WriteJson(gf.Success().SetData(g.Map{"departments": rows}))
		})
		api.GET("/status", func(r *ghttp.Request) {
			if !auth(r) {
				return
			}
			r.Response.WriteJson(gf.Success().SetData(client.Status()))
		})
		for _, kind := range []string{"contact", "app", "checkin", "approval"} {
			k := kind
			api.POST("/diagnostics/"+k, func(r *ghttp.Request) {
				if !auth(r) {
					return
				}
				ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
				defer cancel()
				result, err := client.Diagnose(ctx, k)
				if err != nil {
					fail(r, err)
					return
				}
				r.Response.WriteJson(gf.Success().SetData(result))
			})
		}
		for _, kind := range []string{"contact", "checkin", "approval", "leave", "out", "trip", "overtime"} {
			k := kind
			api.POST("/sync/"+k, func(r *ghttp.Request) {
				if !auth(r) {
					return
				}
				var in struct {
					Start string `json:"start_time"`
					End   string `json:"end_time"`
				}
				if r.Parse(&in) != nil {
					fail(r, errors.New("invalid sync request"))
					return
				}
				var start, end time.Time
				var err error
				if in.Start != "" {
					start, err = core.ParseTime(in.Start)
					if err != nil {
						fail(r, errors.New("invalid start_time"))
						return
					}
				}
				if in.End != "" {
					end, err = core.ParseTime(in.End)
					if err != nil {
						fail(r, errors.New("invalid end_time"))
						return
					}
				}
				// The plugin lifetime context, not the HTTP request context, owns the job.
				id, err := client.Start(ctx, k, start, end)
				if err != nil {
					fail(r, err)
					return
				}
				r.Response.WriteJson(gf.Success().SetData(g.Map{"run_id": id, "status": "running"}))
			})
		}
		api.GET("/sync/runs", func(r *ghttp.Request) {
			if !auth(r) {
				return
			}
			rows, err := core.Model("wecom_sync_run").Ctx(r.Context()).OrderDesc("started_at").Limit(50).All()
			if err != nil {
				fail(r, dbError("run list"))
				return
			}
			r.Response.WriteJson(gf.Success().SetData(rows))
		})
		api.GET("/templates", func(r *ghttp.Request) {
			if !auth(r) {
				return
			}
			rows, err := core.Model("wecom_approval_template").Ctx(r.Context()).OrderDesc("updated_at").All()
			if err != nil {
				fail(r, dbError("template list"))
				return
			}
			r.Response.WriteJson(gf.Success().SetData(rows))
		})
		api.POST("/templates", func(r *ghttp.Request) {
			if !auth(r) {
				return
			}
			var in Template
			if r.Parse(&in) != nil {
				fail(r, errors.New("invalid approval template"))
				return
			}
			if err := SaveTemplate(r.Context(), in); err != nil {
				fail(r, err)
				return
			}
			r.Response.WriteJson(gf.Success().SetData(true))
		})
		api.GET("/exceptions", func(r *ghttp.Request) {
			if !auth(r) {
				return
			}
			result, err := exceptionRecords(r.Context())
			if err != nil {
				fail(r, err)
				return
			}
			r.Response.WriteJson(gf.Success().SetData(result))
		})
		api.GET("/departments/mapping", func(r *ghttp.Request) {
			if !auth(r) {
				return
			}
			rows, err := departmentMappings(r.Context())
			if err != nil {
				fail(r, dbError("department list"))
				return
			}
			r.Response.WriteJson(gf.Success().SetData(rows))
		})
		api.POST("/departments/mapping", func(r *ghttp.Request) {
			if !auth(r) {
				return
			}
			if !client.claim() {
				fail(r, ErrSyncRunning)
				return
			}
			defer client.release()
			var in struct {
				WeComID int64 `json:"wecom_id"`
				HostID  int64 `json:"host_dept_id"`
			}
			if r.Parse(&in) != nil || in.WeComID <= 0 || in.HostID < 0 {
				fail(r, errors.New("invalid department mapping"))
				return
			}
			if in.HostID > 0 {
				ok, err := core.HostModel("auth_dept").Ctx(r.Context()).Where("id", in.HostID).Exist()
				if err != nil || !ok {
					fail(r, errors.New("host department does not exist"))
					return
				}
				used, err := core.Model("wecom_department").Ctx(r.Context()).Where("host_dept_id", in.HostID).WhereNot("wecom_id", in.WeComID).Exist()
				if err != nil || used {
					fail(r, errors.New("该本地部门已绑定其他企业微信部门，请先核对组织映射"))
					return
				}
			}
			ok, err := core.Model("wecom_department").Ctx(r.Context()).Where("wecom_id", in.WeComID).Exist()
			if err != nil || !ok {
				fail(r, errors.New("synchronize this enterprise WeChat department first"))
				return
			}
			if _, err = core.Model("wecom_department").Ctx(r.Context()).Where("wecom_id", in.WeComID).Update(g.Map{"host_dept_id": in.HostID}); err != nil {
				fail(r, dbError("department binding"))
				return
			}
			r.Response.WriteJson(gf.Success().SetData(true))
		})
	})
}
func SaveTemplate(ctx context.Context, in Template) error {
	in.TemplateID = strings.TrimSpace(in.TemplateID)
	if in.TemplateID == "" || len(in.TemplateID) > 128 {
		return errors.New("template_id is required and must not exceed 128 bytes")
	}
	if !validApprovalKind(in.ApprovalType) {
		return errors.New("approval_type must be leave, out, trip or overtime")
	}
	if in.Enabled != 0 && in.Enabled != 1 {
		return errors.New("enabled must be 0 or 1")
	}
	if in.Mapping.AttendanceControlID == "" {
		return errors.New("attendance_control_id is required; obtain the exact control ID from the WeCom template")
	}
	return upsert(ctx, "wecom_approval_template", "template_id", in.TemplateID, g.Map{"id": stableID("template:" + in.TemplateID), "template_id": in.TemplateID, "approval_type": in.ApprovalType, "enabled": in.Enabled, "mapping_json": mustJSON(in.Mapping), "created_at": now(), "updated_at": now()})
}
func auth(r *ghttp.Request) bool {
	if core.UID(r.Context()) <= 0 {
		r.Response.WriteStatus(http.StatusUnauthorized)
		return false
	}
	return true
}
func fail(r *ghttp.Request, err error) { r.Response.WriteJson(gf.Failed().SetMsg(err.Error())) }
