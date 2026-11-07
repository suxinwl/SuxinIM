// =================
// 开发工具-插件打包、安装、卸载
// =================
package clogic_developer

import "github.com/spf13/viper"

// 获取配置
type App struct {
	Version      string `yaml:"version"`
	Title        string `yaml:"title"`
	Des          string `yaml:"des"`
	Name         string `yaml:"name"`
	Isinstall    bool   `yaml:"isinstall"`
	Installcover bool   `yaml:"installcover"`
	IsModTidy    bool   `yaml:"isModTidy"`
	CommandLines string `yaml:"commandLines"`
	GoFiles      string `yaml:"goFiles"`
	VueFiles     string `yaml:"vueFiles"`
	Modellist    string `yaml:"modellist"`
}

type Sqldb struct {
	Packtables   string `yaml:"packtables"`
	Adminmenuids string `yaml:"adminmenuids"`
}
type Config struct {
	App   App   `yaml:"app"`
	Sqldb Sqldb `yaml:"sqldb"`
}

// 读取Yaml配置文件，并转换成Config对象  struct结构
func GetInstallConfig(path string) (*Config, error) {
	var config *Config
	vip := viper.New()
	vip.AddConfigPath(path)     //设置读取的文件路径
	vip.SetConfigName("config") //设置读取的文件名
	vip.SetConfigType("yaml")   //设置文件的类型
	//尝试进行配置读取
	if err := vip.ReadInConfig(); err != nil {
		return nil, err
	}
	verr := vip.Unmarshal(&config)
	if verr != nil {
		return nil, verr
	}
	return config, nil
}
