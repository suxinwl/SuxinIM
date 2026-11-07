package middleware

import "testing"

func TestDepartmentCandidateAndSwitchUseExistingDepartmentRBACOnly(t *testing.T) {
	candidates := departmentPermissionPaths("/admin/system/dept/getLeaderOptions")
	foundRead, foundWrite := false, false
	for _, path := range candidates {
		if path == "/admin/system/dept/getList" {
			foundRead = true
		}
		if path == "/admin/system/dept/save" {
			foundWrite = true
		}
	}
	if !foundRead || !foundWrite {
		t.Fatal("department admins cannot read owner candidates")
	}
	for _, path := range departmentPermissionPaths("/admin/system/dept/upIMGroup") {
		if path == "/admin/system/dept/getList" {
			t.Fatal("read-only department grant can toggle groups")
		}
	}
	paths := departmentPermissionPaths("/admin/system/account/save")
	if len(paths) != 1 || paths[0] != "/admin/system/account/save" {
		t.Fatal("unrelated route gained department privileges")
	}
}
