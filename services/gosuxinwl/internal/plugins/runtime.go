package plugins

import "github.com/suxinwl/GoSuxin/framework/net/ghttp"

var RuntimeManaged func(string) bool
var RuntimeEnabled func(string) bool
var RuntimeSetInstalled func(string, bool) error
var RuntimeServe func(string, *ghttp.Request) bool

func Managed(name string) bool { return RuntimeManaged != nil && RuntimeManaged(name) }

// SuspendNative releases jobs and locks before a worker takes ownership.
func SuspendNative(name string) {
	operations.Lock()
	defer operations.Unlock()
	if stop := running[name]; stop != nil {
		stop()
		delete(running, name)
	}
}

func ResumeNative(name string) error {
	operations.Lock()
	defer operations.Unlock()
	for _, p := range registered {
		if p.Name == name && p.Start != nil && Enabled(name) && running[name] == nil {
			stop, err := p.Start(hostContext, hostServer)
			if err != nil {
				return err
			}
			if stop != nil {
				running[name] = stop
			}
		}
	}
	return nil
}

func serveRuntime(name string, r *ghttp.Request) bool {
	if RuntimeServe != nil && RuntimeServe(name, r) {
		r.ExitAll()
		return true
	}
	return false
}
