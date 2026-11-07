// Package plugins provides compile-time extension hooks for source plugins.
package plugins

import (
	"context"
	"fmt"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"strings"
)

type Plugin struct {
	Name        string
	Title       string
	Version     string
	Description string
	Entry       string
	MenuRoots   []string
	Admin       []interface{}
	Public      []interface{}
	Bind        func(*ghttp.Server)
	Start       func(context.Context, *ghttp.Server) (func(), error)
}

var registered []Plugin

func Register(p Plugin) { registered = append(registered, p) }
func Active(name string) bool {
	for _, p := range registered {
		if strings.EqualFold(p.Name, name) {
			return true
		}
	}
	return false
}
func BindAdmin(group *ghttp.RouterGroup) {
	for _, p := range registered {
		if len(p.Admin) > 0 {
			group.Clone().Middleware(Gate(p.Name)).Bind(p.Admin...)
		}
	}
}
func BindPublic(group *ghttp.RouterGroup) {
	for _, p := range registered {
		if len(p.Public) > 0 {
			group.Clone().Middleware(Gate(p.Name)).Bind(p.Public...)
		}
	}
}
func Start(ctx context.Context, s *ghttp.Server) (func(), error) {
	if err := loadState(); err != nil {
		return nil, err
	}
	hostContext, hostServer = ctx, s
	stop := stopAll
	for _, p := range registered {
		if p.Bind != nil {
			p.Bind(s)
		}
		if p.Start == nil || !Enabled(p.Name) || Managed(p.Name) {
			continue
		}
		done, err := p.Start(ctx, s)
		if err != nil {
			stop()
			return nil, fmt.Errorf("plugin %s: %w", p.Name, err)
		}
		if done != nil {
			running[p.Name] = done
		}
	}
	return stop, nil
}
