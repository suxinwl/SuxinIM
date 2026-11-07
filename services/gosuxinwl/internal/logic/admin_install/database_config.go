package admininstall

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/suxinwl/GoSuxin/framework/encoding/gyaml"
)

// Update only database.default; Redis has an independent password.
func writeDatabaseConfig(path string, values map[string]string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	config, err := gyaml.Decode(content)
	if err != nil {
		return err
	}
	database, ok := config["database"].(map[string]any)
	if !ok {
		return fmt.Errorf("missing database configuration")
	}
	defaults, ok := database["default"].(map[string]any)
	if !ok {
		return fmt.Errorf("missing database.default configuration")
	}
	for _, key := range []string{"host", "port", "user", "pass", "name", "prefix", "type", "version", "extra"} {
		if value, exists := values[key]; exists {
			defaults[key] = value
		}
	}
	if values["type"] == "sqlite" {
		defaults["maxOpen"], defaults["maxIdle"] = 1, 1
	} else if values["type"] == "mysql" {
		delete(defaults, "maxOpen")
		delete(defaults, "maxIdle")
	}
	// A stale link would override the individual connection fields.
	if values["type"] != "" {
		delete(defaults, "link")
	}
	updated, err := gyaml.Encode(config)
	if err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".db-config-*.yaml")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())
	defer temporary.Close()
	if err = temporary.Chmod(info.Mode().Perm()); err != nil {
		return err
	}
	if _, err = temporary.Write(updated); err != nil {
		return err
	}
	if err = temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporary.Name(), path)
}

// Keep the CLI on the same database without string-editing YAML credentials.
func writeCLIConfig(path string, values map[string]string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	config, err := gyaml.Decode(content)
	if err != nil {
		return err
	}
	cli, ok := config["suxincli"].(map[string]any)
	if !ok {
		return fmt.Errorf("missing suxincli configuration")
	}
	gen, ok := cli["gen"].(map[string]any)
	if !ok {
		return fmt.Errorf("missing suxincli.gen configuration")
	}
	link := "mysql:" + mysqlDSN(values, values["name"])
	if values["type"] == "sqlite" {
		link = "sqlite::@file(" + values["name"] + ")"
	}
	gen["dao"] = []map[string]any{{"link": link, "removePrefix": values["prefix"], "descriptionTag": true, "noModelComment": true}}
	updated, err := gyaml.Encode(config)
	if err != nil {
		return err
	}
	return os.WriteFile(path, updated, 0600)
}
