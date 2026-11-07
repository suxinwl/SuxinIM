package httpstream_test

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/utility/httpstream"
)

type outputState struct {
	status, buffered int
}

func liveServer(t *testing.T, suffix string, handler ghttp.HandlerFunc) string {
	t.Helper()
	s := ghttp.GetServer(fmt.Sprintf("%s-%s-%d", t.Name(), suffix, time.Now().UnixNano()))
	s.SetAddr("127.0.0.1:0")
	s.SetDumpRouterMap(false)
	s.SetAccessLogEnabled(false)
	s.SetLogStdout(false)
	s.SetFileServerEnabled(false)
	s.BindHandler("/*path", handler)
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Shutdown() })
	return fmt.Sprintf("http://127.0.0.1:%d", s.GetListenedPort())
}

// Exercise both real framework servers, rather than ResponseRecorder alone:
// ServeContent in a plugin and a host reverse proxy with immediate flushing.
func TestStreamingContentThroughFrameworkProxy(t *testing.T) {
	content := []byte("0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJ")
	modified := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	pluginOutput := make(chan outputState, 32)
	hostOutput := make(chan outputState, 32)
	releaseBody := make(chan struct{}, 1)
	plugin := liveServer(t, "plugin", func(r *ghttp.Request) {
		defer func() { pluginOutput <- outputState{r.Response.Status, r.Response.BufferLength()} }()
		w := httpstream.New(r.Response)
		switch r.URL.Path {
		case "/empty204":
			w.WriteHeader(http.StatusNoContent)
		case "/empty404":
			w.WriteHeader(http.StatusNotFound)
		case "/error":
			http.Error(w, "Rejected request", http.StatusForbidden)
		case "/first-flush":
			w.Header().Set("Content-Length", "10")
			w.WriteHeader(http.StatusPartialContent)
			w.Flush()
			_, _ = w.Write(content[:10])
		case "/held-body":
			w.Header().Set("Content-Length", "20")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(content[:10])
			w.Flush()
			select {
			case <-releaseBody:
				_, _ = w.Write(content[10:20])
			case <-r.Context().Done():
			}
		default:
			w.Header().Set("ETag", `"stream-test"`)
			http.ServeContent(w, r.Request, "manual.txt", modified, bytes.NewReader(content))
		}
	})
	target, _ := url.Parse(plugin)
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.FlushInterval = -1
	host := liveServer(t, "host", func(r *ghttp.Request) {
		proxy.ServeHTTP(httpstream.New(r.Response), r.Request)
		hostOutput <- outputState{r.Response.Status, r.Response.BufferLength()}
	})
	client := &http.Client{Timeout: 5 * time.Second}
	readState := func(ch <-chan outputState, want int) {
		t.Helper()
		select {
		case result := <-ch:
			if result.status != want || result.buffered != 0 {
				t.Fatalf("incorrect logged status or whole-file buffering: %+v want status %d", result, want)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("stream handler did not finish")
		}
	}
	for _, server := range []struct{ name, address string }{{"plugin", plugin}, {"proxy", host}} {
		t.Run(server.name, func(t *testing.T) {
			for _, test := range []struct {
				name, path, method string
				headers            map[string]string
				status             int
				body               []byte
			}{
				{"full", "/file", "GET", nil, 200, content},
				{"range-ten", "/file", "GET", map[string]string{"Range": "bytes=0-9"}, 206, content[:10]},
				{"range-gzip-header", "/file", "GET", map[string]string{"Range": "bytes=0-9", "Accept-Encoding": "gzip"}, 206, content[:10]},
				{"range-thirty", "/file", "GET", map[string]string{"Range": "bytes=0-29"}, 206, content[:30]},
				{"head", "/file", "HEAD", nil, 200, nil},
				{"head-range", "/file", "HEAD", map[string]string{"Range": "bytes=0-9"}, 206, nil},
				{"not-modified", "/file", "GET", map[string]string{"If-None-Match": `"stream-test"`}, 304, nil},
				{"no-content", "/empty204", "GET", nil, 204, nil},
				{"explicit-empty-error", "/empty404", "GET", nil, 404, nil},
				{"error-with-body", "/error", "GET", nil, 403, []byte("Rejected request\n")},
				{"flush-headers-before-body", "/first-flush", "GET", nil, 206, content[:10]},
			} {
				t.Run(test.name, func(t *testing.T) {
					req, _ := http.NewRequest(test.method, server.address+test.path, nil)
					for k, v := range test.headers {
						req.Header.Set(k, v)
					}
					res, err := client.Do(req)
					if err != nil {
						t.Fatal(err)
					}
					got, err := io.ReadAll(res.Body)
					res.Body.Close()
					if err != nil || res.StatusCode != test.status || !bytes.Equal(got, test.body) {
						t.Fatalf("status=%d body=%q readError=%v; want status=%d body=%q", res.StatusCode, got, err, test.status, test.body)
					}
					readState(pluginOutput, test.status)
					if server.name == "proxy" {
						readState(hostOutput, test.status)
					}
				})
			}
			t.Run("multiple-ranges", func(t *testing.T) {
				req, _ := http.NewRequest("GET", server.address+"/file", nil)
				req.Header.Set("Range", "bytes=0-2,10-12")
				res, err := client.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				defer res.Body.Close()
				mediaType, parameters, err := mime.ParseMediaType(res.Header.Get("Content-Type"))
				if err != nil || mediaType != "multipart/byteranges" || res.StatusCode != 206 {
					t.Fatalf("invalid multi-range headers: %v %d %s", err, res.StatusCode, mediaType)
				}
				parts := multipart.NewReader(res.Body, parameters["boundary"])
				for _, want := range [][]byte{content[:3], content[10:13]} {
					part, err := parts.NextPart()
					if err != nil {
						t.Fatal(err)
					}
					got, err := io.ReadAll(part)
					if err != nil || !bytes.Equal(got, want) {
						t.Fatalf("incorrect multipart bytes: %q %v", got, err)
					}
				}
				if _, err := parts.NextPart(); err != io.EOF {
					t.Fatalf("unexpected additional part: %v", err)
				}
				readState(pluginOutput, 206)
				if server.name == "proxy" {
					readState(hostOutput, 206)
				}
			})
			t.Run("first-chunk-before-handler-finishes", func(t *testing.T) {
				res, err := client.Get(server.address + "/held-body")
				if err != nil {
					t.Fatal(err)
				}
				defer res.Body.Close()
				first := make([]byte, 10)
				if _, err := io.ReadFull(res.Body, first); err != nil || !bytes.Equal(first, content[:10]) {
					t.Fatalf("initial chunk buffered until handler completed: %q %v", first, err)
				}
				releaseBody <- struct{}{}
				last, err := io.ReadAll(res.Body)
				if err != nil || !bytes.Equal(last, content[10:20]) {
					t.Fatalf("remaining streamed bytes changed: %q %v", last, err)
				}
				readState(pluginOutput, 200)
				if server.name == "proxy" {
					readState(hostOutput, 200)
				}
			})
		})
	}
}

func TestBufferedErrorStillHasDefaultBody(t *testing.T) {
	address := liveServer(t, "buffered", func(r *ghttp.Request) { r.Response.WriteHeader(http.StatusNotFound) })
	res, err := (&http.Client{Timeout: 5 * time.Second}).Get(address + "/missing")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil || res.StatusCode != 404 || strings.TrimSpace(string(body)) != "Not Found" {
		t.Fatalf("default framework error body changed: status=%d body=%q err=%v", res.StatusCode, body, err)
	}
}
