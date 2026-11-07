package runtimeplugin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWorkerInstallRollbackUninstallAndRestore(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a real worker fixture")
	}
	buildDir := t.TempDir()
	source := `package main
import("net/http";"os";"io";"path/filepath")
func main(){
 if _,e:=os.Stat(filepath.Join(os.Getenv("SUXIN_PLUGIN_PACKAGE"),"public/fail"));e==nil{os.Exit(9)}
 s:=&http.Server{Addr:os.Getenv("SUXIN_PLUGIN_ADDR"),Handler:http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if r.Header.Get("X-Suxin-Plugin-Secret")!=os.Getenv("SUXIN_PLUGIN_SECRET"){w.WriteHeader(403);return}
  if r.URL.Path=="/__plugin_health"{w.Write([]byte("ok"));return}
  w.Write([]byte("worker:"+r.URL.Path))
 })}
 go func(){io.Copy(io.Discard,os.Stdin);s.Close()}()
 s.ListenAndServe()
}`
	input := filepath.Join(buildDir, "main.go")
	if err := os.WriteFile(input, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(buildDir, "sampleplugin")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	command := exec.Command("go", "build", "-o", binary, input)
	command.Env = append(os.Environ(), "GOWORK=off")
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("fixture build: %v %s", err, out)
	}
	executable, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	archive := func(version string, fail bool) string {
		return fixture(t, func(m *Manifest, files map[string][]byte) {
			m.Version = version
			for path := range files {
				if strings.HasPrefix(path, "bin/") {
					files[path] = executable
				}
			}
			if fail {
				files["public/fail"] = []byte("bad startup")
			}
			for path, data := range files {
				sum := sha256.Sum256(data)
				m.Files[path] = hex.EncodeToString(sum[:])
			}
		})
	}
	original, broken := archive("1.0.0", false), archive("2.0.0", true)
	// Server installation must work without compilers or shell commands on PATH.
	t.Setenv("PATH", "")
	root := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := &Manager{name: "sampleplugin", root: root, ctx: ctx}
	defer m.Close()
	if err = m.Install(ctx, original); err != nil {
		t.Fatal(err)
	}
	proxy := func(want int) {
		t.Helper()
		out := httptest.NewRecorder()
		m.ServeHTTP(out, httptest.NewRequest("GET", "/plugins/sampleplugin/public/hello", nil))
		if out.Code != want {
			t.Fatalf("proxy status=%d body=%s", out.Code, out.Body.String())
		}
		if want == 200 && out.Body.String() != "worker:/public/hello" {
			t.Fatal(out.Body.String())
		}
	}
	proxy(200)
	previousPID := m.running.cmd.Process.Pid
	if err = m.Install(ctx, broken); err == nil {
		t.Fatal("accepted failed worker")
	}
	if m.Status()["version"] != "1.0.0" || m.running == nil {
		t.Fatal("rollback did not restore release")
	}
	if m.running.cmd.Process.Pid == previousPID {
		t.Fatal("fixture did not exercise process replacement")
	}
	proxy(200)
	if err = m.SetInstalled(false); err != nil {
		t.Fatal(err)
	}
	proxy(404)
	record, err := os.ReadFile(filepath.Join(m.base(), "active.json"))
	if err != nil {
		t.Fatal(err)
	}
	var active activeRecord
	if err = json.Unmarshal(record, &active); err != nil || !active.Disabled {
		t.Fatal("uninstall state not persisted")
	}
	if err = m.SetInstalled(true); err != nil {
		t.Fatal(err)
	}
	proxy(200)
	if err = m.SetInstalled(false); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(m.dir); err != nil {
		t.Fatal("uninstall deleted retained release")
	}
}

func TestNativeAliasesDoNotCaptureHostRoutes(t *testing.T) {
	for name, path := range map[string]string{"ebook": "/admin/album/list", "privatecode": "/admin/privatecode/content/list", "analysis": "/admin/analysis/overview"} {
		if !NativePath(name, path) {
			t.Fatalf("missing alias %s", path)
		}
		for _, host := range []string{"/admin/user/login", "/admin/developer/packinstall/installCode", "/suxinweb/home"} {
			if NativePath(name, host) {
				t.Fatalf("captured host path %s", host)
			}
		}
	}
}
