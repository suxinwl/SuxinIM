package builtinai

import (
	"context"
	"github.com/suxinwl/GoSuxin/internal/dao"
)

func ensureMenu(ctx context.Context) error {
	parent, err := dao.AuthRule.Ctx(ctx).Where("type", 0).Where("routename", "developer").Value("id")
	if err != nil {
		return err
	}
	parentID := int64(0)
	if parent != nil {
		parentID = parent.Int64()
	}
	id, err := dao.AuthRule.Ctx(ctx).Where("type", 1).Where("routename", "developer_ai").Value("id")
	if err != nil {
		return err
	}
	pageID := int64(0)
	if id != nil {
		pageID = id.Int64()
	}
	if pageID == 0 {
		pageID, err = dao.AuthRule.Ctx(ctx).Data(map[string]any{"uid": 0, "title": "AI / MCP", "des": "主系统内置AI与开发工具接口", "locale": "menu.developer.ai", "weigh": 20, "type": 1, "pid": parentID, "icon": "icon-code", "routepath": "/developer/ai", "routename": "developer_ai", "component": "developer/ai/index", "redirect": "", "path": "/developer/ai", "permission": "developer.ai", "requiresauth": 1, "hideinmenu": 0, "hidechildreninmenu": 0, "activemenu": 0, "noaffix": 0, "onlypage": 0, "isext": 0, "keepalive": 0, "status": 0}).InsertAndGetId()
		if err != nil {
			return err
		}
	}
	actions := []struct{ path, title, key string }{{"config", "模型配置", "config"}, {"generate", "模型调用", "generate"}, {"tokens", "开发令牌管理", "tokens"}, {"tokens/revoke", "撤销开发令牌", "tokens_revoke"}, {"interfaces", "开发接口目录", "interfaces"}}
	for _, action := range actions {
		path := "/admin/developer/ai/" + action.path
		exists, e := dao.AuthRule.Ctx(ctx).Where("type", 2).Where("path", path).Exist()
		if e != nil {
			return e
		}
		if exists {
			continue
		}
		_, e = dao.AuthRule.Ctx(ctx).Data(map[string]any{"uid": 0, "pid": pageID, "title": action.title, "des": action.title, "locale": "developer.ai." + action.key, "type": 2, "icon": "icon-code", "routepath": "/developer/ai", "routename": "developer_ai_" + action.key, "component": "developer/ai/index", "redirect": "", "path": path, "permission": "developer.ai." + action.key, "status": 0, "requiresauth": 1}).Insert()
		if e != nil {
			return e
		}
	}
	return nil
}
