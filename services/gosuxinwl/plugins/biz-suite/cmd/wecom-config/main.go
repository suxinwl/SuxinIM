// wecom-config transfers connection configuration without printing secrets or
// exporting employee/business rows. Export must run under the source account.
package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"strings"
	"time"

	_ "github.com/suxinwl/GoSuxin/framework/contrib/drivers/mysql"
	_ "github.com/suxinwl/GoSuxin/framework/contrib/drivers/sqlite"
	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/im"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/migration"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/wecom"
	"gopkg.in/yaml.v3"
)

func privateWrite(path string, value []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if _, err = f.Write(value); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		u, e := user.Current()
		if e != nil {
			return e
		}
		if e = exec.Command("icacls", path, "/inheritance:r", "/grant:r", "*"+u.Uid+":(F)").Run(); e != nil {
			return fmt.Errorf("cannot restrict configuration file ACL")
		}
	}
	return nil
}
func run() error {
	mode := flag.String("mode", "", "export or restore")
	config := flag.String("config", "manifest/config/config.yaml", "private database configuration")
	file := flag.String("file", "", "encrypted transfer file")
	keyFile := flag.String("key-file", "", "separately secured portable transfer key file")
	flag.Parse()
	if (*mode != "export" && *mode != "restore") || *file == "" || *keyFile == "" {
		return fmt.Errorf("mode, file and key-file are required")
	}
	raw, err := os.ReadFile(*config)
	if err != nil {
		return err
	}
	var settings struct {
		Database struct {
			Default gdb.ConfigNode `yaml:"default"`
		} `yaml:"database"`
	}
	if yaml.Unmarshal(raw, &settings) != nil {
		return fmt.Errorf("invalid database configuration")
	}
	db, err := gdb.New(settings.Database.Default)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	defer db.Close(ctx)
	if err = core.Init(db); err != nil {
		return err
	}
	if *mode == "export" {
		key := make([]byte, 32)
		if _, err = rand.Read(key); err != nil {
			return err
		}
		sealed, e := wecom.ExportConnection(ctx, key)
		if e != nil {
			return e
		}
		if err = privateWrite(*keyFile, []byte(base64.StdEncoding.EncodeToString(key))); err != nil {
			return err
		}
		return privateWrite(*file, sealed)
	}
	raw, err = os.ReadFile(*keyFile)
	if err != nil {
		return err
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		return err
	}
	sealed, err := os.ReadFile(*file)
	if err != nil {
		return err
	}
	if err = migration.EnsureSchema(ctx, im.Schema()...); err != nil {
		return err
	}
	return wecom.RestoreConnection(ctx, key, sealed)
}
func main() {
	if run() != nil {
		fmt.Fprintln(os.Stderr, "Enterprise WeChat configuration transfer failed; credentials were not printed.")
		os.Exit(1)
	}
	fmt.Println("Enterprise WeChat connection configuration transferred securely; no business data included.")
}
