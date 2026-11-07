package admindatacenter

import (
	"context"
	"fmt"
	"github.com/suxinwl/GoSuxin/api/admin/datacenter"
	"github.com/suxinwl/GoSuxin/internal/dao"
	"github.com/suxinwl/GoSuxin/internal/model/entity"
	"github.com/suxinwl/GoSuxin/utility/gf"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/suxinwl/GoSuxin/framework/util/gconv"
)

// 获取邮箱数据
func (s *sAdmindatacenter) GetEmail(ctx context.Context, req *datacenter.GetEmailReq) (res *gf.R) {
	// data, err := dao.Email.Ctx(ctx).Where("data_from", req.DataFrom).One()
	data := entity.Email{}
	err := dao.Email.Ctx(ctx).Where("data_from", req.DataFrom).Scan(&data)
	if err != nil {
		res = gf.Failed().SetMsg("获取邮箱失败").SetData(err)
		return
	}
	res = gf.Success().SetMsg("获取邮箱配置数据").SetData(gconv.MapDeep(data, "orm"))
	return
}

// 保存邮箱数据
func (s *sAdmindatacenter) SaveEmail(ctx context.Context, req *datacenter.SaveEmailReq) (res *gf.R) {
	GetID, _ := dao.Email.Ctx(ctx).Where("data_from", req.DataFrom).Value("id")
	if GetID == nil {
		addId, err := dao.Email.Ctx(ctx).Data(req).InsertAndGetId()
		if err != nil {
			res = gf.Failed().SetMsg("添加失败").SetData(err)
		} else {
			res = gf.Success().SetMsg("添加成功！").SetData(addId)
		}
	} else {
		_, err := dao.Email.Ctx(ctx).Data(req).Where("id", GetID).Update()
		if err != nil {
			res = gf.Failed().SetMsg("更新失败").SetData(err)
		} else {
			res = gf.Success().SetMsg("更新成功！")
		}
	}
	return
}

// 获取安装的代码仓配置
func (s *sAdmindatacenter) GetCodestoreConfig(ctx context.Context, req *datacenter.GetCodestoreConfigReq) (res *gf.R) {
	path, _ := os.Getwd()
	configPath := filepath.Join(path, "/manifest/config/code")
	go_app_dir, _ := gf.GetAllFileArray(configPath)
	var list []interface{} = make([]interface{}, 0)
	for _, val := range gf.Strings(go_app_dir["files"]) {
		configstr := gf.ReaderFileByline(filepath.Join(path, "/manifest/config/code", val))
		fileName := strings.Split(val, ".")
		install_cofig, _ := gf.GetYmlConfigData(filepath.Join(path, "/manifest/config/code"), fileName[0])
		data_item := install_cofig.(map[string]interface{})
		if data_item["conftype"] == "configuration" {
			var new_data []map[string]interface{}
			for k, v := range data_item["data"].(map[string]interface{}) {
				keyname := k
				for _, cfstr := range configstr {
					cf_str := gf.String(cfstr)
					cf_str_arr := strings.Split(cf_str, "#")
					if strings.Contains(cf_str, k) && len(cf_str_arr) == 2 {
						keyname = cf_str_arr[1]
					}
				}
				new_data = append(new_data, gf.Map{"keyname": keyname, "keyfield": k, "keyvalue": v})
			}
			data_item["name"] = fileName[0]
			data_item["data"] = new_data
			list = append(list, install_cofig)
		}
	}
	res = gf.Success().SetMsg("获取安装的代码仓配置列表").SetData(list)
	return
}

// 修改安装的代码仓配置
func (s *sAdmindatacenter) SaveCodeStoreConfig(ctx context.Context, req *datacenter.SaveCodeStoreConfigReq) (res *gf.R) {
	var upAppconf gf.Map = make(map[string]interface{}, 0)
	for _, item := range req.Data {
		// item := val.(map[string]interface{})
		var val_data interface{}
		switch item["keyvalue"].(type) {
		case string: // 处理 string 类型
			val_str := gf.String(item["keyvalue"])
			if val_str == "" {
				val_str = "\"\""
			} else {
				val_str = strconv.Quote(val_str)
			}
			val_data = val_str
		default:
			val_data = gf.String(item["keyvalue"])
		}
		upAppconf[gf.String(item["keyfield"])] = fmt.Sprintf("%s  #%v", val_data, item["keyname"])
	}
	gf.UpConfigFild("/manifest/config/code/"+req.Name+".yaml", upAppconf, "  ")
	res = gf.Success().SetMsg("修改安装的代码仓配置")
	return
}

// 更新配置使用状态
func (s *sAdmindatacenter) UpConfigStatus(ctx context.Context, req *datacenter.UpConfigStatusReq) (res *gf.R) {
	//如果改为true，则把其他通用标识改为false
	if req.Status {
		path, _ := os.Getwd()
		configPath := filepath.Join(path, "/resource/config")
		go_app_dir, _ := gf.GetAllFileArray(configPath)
		for _, val := range gf.Strings(go_app_dir["files"]) {
			fileName := strings.Split(val, ".")
			install_cofig, _ := gf.GetYmlConfigData(filepath.Join(path, "/resource/config"), fileName[0])
			data_item := install_cofig.(map[string]interface{})
			if fileName[0] != req.Name && gf.String(data_item["pluginident"]) == req.Pluginident && gf.Bool(data_item["status"]) {
				gf.UpConfigFild("/manifest/config/code/"+val, gf.Map{"status": false}, "")
			}
		}
	}
	gf.UpConfigFild("/manifest/config/code/"+req.Name+".yaml", gf.Map{"status": req.Status}, "")
	res = gf.Success().SetMsg("更新配置使用状态成功")
	return
}
