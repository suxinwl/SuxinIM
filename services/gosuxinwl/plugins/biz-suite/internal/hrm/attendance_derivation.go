package hrm

import (
	"context"
	"crypto/sha256"
	"fmt"
	"runtime"
	"sort"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
)

func attEnsureDerivedRosters(ctx context.Context, request AttBatchRequest) error {
	_, err := attPrepareDerivedRosters(ctx, request)
	return err
}

func attPrepareDerivedRosters(ctx context.Context, request AttBatchRequest) (string, error) {
	return attDeriveRosterChunks(ctx, request, attEnsureDerivedRostersLocked)
}

// Only one employee's bounded date range holds the shared writer lock. Rule
// publishing, manual roster edits and month closing can run between employees.
// Each chunk still performs its month-lock checks and writes under the same
// lock as those actions; no worker write can cross a completed month close.
func attDeriveRosterChunks(ctx context.Context, request AttBatchRequest, derive func(context.Context, AttBatchRequest) error) (string, error) {
	var baseline string
	for _, employee := range request.EmployeeIDs {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		err := func() error {
			attMu.Lock()
			defer attMu.Unlock()
			current, err := attDerivedInputs(ctx, request)
			if err != nil {
				return err
			}
			if baseline == "" {
				baseline = current
			} else if current != baseline {
				return fmt.Errorf("自动排班期间制度或人员范围发生变化，请重新试算")
			}
			chunk := request
			chunk.EmployeeIDs = []string{employee}
			return derive(ctx, chunk)
		}()
		if err != nil {
			return "", err
		}
		// Let already waiting request goroutines run before the worker attempts
		// to reacquire the mutex for another employee.
		runtime.Gosched()
	}
	return baseline, nil
}

// Compare policy inputs rather than generated roster rows, which this worker
// intentionally updates. Ignore timestamps/drafts so an unrelated draft save
// does not fail an otherwise valid calculation.
func attDerivedInputs(ctx context.Context, request AttBatchRequest) (string, error) {
	start, err := attDate(request.StartDate)
	if err != nil {
		return "", err
	}
	end, err := attDate(request.EndDate)
	if err != nil {
		return "", err
	}
	rules, parents, err := attPublishedRules(ctx)
	if err != nil {
		return "", err
	}
	sort.Slice(rules, func(i, j int) bool { return rules[i].ID < rules[j].ID })
	people, err := attEmployees(ctx)
	if err != nil {
		return "", err
	}
	selected := map[string]AttEmployee{}
	for _, id := range request.EmployeeIDs {
		if person, ok := people[id]; ok {
			selected[id] = person
		}
	}
	holidayRows, err := core.Model("biz_hrm_attendance_holiday").Ctx(ctx).
		WhereBetween("holiday_date", start.AddDate(0, 0, -1).Format("2006-01-02"), end.AddDate(0, 0, 1).Format("2006-01-02")).
		Fields("holiday_date,is_workday").Order("holiday_date,id").All()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(attJSON(g.Map{"rules": rules, "parents": parents, "people": selected, "holidays": holidayRows})))
	return fmt.Sprintf("%x", sum), nil
}

func attVerifyDerivedInputs(ctx context.Context, request AttBatchRequest, baseline string) error {
	attMu.Lock()
	defer attMu.Unlock()
	current, err := attDerivedInputs(ctx, request)
	if err != nil {
		return err
	}
	if baseline != current {
		return fmt.Errorf("自动排班后制度或人员范围发生变化，请重新试算")
	}
	return nil
}
