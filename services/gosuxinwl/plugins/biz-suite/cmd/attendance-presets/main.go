// attendance-presets only reads the supplied workbook and writes a reviewable
// draft artifact. No database or deployment configuration is loaded.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/hrm"
)

func main() {
	input := flag.String("input", "", "source workbook")
	output := flag.String("output", "", "new draft JSON output")
	flag.Parse()
	if *input == "" || *output == "" {
		panic("input and output are required")
	}
	f, err := os.Open(*input)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	preview, err := hrm.ParseAttendanceWorkbook(f, filepath.Base(*input))
	if err != nil {
		panic(err)
	}
	data, err := json.MarshalIndent(map[string]any{"source_sha256": preview.SHA256, "source_filename": preview.Filename, "presets": hrm.PreviewProductionWorkSchedules(preview)}, "", "  ")
	if err != nil {
		panic(err)
	}
	out, err := os.OpenFile(*output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		panic(err)
	}
	if _, err = out.Write(data); err != nil {
		out.Close()
		panic(err)
	}
	if err = out.Close(); err != nil {
		panic(err)
	}
	fmt.Printf("Draft artifact written; source SHA256 %s; no business data changed.\n", preview.SHA256)
}
