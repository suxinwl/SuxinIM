package wecom

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// DiagnosticResult describes only the listed read-only probes. A successful
// token or one API call does not establish permissions for other endpoints.
type DiagnosticResult struct {
	Kind                 string            `json:"kind"`
	Status               string            `json:"status"`
	CredentialConfigured bool              `json:"credential_configured"`
	CheckedAt            string            `json:"checked_at"`
	Checks               []DiagnosticCheck `json:"checks"`
}

type DiagnosticCheck struct {
	Name     string         `json:"name"`
	Endpoint string         `json:"endpoint"`
	Status   string         `json:"status"`
	Code     int            `json:"code,omitempty"`
	Message  string         `json:"message"`
	Details  map[string]any `json:"details,omitempty"`
}

// Diagnose does not persist credentials or business records. It uses the same
// operation lock as synchronization so settings cannot change during a probe.
// Upstream failures become sanitized check results; only an invalid kind or
// another active operation is returned as an error to the caller.
func (c *Client) Diagnose(ctx context.Context, kind string) (DiagnosticResult, error) {
	result := DiagnosticResult{Kind: kind, Status: "missing", CheckedAt: time.Now().UTC().Format(time.RFC3339), Checks: []DiagnosticCheck{}}
	switch kind {
	case "contact", "app", "checkin", "approval":
	default:
		return result, errors.New("诊断类型仅支持 contact、app、checkin、approval")
	}
	if c == nil {
		result.Checks = append(result.Checks, DiagnosticCheck{Name: "credentials", Status: "skipped", Message: "企业微信尚未配置"})
		return result, nil
	}
	if !c.claim() {
		return result, ErrSyncRunning
	}
	defer c.release()
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	result.CredentialConfigured = c.CorpID != "" && c.secret(kind) != ""
	if !result.CredentialConfigured {
		message := "请先配置企业 ID 和自建应用 Secret"
		if kind == "contact" {
			message = "请先配置企业 ID 和通讯录读取凭据；可在通讯录读取槽填写具备相应权限的企业自建应用 Secret。本检查不会自动使用另一凭据槽的 Secret"
		}
		result.Checks = append(result.Checks, DiagnosticCheck{Name: "credentials", Status: "skipped", Message: message})
		return result, nil
	}
	if kind == "app" && c.AgentID == "" {
		result.Checks = append(result.Checks, DiagnosticCheck{Name: "application", Endpoint: "/agent/get", Status: "skipped", Message: "请先配置自建应用 AgentID；打卡和审批接口不以 AgentID 作为取 Token 的参数"})
		return result, nil
	}
	if _, err := c.accessToken(ctx, kind); err != nil {
		result.Status = "failed"
		result.Checks = append(result.Checks, diagnosticFailure("token", "/gettoken", err, "请核对企业 ID、所选凭据、应用启用状态及可信 IP；Token 成功也不代表已取得业务接口权限"))
		return result, nil
	}
	result.Checks = append(result.Checks, DiagnosticCheck{Name: "token", Endpoint: "/gettoken", Status: "success", Message: "已取得所选凭据的 Token；Token 仅保存在服务器，业务接口权限仍需分别检查"})
	result.Status = "success"
	switch kind {
	case "contact":
		c.diagnoseContact(ctx, &result)
	case "app":
		c.diagnoseApp(ctx, &result)
	case "checkin":
		var response struct {
			apiError
			Groups []jsonRecord `json:"group"`
		}
		const endpoint = "/checkin/getcorpcheckinoption"
		if err := c.api(ctx, "checkin", http.MethodPost, endpoint, nil, struct{}{}, &response); err != nil {
			result.Status = "failed"
			result.Checks = append(result.Checks, diagnosticFailure("checkin_rules", endpoint, err, "请将此自建应用加入「打卡－可调用接口的应用」并核对可见范围；新企业不应使用系统打卡应用 Secret；本检查仅验证企业规则读取接口"))
		} else {
			result.Checks = append(result.Checks, DiagnosticCheck{Name: "checkin_rules", Endpoint: endpoint, Status: "success", Message: "企业打卡规则读取成功；未执行同步，未测试日报、月报、设备或其他接口", Details: map[string]any{"rule_count": len(response.Groups)}})
		}
	case "approval":
		var response struct {
			apiError
			Numbers []string `json:"sp_no_list"`
		}
		end := time.Now()
		body := map[string]any{"starttime": strconv.FormatInt(end.Add(-24*time.Hour).Unix(), 10), "endtime": strconv.FormatInt(end.Unix(), 10), "new_cursor": "", "size": 1}
		const endpoint = "/oa/getapprovalinfo"
		if err := c.api(ctx, "approval", http.MethodPost, endpoint, nil, body, &response); err != nil {
			result.Status = "failed"
			result.Checks = append(result.Checks, diagnosticFailure("approval_list", endpoint, err, "请在审批应用的接口权限中授权此自建应用，并核对可见范围和模板权限；本检查仅查询最近一天的一条审批单号，不读取详情、不写入业务库"))
		} else {
			result.Checks = append(result.Checks, DiagnosticCheck{Name: "approval_list", Endpoint: endpoint, Status: "success", Message: "审批单号查询成功；零条记录也属于正常响应，详情和模板权限尚未测试", Details: map[string]any{"sample_count": len(response.Numbers)}})
		}
	}
	return result, nil
}

func (c *Client) diagnoseContact(ctx context.Context, result *DiagnosticResult) {
	const guidance = "本检查仅使用通讯录读取槽的 Secret，不会自动切换到另一套凭据。若使用「管理工具－通讯录同步」Secret，其资料查询接口可能已停用或仅允许读取 ID；需要姓名、部门名时，请在通讯录读取槽手动配置具备相应授权的企业自建应用 Secret，并核对可见范围与可信 IP。可读字段以检查结果和实际授权为准；手机号、邮箱等敏感信息还受成员授权限制"
	var ids struct {
		apiError
		Departments []struct {
			ID int64 `json:"id"`
		} `json:"department_id"`
	}
	var detailed struct {
		apiError
		Departments []Department `json:"department"`
	}
	var sampleDepartment int64
	readable := 0
	const idsEndpoint = "/department/simplelist"
	if err := c.api(ctx, "contact", http.MethodGet, idsEndpoint, nil, nil, &ids); err != nil {
		result.Checks = append(result.Checks, diagnosticFailure("department_ids", idsEndpoint, err, guidance))
		result.Status = "limited"
	} else {
		readable++
		result.Checks = append(result.Checks, DiagnosticCheck{Name: "department_ids", Endpoint: idsEndpoint, Status: "success", Message: "部门 ID 列表读取成功；此结果不代表部门名称或成员资料可读", Details: map[string]any{"department_count": len(ids.Departments)}})
		for _, department := range ids.Departments {
			if department.ID > 0 {
				sampleDepartment = department.ID
				break
			}
		}
	}
	const departmentsEndpoint = "/department/list"
	if err := c.api(ctx, "contact", http.MethodGet, departmentsEndpoint, nil, nil, &detailed); err != nil {
		result.Checks = append(result.Checks, diagnosticFailure("department_details", departmentsEndpoint, err, guidance))
		result.Status = "limited"
	} else {
		readable++
		hasDepartmentName := false
		for _, department := range detailed.Departments {
			hasDepartmentName = hasDepartmentName || strings.TrimSpace(department.Name) != ""
			if department.ID > 0 && sampleDepartment == 0 {
				sampleDepartment = department.ID
			}
		}
		message := "部门详情读取成功；只返回数量和名称可见性，不修改本地部门"
		if !hasDepartmentName {
			result.Status = "limited"
			message = "部门详情接口响应成功，但没有取得部门名称，不能确认完整部门架构可读；请核对可见范围和通讯录权限"
		}
		result.Checks = append(result.Checks, DiagnosticCheck{Name: "department_details", Endpoint: departmentsEndpoint, Status: "success", Message: message, Details: map[string]any{"department_count": len(detailed.Departments), "department_name_available": hasDepartmentName}})
	}
	const usersEndpoint = "/user/list"
	if sampleDepartment == 0 {
		if readable == 0 {
			result.Status = "failed"
		} else {
			result.Status = "limited"
		}
		result.Checks = append(result.Checks, DiagnosticCheck{Name: "member_details", Endpoint: usersEndpoint, Status: "skipped", Message: "未取得可用部门 ID，无法抽样检查成员详情；请核对可见范围，不能据此认定通讯录同步可用"})
		return
	}
	var members struct {
		apiError
		Users []User `json:"userlist"`
	}
	if err := c.api(ctx, "contact", http.MethodGet, usersEndpoint, map[string]string{"department_id": strconv.FormatInt(sampleDepartment, 10)}, nil, &members); err != nil {
		result.Checks = append(result.Checks, diagnosticFailure("member_details", usersEndpoint, err, guidance))
		result.Status = "limited"
		return
	}
	hasName, hasMobile, hasEmail := false, false, false
	for _, user := range members.Users {
		name := strings.TrimSpace(user.Name)
		hasName = hasName || (name != "" && name != user.UserID)
		hasMobile = hasMobile || user.Mobile != ""
		hasEmail = hasEmail || user.Email != ""
	}
	message := "单个部门成员接口读取成功；仅报告字段是否有值，不返回成员信息。手机号和邮箱未返回时可能受敏感信息授权限制，也可能未填写，不能据此推断具体原因"
	if len(members.Users) == 0 {
		result.Status = "limited"
		message = "抽样部门没有返回成员，成员字段可见性尚未得到验证；请核对部门成员与应用可见范围"
	} else if !hasName {
		result.Status = "limited"
		message = "抽样返回成员，但未取得姓名；当前资料不足以确认完整人员同步可用。请核对通讯录接口权限与可见范围"
	}
	result.Checks = append(result.Checks, DiagnosticCheck{Name: "member_details", Endpoint: usersEndpoint, Status: "success", Message: message, Details: map[string]any{"sample_user_count": len(members.Users), "name_available": hasName, "mobile_available": hasMobile, "email_available": hasEmail}})
}

func (c *Client) diagnoseApp(ctx context.Context, result *DiagnosticResult) {
	var response struct {
		apiError
		AgentID int64  `json:"agentid"`
		Name    string `json:"name"`
		Close   int    `json:"close"`
		Users   struct {
			Users []jsonRecord `json:"user"`
		} `json:"allow_userinfos"`
		Parties struct {
			IDs []int64 `json:"partyid"`
		} `json:"allow_partys"`
		Tags struct {
			IDs []int64 `json:"tagid"`
		} `json:"allow_tags"`
	}
	const endpoint = "/agent/get"
	if err := c.api(ctx, "app", http.MethodGet, endpoint, map[string]string{"agentid": c.AgentID}, nil, &response); err != nil {
		result.Status = "failed"
		result.Checks = append(result.Checks, diagnosticFailure("application", endpoint, err, "请核对 AgentID 是否属于该 Secret 对应的已启用自建应用；读取应用成功也不代表打卡或审批已授权"))
		return
	}
	if strconv.FormatInt(response.AgentID, 10) != c.AgentID {
		result.Status = "failed"
		result.Checks = append(result.Checks, DiagnosticCheck{Name: "application", Endpoint: endpoint, Status: "failed", Message: "接口返回的应用 ID 与所配置 AgentID 不一致，请核对应用配置"})
		return
	}
	check := DiagnosticCheck{Name: "application", Endpoint: endpoint, Status: "success", Message: "已读取所配置自建应用及其可见范围；打卡和审批须分别诊断，成员手机号和邮箱授权不在本项检查范围内", Details: map[string]any{"agent_id": response.AgentID, "app_name": response.Name, "app_enabled": response.Close != 1, "visible_user_count": len(response.Users.Users), "visible_department_ids": response.Parties.IDs, "visible_tag_count": len(response.Tags.IDs)}}
	if response.Close == 1 {
		result.Status = "failed"
		check.Status = "failed"
		check.Message = "自建应用已停用，不能视为可用连接；请先在企业微信管理后台启用该应用，再分别检查打卡和审批授权"
	}
	result.Checks = append(result.Checks, check)
}

func diagnosticFailure(name, endpoint string, err error, guidance string) DiagnosticCheck {
	check := DiagnosticCheck{Name: name, Endpoint: endpoint, Status: "failed", Message: "接口检查未完成；请检查网络、可信 IP 和所选凭据。"}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		check.Code = apiErr.Code
		check.Message = fmt.Sprintf("企业微信返回错误码 %d。", apiErr.Code)
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		check.Message = "接口检查已取消或超时。"
	}
	// Do not append err.Error(): even a transport error can contain a token URL.
	check.Message += guidance
	return check
}
