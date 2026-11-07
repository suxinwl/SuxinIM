package gf

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// 更新file_path路径配置文件内容，file_path已拼接项目目录路径
func UpConfigFild(file_path string, parameter map[string]interface{}, emptystr string) error {
	path, _ := os.Getwd()
	configPath := filepath.Join(path, file_path)
	f, err := os.Open(configPath)
	if err != nil {
		return err
	}
	defer f.Close()
	buf := bufio.NewReader(f)
	var result = ""
	var is_hose = false
	for {
		is_hose = false
		a, _, c := buf.ReadLine()
		if c == io.EOF {
			break
		}
		for keys, Val := range parameter {
			if strings.Contains(string(a), keys) {
				is_hose = true
				datestr := strings.ReplaceAll(string(a), string(a), fmt.Sprintf("%s%v: %v\n", emptystr, keys, Val))
				result += datestr
			}
		}
		if !is_hose {
			result += string(a) + "\n"
		}
	}
	fw, err := os.OpenFile(configPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666) //os.O_TRUNC清空文件重新写入，否则原文件内容可能残留
	w := bufio.NewWriter(fw)
	w.WriteString(result)
	if err != nil {
		return err
	}
	w.Flush()
	return nil
}

// 读取/manifest/config/code下指定文件配置内容
func GetConfByFile(packName string) (interface{}, error) {
	path, _ := os.Getwd()
	data, err := GetYmlConfigData(filepath.Join(path, "/manifest/config/code"), packName)
	return data, err
}
