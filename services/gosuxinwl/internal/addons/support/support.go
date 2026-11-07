// Package support provides host integration shared by the source add-ons.
package support

import (
	"context"
	"fmt"
	"os"

	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/os/gtime"
	"github.com/suxinwl/GoSuxin/framework/util/gconv"
)

func Ready() (bool, error) {
	if _, err := os.Stat("devsource/developer/install/install.lock"); os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	if g.DB().GetConfig().Prefix != "gf_" {
		return false, fmt.Errorf("源码插件需要 gf_ 数据表前缀")
	}
	return true, nil
}

func UID(ctx context.Context) int64 { return gconv.Int64(ctx.Value("uid")) }

func CanManageAll(ctx context.Context) bool {
	if UID(ctx) < 1 {
		return false
	}
	roles, err := g.Model("auth_role_access").Ctx(ctx).Where("uid", UID(ctx)).Array("role_id")
	if err != nil || len(roles) == 0 {
		return false
	}
	n, err := g.Model("auth_role").Ctx(ctx).WhereIn("id", roles).Where("rules", "*").Count()
	return err == nil && n > 0
}

type Action struct{ Path, Title string }

// SeedMenu only inserts missing routes. Existing permissions and edits survive reinstalls.
func SeedMenu(ctx context.Context, name, title, component, icon string, order int, actions []Action) error {
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		add := func(route string, data g.Map) (int64, error) {
			row, err := tx.Model("auth_rule").Where("routename", route).One()
			if err != nil {
				return 0, err
			}
			if !row.IsEmpty() {
				return row["id"].Int64(), nil
			}
			for _, field := range []string{"des", "locale", "icon", "routepath", "component", "redirect", "path", "permission"} {
				if _, present := data[field]; !present {
					data[field] = ""
				}
			}
			data["routename"] = route
			data["uid"] = 1
			data["status"] = 0
			data["requiresauth"] = 1
			data["createtime"] = gtime.Now()
			return tx.Model("auth_rule").Data(data).InsertAndGetId()
		}
		root, err := add(name, g.Map{"title": title, "routepath": "/" + name, "component": component, "icon": icon, "type": 1, "pid": 0, "weigh": order})
		if err != nil {
			return err
		}
		for i, a := range actions {
			if _, err := add(name+":"+a.Path, g.Map{"title": a.Title, "type": 2, "pid": root, "path": "/admin/" + name + "/" + a.Path, "permission": name + ":" + a.Path, "hideinmenu": 1, "weigh": i}); err != nil {
				return err
			}
		}
		return nil
	})
}
