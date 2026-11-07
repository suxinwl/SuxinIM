package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/suxinwl/GoSuxin/framework/contrib/drivers/mysql"
	_ "github.com/suxinwl/GoSuxin/framework/contrib/drivers/sqlite"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/os/gcfg"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/hrm"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/im"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/migration"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/wecom"
)

type report struct {
	Action      string `json:"action"`
	OK          bool   `json:"ok"`
	File        string `json:"file,omitempty"`
	ID          string `json:"id,omitempty"`
	Month       string `json:"month,omitempty"`
	Kind        string `json:"kind,omitempty"`
	Count       int    `json:"count"`
	Failed      int    `json:"failed,omitempty"`
	Status      string `json:"status,omitempty"`
	Jobs        int    `json:"jobs,omitempty"`
	PartialJobs int    `json:"partial_jobs,omitempty"`
	Mapped      int    `json:"mapped,omitempty"`
	Drafts      int    `json:"drafts,omitempty"`
	Issues      int    `json:"issues,omitempty"`
	Date        string `json:"date,omitempty"`
	Start       string `json:"start,omitempty"`
	End         string `json:"end,omitempty"`
	ErrorCode   int    `json:"error_code,omitempty"`
	Error       string `json:"error,omitempty"`
	Skipped     bool   `json:"skipped,omitempty"`
	Note        string `json:"note,omitempty"`
}

func emit(value any) { _ = json.NewEncoder(os.Stdout).Encode(value) }
func failure(action string, err error) report {
	r := report{Action: action, Error: "操作未完成，请检查对应配置或后台诊断记录"}
	var api *wecom.APIError
	var partial *wecom.PartialError
	if errors.As(err, &partial) {
		r.Status = "partial"
		r.Failed = max(partial.Failed, 0)
		r.Error = "部分记录未能规范化，请查看同步异常；继续处理后续窗口"
	} else if errors.As(err, &api) {
		r.ErrorCode = api.Code
		r.Error = "企业微信接口返回错误"
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		r.Error = "操作取消或超时"
	} else if errors.Is(err, wecom.ErrSyncRunning) {
		r.Error = "已有同步任务运行，请稍后再试"
	}
	return r
}

func run() (code int) {
	defer func() {
		if recover() != nil {
			emit(report{Action: "initialize", Error: "初始化失败，请检查宿主配置；连接信息未输出"})
			code = 1
		}
	}()
	config := flag.String("config", "", "宿主配置文件；默认沿用GoSuxin配置发现")
	data := flag.String("plugin-data", "", "现有插件私有数据绝对路径；默认SUXIN_PLUGIN_DATA")
	docs := flag.String("docs", "docs", "包含8月、9月和部门上班时间工作簿的目录")
	imports := flag.Bool("import-sources", false, "导入3份来源工作簿并仅生成制度草稿")
	rules := flag.Bool("wecom-rules", false, "已停用，仅保留旧命令兼容提示")
	rebuild := flag.Bool("rebuild-local-rules", false, "重建本地规则草稿并归档旧来源规则，不发布缺少窗口的制度")
	actor := flag.Int64("actor", 0, "本地规则修订审计操作人的有效宿主管理员ID")
	recalcStart := flag.String("recalculate-start", "", "本地重算开始日期YYYY-MM-DD")
	recalcEnd := flag.String("recalculate-end", "", "本地重算结束日期YYYY-MM-DD（含当天）")
	probe := flag.Bool("probe-history", false, "只对1名有效映射员工探测2026-08-03/08-10/09-01数据是否可读")
	month := flag.String("backfill-month", "", "显式回填YYYY-MM打卡及请假/外出/出差/加班；不改组织")
	dateStart := flag.String("backfill-start", "", "指定回填开始日期YYYY-MM-DD；须同时提供结束日期，不与月份同时使用")
	dateEnd := flag.String("backfill-end", "", "指定回填结束日期YYYY-MM-DD（含当天）；最多7个已结束日期")
	flag.Parse()
	if *rules {
		emit(report{Action: "wecom-rules", Error: "企业微信考勤规则同步已停用，请配置本地制度"})
		return 2
	}
	if !*imports && !*rules && !*rebuild && *recalcStart == "" && *recalcEnd == "" && !*probe && *month == "" && *dateStart == "" && *dateEnd == "" {
		flag.Usage()
		return 2
	}
	if e := validateBackfillOptions(*month, *dateStart, *dateEnd, time.Now()); e != nil {
		emit(report{Action: "validate", Error: e.Error()})
		return 2
	}
	if *data != "" {
		if !filepath.IsAbs(*data) {
			emit(report{Action: "validate", Error: "插件数据目录必须为既有绝对路径"})
			return 2
		}
		info, e := os.Stat(*data)
		if e != nil || !info.IsDir() {
			emit(report{Action: "validate", Error: "插件数据目录不存在"})
			return 2
		}
		if e = os.Setenv("SUXIN_PLUGIN_DATA", *data); e != nil {
			return 1
		}
	}
	if *config != "" {
		adapter, ok := g.Cfg().GetAdapter().(*gcfg.AdapterFile)
		if !ok {
			emit(report{Action: "initialize", Error: "无法读取宿主配置适配器"})
			return 1
		}
		adapter.SetFileName(*config)
	}
	if e := core.Init(g.DB()); e != nil {
		emit(failure("initialize", e))
		return 1
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	if e := migration.EnsureSchema(ctx, im.Schema()...); e != nil {
		emit(failure("schema", e))
		return 1
	}
	defer core.DB().Close(context.Background())
	// A CLI is a separate process: it cannot rely on a worker's in-memory mutex.
	busy, e := core.Model("wecom_sync_run").Ctx(ctx).Where("status", "running").Exist()
	if e != nil {
		emit(failure("sync-status", e))
		return 1
	}
	if busy {
		emit(failure("sync-status", wecom.ErrSyncRunning))
		return 1
	}
	if *rebuild || *recalcStart != "" || *recalcEnd != "" {
		ctx = context.WithValue(ctx, "uid", *actor)
		active, err := core.HostUserExists(ctx, *actor)
		if err != nil || !active || !core.Has(ctx, "biz-suite.hrm.manage") {
			emit(report{Action: "local-rules", Error: "须通过--actor指定具有HRM管理权限的有效宿主用户"})
			return 2
		}
		if (*recalcStart == "") != (*recalcEnd == "") {
			emit(report{Action: "validate", Error: "本地重算须同时指定开始、结束日期"})
			return 2
		}
		if *rebuild {
			if err := rebuildLocal(ctx, *docs); err != nil {
				emit(report{Action: "rebuild-local-rules", Error: err.Error()})
				return 1
			}
		}
		if *recalcStart != "" {
			ids, err := hrm.QueueAttendanceRecalculation(ctx, *recalcStart, *recalcEnd, nil)
			if err != nil {
				emit(report{Action: "recalculate-local", Error: err.Error()})
				return 1
			}
			emit(report{Action: "recalculate-local", OK: true, Start: *recalcStart, End: *recalcEnd, Jobs: len(ids), Note: "仅使用本地证据，后台处理；窗口未配置的人员显示待配置"})
		}
	}
	if *imports {
		if e := importSources(ctx, *docs); e != nil {
			return 1
		}
	}
	if !*rules && !*probe && *month == "" && *dateStart == "" {
		return 0
	}
	client := wecom.NewFromEnv()
	if e := client.LoadSettings(ctx); e != nil {
		emit(failure("credentials", e))
		return 1
	}
	wecom.SetAttendanceChangeHook(hrm.AttendanceChanged)
	if *probe {
		results, e := client.ProbeHistoricalCheckins(ctx, []string{"2026-08-03", "2026-08-10", "2026-09-01"})
		if e != nil {
			emit(failure("probe-history", e))
			return 1
		}
		failed := false
		for _, r := range results {
			emit(report{Action: "probe-history", Date: r.Date, Count: r.Count, OK: r.OK, ErrorCode: r.ErrorCode, Error: r.Error})
			failed = failed || !r.OK
		}
		if failed {
			return 1
		}
	}
	if *month != "" {
		if e := backfill(ctx, client, *month); e != nil {
			return 1
		}
	}
	if *dateStart != "" {
		start, end, e := completedDateRange(*dateStart, *dateEnd, time.Now())
		if e != nil {
			emit(report{Action: "validate", Error: "回填日期须为最多7个连续已结束日期"})
			return 2
		}
		if e := backfillPeriod(ctx, client, "", start, end); e != nil {
			return 1
		}
	}
	return 0
}

func rebuildLocal(ctx context.Context, dir string) error {
	file, err := os.Open(filepath.Join(dir, "部门上班时间.xlsx"))
	if err != nil {
		return fmt.Errorf("无法打开部门上班时间工作簿")
	}
	defer file.Close()
	id, preview, _, err := hrm.ImportAttendanceEvidence(ctx, file, "部门上班时间.xlsx")
	if err != nil {
		return fmt.Errorf("来源工作簿导入失败: %w", err)
	}
	ids, err := hrm.RebuildLocalAttendanceSources(ctx, id)
	if err != nil {
		return err
	}
	emit(report{Action: "rebuild-local-rules", OK: true, ID: id, Count: len(preview.Rules), Drafts: len(ids), Note: "旧来源规则已保留审计并退出可用范围；新草稿从2026-10-01起，未知窗口留空待配置；8、9月基准未修改"})
	return nil
}

func importSources(ctx context.Context, dir string) error {
	for _, name := range []string{"8月考勤数据.xlsx", "9月考勤数据.xlsx", "部门上班时间.xlsx"} {
		file, e := os.Open(filepath.Join(dir, name))
		if e != nil {
			emit(report{Action: "import-source", File: name, Error: "无法打开来源文件"})
			return errors.New("source unavailable")
		}
		id, preview, mappings, e := hrm.ImportAttendanceEvidence(ctx, file, name)
		_ = file.Close()
		if e != nil {
			r := failure("import-source", e)
			r.File = name
			emit(r)
			return e
		}
		r := report{Action: "import-source", OK: true, File: name, ID: id, Kind: preview.Kind, Month: preview.Month, Count: len(preview.Employees), Mapped: len(mappings), Issues: len(preview.Issues)}
		if preview.Kind == "rules" {
			row, e := core.Model("biz_att_import").Ctx(ctx).Fields("status").Where("id", id).One()
			if e != nil {
				emit(failure("rule-draft-status", e))
				return e
			}
			if row["status"].String() != "drafts_created" {
				ids, e := hrm.CreateImportedRuleDrafts(ctx, id)
				if e != nil {
					emit(failure("import-rule-drafts", e))
					return e
				}
				r.Drafts = len(ids)
			}
		}
		emit(r)
	}
	return nil
}

func monthRange(value string, now time.Time) (time.Time, time.Time, error) {
	start, e := time.ParseInLocation("2006-01", value, core.Location)
	if e != nil || start.Format("2006-01") != value || strings.TrimSpace(value) != value {
		return time.Time{}, time.Time{}, errors.New("invalid month")
	}
	end := start.AddDate(0, 1, 0)
	if end.After(now) {
		return time.Time{}, time.Time{}, errors.New("month is not complete")
	}
	return start, end, nil
}

func validateBackfillOptions(month, first, last string, now time.Time) error {
	if first != "" || last != "" {
		if month != "" {
			return errors.New("回填月份与指定日期区间不能同时使用")
		}
		if _, _, e := completedDateRange(first, last, now); e != nil {
			return errors.New("须同时填写开始和结束日期YYYY-MM-DD，区间含结束日、最多7天且全部日期已结束")
		}
	} else if month != "" {
		if _, _, e := monthRange(month, now); e != nil {
			return errors.New("回填月份须是已结束的YYYY-MM月份")
		}
	}
	return nil
}

// The public end date is inclusive. Internally use [start, end) so the shared
// runner can pass exactly 23:59:59 on the last requested date to WeCom.
func completedDateRange(first, last string, now time.Time) (time.Time, time.Time, error) {
	start, e := time.ParseInLocation("2006-01-02", first, core.Location)
	if e != nil || start.Format("2006-01-02") != first {
		return time.Time{}, time.Time{}, errors.New("invalid start date")
	}
	lastDay, e := time.ParseInLocation("2006-01-02", last, core.Location)
	if e != nil || lastDay.Format("2006-01-02") != last {
		return time.Time{}, time.Time{}, errors.New("invalid end date")
	}
	end := lastDay.AddDate(0, 0, 1)
	if lastDay.Before(start) || end.After(start.AddDate(0, 0, 7)) || end.After(now) {
		return time.Time{}, time.Time{}, errors.New("range must contain at most seven completed dates")
	}
	return start, end, nil
}

func backfill(ctx context.Context, client *wecom.Client, month string) error {
	start, end, e := monthRange(month, time.Now())
	if e != nil {
		return e
	}
	return backfillPeriod(ctx, client, month, start, end)
}

func backfillPeriod(ctx context.Context, client *wecom.Client, month string, start, end time.Time) error {
	kinds := []string{"checkin"}
	for _, kind := range []string{"leave", "out", "trip", "overtime"} {
		enabled, e := core.Model("wecom_approval_template").Ctx(ctx).Where("approval_type", kind).Where("enabled", 1).Exist()
		if e != nil {
			emit(failure("approval-template", e))
			return e
		}
		if enabled {
			kinds = append(kinds, kind)
		} else {
			r := report{Action: "backfill", Kind: kind, Month: month, Skipped: true, Note: "该类审批模板未启用，未执行同步"}
			if month == "" {
				r.Start, r.End = start.Format(time.RFC3339), end.Add(-time.Second).Format(time.RFC3339)
			}
			emit(r)
		}
	}
	return backfillWindows(ctx, month, start, end, kinds, client.Run, func(r report) { emit(r) })
}

// Isolated from configuration/database setup so continuation and fatal-stop
// behavior can be verified without touching production credentials or data.
func backfillWindows(ctx context.Context, month string, start, end time.Time, kinds []string, execute func(context.Context, string, time.Time, time.Time) (string, int, error), output func(report)) error {
	summary := report{Action: "backfill-summary", Month: month, Status: "success", OK: true}
	if month == "" {
		summary.Start, summary.End = start.Format(time.RFC3339), end.Add(-time.Second).Format(time.RFC3339)
	}
	for from := start; from.Before(end); {
		until := from.AddDate(0, 0, 7)
		if until.After(end) {
			until = end
		}
		for _, kind := range kinds {
			job, cancel := context.WithTimeout(ctx, 30*time.Minute)
			id, count, e := execute(job, kind, from, until.Add(-time.Second))
			cancel()
			r := report{Action: "backfill", Kind: kind, Month: month, Start: from.Format(time.RFC3339), End: until.Add(-time.Second).Format(time.RFC3339), ID: id, Count: count, OK: e == nil, Status: "success"}
			summary.Jobs++
			summary.Count += count
			if e != nil {
				safe := failure("backfill", e)
				r.Error, r.ErrorCode, r.Failed = safe.Error, safe.ErrorCode, safe.Failed
				r.Status = "failed"
				var partial *wecom.PartialError
				if errors.As(e, &partial) {
					r.Status = "partial"
					summary.Status = "partial"
					summary.OK = false
					summary.PartialJobs++
					summary.Failed += r.Failed
					output(r)
					continue
				}
			}
			output(r)
			if e != nil {
				summary.OK = false
				summary.Status = "failed"
				summary.Error, summary.ErrorCode = r.Error, r.ErrorCode
				output(summary)
				return e
			}
		}
		from = until
	}
	if summary.PartialJobs > 0 {
		summary.Error = "全部类型与时间窗已处理，仍有未规范化记录，请核对同步异常"
		output(summary)
		return &wecom.PartialError{Failed: summary.Failed}
	}
	output(summary)
	return nil
}

func main() { os.Exit(run()) }
