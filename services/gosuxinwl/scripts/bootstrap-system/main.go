// bootstrap-system initializes an empty database without an interactive web
// installer. A password is read from a private file, never a command argument.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	admininstall "github.com/suxinwl/GoSuxin/internal/logic/admin_install"
	"gopkg.in/yaml.v3"
)

func run() error {
	configPath := flag.String("config", "manifest/config/config.yaml", "private database configuration")
	seedDir := flag.String("seed-directory", "devsource/developer/install", "system schema directory")
	username := flag.String("admin", "sysadmin", "only administrator username")
	passwordFile := flag.String("password-file", "", "restricted file containing an initial password of at least 16 characters")
	lock := flag.String("install-lock", "devsource/developer/install/install.lock", "disable the online installer after success")
	flag.Parse()
	if *passwordFile == "" {
		return fmt.Errorf("password-file required")
	}
	raw, err := os.ReadFile(*configPath)
	if err != nil {
		return fmt.Errorf("cannot read database configuration")
	}
	var document struct {
		Database struct {
			Default map[string]string `yaml:"default"`
		} `yaml:"database"`
	}
	if yaml.Unmarshal(raw, &document) != nil {
		return fmt.Errorf("invalid database configuration")
	}
	password, err := os.ReadFile(*passwordFile)
	if err != nil {
		return fmt.Errorf("cannot read administrator password file")
	}
	for len(password) > 0 && (password[len(password)-1] == '\r' || password[len(password)-1] == '\n') {
		password = password[:len(password)-1]
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err = admininstall.BootstrapEmpty(ctx, document.Database.Default, *seedDir, *username, string(password)); err != nil {
		return fmt.Errorf("bootstrap failed; target must be an empty database and credentials must be valid")
	}
	if err = os.MkdirAll(filepath.Dir(*lock), 0700); err != nil {
		return err
	}
	return os.WriteFile(*lock, []byte("installed\n"), 0600)
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("System schema and one administrator initialized; no sample business data imported.")
}
