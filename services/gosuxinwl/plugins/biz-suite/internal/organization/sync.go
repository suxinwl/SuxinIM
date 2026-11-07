package organization

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/wecomprofile"
)

type Input struct {
	AllowedDepartments map[int64]bool
	AllowedUsers       map[string]bool
}
type Conflict struct {
	EntityType     string `json:"entity_type"`
	Name           string `json:"name"`
	EmployeeNo     string `json:"employee_no"`
	DepartmentPath string `json:"department_path"`
	Reason         string `json:"reason"`
}
type Report struct {
	DepartmentsCreated int        `json:"departments_created"`
	DepartmentsUpdated int        `json:"departments_updated"`
	AccountsCreated    int        `json:"accounts_created"`
	AccountsUpdated    int        `json:"accounts_updated"`
	EmployeesLinked    int        `json:"employees_linked"`
	Skipped            int        `json:"skipped"`
	ConflictCount      int        `json:"conflict_count"`
	Conflicts          []Conflict `json:"conflicts"`
	FinishedAt         string     `json:"finished_at"`
}
type department struct {
	id, parent, hostID int64
	name               string
	order              int
}
type member struct {
	UserID         string  `json:"userid"`
	Name           string  `json:"name"`
	Department     []int64 `json:"department"`
	MainDepartment int64   `json:"main_department"`
}

type mappingConflict string

func (e mappingConflict) Error() string { return string(e) }

func LastReport(ctx context.Context) (any, error) {
	row, err := core.Model("biz_org_sync_state").Ctx(ctx).Where("id", 1).One()
	if err != nil {
		return nil, err
	}
	var result any
	if len(row) == 0 {
		return nil, nil
	}
	if err = json.Unmarshal([]byte(row["report_json"].String()), &result); err != nil {
		return nil, fmt.Errorf("组织同步结果读取失败")
	}
	return result, nil
}

// Sync uses a local snapshot only. All host mutations and business mappings are
// committed together, under a database row lock shared by worker processes.
func Sync(ctx context.Context, in Input) (Report, error) {
	report := Report{Conflicts: []Conflict{}}
	err := locked(ctx, func(ctx context.Context, tx gdb.TX) error {
		exclusions, err := core.LoadExplicitExclusions(ctx, tx)
		if err != nil {
			return err
		}
		if _, err = PruneInTransaction(ctx, tx, exclusions, false); err != nil {
			return err
		}
		deps, err := model(ctx, tx, "wecom_department").OrderAsc("wecom_id").All()
		if err != nil {
			return err
		}
		directory := map[int64]department{}
		for _, r := range deps {
			d := department{r["wecom_id"].Int64(), r["parent_id"].Int64(), r["host_dept_id"].Int64(), r["name"].String(), r["order_no"].Int()}
			directory[d.id] = d
		}
		needed := map[int64]bool{}
		for id, allowed := range in.AllowedDepartments {
			if !allowed || exclusions.Departments[id] {
				continue
			}
			for seen := map[int64]bool{}; id > 0 && !seen[id]; id = directory[id].parent {
				seen[id] = true
				needed[id] = true
			}
		}
		resolved, visiting, failed := map[int64]int64{}, map[int64]bool{}, map[int64]bool{}
		var ensureDepartment func(int64) (int64, error)
		ensureDepartment = func(id int64) (int64, error) {
			if id == 0 {
				return 0, nil
			}
			if exclusions.Departments[id] {
				return 0, mappingConflict("部门已被同步过滤规则排除")
			}
			if v := resolved[id]; v > 0 {
				return v, nil
			}
			if failed[id] {
				return 0, mappingConflict("上级部门映射失败")
			}
			d, ok := directory[id]
			if !ok {
				return 0, mappingConflict("通讯录缺少上级部门，请先扩大可见范围并同步")
			}
			if visiting[id] {
				return 0, mappingConflict("部门层级存在循环")
			}
			visiting[id] = true
			defer delete(visiting, id)
			if strings.TrimSpace(d.name) == "" || utf8.RuneCountInString(d.name) > 100 {
				return 0, mappingConflict("部门名称为空或超过100字符")
			}
			parent, err := ensureDepartment(d.parent)
			if err != nil {
				return 0, err
			}
			binding, err := model(ctx, tx, "biz_org_binding").Where("entity_type", "department").Where("external_id", strconv.FormatInt(id, 10)).One()
			if err != nil {
				return 0, err
			}
			if d.hostID == 0 && len(binding) > 0 {
				return 0, mappingConflict("原自动映射已被解除，请人工确认后重新绑定")
			}
			if d.hostID > 0 {
				row, err := host(ctx, tx, "auth_dept").Where("id", d.hostID).WhereNull("deletetime").One()
				if err != nil {
					return 0, err
				}
				if len(row) == 0 || row["status"].Int() != 0 {
					return 0, mappingConflict("已绑定本地部门已删除或停用，请人工处理")
				}
				if len(binding) > 0 && binding["host_id"].Int64() == d.hostID {
					if err := validateParent(ctx, tx, d.hostID, parent); err != nil {
						return 0, err
					}
					if row["name"].String() != d.name || row["pid"].Int64() != parent || row["weigh"].Int() != d.order {
						if _, err = host(ctx, tx, "auth_dept").Where("id", d.hostID).Update(g.Map{"name": d.name, "pid": parent, "weigh": d.order, "updatetime": core.Now()}); err != nil {
							return 0, err
						}
						report.DepartmentsUpdated++
					}
				}
			} else {
				if err := validateParent(ctx, tx, 0, parent); err != nil {
					return 0, err
				}
				d.hostID, err = host(ctx, tx, "auth_dept").Data(g.Map{"business_id": 0, "account_id": 0, "name": d.name, "pid": parent, "weigh": d.order, "status": 0, "remark": "企业微信组织架构同步", "createtime": core.Now(), "updatetime": core.Now()}).InsertAndGetId()
				if err != nil {
					return 0, err
				}
				if _, err = model(ctx, tx, "biz_org_binding").Data(g.Map{"entity_type": "department", "external_id": strconv.FormatInt(id, 10), "host_id": d.hostID, "created_at": core.Now()}).Insert(); err != nil {
					return 0, err
				}
				if _, err = model(ctx, tx, "wecom_department").Where("wecom_id", id).Update(g.Map{"host_dept_id": d.hostID}); err != nil {
					return 0, err
				}
				report.DepartmentsCreated++
			}
			directory[id] = d
			resolved[id] = d.hostID
			return d.hostID, nil
		}
		ids := []int64{}
		for id := range needed {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		for _, id := range ids {
			if _, err := ensureDepartment(id); err != nil {
				var conflict mappingConflict
				if !errors.As(err, &conflict) {
					return err
				}
				failed[id] = true
				report.add("department", directory[id].name, "", departmentPath(directory, id), err.Error())
			}
		}
		users, err := model(ctx, tx, "wecom_user").OrderAsc("wecom_userid").All()
		if err != nil {
			return err
		}
		// Preflight the entire observed directory, including excluded staff, so an
		// old duplicate number cannot silently claim somebody else's login.
		numbers := map[string]int{}
		for _, row := range users {
			if no := wecomprofile.EmployeeNoFromRaw(row["raw_json"].String()); no != "" {
				numbers[strings.ToLower(no)]++
			}
		}
		role, err := model(ctx, tx, "biz_org_role").Where("role_key", "employee").Value("role_id")
		if err != nil {
			return err
		}
		for _, row := range users {
			uid, name := row["wecom_userid"].String(), row["name"].String()
			if !in.AllowedUsers[uid] || exclusions.Users[uid] {
				report.Skipped++
				continue
			}
			var user member
			if json.Unmarshal([]byte(row["raw_json"].String()), &user) != nil {
				report.add("employee", name, "", "", "成员原始数据无法解析")
				continue
			}
			if len(user.Department) == 0 {
				_ = json.Unmarshal([]byte(row["department_ids"].String()), &user.Department)
			}
			primary, reason := primaryDepartment(user)
			path := departmentPath(directory, primary)
			no := wecomprofile.EmployeeNoFromRaw(row["raw_json"].String())
			employees, err := model(ctx, tx, "biz_hrm_employee").Where("wecom_userid", uid).Limit(2).All()
			if err != nil {
				return err
			}
			if len(employees) != 1 {
				report.add("employee", name, no, path, "员工档案尚未唯一匹配，请先完成通讯录同步或人工绑定")
				continue
			}
			employee := employees[0]
			if employee["status"].String() == "filtered" {
				report.add("employee", name, no, path, "员工已按过滤规则归档，请先恢复部门、账号并重新同步通讯录")
				continue
			}
			employeeID := employee["id"].String()
			if reason != "" {
				report.add("employee", name, no, path, reason)
				continue
			}
			deptID := resolved[primary]
			if deptID == 0 {
				report.add("employee", name, no, path, "主部门不在同步范围内或本地部门映射失败")
				continue
			}
			if employee["dept_id"].Int64() != deptID {
				if _, err = model(ctx, tx, "biz_hrm_employee").Where("id", employeeID).Update(g.Map{"dept_id": deptID, "updated_at": core.Now()}); err != nil {
					return err
				}
			}
			if no == "" {
				no = employee["employee_no"].String()
			}
			if reason = validAccount(name, no, numbers); reason != "" {
				report.add("employee", name, no, path, reason)
				continue
			}
			// The local unique employee_no field may already belong to a different
			// archive. Do not merge or reassign that archive automatically.
			other, err := model(ctx, tx, "biz_hrm_employee").Where("employee_no", no).WhereNot("id", employeeID).Exist()
			if err != nil {
				return err
			}
			if other {
				report.add("employee", name, no, path, "工号已被另一份本地员工档案占用，请核对后处理")
				continue
			}
			binding, err := model(ctx, tx, "biz_org_binding").Where("entity_type", "account").Where("external_id", uid).One()
			if err != nil {
				return err
			}
			accountID := employee["user_id"].Int64()
			owned := len(binding) > 0 && binding["host_id"].Int64() == accountID
			if accountID == 0 && len(binding) > 0 {
				report.add("employee", name, no, path, "原自动账号关联已解除，请人工确认后重新绑定")
				continue
			}
			q := host(ctx, tx, "admin").Where("username", no)
			if accountID > 0 {
				q = q.WhereNot("id", accountID)
			}
			collision, err := q.Exist()
			if err != nil {
				return err
			}
			if collision {
				report.add("employee", name, no, path, "登录工号已被本地账号占用，不自动接管已有账号")
				continue
			}
			if accountID > 0 {
				account, err := host(ctx, tx, "admin").Where("id", accountID).WhereNull("deletetime").One()
				if err != nil {
					return err
				}
				if len(account) == 0 {
					report.add("employee", name, no, path, "已绑定本地账号已删除，请人工处理")
					continue
				}
				if !owned {
					if account["username"].String() != no || account["name"].String() != name || account["dept_id"].Int64() != deptID {
						report.add("employee", name, no, path, "人工绑定账号的姓名、工号或部门不一致，请由管理员确认修改")
						continue
					}
				} else {
					data := g.Map{"name": name, "nickname": name, "username": no, "dept_id": deptID}
					changed := account["name"].String() != name || account["nickname"].String() != name || account["username"].String() != no || account["dept_id"].Int64() != deptID
					if row["status"].Int() == 5 && account["status"].Int() != 1 {
						data["status"] = 1
						changed = true
					}
					if changed {
						data["updatetime"] = core.Now()
						if _, err = host(ctx, tx, "admin").Where("id", accountID).Update(data); err != nil {
							return err
						}
						report.AccountsUpdated++
					}
				}
			} else {
				if row["status"].Int() != 1 {
					report.Skipped++
					continue
				}
				if err = validateDefaultRole(ctx, tx, role.Int64()); err != nil {
					report.add("employee", name, no, path, err.Error())
					continue
				}
				password, salt, err := randomCredentials()
				if err != nil {
					return err
				}
				accountID, err = host(ctx, tx, "admin").Data(g.Map{"account_id": 0, "dept_id": deptID, "username": no, "name": name, "nickname": name, "password": password, "salt": salt, "status": 1, "avatar": "/resource/static/brand/default-avatar.png?v=puty2", "email": "", "mobile": "", "tel": "", "remark": "企业微信同步：请管理员设置密码后启用", "createtime": core.Now(), "updatetime": core.Now()}).InsertAndGetId()
				if err != nil {
					return err
				}
				if _, err = model(ctx, tx, "biz_org_binding").Data(g.Map{"entity_type": "account", "external_id": uid, "host_id": accountID, "created_at": core.Now()}).Insert(); err != nil {
					return err
				}
				if _, err = host(ctx, tx, "auth_role_access").Data(g.Map{"uid": accountID, "role_id": role.Int64()}).Insert(); err != nil {
					return err
				}
				report.AccountsCreated++
			}
			if _, err = model(ctx, tx, "biz_hrm_employee").Where("id", employeeID).Update(g.Map{"user_id": accountID, "employee_no": no, "dept_id": deptID, "updated_at": core.Now()}); err != nil {
				return err
			}
			if _, err = model(ctx, tx, "wecom_user").Where("wecom_userid", uid).Update(g.Map{"employee_id": employeeID, "user_id": accountID}); err != nil {
				return err
			}
			report.EmployeesLinked++
		}
		report.FinishedAt = core.Now()
		report.ConflictCount = len(report.Conflicts)
		_, err = model(ctx, tx, "biz_org_sync_state").Where("id", 1).Update(g.Map{"report_json": jsonText(report), "updated_at": core.Now()})
		return err
	})
	if err != nil {
		return Report{}, fmt.Errorf("组织与账号同步失败，已回滚本次数据库变更: %w", err)
	}
	return report, nil
}

func (r *Report) add(kind, name, no, path, reason string) {
	r.Conflicts = append(r.Conflicts, Conflict{kind, name, no, path, reason})
}

func validateParent(ctx context.Context, tx gdb.TX, child, parent int64) error {
	seen := map[int64]bool{}
	for parent > 0 {
		if parent == child || seen[parent] {
			return mappingConflict("当前部门映射会造成本地组织循环，请调整人工映射")
		}
		seen[parent] = true
		row, err := host(ctx, tx, "auth_dept").Where("id", parent).WhereNull("deletetime").Where("status", 0).One()
		if err != nil {
			return err
		}
		if len(row) == 0 {
			return mappingConflict("本地上级部门已删除或停用，请人工处理")
		}
		parent = row["pid"].Int64()
	}
	return nil
}
func primaryDepartment(u member) (int64, string) {
	if u.MainDepartment > 0 {
		for _, id := range u.Department {
			if id == u.MainDepartment {
				return id, ""
			}
		}
		return 0, "主部门不在成员所属部门中，请核对通讯录"
	}
	if len(u.Department) == 1 && u.Department[0] > 0 {
		return u.Department[0], ""
	}
	return 0, "成员属于多个部门且缺少主部门，无法确定本地归属"
}
func departmentPath(deps map[int64]department, id int64) string {
	parts := []string{}
	seen := map[int64]bool{}
	for id > 0 && !seen[id] {
		seen[id] = true
		d, ok := deps[id]
		if !ok {
			break
		}
		parts = append([]string{d.name}, parts...)
		id = d.parent
	}
	return strings.Join(parts, " / ")
}
func validAccount(name, no string, numbers map[string]int) string {
	if no == "" {
		return "缺少明确工号，请补充企业微信通讯录的工号字段"
	}
	if numbers[strings.ToLower(no)] > 1 {
		return "企业微信通讯录存在重复工号，请核对后再同步"
	}
	if utf8.RuneCountInString(no) > 20 || strings.IndexFunc(no, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
		return "工号须为1至20字符且不能含空白或控制字符"
	}
	if strings.TrimSpace(name) == "" || utf8.RuneCountInString(name) > 50 {
		return "员工姓名为空或超过本地账号50字符限制"
	}
	return ""
}
func randomCredentials() (string, string, error) {
	secret := make([]byte, 32)
	saltBytes := make([]byte, 12)
	if _, err := rand.Read(secret); err != nil {
		return "", "", err
	}
	if _, err := rand.Read(saltBytes); err != nil {
		return "", "", err
	}
	// Random values are discarded; administrators must set a usable password.
	salt := hex.EncodeToString(saltBytes)
	inner := fmt.Sprintf("%x", md5.Sum(secret))
	hash := fmt.Sprintf("%x", md5.Sum([]byte(inner+salt)))
	return hash, salt, nil
}
func validateDefaultRole(ctx context.Context, tx gdb.TX, id int64) error {
	r, err := host(ctx, tx, "auth_role").Where("id", id).Where("status", 0).One()
	if err != nil {
		return err
	}
	if len(r) == 0 || r["rules"].String() == "*" {
		return fmt.Errorf("默认员工角色已删除、停用或权限异常，请管理员处理")
	}
	var btns []int64
	if json.Unmarshal([]byte(r["btns"].String()), &btns) != nil {
		return fmt.Errorf("默认员工角色权限格式异常")
	}
	allowed := map[string]bool{"runtime.plugin.biz-suite.admin": true, "biz-suite.hrm.self": true, "biz-suite.im.read": true, "biz-suite.im.write": true}
	if len(btns) > 0 {
		rows, err := host(ctx, tx, "auth_rule").WhereIn("id", btns).All()
		if err != nil {
			return err
		}
		for _, row := range rows {
			if !allowed[row["permission"].String()] {
				return fmt.Errorf("默认员工角色包含额外权限，停止自动分配，请管理员处理")
			}
		}
	}
	return nil
}
