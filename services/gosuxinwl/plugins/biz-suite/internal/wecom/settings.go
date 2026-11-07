package wecom

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/organization"
)

// Secrets are deliberately absent from the public settings model. Write-only
// credential updates are encrypted in a separate table.
type Settings struct {
	CorpID                 string   `json:"corp_id"`
	AgentID                string   `json:"agent_id"`
	IncludeDepartments     []int64  `json:"include_department_ids"`
	ExcludeDepartments     []int64  `json:"exclude_department_ids"`
	IncludeDepartmentNames []string `json:"include_department_names"`
	ExcludeDepartmentNames []string `json:"exclude_department_names"`
	IncludeUsers           []string `json:"include_userids"`
	ExcludeUsers           []string `json:"exclude_userids"`
	IncludeUserNames       []string `json:"include_user_names"`
	ExcludeUserNames       []string `json:"exclude_user_names"`
	IncludeSubdepartments  bool     `json:"include_subdepartments"`
}

func (c *Client) LoadSettings(ctx context.Context) error {
	c.runMu.Lock()
	defer c.runMu.Unlock()
	c.envCorpID, c.envAgentID = c.CorpID, c.AgentID
	c.envContactSecret, c.envAppSecret, c.envLegacySecret = c.ContactSecret, c.AppSecret, c.Secret
	s := Settings{IncludeSubdepartments: true}
	row, err := core.Model("wecom_settings").Ctx(ctx).Where("id", 1).One()
	if err != nil {
		return dbError("settings load")
	}
	if len(row) > 0 && json.Unmarshal([]byte(row["settings_json"].String()), &s) != nil {
		return errors.New("invalid saved enterprise WeChat settings")
	}
	if err = s.validate(); err != nil {
		return err
	}
	c.applySettings(s)
	return c.loadCredentials(ctx)
}

func (c *Client) applySettings(s Settings) {
	c.mu.Lock()
	defer c.mu.Unlock()
	corp, agent := s.CorpID, s.AgentID
	if corp == "" {
		corp = c.envCorpID
	}
	if agent == "" {
		agent = c.envAgentID
	}
	if corp != c.CorpID {
		c.tokens = nil
	}
	c.CorpID, c.AgentID, c.settings = corp, agent, s
}

func (c *Client) SaveSettings(ctx context.Context, s Settings) error {
	return c.SaveConfiguration(ctx, s, CredentialUpdate{})
}

func (c *Client) SaveConfiguration(ctx context.Context, s Settings, credentials CredentialUpdate) error {
	if err := s.validate(); err != nil {
		return err
	}
	if err := credentials.validate(); err != nil {
		return err
	}
	c.runMu.Lock()
	defer c.runMu.Unlock()
	if c.running {
		return errors.New("同步运行中，请等待批次结束后保存配置")
	}
	// Never generate a replacement key over an already encrypted database. A
	// missing/changed key must be restored before any configuration is committed.
	stored, err := core.Model("wecom_credentials").Ctx(ctx).All()
	if err != nil {
		return dbError("credentials check")
	}
	for _, row := range stored {
		if _, err = decryptCredential(row["scope"].String(), row["encrypted_secret"].String()); err != nil {
			return err
		}
	}
	corp := s.CorpID
	if corp == "" {
		corp = c.envCorpID
	}
	if corp != c.CorpID {
		// IDs and external references belong to one corporation. Avoid silently
		// mixing two tenants in the same existing business database.
		for _, table := range []string{"wecom_department", "wecom_user", "wecom_checkin_raw", "wecom_approval_raw"} {
			exists, err := core.Model(table).Ctx(ctx).Exist()
			if err != nil {
				return dbError("settings company check")
			}
			if exists {
				return errors.New("已有企业微信同步数据，不能更换企业 ID；新企业请使用独立数据库")
			}
		}
	}
	changes := map[string]string{}
	for scope, value := range map[string]string{"contact": credentials.ContactSecret, "app": credentials.AppSecret} {
		if value == "" {
			continue
		}
		sealed, err := encryptCredential(scope, value)
		if err != nil {
			return err
		}
		changes[scope] = sealed
	}
	err = organization.WithTransaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		data := g.Map{"id": 1, "settings_json": mustJSON(s), "updated_at": now()}
		exists, err := tx.Model("wecom_settings").Where("id", 1).Exist()
		if err != nil {
			return err
		}
		if exists {
			_, err = tx.Model("wecom_settings").Where("id", 1).Update(data)
		} else {
			_, err = tx.Model("wecom_settings").Data(data).Insert()
		}
		if err != nil {
			return err
		}
		for scope, clear := range map[string]bool{"contact": credentials.ClearContact, "app": credentials.ClearApp} {
			if clear {
				if _, err = tx.Model("wecom_credentials").Where("scope", scope).Delete(); err != nil {
					return err
				}
			}
		}
		for scope, sealed := range changes {
			data = g.Map{"scope": scope, "encrypted_secret": sealed, "updated_at": now()}
			exists, err = tx.Model("wecom_credentials").Where("scope", scope).Exist()
			if err != nil {
				return err
			}
			if exists {
				_, err = tx.Model("wecom_credentials").Where("scope", scope).Update(data)
			} else {
				_, err = tx.Model("wecom_credentials").Data(data).Insert()
			}
			if err != nil {
				return err
			}
		}
		exclusions, err := core.ExplicitExclusionsForRules(ctx, s.exclusionRules(), tx)
		if err != nil {
			return err
		}
		_, err = organization.PruneInTransaction(ctx, tx, exclusions, false)
		return err
	})
	if err != nil {
		return dbError("configuration save")
	}
	c.applySettings(s)
	c.mu.Lock()
	if credentials.ClearContact {
		c.ContactSecret, c.savedContactSecret = c.envContactSecret, false
	}
	if credentials.ClearApp {
		c.AppSecret, c.savedAppSecret = c.envAppSecret, false
	}
	if credentials.ContactSecret != "" {
		c.ContactSecret, c.savedContactSecret = credentials.ContactSecret, true
	}
	if credentials.AppSecret != "" {
		c.AppSecret, c.savedAppSecret = credentials.AppSecret, true
	}
	if len(changes) > 0 || credentials.ClearContact || credentials.ClearApp {
		c.tokens = nil
	}
	c.mu.Unlock()
	return nil
}

func (s Settings) exclusionRules() core.ExclusionRules {
	return core.ExclusionRules{ExcludeDepartments: s.ExcludeDepartments, ExcludeDepartmentNames: s.ExcludeDepartmentNames, ExcludeUsers: s.ExcludeUsers, ExcludeUserNames: s.ExcludeUserNames, IncludeSubdepartments: s.IncludeSubdepartments}
}

var corpPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
var agentPattern = regexp.MustCompile(`^[0-9]{1,32}$`)
var userPattern = regexp.MustCompile(`^[A-Za-z0-9_@.\-]{1,128}$`)

func (s *Settings) validate() error {
	s.CorpID, s.AgentID = strings.TrimSpace(s.CorpID), strings.TrimSpace(s.AgentID)
	if s.CorpID != "" && !corpPattern.MatchString(s.CorpID) {
		return errors.New("企业 ID 格式无效")
	}
	if s.AgentID != "" && !agentPattern.MatchString(s.AgentID) {
		return errors.New("应用 ID 应为数字")
	}
	for _, ids := range []*[]int64{&s.IncludeDepartments, &s.ExcludeDepartments} {
		if len(*ids) > 10000 {
			return errors.New("部门过滤最多支持 10000 项")
		}
		for _, id := range *ids {
			if id <= 0 {
				return errors.New("部门 ID 必须为正整数")
			}
		}
		*ids = slices.Clone(*ids)
		slices.Sort(*ids)
		*ids = slices.Compact(*ids)
	}
	for _, ids := range []*[]string{&s.IncludeUsers, &s.ExcludeUsers} {
		if len(*ids) > 10000 {
			return errors.New("人员过滤最多支持 10000 项")
		}
		*ids = slices.Clone(*ids)
		for i, id := range *ids {
			id = strings.TrimSpace(id)
			if !userPattern.MatchString(id) {
				return errors.New("人员 userid 格式无效")
			}
			(*ids)[i] = id
		}
		slices.Sort(*ids)
		*ids = slices.Compact(*ids)
	}
	for _, names := range []*[]string{&s.IncludeDepartmentNames, &s.ExcludeDepartmentNames} {
		if err := normalizeFilterNames(names, 160); err != nil {
			return err
		}
	}
	for _, names := range []*[]string{&s.IncludeUserNames, &s.ExcludeUserNames} {
		if err := normalizeFilterNames(names, 128); err != nil {
			return err
		}
	}
	return nil
}

func normalizeFilterNames(names *[]string, maxLength int) error {
	if len(*names) > 10000 {
		return errors.New("名称过滤最多支持 10000 项")
	}
	// Settings is passed by value, but its slices can still belong to the caller.
	*names = slices.Clone(*names)
	for i, name := range *names {
		name = strings.TrimSpace(name)
		if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > maxLength || strings.ContainsFunc(name, unicode.IsControl) {
			return errors.New("过滤名称不能为空、超出长度限制或包含控制字符")
		}
		(*names)[i] = name
	}
	slices.Sort(*names)
	*names = slices.Compact(*names)
	return nil
}

func (c *Client) Settings() Settings {
	c.runMu.Lock()
	defer c.runMu.Unlock()
	s := c.settings
	s.IncludeDepartments = slices.Clone(s.IncludeDepartments)
	s.ExcludeDepartments = slices.Clone(s.ExcludeDepartments)
	s.IncludeDepartmentNames = slices.Clone(s.IncludeDepartmentNames)
	s.ExcludeDepartmentNames = slices.Clone(s.ExcludeDepartmentNames)
	s.IncludeUsers = slices.Clone(s.IncludeUsers)
	s.ExcludeUsers = slices.Clone(s.ExcludeUsers)
	s.IncludeUserNames = slices.Clone(s.IncludeUserNames)
	s.ExcludeUserNames = slices.Clone(s.ExcludeUserNames)
	return s
}
func (c *Client) Status() g.Map {
	c.runMu.Lock()
	defer c.runMu.Unlock()
	metadata := func(scope string) g.Map {
		configured, saved, variable := c.ContactSecret != "", c.savedContactSecret, "WECOM_CONTACT_SECRET"
		if scope == "app" {
			configured, saved, variable = c.secret("app") != "", c.savedAppSecret, "WECOM_APP_SECRET"
		}
		source := "missing"
		if configured {
			source = "environment"
		}
		if scope == "app" && c.AppSecret == "" && c.Secret != "" {
			source, variable = "legacy_environment", "WECOM_SECRET"
		}
		if saved {
			source = "saved"
		}
		return g.Map{"configured": configured, "source": source, "has_saved": saved, "environment_variable": variable}
	}
	return g.Map{"configured": c.configured(), "corp_id": c.CorpID, "agent_id": c.AgentID, "secret_configured": c.secret("app") != "", "running": c.running,
		"automatic_schedule":          automaticSchedule(time.Now()),
		"scopes":                      g.Map{"contact": c.secret("contact") != "", "checkin": c.secret("checkin") != "", "approval": c.secret("approval") != ""},
		"credentials":                 g.Map{"contact": metadata("contact"), "app": metadata("app")},
		"legacy_credential_variables": c.LegacyCredentialVariables}
}

type syncFilter struct {
	settings        Settings
	parents         map[int64]int64
	departmentNames map[int64]string
	userNames       map[string]string
	unknownUsers    map[string]bool
}

func (c *Client) filter(ctx context.Context) (syncFilter, error) {
	f := syncFilter{settings: c.Settings(), parents: map[int64]int64{}, departmentNames: map[int64]string{}, userNames: map[string]string{}, unknownUsers: map[string]bool{}}
	rows, err := core.Model("wecom_department").Ctx(ctx).Fields("wecom_id,parent_id,name").All()
	if err != nil {
		return f, dbError("department filter")
	}
	for _, r := range rows {
		id := r["wecom_id"].Int64()
		f.parents[id] = r["parent_id"].Int64()
		f.departmentNames[id] = strings.TrimSpace(r["name"].String())
	}
	users, err := loadFilterUsers(ctx, f.departmentNames)
	if err != nil {
		return f, err
	}
	for _, u := range users {
		f.userNames[u.UserID] = u.Name
		f.unknownUsers[u.UserID] = u.ScopeUnknown
	}
	return f, nil
}

func (f syncFilter) matches(id int64, ids []int64) bool {
	return f.matchesDepartment(id, ids, nil)
}

func (f syncFilter) matchesDepartment(id int64, ids []int64, names []string) bool {
	seen := map[int64]bool{}
	for id > 0 && !seen[id] {
		name := f.departmentNames[id]
		if slices.Contains(ids, id) || (name != "" && slices.Contains(names, name)) {
			return true
		}
		if !f.settings.IncludeSubdepartments {
			break
		}
		seen[id] = true
		id = f.parents[id]
	}
	return false
}

func (f syncFilter) departmentAllowed(id int64) bool {
	s := f.settings
	return !f.matchesDepartment(id, s.ExcludeDepartments, s.ExcludeDepartmentNames) &&
		(len(s.IncludeDepartments)+len(s.IncludeDepartmentNames) == 0 || f.matchesDepartment(id, s.IncludeDepartments, s.IncludeDepartmentNames))
}

func (f syncFilter) allows(uid string, departments []int64) bool {
	return !f.unknownUsers[uid] && f.allowsNamed(uid, f.userNames[uid], departments)
}

func (f syncFilter) allowsNamed(uid, name string, departments []int64) bool {
	s := f.settings
	name = actualUserName(uid, name)
	// An unknown real name cannot prove that a name exclusion does not match.
	if name == "" && len(s.ExcludeUserNames) > 0 {
		return false
	}
	if uid == "" || slices.Contains(s.ExcludeUsers, uid) || (name != "" && slices.Contains(s.ExcludeUserNames, name)) {
		return false
	}
	if len(s.IncludeUsers)+len(s.IncludeUserNames) > 0 && !slices.Contains(s.IncludeUsers, uid) && (name == "" || !slices.Contains(s.IncludeUserNames, name)) {
		return false
	}
	// Unknown membership cannot bypass either ID or name department filters.
	if len(departments) == 0 && len(s.IncludeDepartments)+len(s.ExcludeDepartments)+len(s.IncludeDepartmentNames)+len(s.ExcludeDepartmentNames) > 0 {
		return false
	}
	included := len(s.IncludeDepartments)+len(s.IncludeDepartmentNames) == 0
	for _, id := range departments {
		if len(s.ExcludeDepartmentNames) > 0 && !f.departmentNamesKnown(id) {
			return false
		}
		if f.matchesDepartment(id, s.ExcludeDepartments, s.ExcludeDepartmentNames) {
			return false
		}
		if f.matchesDepartment(id, s.IncludeDepartments, s.IncludeDepartmentNames) {
			included = true
		}
	}
	return included
}

// A name exclusion needs the complete visible ancestor path when descendants
// are included. A missing name, missing parent link or cycle is not evidence
// that the member is outside the excluded subtree.
func (f syncFilter) departmentNamesKnown(id int64) bool {
	if id <= 0 {
		return false
	}
	seen := map[int64]bool{}
	for id > 0 {
		if seen[id] || f.departmentNames[id] == "" {
			return false
		}
		if !f.settings.IncludeSubdepartments {
			return true
		}
		seen[id] = true
		parent, known := f.parents[id]
		if !known || parent < 0 {
			return false
		}
		id = parent
	}
	return true
}

func (f syncFilter) applicantAllowed(ctx context.Context, uid string) (bool, error) {
	if f.unknownUsers[uid] {
		return false, nil
	}
	s := f.settings
	if len(s.IncludeDepartments)+len(s.ExcludeDepartments)+len(s.IncludeDepartmentNames)+len(s.ExcludeDepartmentNames)+len(s.IncludeUsers)+len(s.ExcludeUsers)+len(s.IncludeUserNames)+len(s.ExcludeUserNames) == 0 {
		return true, nil
	}
	row, err := core.Model("wecom_user").Ctx(ctx).Where("wecom_userid", uid).Fields("department_ids").One()
	if err != nil {
		return false, dbError("approval applicant scope")
	}
	var departments []int64
	if len(row) > 0 {
		_ = json.Unmarshal([]byte(row["department_ids"].String()), &departments)
	}
	return f.allows(uid, departments), nil
}

// Both WeCom and older HRM imports can use userid as a placeholder name. Such
// values do not establish a person's real name for filtering or suggestions.
func actualUserName(uid, name string) string {
	name = strings.TrimSpace(name)
	if name == uid {
		return ""
	}
	return name
}

type filterUserOption struct {
	UserID          string   `json:"wecom_userid"`
	Name            string   `json:"name"`
	DepartmentIDs   string   `json:"department_ids"`
	DepartmentNames []string `json:"department_names"`
	ScopeUnknown    bool     `json:"-"`
}

// Only explicitly bound HRM records can fill a missing name. Never infer a
// userid or department membership from a person's name, phone or local dept ID.
func loadFilterUsers(ctx context.Context, departmentNames map[int64]string) ([]filterUserOption, error) {
	rows, err := core.Model("wecom_user").Ctx(ctx).Fields("wecom_userid,name,department_ids,processing_error").All()
	if err != nil {
		return nil, dbError("filter users")
	}
	users := map[string]filterUserOption{}
	for _, row := range rows {
		uid := row["wecom_userid"].String()
		if uid == "" {
			continue
		}
		ids := []int64{}
		if json.Unmarshal([]byte(row["department_ids"].String()), &ids) != nil || ids == nil {
			ids = []int64{}
		}
		names := []string{}
		for _, id := range ids {
			if name := departmentNames[id]; name != "" && !slices.Contains(names, name) {
				names = append(names, name)
			}
		}
		users[uid] = filterUserOption{UserID: uid, Name: actualUserName(uid, row["name"].String()), DepartmentIDs: mustJSON(ids), DepartmentNames: names, ScopeUnknown: row["processing_error"].String() == contactScopeUnobserved}
	}
	employees, err := core.Model("biz_hrm_employee").Ctx(ctx).Fields("wecom_userid,name").All()
	if err != nil {
		return nil, dbError("filter employee mapping")
	}
	counts, employeeNames := map[string]int{}, map[string]string{}
	for _, row := range employees {
		uid := row["wecom_userid"].String()
		if uid == "" {
			continue
		}
		counts[uid]++
		employeeNames[uid] = actualUserName(uid, row["name"].String())
	}
	for uid, count := range counts {
		if count != 1 {
			continue
		}
		u, exists := users[uid]
		if !exists {
			u = filterUserOption{UserID: uid, DepartmentIDs: "[]", DepartmentNames: []string{}}
		}
		if u.Name == "" {
			u.Name = employeeNames[uid]
		}
		users[uid] = u
	}
	ids := make([]string, 0, len(users))
	for uid := range users {
		ids = append(ids, uid)
	}
	slices.Sort(ids)
	out := make([]filterUserOption, 0, len(ids))
	for _, uid := range ids {
		out = append(out, users[uid])
	}
	return out, nil
}
