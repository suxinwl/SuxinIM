package middleware

import (
	"fmt"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/utility/httpstream"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestHandlerResponseDoesNotAppendToStreamedOutput(t *testing.T) {
	s := ghttp.GetServer(fmt.Sprintf("response-stream-%d", time.Now().UnixNano()))
	s.SetAddr("127.0.0.1:0")
	s.SetDumpRouterMap(false)
	s.SetLogStdout(false)
	s.SetFileServerEnabled(false)
	s.Group("/", func(group *ghttp.RouterGroup) {
		group.Middleware(HandlerResponse)
		group.ALL("/*path", func(r *ghttp.Request) {
			w := httpstream.New(r.Response)
			switch r.URL.Path {
			case "/json":
				_, _ = w.Write([]byte(`{"code":0,"data":[1,2,3]}`))
			case "/empty":
				w.WriteHeader(204)
			case "/missing":
				w.WriteHeader(404)
			case "/range":
				w.WriteHeader(206)
				w.Flush()
				_, _ = w.Write([]byte("binary-data"))
			}
		})
	})
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	defer s.Shutdown()
	for _, tc := range []struct {
		path, body string
		status     int
	}{{"/json", `{"code":0,"data":[1,2,3]}`, 200}, {"/empty", "", 204}, {"/missing", "", 404}, {"/range", "binary-data", 206}} {
		response, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d%s", s.GetListenedPort(), tc.path))
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil || string(body) != tc.body || response.StatusCode != tc.status {
			t.Fatalf("%s: status=%d body=%q err=%v", tc.path, response.StatusCode, body, err)
		}
	}
}
