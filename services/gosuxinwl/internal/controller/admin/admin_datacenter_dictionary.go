package admin

import (
	"context"

	"github.com/suxinwl/GoSuxin/api/admin/datacenter"
	"github.com/suxinwl/GoSuxin/internal/service"
)

func (c *ControllerDatacenter) DictionaryList(ctx context.Context, req *datacenter.DictionaryListReq) (res *datacenter.DictionaryListRes, err error) {
	res = &datacenter.DictionaryListRes{R: service.Admindatacenter().DictionaryList(ctx, req)}
	return
}
func (c *ControllerDatacenter) DictionarySave(ctx context.Context, req *datacenter.DictionarySaveReq) (res *datacenter.DictionarySaveRes, err error) {
	res = &datacenter.DictionarySaveRes{R: service.Admindatacenter().DictionarySave(ctx, req)}
	return
}
func (c *ControllerDatacenter) DictionaryStatus(ctx context.Context, req *datacenter.DictionaryStatusReq) (res *datacenter.DictionaryStatusRes, err error) {
	res = &datacenter.DictionaryStatusRes{R: service.Admindatacenter().DictionaryStatus(ctx, req)}
	return
}
func (c *ControllerDatacenter) DictionaryDel(ctx context.Context, req *datacenter.DictionaryDelReq) (res *datacenter.DictionaryDelRes, err error) {
	res = &datacenter.DictionaryDelRes{R: service.Admindatacenter().DictionaryDel(ctx, req)}
	return
}
func (c *ControllerDatacenter) GetTableDataForm(ctx context.Context, req *datacenter.GetTableDataFormReq) (res *datacenter.GetTableDataFormRes, err error) {
	res = &datacenter.GetTableDataFormRes{R: service.Admindatacenter().GetTableDataForm(ctx, req)}
	return
}
