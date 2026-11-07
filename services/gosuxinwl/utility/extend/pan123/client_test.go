package pan123

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func reply(status int, data any) *http.Response {
	b, _ := json.Marshal(data)
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(string(b))), Header: make(http.Header)}
}
func ok(data any) *http.Response { return reply(200, g.Map{"code": 0, "data": data}) }
func testClient(t *testing.T, fn roundTrip) *panClient {
	t.Helper()
	t.Chdir(t.TempDir())
	return &panClient{p: Config{ClientID: t.Name(), ClientSecret: "test-only", UID: 7}, http: &http.Client{Transport: fn}}
}
func tokenResponse() *http.Response {
	return ok(g.Map{"accessToken": "valid", "expiredAt": time.Now().Add(time.Hour).Format(time.RFC3339)})
}

func TestTokenConcurrentRefreshAndDiskReuse(t *testing.T) {
	var calls atomic.Int32
	c := testClient(t, func(r *http.Request) (*http.Response, error) { calls.Add(1); return tokenResponse(), nil })
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			token, e := c.accessToken(context.Background(), "")
			if e != nil || token != "valid" {
				t.Errorf("token=%q err=%v", token, e)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("issued %d tokens", calls.Load())
	}
	other := &panClient{p: c.p, http: c.http}
	if _, e := other.accessToken(context.Background(), ""); e != nil {
		t.Fatal(e)
	}
	if calls.Load() != 1 {
		t.Fatal("disk token not reused")
	}
	if _, e := c.accessToken(context.Background(), "valid"); e != nil {
		t.Fatal(e)
	}
	if calls.Load() != 2 {
		t.Fatal("invalid token was not refreshed")
	}
}
func Test401Refresh(t *testing.T) {
	var requests, tokens int
	c := testClient(t, func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "access_token") {
			tokens++
			return tokenResponse(), nil
		}
		requests++
		if r.Header.Get("Platform") != "open_platform" || r.Header.Get("Authorization") == "" {
			t.Error("missing official headers")
		}
		if requests == 1 {
			return reply(200, g.Map{"code": 401}), nil
		}
		return ok(g.Map{"uid": 7}), nil
	})
	c.token = "expired"
	c.expires = time.Now().Add(time.Hour)
	if _, e := c.request(context.Background(), "GET", "https://open-api.123pan.com/api/v1/user/info", nil, true); e != nil {
		t.Fatal(e)
	}
	if requests != 2 || tokens != 1 {
		t.Fatalf("requests %d tokens %d", requests, tokens)
	}
}
func TestUploadLifecycle(t *testing.T) {
	for _, mode := range []string{"multipart", "verifying", "verification-cancel", "completion-failure", "reuse", "checksum-failure", "slice-failure", "invalid-create"} {
		t.Run(mode, func(t *testing.T) {
			data := []byte("full-file-content")
			sum := md5.Sum(data)
			etag := hex.EncodeToString(sum[:])
			slices := 0
			completions := 0
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			c := testClient(t, func(r *http.Request) (*http.Response, error) {
				switch r.URL.Path {
				case "/api/v1/access_token":
					return tokenResponse(), nil
				case "/upload/v2/file/create":
					var body struct {
						Etag string
						Size int64
					}
					json.NewDecoder(r.Body).Decode(&body)
					if body.Etag != etag || body.Size != int64(len(data)) {
						t.Error("wrong complete-file digest/size")
					}
					if mode == "reuse" {
						return ok(g.Map{"reuse": true, "fileID": 99}), nil
					}
					if mode == "invalid-create" {
						return ok(g.Map{}), nil
					}
					return ok(g.Map{"reuse": false, "preuploadID": "pre", "sliceSize": 4, "servers": []string{"http://openapi-upload.123242.com"}}), nil
				case "/upload/v2/file/slice":
					if r.URL.Scheme != "https" {
						t.Error("upload must use TLS")
					}
					if mode == "slice-failure" {
						return reply(400, g.Map{"code": 400}), nil
					}
					reader, e := r.MultipartReader()
					if e != nil {
						t.Fatal(e)
					}
					fields := map[string][]byte{}
					for {
						p, e := reader.NextPart()
						if e == io.EOF {
							break
						}
						if e != nil {
							t.Fatal(e)
						}
						fields[p.FormName()], _ = io.ReadAll(p)
					}
					partHash := md5.Sum(fields["slice"])
					if fields["sliceNo"] == nil || string(fields["sliceMD5"]) != hex.EncodeToString(partHash[:]) {
						t.Error("invalid slice metadata")
					}
					offset := slices * 4
					end := min(offset+4, len(data))
					if string(fields["slice"]) != string(data[offset:end]) {
						t.Error("wrong slice bytes")
					}
					slices++
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
				case "/upload/v2/file/upload_complete":
					completions++
					if mode == "verification-cancel" {
						cancel()
						return reply(200, g.Map{"code": 20103, "message": "文件正在校验中,请间隔1秒后再试"}), nil
					}
					if mode == "completion-failure" {
						return reply(200, g.Map{"code": 20104, "message": "failure"}), nil
					}
					if mode == "verifying" && completions < 3 {
						return reply(200, g.Map{"code": 20103, "message": "文件正在校验中,请间隔1秒后再试"}), nil
					}
					return ok(g.Map{"completed": true, "fileID": 99}), nil
				case "/api/v1/file/detail":
					actual := etag
					if mode == "checksum-failure" {
						actual = "wrong"
					}
					return ok(g.Map{"size": len(data), "etag": actual, "status": 0, "trashed": 0}), nil
				case "/api/v1/file/trash":
					return ok(nil), nil
				}
				t.Fatalf("unexpected %s", r.URL)
				return nil, nil
			})
			path := filepath.Join(t.TempDir(), "upload.bin")
			os.WriteFile(path, data, 0600)
			id, e := c.upload(ctx, path, "file.bin", 0)
			if mode == "verifying" && completions != 3 {
				t.Fatalf("completion calls: %d", completions)
			}
			if mode == "completion-failure" && completions != 1 {
				t.Fatal("retried permanent failure")
			}
			if mode == "multipart" || mode == "reuse" || mode == "verifying" {
				if e != nil || id != 99 {
					t.Fatalf("id %d error %v", id, e)
				}
				if mode == "multipart" && slices != 5 {
					t.Fatalf("slices %d", slices)
				}
			} else if e == nil {
				t.Fatal("expected failure")
			}
		})
	}
}
func TestCancellationAndFiniteRetries(t *testing.T) {
	t.Run("cancel", func(t *testing.T) {
		c := testClient(t, func(r *http.Request) (*http.Response, error) { t.Fatal("request after cancel"); return nil, nil })
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, e := c.request(ctx, "GET", "https://open-api.123pan.com/api/v1/user/info", nil, false)
		if e == nil {
			t.Fatal("expected cancellation")
		}
	})
	t.Run("429", func(t *testing.T) {
		calls := 0
		c := testClient(t, func(r *http.Request) (*http.Response, error) { calls++; return reply(429, g.Map{"code": 429}), nil })
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		_, e := c.request(ctx, "GET", "https://open-api.123pan.com/api/v1/user/info", nil, false)
		if e == nil || calls != 1 {
			t.Fatalf("calls=%d err=%v", calls, e)
		}
	})
	t.Run("network-create", func(t *testing.T) {
		calls := 0
		c := testClient(t, func(r *http.Request) (*http.Response, error) { calls++; return nil, fmt.Errorf("network unavailable") })
		_, e := c.request(context.Background(), "POST", "https://open-api.123pan.com/upload/v2/file/create", nil, false)
		if e == nil || calls != 1 {
			t.Fatalf("non-idempotent create retried: %d", calls)
		}
	})
}
func TestReferenceAndSignature(t *testing.T) {
	ref := Prefix + "7/99/pan123" + strings.Repeat("a", 32) + ".png"
	_, uid, id, e := ParseReference("https://example.com" + ref)
	if e != nil || uid != 7 || id != 99 {
		t.Fatal("invalid reference parse")
	}
	for _, bad := range []string{Prefix + "0/1/a.png", Prefix + "7/99/pan123bad.png", "/resource/uploads/a.png", Prefix + "7/99/../../secret"} {
		if _, _, _, e = ParseReference(bad); e == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	expires := time.Unix(2000000000, 0)
	raw := "https://cdn.example.com/a%20b/image.png"
	signed, e := signPanURL(raw, 7, "private", expires)
	if e != nil {
		t.Fatal(e)
	}
	u, _ := url.Parse(signed)
	parts := strings.Split(u.Query().Get("auth_key"), "-")
	if len(parts) != 4 {
		t.Fatal("invalid auth_key")
	}
	h := md5.Sum([]byte("/a%20b/image.png-" + strings.Join(parts[:3], "-") + "-private"))
	if parts[0] != "2000000000" || parts[2] != "7" || parts[3] != hex.EncodeToString(h[:]) {
		t.Fatal("signature mismatch")
	}
	off := false
	p := Config{URLAuth: &off}
	got, e := profileCDNURL(raw, p, expires)
	if e != nil || got != raw {
		t.Fatal("unsigned mode changed URL")
	}
	if allowedPanURL("https://open-api.123pan.com.evil.test") || validPanCDNURL("http://cdn.example.com/a") || validPanCDNURL("https://127.0.0.1/a") {
		t.Fatal("unsafe destination accepted")
	}
}

func TestConnectionFailureStillRecyclesTestFile(t *testing.T) {
	var etag string
	var size int64
	recycled := 0
	c := testClient(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/v1/access_token":
			return tokenResponse(), nil
		case "/api/v1/user/info":
			return ok(g.Map{"uid": 7}), nil
		case "/upload/v2/file/create":
			var in struct {
				Etag string
				Size int64
			}
			json.NewDecoder(r.Body).Decode(&in)
			etag = in.Etag
			size = in.Size
			return ok(g.Map{"reuse": true, "fileID": 99}), nil
		case "/api/v1/file/detail":
			return ok(g.Map{"etag": etag, "size": size, "status": 0, "trashed": 0}), nil
		case "/api/v1/direct-link/url":
			return ok(g.Map{"url": "https://cdn.example.com/test.png"}), nil
		case "/test.png":
			return reply(403, nil), nil
		case "/api/v1/file/trash":
			recycled++
			return ok(nil), nil
		}
		t.Fatalf("unexpected %s", r.URL)
		return nil, nil
	})
	off := false
	c.p.URLAuth = &off
	b, _ := json.Marshal(c.p)
	key := fmt.Sprintf("%x", sha256.Sum256(b))
	panClients.Store(key, c)
	defer panClients.Delete(key)
	steps, _, e := Test(context.Background(), c.p)
	if e == nil || recycled != 1 {
		t.Fatalf("error=%v recycled=%d", e, recycled)
	}
	if len(steps) < 2 || !steps[len(steps)-1].OK || steps[len(steps)-2].OK {
		t.Fatalf("missing cleanup/failure stages: %+v", steps)
	}
}

func TestMissingStatusCodeIsNotSuccess(t *testing.T) {
	c := testClient(t, func(*http.Request) (*http.Response, error) {
		return reply(200, g.Map{"data": g.Map{"url": "https://cdn.example.com/a"}}), nil
	})
	if _, e := c.request(context.Background(), "GET", "https://open-api.123pan.com/api/v1/direct-link/url", nil, false); e == nil {
		t.Fatal("malformed response accepted as success")
	}
}
