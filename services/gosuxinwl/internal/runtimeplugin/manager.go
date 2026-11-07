package runtimeplugin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

type worker struct {
	cmd   *exec.Cmd
	done  chan error
	stdin io.WriteCloser
	proxy *httputil.ReverseProxy
}
type Manager struct {
	name      string
	mu        sync.RWMutex
	root      string
	running   *worker
	dir       string
	lastError string
	closed    bool
	disabled  bool
	ctx       context.Context
}
type activeRecord struct {
	Package  string `json:"package"`
	Disabled bool   `json:"disabled,omitempty"`
}

func (m *Manager) pluginName() string { return m.name }

func nonce() string {
	b := make([]byte, 24)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
func (m *Manager) base() string { return filepath.Join(m.root, "storage/plugins/"+m.pluginName()) }
func (m *Manager) Status() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	title, version := m.pluginName(), ""
	if m.dir != "" {
		var manifest Manifest
		if body, err := os.ReadFile(filepath.Join(m.dir, "plugin.json")); err == nil && json.Unmarshal(body, &manifest) == nil && manifest.Name == m.pluginName() {
			version = manifest.Version
			if manifest.Title != "" {
				title = manifest.Title
			}
		}
	}
	return map[string]interface{}{"name": m.pluginName(), "title": title, "version": version, "entry": "/runtime-plugins/" + m.name, "description": "独立运行插件，上传安装即可启用，无需重新构建主程序。", "installed": m.dir != "" && !m.disabled, "active": m.running != nil, "error": m.lastError, "runtime": true, "platform": runtime.GOOS + "/" + runtime.GOARCH}
}
func (m *Manager) start(ctx context.Context) error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	m.root = root
	m.mu.Lock()
	m.ctx = ctx
	b, e := os.ReadFile(filepath.Join(m.base(), "active.json"))
	if e == nil {
		var a activeRecord
		if json.Unmarshal(b, &a) != nil || len(a.Package) != 64 || strings.Trim(a.Package, "0123456789abcdef") != "" {
			m.lastError = "插件启用记录损坏"
		} else {
			m.dir = filepath.Join(m.base(), "packages", a.Package)
			m.disabled = a.Disabled
			if !m.disabled {
				m.running, e = m.launch(ctx, m.dir)
			}
			if e != nil {
				m.lastError = e.Error()
			}
		}
	} else if !os.IsNotExist(e) {
		m.lastError = e.Error()
	}
	m.mu.Unlock()
	go m.monitor(ctx)
	return nil
}
func (m *Manager) monitor(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	attempts := 0
	for {
		select {
		case <-ctx.Done():
			m.Close()
			return
		case <-ticker.C:
			m.mu.Lock()
			if m.closed {
				m.mu.Unlock()
				return
			}
			if m.running != nil {
				select {
				case e := <-m.running.done:
					m.lastError = fmt.Sprintf("插件进程退出: %v", e)
					m.running = nil
				default:
				}
			}
			if m.running == nil && m.dir != "" && !m.disabled && attempts < 5 {
				attempts++
				w, e := m.launch(ctx, m.dir)
				if e != nil {
					m.lastError = e.Error()
				} else {
					m.running = w
					m.lastError = ""
				}
			}
			m.mu.Unlock()
		}
	}
}
func stop(w *worker) {
	if w != nil {
		w.stdin.Close()
		select {
		case <-w.done:
		case <-time.After(5 * time.Second):
			_ = w.cmd.Process.Kill()
			<-w.done
		}
	}
}
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return
	}
	m.closed = true
	stop(m.running)
	m.running = nil
}

func (m *Manager) launch(ctx context.Context, dir string) (*worker, error) {
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		return nil, e
	}
	addr := listener.Addr().String()
	listener.Close()
	secret := nonce()
	exe := filepath.Join(dir, "bin/"+m.pluginName())
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	if err := os.MkdirAll(filepath.Join(m.root, "runtime/plugins"), 0700); err != nil {
		return nil, err
	}
	dataDir := filepath.Join(m.base(), "data")
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, err
	}
	log, e := os.OpenFile(filepath.Join(m.root, "runtime/plugins/"+m.pluginName()+".log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		return nil, e
	}
	cmd := exec.Command(exe)
	cmd.Dir = m.root
	cmd.Env = append(os.Environ(), "SUXIN_PLUGIN_NAME="+m.name, "SUXIN_PLUGIN_ADDR="+addr, "SUXIN_PLUGIN_SECRET="+secret, "SUXIN_PLUGIN_PACKAGE="+dir, "SUXIN_PLUGIN_DATA="+dataDir, "SUXIN_PLUGIN_BASE=/plugins/"+m.pluginName())
	cmd.Stdout = log
	cmd.Stderr = log
	stdin, e := cmd.StdinPipe()
	if e != nil {
		log.Close()
		return nil, e
	}
	if e = cmd.Start(); e != nil {
		stdin.Close()
		log.Close()
		return nil, e
	}
	w := &worker{cmd: cmd, stdin: stdin, done: make(chan error, 1)}
	go func() { w.done <- cmd.Wait(); log.Close() }()
	client := &http.Client{Timeout: time.Second, Transport: &http.Transport{Proxy: nil}}
	deadline := time.NewTimer(45 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			stop(w)
			return nil, ctx.Err()
		case e := <-w.done:
			stdin.Close()
			return nil, fmt.Errorf("插件 %s 启动失败（查看 runtime/plugins/%s.log）: %v", m.pluginName(), m.pluginName(), e)
		case <-deadline.C:
			stop(w)
			return nil, fmt.Errorf("插件启动超时，请查看 runtime/plugins/%s.log", m.pluginName())
		case <-ticker.C:
			req, _ := http.NewRequestWithContext(ctx, "GET", "http://"+addr+"/__plugin_health", nil)
			req.Header.Set("X-Suxin-Plugin-Secret", secret)
			resp, e := client.Do(req)
			if e != nil {
				continue
			}
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode != 200 {
				continue
			}
			target, _ := url.Parse("http://" + addr)
			proxy := httputil.NewSingleHostReverseProxy(target)
			director := proxy.Director
			proxy.Director = func(r *http.Request) {
				originalHost := r.Host
				forwardHostHTTPS(r)
				prefix := "/plugins/" + m.pluginName()
				originalPath := r.URL.Path
				if r.URL.Path == prefix || strings.HasPrefix(r.URL.Path, prefix+"/") {
					r.URL.Path = strings.TrimPrefix(r.URL.Path, prefix)
					r.URL.RawPath = ""
					if r.URL.Path == "" {
						r.URL.Path = "/"
					}
				}
				r.Header.Del("X-Suxin-Local-Website")
				r.Header.Set("X-Suxin-Plugin-Base", prefix)
				director(r)
				r.Host = originalHost
				r.Header.Del("X-Suxin-Plugin-User")
				if strings.HasPrefix(originalPath, "/admin/") || strings.HasPrefix(originalPath, prefix+"/admin") || strings.HasPrefix(originalPath, prefix+"/api/admin/") {
					if user := r.Context().Value("user"); user != nil {
						encoded, _ := json.Marshal(user)
						r.Header.Set("X-Suxin-Plugin-User", string(encoded))
					}
				}
				stripHostCredentials(m.name, originalPath, r)
				r.Header.Del("X-Suxin-Plugin-Secret")
				r.Header.Set("X-Suxin-Plugin-Secret", secret)
			}
			proxy.ErrorHandler = func(rw http.ResponseWriter, r *http.Request, e error) {
				http.Error(rw, "插件暂不可用，请在代码仓库检查状态", http.StatusBadGateway)
			}
			proxy.FlushInterval = -1
			w.proxy = proxy
			return w, nil
		}
	}
}

// The loopback worker must see the original TLS state for absolute media URLs
// and Secure cookies. Never forward a caller-supplied private transport flag.
func forwardHostHTTPS(r *http.Request) {
	r.Header.Set("X-Suxin-Plugin-Remote-Addr", r.RemoteAddr)
	r.Header.Del("X-Suxin-Plugin-HTTPS")
	if r.TLS != nil {
		r.Header.Set("X-Suxin-Plugin-HTTPS", "1")
	}
}

func (m *Manager) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(strings.TrimPrefix(r.URL.Path, "/plugins/"+m.name), "/__plugin_") {
		http.NotFound(w, r)
		return
	}
	m.mu.RLock()
	running := m.running
	installed := m.dir != "" && !m.disabled
	var proxy *httputil.ReverseProxy
	if running != nil {
		proxy = running.proxy
	}
	m.mu.RUnlock()
	if proxy == nil {
		if installed {
			http.Error(w, "插件未运行，请在代码仓库检查状态", 503)
		} else {
			http.NotFound(w, r)
		}
		return
	}
	proxy.ServeHTTP(w, r)
}

func (m *Manager) Install(ctx context.Context, filename string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return errors.New("服务正在关闭")
	}
	if m.root == "" {
		return errors.New("运行插件宿主未启动")
	}
	base := filepath.Join(m.base(), "packages")
	if err := os.MkdirAll(base, 0700); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(base, ".stage-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	manifest, digest, err := unpack(filename, stage)
	if err == nil && manifest.Name != m.pluginName() {
		err = errors.New("插件标识与安装目标不一致")
	}
	if err != nil {
		return err
	}
	target := filepath.Join(base, digest)
	if _, e := os.Stat(target); os.IsNotExist(e) {
		if err = os.Rename(stage, target); err != nil {
			return err
		}
	} else if e != nil {
		return e
	}
	previous := m.dir
	previousDisabled := m.disabled
	stop(m.running)
	m.running = nil
	w, err := m.launch(m.ctx, target)
	if err == nil {
		marker, _ := json.Marshal(activeRecord{Package: digest})
		tmp, e := os.CreateTemp(m.base(), ".active-")
		if e == nil {
			_, e = tmp.Write(marker)
			if e == nil {
				e = tmp.Sync()
			}
			ce := tmp.Close()
			if e == nil {
				e = ce
			}
			if e == nil {
				e = os.Rename(tmp.Name(), filepath.Join(m.base(), "active.json"))
			}
			os.Remove(tmp.Name())
		}
		err = e
	}
	if err != nil {
		stop(w)
		m.lastError = err.Error()
		if previous != "" && !previousDisabled {
			m.running, _ = m.launch(m.ctx, previous)
		}
		return err
	}
	m.running = w
	m.dir = target
	m.disabled = false
	m.lastError = ""
	return nil
}

// SetInstalled retains immutable releases and business data for reinstall.
func (m *Manager) SetInstalled(installed bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.dir == "" {
		return errors.New("插件运行包不可用")
	}
	var next *worker
	var err error
	if installed && m.running == nil {
		next, err = m.launch(m.ctx, m.dir)
		if err != nil {
			m.lastError = err.Error()
			return err
		}
	}
	record, _ := json.Marshal(activeRecord{Package: filepath.Base(m.dir), Disabled: !installed})
	tmp, err := os.CreateTemp(m.base(), ".active-")
	if err != nil {
		stop(next)
		return err
	}
	defer os.Remove(tmp.Name())
	_, err = tmp.Write(record)
	if err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), filepath.Join(m.base(), "active.json"))
	}
	if err != nil {
		stop(next)
		return err
	}
	if !installed {
		stop(m.running)
		m.running = nil
	} else if next != nil {
		m.running = next
	}
	m.disabled = !installed
	m.lastError = ""
	return nil
}
