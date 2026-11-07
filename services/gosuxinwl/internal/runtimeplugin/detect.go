package runtimeplugin

import (
	"archive/zip"
	"encoding/json"
	"io"
)

// Distinguish runtime archives from historical source archives before install.
func IsRuntime(file string) bool {
	z, err := zip.OpenReader(file)
	if err != nil {
		return false
	}
	defer z.Close()
	for _, entry := range z.File {
		if entry.Name != "plugin.json" {
			continue
		}
		reader, err := entry.Open()
		if err != nil {
			return false
		}
		defer reader.Close()
		var manifest Manifest
		return json.NewDecoder(io.LimitReader(reader, 1<<20)).Decode(&manifest) == nil && manifest.Format == Format
	}
	return false
}
