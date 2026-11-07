package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"

	_ "github.com/suxinwl/GoSuxin/framework/contrib/drivers/mysql"
	_ "github.com/suxinwl/GoSuxin/framework/contrib/drivers/sqlite"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/os/gcfg"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/etl"
)

func run() (code int) {
	// Configuration/driver panics can contain DSNs. Report a safe error instead.
	defer func() {
		if recover() != nil {
			fmt.Fprintln(os.Stderr, "ETL initialization failed; verify the host database configuration and installed plugin schema")
			code = 1
		}
	}()
	input := flag.String("input", "", "path to a version 1 normalized ruoyi JSON export")
	config := flag.String("config", "", "existing host configuration file (default uses GoSuxin config discovery)")
	apply := flag.Bool("apply", false, "apply imports; default is a read-only dry run")
	flag.Parse()
	if *input == "" {
		fmt.Fprintln(os.Stderr, "-input is required; omit -apply to preview without writes")
		return 2
	}
	f, err := os.Open(*input)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cannot open migration input")
		return 1
	}
	defer f.Close()
	document, err := etl.Decode(f)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if *config != "" {
		adapter, ok := g.Cfg().GetAdapter().(*gcfg.AdapterFile)
		if !ok {
			fmt.Fprintln(os.Stderr, "file configuration adapter is unavailable")
			return 1
		}
		adapter.SetFileName(*config)
	}
	if err = core.Init(g.DB()); err != nil {
		fmt.Fprintln(os.Stderr, "cannot initialize business database from host configuration")
		return 1
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	report, err := etl.Run(ctx, document, *apply)
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if e := encoder.Encode(report); e != nil {
		fmt.Fprintln(os.Stderr, "cannot write migration report")
		return 1
	}
	if err != nil {
		if errors.Is(err, context.Canceled) {
			fmt.Fprintln(os.Stderr, "migration canceled")
		} else {
			fmt.Fprintln(os.Stderr, err)
		}
		return 1
	}
	if report.Failed > 0 {
		return 1
	}
	return 0
}
func main() { os.Exit(run()) }
