package enterprise

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"openim38.local/identity"
)

type departmentCall struct {
	path string
	body request
}
type departmentFixture struct {
	IM
	sync.Mutex
	group, owner, name, failPath string
	muted                        bool
	exists                       bool
	members                      []User
	calls                        []departmentCall
	memberReads                  int
	memberMutes                  map[string]int64
}

func (f *departmentFixture) GroupMembers(context.Context, string) ([]User, error) {
	f.Lock()
	defer f.Unlock()
	f.memberReads++
	return append([]User(nil), f.members...), nil
}
func (f *departmentFixture) snapshot() ([]departmentCall, []User, string) {
	f.Lock()
	defer f.Unlock()
	return append([]departmentCall(nil), f.calls...), append([]User(nil), f.members...), f.owner
}
func (f *departmentFixture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var q request
	_ = json.NewDecoder(r.Body).Decode(&q)
	f.Lock()
	f.calls = append(f.calls, departmentCall{r.URL.Path, q})
	if r.URL.Path == f.failPath {
		f.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"errCode": 1006, "errMsg": "injected department dependency failure"})
		return
	}
	var out any = map[string]any{}
	switch r.URL.Path {
	case "/group/get_groups_info":
		groups := []Group{}
		if f.exists {
			groups = append(groups, Group{GroupID: f.group, GroupName: f.name, OwnerUserID: f.owner})
		}
		out = map[string]any{"groupInfos": groups}
	case "/group/create_group":
		f.exists, f.owner = true, q.str("ownerUserID")
		var info struct {
			GroupName string `json:"groupName"`
		}
		_ = q.decode("groupInfo", &info)
		f.name = info.GroupName
		f.members = []User{{UserID: f.owner, RoleLevel: 100}}
		for _, id := range q.strings("memberUserIDs") {
			f.members = append(f.members, User{UserID: id, RoleLevel: 20})
		}
	case "/group/invite_user_to_group":
		for _, id := range q.strings("invitedUserIDs") {
			f.members = append(f.members, User{UserID: id, RoleLevel: 20})
		}
	case "/group/transfer_group":
		f.owner = q.str("newOwnerUserID")
		for n := range f.members {
			f.members[n].RoleLevel = 20
			if f.members[n].UserID == f.owner {
				f.members[n].RoleLevel = 100
			}
		}
	case "/group/kick_group":
		clean := []User{}
		for _, member := range f.members {
			if !contains(q.strings("kickedUserIDs"), member.UserID) {
				clean = append(clean, member)
			}
		}
		f.members = clean
	case "/group/set_group_info_ex":
		f.name = q.str("groupName")
	case "/group/mute_group":
		f.muted = true
	case "/group/cancel_mute_group":
		f.muted = false
	case "/group/mute_group_member":
		f.memberMutes[q.str("userID")] = q.num("mutedSeconds")
	case "/group/cancel_mute_group_member":
		delete(f.memberMutes, q.str("userID"))
	}
	f.Unlock()
	_ = json.NewEncoder(w).Encode(map[string]any{"errCode": 0, "data": out})
}

func departmentBridge(t *testing.T, exists bool) (*HRMBridge, *departmentFixture, request) {
	t.Helper()
	r := miniredis.RunT(t)
	db := redis.NewClient(&redis.Options{Addr: r.Addr()})
	t.Cleanup(func() { _ = db.Close() })
	state := &identity.Store{Redis: db}
	for _, user := range []string{"emp_leader", "emp_member"} {
		if err := state.PutProjection(context.Background(), identity.Projection{UserID: user, Enabled: true, Version: 1}); err != nil {
			t.Fatal(err)
		}
	}
	f := &departmentFixture{exists: exists, group: "dept_" + identity.Hash("department:42")[:32], owner: "hrm-im-system", members: []User{{UserID: "hrm-im-system", RoleLevel: 100}, {UserID: "emp_old", RoleLevel: 20}}}
	server := httptest.NewServer(f)
	t.Cleanup(server.Close)
	b := &HRMBridge{State: state, Server: &Server{Store: NewMemoryStore(), IM: f, Upstream: &Upstream{IMBase: server.URL, adminToken: "fixture-admin", tokenExpiry: time.Now().Add(time.Hour)}}}
	q := asRequest(map[string]any{"departmentID": "42", "name": "研发部", "version": 1, "groupEnabled": true, "leaderUserID": "emp_leader", "memberUserIDs": []string{"emp_leader", "emp_member"}, "frozen": false})
	return b, f, q
}

func TestDepartmentGroupsDefaultOffAndExplicitOffPreserveExistingState(t *testing.T) {
	for _, exists := range []bool{false, true} {
		b, f, q := departmentBridge(t, exists)
		beforeCalls, beforeMembers, beforeOwner := f.snapshot()
		delete(q, "groupEnabled")
		if _, err := b.reconcileDepartment(context.Background(), q); err != nil {
			t.Fatal(err)
		}
		calls, members, owner := f.snapshot()
		if len(calls) != len(beforeCalls) || !reflect.DeepEqual(members, beforeMembers) || owner != beforeOwner {
			t.Fatal("off/default department modified native group")
		}
		q["version"], q["groupEnabled"] = json.RawMessage(`2`), json.RawMessage(`false`)
		if _, err := b.reconcileDepartment(context.Background(), q); err != nil {
			t.Fatal(err)
		}
		calls, _, _ = f.snapshot()
		if len(calls) != 0 {
			t.Fatal("explicit off synchronized group")
		}
	}
}

func TestDepartmentGroupOnRequiresEligibleIncludedLeaderBeforeUpstream(t *testing.T) {
	for _, leader := range []string{"", "hrm-im-system", "emp_missing"} {
		b, f, q := departmentBridge(t, false)
		q["leaderUserID"] = mustJSON(leader)
		if _, err := b.reconcileDepartment(context.Background(), q); err == nil {
			t.Fatal("invalid leader accepted")
		}
		calls, _, _ := f.snapshot()
		if len(calls) != 0 {
			t.Fatal("invalid leader reached native mutation")
		}
	}
	b, f, q := departmentBridge(t, false)
	_ = b.State.PutProjection(context.Background(), identity.Projection{UserID: "emp_leader", Enabled: false, Version: 2})
	if _, err := b.reconcileDepartment(context.Background(), q); err == nil {
		t.Fatal("disabled leader accepted")
	}
	calls, _, _ := f.snapshot()
	if len(calls) != 0 {
		t.Fatal("disabled leader reached upstream")
	}
}

func TestDepartmentGroupCreateUsesLeaderAsOwnerWithoutDuplicatingMembership(t *testing.T) {
	b, f, q := departmentBridge(t, false)
	if _, err := b.reconcileDepartment(context.Background(), q); err != nil {
		t.Fatal(err)
	}
	calls, members, owner := f.snapshot()
	if owner != "emp_leader" || len(members) != 2 {
		t.Fatal("department owner/members not HRM controlled")
	}
	created := false
	for _, call := range calls {
		if call.path == "/group/create_group" {
			created = true
			if call.body.str("ownerUserID") != "emp_leader" || contains(call.body.strings("memberUserIDs"), "emp_leader") {
				t.Fatal("owner duplicated in ordinary member list")
			}
		}
		if call.path == "/user/user_register" {
			t.Fatal("created legacy service identity instead of leader owned group")
		}
	}
	if !created {
		t.Fatal("enabled department group not created")
	}
}

func TestDepartmentLeaderJoinsThenTakesOwnershipBeforeOldOwnerRemoval(t *testing.T) {
	b, f, q := departmentBridge(t, true)
	if _, err := b.reconcileDepartment(context.Background(), q); err != nil {
		t.Fatal(err)
	}
	calls, members, owner := f.snapshot()
	order := []string{}
	for _, call := range calls {
		if call.path == "/group/invite_user_to_group" || call.path == "/group/transfer_group" || call.path == "/group/kick_group" {
			order = append(order, call.path)
		}
	}
	if !reflect.DeepEqual(order, []string{"/group/invite_user_to_group", "/group/transfer_group", "/group/kick_group"}) {
		t.Fatal("unsafe owner transition order", order)
	}
	if owner != "emp_leader" || len(members) != 2 {
		t.Fatal("leader transfer failed to remove former members")
	}
}

func TestDepartmentFailedOwnerTransferDoesNotKickOldMembersOrMarkApplied(t *testing.T) {
	b, f, q := departmentBridge(t, true)
	f.failPath = "/group/transfer_group"
	if _, err := b.reconcileDepartment(context.Background(), q); err == nil {
		t.Fatal("failed transfer reported success")
	}
	calls, _, owner := f.snapshot()
	for _, call := range calls {
		if call.path == "/group/kick_group" {
			t.Fatal("members removed before successful ownership transfer")
		}
	}
	if owner != "hrm-im-system" {
		t.Fatal("failed transfer changed owner")
	}
	if _, err := b.Server.Store.Get(context.Background(), "hrm_department:42"); !errors.Is(err, ErrNotFound) {
		t.Fatal("failed sync advanced applied projection")
	}
}

func TestDepartmentStaleEnableCannotOverrideRecordedOff(t *testing.T) {
	b, f, q := departmentBridge(t, true)
	q["groupEnabled"], q["version"] = json.RawMessage(`false`), json.RawMessage(`3`)
	if _, err := b.reconcileDepartment(context.Background(), q); err != nil {
		t.Fatal(err)
	}
	q["groupEnabled"], q["version"] = json.RawMessage(`true`), json.RawMessage(`2`)
	if _, err := b.reconcileDepartment(context.Background(), q); err != nil {
		t.Fatal(err)
	}
	calls, _, _ := f.snapshot()
	if len(calls) != 0 {
		t.Fatal("stale enabled version mutated disabled department group")
	}
}

func departmentWrites(calls []departmentCall) []string {
	var paths []string
	for _, call := range calls {
		if call.path != "/group/get_groups_info" {
			paths = append(paths, call.path)
		}
	}
	return paths
}

func TestDepartmentSameVersionStillReadsNativeButEmitsNoDuplicateMutations(t *testing.T) {
	b, f, q := departmentBridge(t, false)
	f.muted = true
	if _, err := b.reconcileDepartment(context.Background(), q); err != nil {
		t.Fatal(err)
	}
	initial, _, _ := f.snapshot()
	before, _ := b.Server.Store.Get(context.Background(), "hrm_department:42")
	for n := 0; n < 3; n++ {
		if _, err := b.reconcileDepartment(context.Background(), q); err != nil {
			t.Fatal(err)
		}
	}
	calls, _, _ := f.snapshot()
	after, _ := b.Server.Store.Get(context.Background(), "hrm_department:42")
	if len(departmentWrites(calls)) != len(departmentWrites(initial)) || len(calls)-len(initial) != 3 || f.memberReads != 4 {
		t.Fatal("same-version reconciliation skipped reads or repeated notifications")
	}
	if before.Version != after.Version || !f.muted {
		t.Fatal("no-diff reconciliation rewrote projection or manual mute state")
	}
}

func TestDepartmentSameVersionRepairsNativeNameMembershipAndOwnerDrift(t *testing.T) {
	b, f, q := departmentBridge(t, false)
	if _, err := b.reconcileDepartment(context.Background(), q); err != nil {
		t.Fatal(err)
	}
	initial, _, _ := f.snapshot()
	f.name, f.owner = "synthetic external rename", "emp_intruder"
	f.members = []User{{UserID: "emp_leader", RoleLevel: 60}, {UserID: "emp_intruder", RoleLevel: 100}}
	if _, err := b.reconcileDepartment(context.Background(), q); err != nil {
		t.Fatal(err)
	}
	calls, members, owner := f.snapshot()
	writes := departmentWrites(calls[len(initial):])
	if !reflect.DeepEqual(writes, []string{"/group/invite_user_to_group", "/group/transfer_group", "/group/kick_group", "/group/set_group_info_ex"}) ||
		owner != "emp_leader" || f.name != q.str("name") || len(members) != 2 {
		t.Fatal("periodic native drift repair lost ordering or desired state", writes)
	}
	if _, err := b.reconcileDepartment(context.Background(), q); err != nil {
		t.Fatal(err)
	}
	final, _, _ := f.snapshot()
	if len(departmentWrites(final)) != len(departmentWrites(calls)) {
		t.Fatal("repaired drift emitted duplicate mutations next cycle")
	}
}

func TestDepartmentReconcileNeverChangesManualMuteOrAdministratorRoles(t *testing.T) {
	for _, muted := range []bool{false, true} {
		for _, frozen := range []bool{false, true} {
			b, f, q := departmentBridge(t, true)
			f.owner, f.name, f.muted = "emp_leader", q.str("name"), muted
			f.memberMutes = map[string]int64{"emp_member": 3600}
			f.members = []User{{UserID: "emp_leader", RoleLevel: 100}, {UserID: "emp_member", RoleLevel: 60}}
			q["frozen"] = mustJSON(frozen)
			for n := 0; n < 2; n++ {
				if _, err := b.reconcileDepartment(context.Background(), q); err != nil {
					t.Fatal(err)
				}
			}
			calls, members, owner := f.snapshot()
			if len(departmentWrites(calls)) != 0 || f.muted != muted || owner != "emp_leader" || members[1].RoleLevel != 60 || len(f.memberMutes) != 1 || f.memberMutes["emp_member"] != 3600 {
				t.Fatal("HRM reconciliation overrode manual group moderation")
			}
		}
	}
}

type departmentFailSave struct {
	Store
	fail bool
}

func (s *departmentFailSave) Save(ctx context.Context, d Document, old int64) error {
	if s.fail && d.ID == "hrm_department:42" {
		s.fail = false
		return errors.New("synthetic acknowledgement unavailable")
	}
	return s.Store.Save(ctx, d, old)
}

func TestDepartmentRetryAfterNativeSuccessButLostACKDoesNotRepeatBusinessWrites(t *testing.T) {
	b, f, q := departmentBridge(t, false)
	b.Server.Store = &departmentFailSave{Store: b.Server.Store, fail: true}
	if _, err := b.reconcileDepartment(context.Background(), q); err == nil {
		t.Fatal("injected ACK failure accepted")
	}
	initial, _, _ := f.snapshot()
	if _, err := b.reconcileDepartment(context.Background(), q); err != nil {
		t.Fatal(err)
	}
	calls, _, _ := f.snapshot()
	if !reflect.DeepEqual(departmentWrites(initial), []string{"/group/create_group"}) || len(departmentWrites(calls)) != 1 {
		t.Fatal("ACK retry repeated already-successful native mutation")
	}
	if _, err := b.Server.Store.Get(context.Background(), "hrm_department:42"); err != nil {
		t.Fatal("retry failed to acknowledge", err)
	}
}

func TestDepartmentRetryAfterFailedTransferSkipsAlreadyInvitedMember(t *testing.T) {
	b, f, q := departmentBridge(t, true)
	f.failPath = "/group/transfer_group"
	if _, err := b.reconcileDepartment(context.Background(), q); err == nil {
		t.Fatal("injected transfer failure accepted")
	}
	initial, _, _ := f.snapshot()
	f.failPath = ""
	if _, err := b.reconcileDepartment(context.Background(), q); err != nil {
		t.Fatal(err)
	}
	calls, _, owner := f.snapshot()
	if owner != "emp_leader" || !reflect.DeepEqual(departmentWrites(calls[len(initial):]), []string{"/group/transfer_group", "/group/kick_group", "/group/set_group_info_ex"}) {
		t.Fatal("partial retry repeated an invitation or lost repair ordering")
	}
}

func TestDepartmentSameVersionEnableAfterOffStillRepairsNative(t *testing.T) {
	b, f, q := departmentBridge(t, true)
	q["groupEnabled"] = mustJSON(false)
	if _, err := b.reconcileDepartment(context.Background(), q); err != nil {
		t.Fatal(err)
	}
	q["groupEnabled"] = mustJSON(true)
	if _, err := b.reconcileDepartment(context.Background(), q); err != nil {
		t.Fatal(err)
	}
	calls, _, owner := f.snapshot()
	if owner != "emp_leader" || len(departmentWrites(calls)) == 0 {
		t.Fatal("same-version re-enable was incorrectly treated as a no-op")
	}
}

func TestDepartmentRealRenameAndLostACKRetryWriteNameExactlyOnce(t *testing.T) {
	b, f, q := departmentBridge(t, false)
	if _, err := b.reconcileDepartment(context.Background(), q); err != nil {
		t.Fatal(err)
	}
	initial, _, _ := f.snapshot()
	q["name"], q["version"] = mustJSON("synthetic renamed department"), mustJSON(2)
	b.Server.Store = &departmentFailSave{Store: b.Server.Store, fail: true}
	if _, err := b.reconcileDepartment(context.Background(), q); err == nil {
		t.Fatal("injected rename acknowledgement failure accepted")
	}
	if _, err := b.reconcileDepartment(context.Background(), q); err != nil {
		t.Fatal(err)
	}
	if _, err := b.reconcileDepartment(context.Background(), q); err != nil {
		t.Fatal(err)
	}
	calls, _, _ := f.snapshot()
	if f.name != q.str("name") || !reflect.DeepEqual(departmentWrites(calls[len(initial):]), []string{"/group/set_group_info_ex"}) {
		t.Fatal("real rename or ACK retry emitted duplicate name notifications")
	}
	var saved struct {
		Version int64 `json:"version"`
	}
	d, err := b.Server.Store.Get(context.Background(), "hrm_department:42")
	if err != nil || json.Unmarshal(d.Data, &saved) != nil || saved.Version != 2 {
		t.Fatal("rename retry failed to acknowledge latest HRM version")
	}
}
