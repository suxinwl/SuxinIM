package admin

import (
	"context"

	"github.com/suxinwl/GoSuxin/api/admin/system"
	"github.com/suxinwl/GoSuxin/internal/service"
)

func (c *ControllerSystem) RuleList(ctx context.Context, req *system.RuleListReq) (res *system.RuleListRes, err error) {
	res = &system.RuleListRes{R: service.Adminsystem().RuleList(ctx, req)}
	return
}

func (c *ControllerSystem) RuleParent(ctx context.Context, req *system.RuleParentReq) (res *system.RuleParentRes, err error) {
	res = &system.RuleParentRes{R: service.Adminsystem().RuleParent(ctx, req)}
	return
}

func (c *ControllerSystem) RuleRoutes(ctx context.Context, req *system.RuleRoutesReq) (res *system.RuleRoutesRes, err error) {
	res = &system.RuleRoutesRes{R: service.Adminsystem().RuleRoutes(ctx, req)}
	return
}

func (c *ControllerSystem) RuleSave(ctx context.Context, req *system.RuleSaveReq) (res *system.RuleSaveRes, err error) {
	res = &system.RuleSaveRes{R: service.Adminsystem().RuleSave(ctx, req)}
	return
}

func (c *ControllerSystem) RuleStatus(ctx context.Context, req *system.RuleStatusReq) (res *system.RuleStatusRes, err error) {
	res = &system.RuleStatusRes{R: service.Adminsystem().RuleStatus(ctx, req)}
	return
}

func (c *ControllerSystem) RuleDel(ctx context.Context, req *system.RuleDelReq) (res *system.RuleDelRes, err error) {
	res = &system.RuleDelRes{R: service.Adminsystem().RuleDel(ctx, req)}
	return
}

func (c *ControllerSystem) RuleContent(ctx context.Context, req *system.RuleContentReq) (res *system.RuleContentRes, err error) {
	res = &system.RuleContentRes{R: service.Adminsystem().RuleContent(ctx, req)}
	return
}

func (c *ControllerSystem) EmptyRecyclebin(ctx context.Context, req *system.EmptyRecyclebinReq) (res *system.EmptyRecyclebinRes, err error) {
	res = &system.EmptyRecyclebinRes{R: service.Adminsystem().EmptyRecyclebin(ctx, req)}
	return
}
