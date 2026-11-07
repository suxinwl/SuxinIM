package enterprise

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"openim38.local/identity"
)

func TestHRMAttachmentMultipartKeepsS3SignatureButRemovesBusinessCredentials(t *testing.T) {
	t.Setenv("HRM_SSO_REQUIRED", "true")
	r := miniredis.RunT(t)
	t.Setenv("HRM_REDIS_URL", "redis://"+r.Addr()+"/2")
	t.Setenv("HRM_BRIDGE_HMAC_KEY", strings.Repeat("k", 32))
	t.Setenv("OBJECT_STORAGE_ACCESS_KEY_ID", "fixture-access")
	t.Setenv("OBJECT_STORAGE_REGION", "us-east-1")
	var allowed atomic.Bool
	allowed.Store(true)
	policy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if allowed.Load() {
			io.WriteString(w, `{"errCode":0,"data":{"allowed":true,"securityVersion":1}}`)
		} else {
			io.WriteString(w, `{"errCode":0,"data":{"allowed":false}}`)
		}
	}))
	defer policy.Close()
	t.Setenv("HRM_ACCESS_CHECK_URL", policy.URL+"/internal/im-access/check")
	stamp := time.Now().UTC().Format("20060102T150405Z")
	auth := "AWS4-HMAC-SHA256 Credential=fixture-access/" + stamp[:8] + "/us-east-1/s3/aws4_request, SignedHeaders=host;x-amz-content-sha256;x-amz-date, Signature=" + strings.Repeat("a", 64)
	const rawPath = "/openim38/hash-object"
	const multipartQuery = "partNumber=1&uploadId=fixture-upload"
	var storageCalls, signedCalls int
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		storageCalls++
		if req.Host != "im.szputy.com:9885" || req.URL.Path != rawPath || req.Header.Get("Cookie") != "" || req.Header.Get("token") != "" || req.Header.Get("chatToken") != "" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if req.URL.RawQuery == multipartQuery {
			if req.Header.Get("Authorization") != auth || req.Header.Get("X-Amz-Date") != stamp || req.Header.Get("X-Amz-Content-Sha256") != "UNSIGNED-PAYLOAD" {
				w.WriteHeader(http.StatusForbidden)
				io.WriteString(w, `<Error><Code>SignatureDoesNotMatch</Code></Error>`)
				return
			}
			signedCalls++
		} else if req.Header.Get("Authorization") != "" || req.Header.Get("X-Amz-Date") != "" || req.Header.Get("X-Amz-Content-Sha256") != "" || req.Header.Get("X-Amz-Security-Token") != "" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_, _ = io.Copy(io.Discard, req.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer storage.Close()
	t.Setenv("OBJECT_STORAGE_UPSTREAM", storage.URL)
	state, err := identity.Global()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := state.PutProjection(ctx, identity.Projection{UserID: "emp_fixture", Enabled: true, Version: 1}); err != nil {
		t.Fatal(err)
	}
	session := identity.Session{UserID: "emp_fixture", SessionID: "sid-multipart-fixture-00001", Platform: 5, Epoch: 1, ExpiresAt: time.Now().Add(time.Hour).Unix()}
	if _, err = state.Exchange(ctx, session, "chat", "im"); err != nil {
		t.Fatal(err)
	}
	raw := "https://im.szputy.com:9885" + rawPath + "?" + multipartQuery
	wrapped, err := state.WrapAttachment(ctx, session, "https://192.168.10.11:9885", raw, true)
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{Store: NewMemoryStore()}
	proof, err := state.IssueResourceProof(ctx, session, "https://192.168.10.11:9885")
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, target, authorization string, size int) *httptest.ResponseRecorder {
		q := httptest.NewRequest(method, target, strings.NewReader(strings.Repeat("x", size)))
		q.RemoteAddr = "127.0.0.1:1234"
		q.Header.Set("X-Puty-Entry", "lan")
		q.Header.Set("X-Real-IP", "192.0.2.1")
		if method == "GET" || method == "HEAD" {
			q.Header.Set(resourceProofHeader, proof)
		}
		q.Header.Set("Authorization", authorization)
		q.Header.Set("X-Amz-Date", stamp)
		q.Header.Set("X-Amz-Content-Sha256", "UNSIGNED-PAYLOAD")
		q.Header.Set("Cookie", "business-cookie=fixture")
		if method != "GET" && method != "HEAD" {
			q.Header.Set("token", "business-im-fixture")
			q.Header.Set("chatToken", "business-chat-fixture")
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, q)
		return w
	}
	// The storage controller uses query presigning through 5 MiB, and header
	// signing above it. MIME does not decide which authentication mode is used.
	for _, size := range []int{5*1024*1024 - 1, 5*1024*1024 + 1} {
		target, authorization := wrapped, auth
		if size < 5*1024*1024 {
			target = strings.SplitN(wrapped, "?", 2)[0] + "?X-Amz-Signature=fixture-query"
			authorization = "Bearer business-fixture"
		}
		if w := request("PUT", target, authorization, size); w.Code != http.StatusOK {
			t.Fatalf("size boundary %d lost its storage signature: HTTP %d", size, w.Code)
		}
	}
	if storageCalls != 2 || signedCalls != 1 {
		t.Fatal("small query or multipart signature route was not exercised")
	}
	// A syntactically valid but forged signature must still be rejected by S3;
	// the proxy is not an alternative to the storage engine's HMAC verifier.
	if w := request("PUT", wrapped, strings.Replace(auth, strings.Repeat("a", 64), strings.Repeat("b", 64), 1), 1); w.Code != http.StatusForbidden || storageCalls != 3 {
		t.Fatal("forged multipart signature bypassed storage verification", w.Code)
	}
	before := storageCalls
	for _, authorization := range []string{"", "Bearer business-fixture", "AWS4-malformed"} {
		if w := request("PUT", wrapped, authorization, 1); w.Code != http.StatusForbidden || storageCalls != before {
			t.Fatal("missing or non-storage credentials reached storage", w.Code)
		}
	}
	wrongPath := strings.Replace(wrapped, rawPath, rawPath+"-other", 1)
	if w := request("PUT", wrongPath, auth, 1); w.Code != http.StatusForbidden || storageCalls != before {
		t.Fatal("wrong-path grant reached storage", w.Code)
	}
	allowed.Store(false)
	if w := request("PUT", wrapped, auth, 1); w.Code != http.StatusForbidden || storageCalls != before {
		t.Fatal("current HRM policy denial reached storage", w.Code)
	}
	allowed.Store(true)
	readURL, err := state.WrapAttachment(ctx, session, "https://192.168.10.11:9885", "https://im.szputy.com:9885"+rawPath+"?X-Amz-Signature=fixture-query", false)
	if err != nil {
		t.Fatal(err)
	}
	if w := request("PUT", readURL, auth, 1); w.Code != http.StatusForbidden || storageCalls != before {
		t.Fatal("read grant accepted an upload", w.Code)
	}
	for _, method := range []string{"GET", "HEAD"} {
		if w := request(method, readURL, auth, 0); w.Code != http.StatusOK {
			t.Fatal("read request leaked S3 or business credentials", w.Code)
		}
	}
	avatarCalls := 0
	avatarStorage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		avatarCalls++
		if req.Host != "hr.szputy.com:9880" || req.URL.Path != "/resource/uploads/fixture-avatar.png" || req.Header.Get("Authorization") != "" || req.Header.Get("Cookie") != "" || req.Header.Get("token") != "" || req.Header.Get("chatToken") != "" || req.Header.Get("X-Amz-Date") != "" || req.Header.Get("X-Amz-Content-Sha256") != "" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer avatarStorage.Close()
	t.Setenv("HRM_RESOURCE_UPSTREAM", avatarStorage.URL)
	avatarURL, err := state.WrapAvatar(ctx, session, "https://192.168.10.11:9885", "https://hr.szputy.com:9880/resource/uploads/fixture-avatar.png")
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"GET", "HEAD"} {
		if w := request(method, avatarURL, auth, 0); w.Code != http.StatusOK {
			t.Fatal("avatar request leaked credentials", w.Code)
		}
	}
	if avatarCalls != 2 {
		t.Fatal("avatar read path not exercised")
	}
	before = storageCalls
	if _, err = state.Revoke(ctx, session.UserID, session.SessionID, 0, false, 0); err != nil {
		t.Fatal(err)
	}
	if w := request("PUT", wrapped, auth, 1); w.Code != http.StatusUnauthorized || storageCalls != before {
		t.Fatal("revoked SID upload reached storage", w.Code)
	}
	session.SessionID = "sid-expiry-fixture-00000001"
	session.ExpiresAt = time.Now().Add(2 * time.Second).Unix()
	if _, err = state.Exchange(ctx, session, "chat-2", "im-2"); err != nil {
		t.Fatal(err)
	}
	expiringURL, err := state.WrapAttachment(ctx, session, "https://192.168.10.11:9885", raw, true)
	if err != nil {
		t.Fatal(err)
	}
	r.FastForward(3 * time.Second)
	if w := request("PUT", expiringURL, auth, 1); w.Code != http.StatusUnauthorized || storageCalls != before {
		t.Fatal("expired SID/grant reached storage", w.Code)
	}
}

func TestAttachmentStorageSignatureRejectsContractAmbiguity(t *testing.T) {
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	t.Setenv("OBJECT_STORAGE_ACCESS_KEY_ID", "fixture-access")
	t.Setenv("OBJECT_STORAGE_REGION", "us-east-1")
	stamp := now.Format("20060102T150405Z")
	auth := "AWS4-HMAC-SHA256 Credential=fixture-access/" + stamp[:8] + "/us-east-1/s3/aws4_request, SignedHeaders=host;x-amz-content-sha256;x-amz-date, Signature=" + strings.Repeat("a", 64)
	request := func() *http.Request {
		r := httptest.NewRequest("PUT", "https://192.168.10.11:9885/attachments/fixture/openim38/hash-object?partNumber=1&uploadId=fixture-upload", nil)
		r.Header.Set("Authorization", auth)
		r.Header.Set("X-Amz-Date", stamp)
		r.Header.Set("X-Amz-Content-Sha256", "UNSIGNED-PAYLOAD")
		return r
	}
	grant := identity.Attachment{Write: true, Path: "/openim38/hash-object"}
	if headers, err := attachmentStorageSignature(request(), grant, now); err != nil || len(headers) != 3 || headers.Get("Authorization") != auth {
		t.Fatal("pinned multipart signature was rejected", err)
	}
	cases := map[string]func(*http.Request){
		"duplicate authorization": func(r *http.Request) { r.Header.Add("Authorization", auth) },
		"duplicate date":          func(r *http.Request) { r.Header.Add("X-Amz-Date", stamp) },
		"duplicate hash":          func(r *http.Request) { r.Header.Add("X-Amz-Content-Sha256", "UNSIGNED-PAYLOAD") },
		"duplicate uploadID":      func(r *http.Request) { r.URL.RawQuery += "&uploadId=other" },
		"duplicate part":          func(r *http.Request) { r.URL.RawQuery += "&partNumber=1" },
		"missing uploadID":        func(r *http.Request) { r.URL.RawQuery = "partNumber=1" },
		"missing part":            func(r *http.Request) { r.URL.RawQuery = "uploadId=fixture" },
		"extra query":             func(r *http.Request) { r.URL.RawQuery += "&untrusted=true" },
		"encoded bad query":       func(r *http.Request) { r.URL.RawQuery += "&bad=%" },
		"zero part":               func(r *http.Request) { r.URL.RawQuery = "uploadId=fixture&partNumber=0" },
		"too many parts":          func(r *http.Request) { r.URL.RawQuery = "uploadId=fixture&partNumber=10001" },
		"noncanonical part":       func(r *http.Request) { r.URL.RawQuery = "uploadId=fixture&partNumber=01" },
		"empty uploadID":          func(r *http.Request) { r.URL.RawQuery = "uploadId=&partNumber=1" },
		"control uploadID":        func(r *http.Request) { r.URL.RawQuery = "uploadId=fixture%0A&partNumber=1" },
		"bearer credential":       func(r *http.Request) { r.Header.Set("Authorization", "Bearer fixture") },
		"wrong access key": func(r *http.Request) {
			r.Header.Set("Authorization", strings.Replace(auth, "fixture-access", "other-access", 1))
		},
		"wrong region": func(r *http.Request) {
			r.Header.Set("Authorization", strings.Replace(auth, "us-east-1", "other-region", 1))
		},
		"wrong service": func(r *http.Request) { r.Header.Set("Authorization", strings.Replace(auth, "/s3/", "/sts/", 1)) },
		"extra signed header": func(r *http.Request) {
			r.Header.Set("Authorization", strings.Replace(auth, "SignedHeaders=host;", "SignedHeaders=content-type;host;", 1))
		},
		"short signature": func(r *http.Request) { r.Header.Set("Authorization", strings.TrimSuffix(auth, "a")) },
		"uppercase signature": func(r *http.Request) {
			r.Header.Set("Authorization", strings.Replace(auth, strings.Repeat("a", 64), strings.Repeat("A", 64), 1))
		},
		"bad payload hash":   func(r *http.Request) { r.Header.Set("X-Amz-Content-Sha256", "arbitrary") },
		"session credential": func(r *http.Request) { r.Header.Set("X-Amz-Security-Token", "unsupported") },
		"invalid date":       func(r *http.Request) { r.Header.Set("X-Amz-Date", "bad") },
		"stale date":         func(r *http.Request) { r.Header.Set("X-Amz-Date", now.Add(-16*time.Minute).Format("20060102T150405Z")) },
		"future date":        func(r *http.Request) { r.Header.Set("X-Amz-Date", now.Add(16*time.Minute).Format("20060102T150405Z")) },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			r := request()
			mutate(r)
			if _, err := attachmentStorageSignature(r, grant, now); err == nil || strings.Contains(err.Error(), "Credential=") || strings.Contains(err.Error(), "Signature=") {
				t.Fatal("ambiguous signature was accepted or leaked into an error")
			}
		})
	}
	for _, changed := range []identity.Attachment{{Path: grant.Path}, {Write: true, Kind: "avatar", Path: grant.Path}} {
		if headers, err := attachmentStorageSignature(request(), changed, now); err != nil || len(headers) != 0 {
			t.Fatal("non-storage/read grant enabled caller credentials")
		}
	}
	for _, method := range []string{"GET", "HEAD", "POST", "DELETE"} {
		r := request()
		r.Method = method
		if headers, err := attachmentStorageSignature(r, grant, now); err != nil || len(headers) != 0 {
			t.Fatal("a non-PUT method enabled caller credentials")
		}
	}
}
