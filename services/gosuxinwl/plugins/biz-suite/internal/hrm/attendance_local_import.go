package hrm

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	eng "github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/attendanceengine"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
)

// Missing punch windows remain null. Source cells remain immutable evidence.
var localSchedulePair = regexp.MustCompile(`(\d{1,2}):(\d{2})\s*-\s*(次日)?(?:(\d{1,2}):(\d{2})|(结束))`)
var localNamedShift = regexp.MustCompile(`(?:白班|夜班|早班|晚班)\s*:`)
var localWindowHint = regexp.MustCompile(`(\d{1,2})\s*:\s*(\d{2})\s*上班需在\s*(\d{1,2})\s*:\s*(\d{2})\s*后?打上班卡`)
var localAlternativeEnd = regexp.MustCompile(`(\d{1,2}:\d{2}\s*-\s*\d{1,2}:\d{2})\s*/\s*(\d{1,2}:\d{2})`)
var localNameNote = regexp.MustCompile(`[（(][^）)]*[）)]`)

func localNormalizeTime(text string) string {
	return strings.NewReplacer("：", ":", "－", "-", "—", "-", "～", "-", "\r", "").Replace(text)
}
func localSecond(value int) *int { return &value }

func localParseShifts(text string) []eng.Shift {
	text = localNormalizeTime(text)
	if m := localAlternativeEnd.FindStringSubmatchIndex(text); m != nil {
		first := text[:m[0]] + text[m[2]:m[3]] + text[m[1]:]
		prefix := text[m[2]:m[3]]
		second := text[:m[0]] + prefix[:strings.LastIndex(prefix, "-")+1] + text[m[4]:m[5]] + text[m[1]:]
		out := append(localParseShifts(first), localParseShifts(second)...)
		for i := range out {
			out[i].ID = fmt.Sprintf("shift%d", i+1)
			out[i].Name += fmt.Sprintf("（候选%d）", i+1)
		}
		return out
	}
	text = localNamedShift.ReplaceAllStringFunc(text, func(s string) string { return "\n" + s })
	shifts := []eng.Shift{}
	for _, line := range strings.Split(text, "\n") {
		pairs := localSchedulePair.FindAllStringSubmatch(line, -1)
		if len(pairs) == 0 {
			continue
		}
		name := "日班"
		for _, label := range []string{"白班", "夜班", "早班", "晚班"} {
			if strings.Contains(line, label+":") {
				name = label
			}
		}
		shift := eng.Shift{ID: fmt.Sprintf("shift%d", len(shifts)+1), Name: name}
		offset, previous := 0, -1
		for i, p := range pairs {
			hour, _ := strconv.Atoi(p[1])
			minute, _ := strconv.Atoi(p[2])
			start := hour*60 + minute + offset
			if start < previous {
				start += 1440
				offset += 1440
			}
			end := 2880
			open := p[6] != ""
			if !open {
				h, _ := strconv.Atoi(p[4])
				m, _ := strconv.Atoi(p[5])
				end = h*60 + m + offset
				if p[3] != "" && end < 1440 {
					end += 1440
				}
				if end <= start {
					end += 1440
				}
			}
			kind := "work"
			if open {
				kind = "overtime"
			}
			shift.Segments = append(shift.Segments, eng.Segment{ID: fmt.Sprintf("segment%d", i+1), Name: fmt.Sprintf("班段%d", i+1), Kind: kind, StartMinute: start, EndMinute: end, OpenEnded: open, PunchMode: "both", LocalWindows: &eng.LocalPunchWindows{}})
			previous = end
			if !open {
				offset = end / 1440 * 1440
			}
		}
		shifts = append(shifts, shift)
	}
	return shifts
}

func localBoundaryWindows(shift *eng.Shift, segment *eng.Segment) *eng.LocalPunchWindows {
	if segment.EvidenceID != "" {
		for i := range shift.EvidencePeriods {
			p := &shift.EvidencePeriods[i]
			if p.ID == segment.EvidenceID {
				if p.LocalWindows == nil {
					p.LocalWindows = &eng.LocalPunchWindows{}
				}
				return p.LocalWindows
			}
		}
	}
	if segment.LocalWindows == nil {
		segment.LocalWindows = &eng.LocalPunchWindows{}
	}
	return segment.LocalWindows
}

func localHintWindows(shift *eng.Shift, hints string) {
	for _, match := range localWindowHint.FindAllStringSubmatch(localNormalizeTime(hints), -1) {
		h, _ := strconv.Atoi(match[1])
		m, _ := strconv.Atoi(match[2])
		want := h*60 + m
		h, _ = strconv.Atoi(match[3])
		m, _ = strconv.Atoi(match[4])
		boundary := (h*60 + m) * 60
		for i := range shift.Segments {
			s := &shift.Segments[i]
			if s.StartMinute%1440 != want {
				continue
			}
			at := boundary + s.StartMinute/1440*86400
			localBoundaryWindows(shift, s).InStartSecond = localSecond(at)
			if i > 0 {
				prev := &shift.Segments[i-1]
				if prev.EvidenceID == "" || prev.EvidenceID != s.EvidenceID {
					localBoundaryWindows(shift, prev).OutEndSecond = localSecond(at)
				}
			}
		}
	}
	if shift.OvertimeVariant != nil {
		localHintWindows(&shift.OvertimeVariant.Shift, hints)
	}
}

func localConditionalShift(normal eng.Shift, overtime eng.Shift) eng.Shift {
	overtime.ID = normal.ID + "-overtime"
	overtime.Name = normal.Name + "（有加班）"
	segments := []eng.Segment{}
	trigger := 0
	for i, segment := range overtime.Segments {
		if i >= len(normal.Segments) {
			segment.Kind = "overtime"
			if segment.StartMinute%1440 == 17*60+45 {
				segment.OvertimeStartMinute = segment.StartMinute + 15
			}
			if trigger == 0 {
				trigger = segment.StartMinute
			}
			segments = append(segments, segment)
			continue
		}
		regular := normal.Segments[i]
		if segment.EndMinute > regular.EndMinute && !segment.OpenEnded {
			// The accounting split does not demand an extra physical punch.
			eid := segment.ID + "-evidence"
			overtime.EvidencePeriods = append(overtime.EvidencePeriods, eng.EvidencePeriod{ID: eid, StartSecond: segment.StartMinute * 60, EndSecond: segment.EndMinute * 60, PunchMode: "both", LocalWindows: segment.LocalWindows})
			work := segment
			work.EndMinute = regular.EndMinute
			work.EvidenceID = eid
			work.LocalWindows = nil
			extra := segment
			extra.ID += "-extra"
			extra.Kind = "overtime"
			extra.StartMinute = regular.EndMinute
			extra.EvidenceID = eid
			extra.LocalWindows = nil
			segments = append(segments, work, extra)
			if trigger == 0 {
				trigger = extra.StartMinute
			}
		} else {
			segment.Kind = "work"
			segments = append(segments, segment)
		}
	}
	overtime.Segments = segments
	if trigger > 0 {
		normal.OvertimeVariant = &eng.ShiftVariant{TriggerMinute: trigger, Shift: overtime}
	}
	return normal
}

func localSourceConfig(source ImportRuleDraft) (AttConfig, []string) {
	category := source.Category
	if category == "" {
		category = "职员"
		if strings.HasPrefix(source.Sheet, "员工") {
			category = "员工"
		}
		if strings.HasPrefix(source.Sheet, "生产职员") {
			category = "生产职员"
		}
	}
	config := AttConfig{Category: category, Calendar: map[string]AttCalendarDay{}, Rule: eng.Rule{EvidenceMode: "local", AttendanceMode: "fixed_complete", StandardMinutes: 480, PunchSelection: "first_last", OvertimeDateMode: "workdate", PartialWorkPolicy: &eng.PartialWorkPolicy{Mode: "actual"}, Overtime: map[eng.Category]eng.OvertimePolicy{}}, Cycle: AttCycle{Mode: "weekly"}}
	conflicts := []string{}
	normal := localParseShifts(source.ScheduleText)
	overtime := localParseShifts(source.OvertimeSchedule)
	for i := range normal {
		if i < len(overtime) {
			normal[i] = localConditionalShift(normal[i], overtime[i])
		}
		localHintWindows(&normal[i], source.CheckinText)
	}
	config.Shifts = normal
	if len(normal) == 0 {
		conflicts = append(conflicts, "缺少可计算班次；免打卡人员也须明确应出勤工时与休息日")
		attSourceWorkSchedule(&config, source, normal)
		return config, conflicts
	}
	shiftID := normal[0].ID
	if len(normal) > 1 {
		config.ShiftSelection = "auto"
	}
	for day := 0; day < 7; day++ {
		entry := AttCalendarDay{ShiftID: shiftID, Category: eng.Workday}
		if day >= 5 {
			entry.Category = eng.Restday
		}
		if category == "职员" && strings.Contains(source.Remarks, "单休") && day == 5 {
			entry.Category = eng.Workday
		}
		if len(normal) > 1 {
			ids := []string{}
			for _, s := range normal {
				ids = append(ids, s.ID)
			}
			entry.CandidateShiftIDs = &ids
		}
		config.Cycle.Weekdays = append(config.Cycle.Weekdays, entry)
	}
	if category == "员工" || category == "生产职员" {
		for _, cat := range []eng.Category{eng.Workday, eng.Restday, eng.Holiday} {
			config.Rule.Overtime[cat] = eng.OvertimePolicy{Enabled: true, RoundingMinutes: 30, RoundingScope: "daily"}
		}
		rest := config.Rule.Overtime[eng.Restday]
		rest.CompleteWorkdayMinutes = 480
		config.Rule.Overtime[eng.Restday] = rest
	}
	if source.DirectoryDepartment == "注塑事业部" && category == "员工" {
		config.Rule.CompleteWorkDeductionMinutes = 30
		config.Rule.PartialWorkPolicy = &eng.PartialWorkPolicy{Mode: "review", ActualRanges: []eng.LocalTimeRange{{StartSecond: 8 * 3600, EndSecond: 12 * 3600}}}
	}
	if category == "职员" {
		minutes := 0
		for _, s := range normal[0].Segments {
			if s.Kind == "work" {
				minutes += s.EndMinute - s.StartMinute
			}
		}
		config.Rule.StandardMinutes = minutes
		if strings.Contains(source.Remarks, "21:30") || strings.Contains(source.Remarks, "21：30") {
			config.Rule.LateDepartureAfterSecond = localSecond(21*3600 + 30*60)
			config.Rule.LateDepartureDelayMinutes = 60
		}
		if strings.Contains(source.Remarks, "大小周") {
			config.Cycle = AttCycle{Mode: "cycle", Anchor: "2026-10-05", Days: []string{shiftID, shiftID, shiftID, shiftID, shiftID, shiftID, "", shiftID, shiftID, shiftID, shiftID, shiftID, "", ""}}
		}
		if strings.Contains(source.Remarks, "AB班") {
			conflicts = append(conflicts, "请确认AB人员分组与各组周休，不能仅由打卡推断应出勤日")
		}
		if strings.Contains(source.Remarks, "倒班") || strings.Contains(source.Remarks, "周末有排班") {
			conflicts = append(conflicts, "请配置轮班人员的休息日；完整打卡可识别班次，但无卡不能认定休息")
		}
		if strings.Contains(source.Remarks, "只打两个卡") && len(config.Shifts[0].Segments) > 0 {
			s := &config.Shifts[0]
			first, last := s.Segments[0], s.Segments[len(s.Segments)-1]
			s.EvidencePeriods = []eng.EvidencePeriod{{ID: "day-evidence", StartSecond: first.StartMinute * 60, EndSecond: last.EndMinute * 60, PunchMode: "both", LocalWindows: &eng.LocalPunchWindows{}}}
			for i := range s.Segments {
				s.Segments[i].EvidenceID = "day-evidence"
				s.Segments[i].LocalWindows = nil
			}
		}
	}
	if category == "生产职员" {
		saturday := localParseShifts(source.SaturdaySchedule)
		if len(saturday) > 0 {
			ids := []string{}
			for i := range saturday {
				saturday[i].ID = "saturday"
				if i > 0 {
					saturday[i].ID += strconv.Itoa(i + 1)
				}
				saturday[i].Name = "周六正班·" + saturday[i].Name
				localHintWindows(&saturday[i], source.CheckinText)
				ids = append(ids, saturday[i].ID)
				config.Shifts = append(config.Shifts, saturday[i])
			}
			config.Cycle.Weekdays[5] = AttCalendarDay{ShiftID: ids[0], Category: eng.Workday, CandidateShiftIDs: &ids}
		}
		if len(normal) > 1 && !strings.Contains(source.Remarks, "留守") {
			conflicts = append(conflicts, "请确认生产职员各候选班次的值班日期及人员分组")
		}
		if strings.Contains(source.Remarks, "计算加班2小时") {
			policy := config.Rule.Overtime[eng.Workday]
			policy.MaximumMinutes = 120
			config.Rule.Overtime[eng.Workday] = policy
			for i := range config.Shifts {
				s := &config.Shifts[i]
				if strings.HasPrefix(s.ID, "saturday") {
					continue
				}
				segments := []eng.Segment{}
				for _, p := range s.Segments {
					if p.StartMinute >= 18*60 {
						p.Kind = "overtime"
						p.OpenEnded = true
						p.EndMinute = 2880
						segments = append(segments, p)
						continue
					}
					if p.EndMinute > 18*60 {
						eid := p.ID + "-evidence"
						s.EvidencePeriods = append(s.EvidencePeriods, eng.EvidencePeriod{ID: eid, StartSecond: p.StartMinute * 60, EndSecond: 2880 * 60, PunchMode: "both", LocalWindows: p.LocalWindows})
						work := p
						work.EndMinute = 17*60 + 30
						work.EvidenceID = eid
						work.LocalWindows = nil
						extra := p
						extra.ID += "-extra"
						extra.Kind = "overtime"
						extra.StartMinute = 18 * 60
						extra.EndMinute = 2880
						extra.OpenEnded = true
						extra.EvidenceID = eid
						extra.LocalWindows = nil
						segments = append(segments, work, extra)
						conflicts = append(conflicts, "连续下午班的17:30至18:00核算口径请确认；源表只注明晚间核定2小时")
					} else {
						segments = append(segments, p)
					}
				}
				s.Segments = segments
			}
		} else {
			conflicts = append(conflicts, "请确认该生产职员个人正班与加班核定口径，不套用其他人员2小时上限")
		}
	}
	attSourceWorkSchedule(&config, source, normal)
	return config, conflicts
}

func localSourceNames(value string) []string {
	value = localNameNote.ReplaceAllString(value, "")
	return strings.FieldsFunc(value, func(r rune) bool { return strings.ContainsRune("、,，;； \n\r", r) })
}

func localRuleGroups(sources []ImportRuleDraft) [][]ImportRuleDraft {
	groups := [][]ImportRuleDraft{}
	indices := map[string]int{}
	for _, s := range sources {
		department := s.DirectoryDepartment
		if department == "" {
			department = s.Department
		}
		key := strings.Join([]string{s.Category, department, s.NamesText, s.Group}, "\x00")
		if i, ok := indices[key]; ok {
			groups[i] = append(groups[i], s)
		} else {
			indices[key] = len(groups)
			groups = append(groups, []ImportRuleDraft{s})
		}
	}
	return groups
}

func localGroupConfig(sources []ImportRuleDraft) (AttConfig, []string) {
	config, conflicts := localSourceConfig(sources[0])
	original := attJSON(config)
	for _, s := range sources[1:] {
		next, notes := localSourceConfig(s)
		if attJSON(next) == original {
			continue
		}
		shiftIDs := map[string]string{}
		for _, shift := range next.Shifts {
			oldID := shift.ID
			shift.ID = fmt.Sprintf("row%d-%s", s.Row, shift.ID)
			shiftIDs[oldID] = shift.ID
			if shift.OvertimeVariant != nil {
				shift.OvertimeVariant.Shift.ID = shift.ID + "-overtime"
			}
			config.Shifts = append(config.Shifts, shift)
		}
		if attHasWorkSchedule(config) && attHasWorkSchedule(next) {
			pairs := []struct {
				to   *[]string
				from []string
			}{
				{&config.WorkSchedule.WeekdayShiftIDs, next.WorkSchedule.WeekdayShiftIDs},
				{&config.WorkSchedule.SaturdayShiftIDs, next.WorkSchedule.SaturdayShiftIDs},
				{&config.WorkSchedule.RestdayShiftIDs, next.WorkSchedule.RestdayShiftIDs},
				{&config.WorkSchedule.DutyShiftIDs, next.WorkSchedule.DutyShiftIDs},
			}
			for _, pair := range pairs {
				for _, oldID := range pair.from {
					if id := shiftIDs[oldID]; id != "" {
						*pair.to = append(*pair.to, id)
					}
				}
			}
		}
		conflicts = append(conflicts, notes...)
	}
	if len(sources) > 1 && len(config.Shifts) > 1 {
		config.ShiftSelection = "auto"
		ids := []string{}
		for _, s := range config.Shifts {
			ids = append(ids, s.ID)
		}
		if attHasWorkSchedule(config) {
			config.Cycle, _ = attWorkScheduleCycle(config)
		} else {
			for i := range config.Cycle.Weekdays {
				config.Cycle.Weekdays[i].CandidateShiftIDs = &ids
			}
		}
	}
	seen := map[string]bool{}
	unique := []string{}
	for _, s := range conflicts {
		if !seen[s] {
			seen[s] = true
			unique = append(unique, s)
		}
	}
	return config, unique
}

// Exact identities and the latest frozen baseline's category are used for
// draft scopes. Unknown categories never inherit a whole department's policy.
func resolveImportedScopes(ctx context.Context, p ImportPreview) (map[string]AttScope, map[string][]string, error) {
	scopes, issues := map[string]AttScope{}, map[string][]string{}
	people, err := attEmployees(ctx)
	if err != nil {
		return nil, nil, err
	}
	categories := map[string]string{}
	baselines, err := core.Model("biz_att_import").Ctx(ctx).Where("kind", "baseline").OrderAsc("month").All()
	if err != nil {
		return nil, nil, err
	}
	for _, row := range baselines {
		var b ImportPreview
		var mapped map[string]string
		if json.Unmarshal([]byte(row["preview_json"].String()), &b) != nil || json.Unmarshal([]byte(row["mappings_json"].String()), &mapped) != nil {
			continue
		}
		for _, e := range b.Employees {
			if id := mapped[e.SourceKey]; id != "" {
				cat := "职员"
				if strings.Contains(e.Sheet, "生产职员") {
					cat = "生产职员"
				} else if strings.Contains(e.Sheet, "员工") {
					cat = "员工"
				}
				categories[id] = cat
			}
		}
	}
	byName := map[string][]AttEmployee{}
	byNo := map[string][]AttEmployee{}
	for _, e := range people {
		if e.EmployeeNo != "" {
			byNo[e.EmployeeNo] = append(byNo[e.EmployeeNo], e)
		}
		byName[e.Name] = append(byName[e.Name], e)
	}
	depts, err := core.HostModel("auth_dept").Ctx(ctx).Fields("id,pid,name").Where("status", 0).All()
	if err != nil {
		return nil, nil, err
	}
	parents := map[int64]int64{}
	byDept := map[string][]int64{}
	for _, d := range depts {
		id := d["id"].Int64()
		parents[id] = d["pid"].Int64()
		byDept[d["name"].String()] = append(byDept[d["name"].String()], id)
	}
	aliases := map[string]string{"财务部": "财富中心", "商务部": "商务", "采购部": "采购中心"}
	explicit := map[string]bool{}
	namedCategories := map[string]map[string]bool{}
	for _, r := range p.Rules {
		for _, name := range localSourceNames(r.NamesText) {
			if len(byName[name]) == 1 {
				explicit[byName[name][0].ID] = true
				if namedCategories[name] == nil {
					namedCategories[name] = map[string]bool{}
				}
				namedCategories[name][r.Category] = true
			}
		}
	}
	for _, r := range p.Rules {
		ids := []string{}
		notes := []string{}
		if names := localSourceNames(r.NamesText); len(names) > 0 {
			for _, name := range names {
				matches := byName[name]
				if len(namedCategories[name]) > 1 {
					notes = append(notes, "人员“"+name+"”在不同考勤分类重复列出，请确认适用分类与制度")
				}
				if len(matches) == 1 && matches[0].EmployeeNo != "" && len(byNo[matches[0].EmployeeNo]) == 1 {
					ids = append(ids, matches[0].ID)
				} else {
					notes = append(notes, "人员“"+name+"”未找到唯一有效姓名与工号映射，请确认")
				}
			}
		} else {
			dept := r.DirectoryDepartment
			if dept == "" {
				dept = r.Department
			}
			matches := byDept[dept]
			if len(matches) == 0 && aliases[dept] != "" {
				matches = byDept[aliases[dept]]
			}
			if len(matches) != 1 {
				notes = append(notes, "请确认本地部门映射："+dept)
			} else {
				root := matches[0]
				for _, e := range people {
					member := false
					seen := map[int64]bool{}
					for id := e.DeptID; id > 0 && !seen[id]; id = parents[id] {
						seen[id] = true
						if id == root {
							member = true
							break
						}
					}
					if member && !explicit[e.ID] && categories[e.ID] == r.Category {
						ids = append(ids, e.ID)
					}
				}
				if r.Group != "职员" && r.Group != "员工" && r.Group != "生产职员" && r.Group != "" {
					ids = nil
					notes = append(notes, "请按姓名、工号指定“"+r.Group+"”人员，不能覆盖整个部门")
				}
			}
		}
		sort.Strings(ids)
		if len(ids) == 0 {
			notes = append(notes, "适用人员尚未确认")
		}
		scopes[r.SourceKey] = AttScope{EmployeeIDs: ids}
		issues[r.SourceKey] = notes
	}
	return scopes, issues, nil
}

// Explicit administrative migration: preserve evidence, baseline imports and
// old versions; create reviewable local drafts, never silently publish windows.
func RebuildLocalAttendanceSources(ctx context.Context, importID string) ([]string, error) {
	return createLocalImportedDrafts(ctx, importID, true)
}

func createLocalImportedDrafts(ctx context.Context, importID string, replaceLegacy bool) ([]string, error) {
	attMu.Lock()
	defer attMu.Unlock()
	_, p, _, err := attReadImport(ctx, importID)
	if err != nil {
		return nil, err
	}
	if p.Kind != "rules" {
		return nil, fmt.Errorf("仅上班时间表可重建本地制度")
	}
	scopes, notes, err := resolveImportedScopes(ctx, p)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	err = core.DB().Transaction(ctx, func(c context.Context, tx gdb.TX) error {
		old, err := core.Model("biz_att_rule").Ctx(c).All()
		if err != nil {
			return err
		}
		for _, r := range old {
			if !replaceLegacy {
				continue
			}
			var source map[string]json.RawMessage
			_ = json.Unmarshal([]byte(r["source_json"].String()), &source)
			if source["local_rebuild"] != nil {
				var previousImport string
				_ = json.Unmarshal(source["import_id"], &previousImport)
				if previousImport != importID && r["status"].String() != "archived" && r["status"].String() != "superseded" {
					return fmt.Errorf("已存在另一份来源建立的本地制度，请先在后台比较并修订版本，不能再次重建覆盖已确认配置")
				}
				continue
			}
			if r["id"].String() != "production-one-conditional-draft-20261029" && source["import_id"] == nil && source["snapshot_id"] == nil {
				continue
			}
			if r["status"].String() == "archived" || r["status"].String() == "superseded" {
				continue
			}
			status := "archived"
			if r["status"].String() == "published" {
				status = "superseded"
			}
			if _, err = core.Model("biz_att_rule").Ctx(c).Where("id", r["id"]).Update(g.Map{"status": status, "updated_at": core.Now()}); err != nil {
				return err
			}
			if err = attAudit(c, "rule", r["id"].String(), "local_rebuild", "按确认方案重建本地制度，原版本保留；缺窗口须配置后生效", r, g.Map{"status": status, "effective_from": "2026-10-01"}); err != nil {
				return err
			}
		}
		for _, group := range localRuleGroups(p.Rules) {
			s := group[0]
			id := externalID("local-rule", p.SHA256+":"+s.SourceKey)
			ids = append(ids, id)
			exists, err := core.Model("biz_att_rule").Ctx(c).Where("id", id).Exist()
			if err != nil {
				return err
			}
			if exists {
				continue
			}
			config, conflicts := localGroupConfig(group)
			config.Rule.ID = id
			config.Rule.Version = "1"
			conflicts = append(conflicts, notes[s.SourceKey]...)
			family, version := id, 1
			if s.Category == "员工" && s.Department == "一部" {
				for _, r := range old {
					if r["id"].String() == "production-one-conditional-draft-20261029" {
						family = r["family_id"].String()
						version = r["version"].Int() + 1
						config.Rule.Version = strconv.Itoa(version)
					}
				}
			}
			name := strings.TrimSpace(s.Department + " " + s.NamesText + " " + s.Group)
			_, err = core.Model("biz_att_rule").Ctx(c).Data(g.Map{"id": id, "family_id": family, "version": version, "name": name, "status": "draft", "effective_from": "2026-10-01", "effective_to": "", "config_json": attJSON(config), "scope_json": attJSON(scopes[s.SourceKey]), "conflicts_json": attJSON(conflicts), "source_json": attJSON(g.Map{"local_rebuild": "2026-11-01", "import_id": importID, "source": s, "sources": group, "scope_basis": "按唯一姓名及工号、当前组织、最新核定基准分类建立草稿范围；未映射的新员工和调岗须另行确认，不恢复过滤人员", "confirmed_corrections": "品质第三组无加班17:30、有加班17:15；分切白班20点核10小时，21点核11小时；未写明窗口待配置"}), "created_by": core.UID(c), "created_at": core.Now(), "updated_at": core.Now()}).Insert()
			if err != nil {
				return err
			}
			if err = attAudit(c, "rule", id, "rebuild_draft", "从2026-10-01修订，用户要求未写明窗口先留待配置", nil, g.Map{"source": s.SourceKey, "family_id": family, "version": version}); err != nil {
				return err
			}
		}
		_, err = core.Model("biz_att_import").Ctx(c).Where("id", importID).Update(g.Map{"status": "drafts_created"})
		return err
	})
	return ids, err
}
