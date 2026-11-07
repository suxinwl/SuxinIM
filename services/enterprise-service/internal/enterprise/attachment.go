package enterprise

import (
	"context"
	"errors"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"openim38.local/identity"
)

type attachmentReference struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Path string `json:"path"`
	Kind string `json:"kind,omitempty"`
}

func allowedIMURL(u *url.URL) bool {
	return u.Scheme == "https" && u.User == nil && u.Fragment == "" && (u.Scheme+"://"+u.Host == envOrigin("PUBLIC_LAN_ORIGIN", "https://192.168.10.11:9885") || u.Scheme+"://"+u.Host == envOrigin("PUBLIC_INTERNET_ORIGIN", "https://im.szputy.com:9885"))
}
func (s *Server) attachmentAuthorize(ctx context.Context, a Actor, q request, r *http.Request) (any, error) {
	if e := onlyFields(q, "url objectID purpose"); e != nil {
		return nil, e
	}
	purpose := q.str("purpose")
	if purpose == "" && !q.has("purpose") {
		purpose = "preview"
	}
	if purpose != "preview" && purpose != "save" {
		return nil, bad("invalid attachment purpose")
	}
	origin := entryOrigin(r)
	if origin == "" {
		return nil, forbidden("trusted entry profile required")
	}
	state, e := identity.Global()
	if e != nil {
		return nil, unavailable("identity registry unavailable")
	}
	session, e := state.Live(ctx, a.SessionID)
	if e != nil {
		return nil, unauthorized("session expired or revoked")
	}
	var ref attachmentReference
	id := q.str("objectID")
	if id != "" {
		ref, _, e = load[attachmentReference](ctx, s.Store, "attachment_object:"+id)
		if e != nil {
			return nil, missing("unknown object ID")
		}
	} else {
		raw := q.str("url")
		u, e := url.Parse(raw)
		if e != nil || u.User != nil || u.Fragment != "" || u.Scheme != "https" {
			return nil, bad("attachment URL is outside the configured entries")
		}
		hrm := u.Scheme+"://"+u.Host == envOrigin("PUBLIC_HRM_LAN_ORIGIN", "https://192.168.10.11:9880") || u.Scheme+"://"+u.Host == envOrigin("PUBLIC_HRM_INTERNET_ORIGIN", "https://hr.szputy.com:9880")
		if !allowedIMURL(u) && !(hrm && strings.HasPrefix(u.Path, "/resource/uploads/")) {
			return nil, bad("attachment URL is outside the configured entries")
		}
		switch {
		case strings.HasPrefix(u.Path, "/api/object/"):
			ref.Name = strings.TrimPrefix(u.Path, "/api/object/")
		case strings.HasPrefix(u.Path, "/object/"):
			ref.Name = strings.TrimPrefix(u.Path, "/object/")
		case strings.HasPrefix(u.Path, "/openim38/"):
			ref.Path = u.EscapedPath()
		case strings.HasPrefix(u.Path, "/resource/uploads/") && hrm:
			ref.URL = raw
			ref.Kind = "avatar"
		case strings.HasPrefix(u.Path, "/attachments/"):
			parts := strings.SplitN(strings.TrimPrefix(u.EscapedPath(), "/attachments/"), "/", 2)
			if len(parts) != 2 {
				return nil, bad("invalid attachment grant")
			}
			ref.Path = "/" + parts[1]
			if strings.HasPrefix(ref.Path, "/resource/uploads/") {
				ref.Kind = "avatar"
				ref.URL = envOrigin("PUBLIC_HRM_INTERNET_ORIGIN", "https://hr.szputy.com:9880") + ref.Path
				ref.Path = ""
			} else if !strings.HasPrefix(ref.Path, "/openim38/") {
				return nil, bad("invalid stable attachment path")
			}
		default:
			return nil, bad("URL is not an attachment object")
		}
	}
	raw := ref.URL
	if ref.Name != "" {
		var signed struct {
			URL string `json:"url"`
		}
		if e = s.Upstream.adminCall(ctx, "/object/access_url", map[string]any{"name": ref.Name}, &signed); e != nil {
			return nil, e
		}
		raw = signed.URL
	}
	if ref.Path != "" {
		raw, e = s.presignObject(ctx, origin, ref.Path)
		if e != nil {
			return nil, e
		}
	}
	var wrapped string
	ip, ipErr := identity.ClientIP(r)
	if ipErr != nil {
		return nil, forbidden("client address is unavailable")
	}
	action, resource := purpose, ""
	if ref.Kind == "avatar" {
		resource = ref.URL
		if parsed, err := url.Parse(ref.URL); err == nil {
			resource = parsed.EscapedPath()
		}
		if purpose == "preview" {
			action = "avatar"
		}
	}
	if e = identity.CheckSessionAccess(ctx, session, ip, action, resource); e != nil {
		return nil, attachmentAccessError(e)
	}
	if ref.Kind == "avatar" {
		wrapped, e = state.WrapAvatarPurpose(ctx, session, origin, raw, purpose)
	} else {
		wrapped, e = state.WrapAttachmentPurpose(ctx, session, origin, raw, false, purpose)
	}
	if e != nil {
		return nil, bad("attachment storage origin is not configured")
	}
	canonical, e := url.Parse(raw)
	if e != nil {
		return nil, bad("invalid stable object path")
	}
	// Object/name, old signed URLs and opaque grants all identify the same
	// immutable bucket key. Entry host, SID and signatures never enter its ID.
	id = identity.Hash(ref.Kind + ":" + canonical.EscapedPath())
	if e = s.Store.Save(ctx, document("attachment_object:"+id, "attachment_object", "", ref), 0); e != nil && !errors.Is(e, ErrConflict) {
		return nil, e
	}
	expires := session.ExpiresAt
	if expires > time.Now().Add(14*time.Minute).Unix() {
		expires = time.Now().Add(14 * time.Minute).Unix()
	}
	proof, e := state.IssueResourceProof(ctx, session, origin)
	if e != nil {
		return nil, unavailable("attachment session proof unavailable")
	}
	return map[string]any{"objectID": id, "url": wrapped, "expiresAt": expires, "purpose": purpose, "resourceProof": proof, "resourceProofExpiresAt": session.ExpiresAt}, nil
}
func (s *Server) attachmentProxy(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "HEAD" && r.Method != "PUT" && r.Method != "POST" && r.Method != "DELETE" {
		writeError(w, bad("attachment method rejected"))
		return
	}
	parts := strings.SplitN(strings.TrimPrefix(r.URL.EscapedPath(), "/attachments/"), "/", 2)
	if len(parts) != 2 || len(parts[0]) != 64 {
		writeError(w, unauthorized("attachment grant required"))
		return
	}
	state, e := identity.Global()
	if e != nil {
		writeError(w, unavailable("identity registry unavailable"))
		return
	}
	grant, e := state.Attachment(r.Context(), parts[0])
	if e != nil {
		writeError(w, unauthorized("attachment grant expired or revoked"))
		return
	}
	if "/"+parts[1] != grant.Path || !grant.Write && r.Method != "GET" && r.Method != "HEAD" {
		writeError(w, forbidden("attachment scope does not match"))
		return
	}
	session, e := state.Live(r.Context(), grant.SessionID)
	if e != nil {
		writeError(w, unauthorized("attachment grant expired or revoked"))
		return
	}
	if r.Method == "GET" || r.Method == "HEAD" {
		if grant.Write {
			writeError(w, forbidden("upload grant cannot read an object"))
			return
		}
		session, e = s.requireAttachmentReadSession(r.Context(), state, grant, r)
		if e != nil {
			writeError(w, e)
			return
		}
	}
	ip, e := identity.ClientIP(r)
	if e != nil {
		writeError(w, forbidden("client address is unavailable"))
		return
	}
	action := grant.Purpose
	if r.Method != "GET" && r.Method != "HEAD" {
		action = "upload"
	}
	resource := ""
	target := os.Getenv("OBJECT_STORAGE_UPSTREAM")
	if grant.Kind == "avatar" {
		if action == "preview" {
			action = "avatar"
		}
		resource = grant.Path
		target = os.Getenv("HRM_RESOURCE_UPSTREAM")
	}
	if e = identity.CheckSessionAccess(r.Context(), session, ip, action, resource); e != nil {
		writeError(w, attachmentAccessError(e))
		return
	}
	storageSignature, e := attachmentStorageSignature(r, grant, time.Now())
	if e != nil {
		writeError(w, e)
		return
	}
	// Transfers can outlive a request's initial authorization. Cancel their
	// upstream transport as soon as the SID or HRM policy becomes unavailable.
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	r = r.WithContext(ctx)
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				checkCtx, done := context.WithTimeout(ctx, 3*time.Second)
				_, err := state.Attachment(checkCtx, parts[0])
				if err == nil && (r.Method == "GET" || r.Method == "HEAD") {
					_, err = s.requireAttachmentReadSession(checkCtx, state, grant, r)
				}
				if err == nil {
					current, liveErr := state.Live(checkCtx, grant.SessionID)
					err = liveErr
					if err == nil {
						err = identity.CheckSessionAccess(checkCtx, current, ip, action, resource)
					}
				}
				done()
				if err != nil {
					cancel()
					return
				}
			}
		}
	}()
	upstream, e := url.Parse(target)
	if e != nil || upstream.Scheme != "http" || upstream.Host == "" || upstream.User != nil {
		writeError(w, unavailable("object storage is unavailable"))
		return
	}
	// Only the transport address changes. Signature inputs are the original
	// source Host, escaped bucket path and byte-for-byte query; HEAD verifies the
	// original signed GET while suppressing the response body.
	escaped := "/" + parts[1]
	decoded, e := url.PathUnescape(escaped)
	if e != nil {
		writeError(w, bad("invalid object path"))
		return
	}
	originalMethod := r.Method
	proxy := &httputil.ReverseProxy{Director: func(up *http.Request) {
		up.URL.Scheme = upstream.Scheme
		up.URL.Host = upstream.Host
		up.Host = grant.SourceHost
		up.URL.Path = decoded
		up.URL.RawPath = escaped
		up.Header.Del("Authorization")
		up.Header.Del("Cookie")
		up.Header.Del("token")
		up.Header.Del("chatToken")
		up.Header.Del(resourceProofHeader)
		up.Header.Del("X-Amz-Date")
		up.Header.Del("X-Amz-Content-Sha256")
		up.Header.Del("X-Amz-Security-Token")
		for key, values := range storageSignature {
			up.Header[key] = append([]string(nil), values...)
		}
		if originalMethod == "HEAD" {
			up.Method = "GET"
		}
	}, FlushInterval: -1, ErrorHandler: func(w http.ResponseWriter, _ *http.Request, _ error) {
		writeError(w, unavailable("object transfer failed"))
	}}
	proxy.ModifyResponse = func(resp *http.Response) error {
		resp.Header.Set("Cache-Control", "private, no-store")
		resp.Header.Set("Pragma", "no-cache")
		if originalMethod == "GET" || originalMethod == "HEAD" {
			if grant.Purpose == "save" {
				resp.Header.Set("Content-Disposition", "attachment")
			} else {
				resp.Header.Set("Content-Disposition", "inline")
			}
		}
		if originalMethod == "HEAD" {
			if resp.Body != nil {
				_ = resp.Body.Close()
			}
			resp.Body = http.NoBody
		}
		return nil
	}
	proxy.ServeHTTP(w, r)
}

func attachmentAccessError(err error) error {
	if errors.Is(err, identity.ErrAccessDenied) {
		return forbidden("attachment access denied")
	}
	return unavailable("attachment access authority unavailable")
}
func (s *Server) presignObject(ctx context.Context, origin, path string) (string, error) {
	decoded, e := url.PathUnescape(path)
	if e != nil || !strings.HasPrefix(decoded, "/openim38/") || strings.ContainsAny(decoded, "\\\x00") {
		return "", bad("invalid object path")
	}
	for _, part := range strings.Split(decoded, "/") {
		if part == "." || part == ".." {
			return "", bad("invalid object path")
		}
	}
	endpoint, e := url.Parse(origin)
	if e != nil {
		return "", e
	}
	access, secret := os.Getenv("OBJECT_STORAGE_ACCESS_KEY_ID"), os.Getenv("OBJECT_STORAGE_SECRET_ACCESS_KEY")
	if access == "" || secret == "" {
		return "", unavailable("object signing is unavailable")
	}
	region := os.Getenv("OBJECT_STORAGE_REGION")
	if region == "" {
		region = "us-east-1"
	}
	client, e := minio.New(endpoint.Host, &minio.Options{Secure: true, Region: region, Creds: credentials.NewStaticV4(access, secret, "")})
	if e != nil {
		return "", e
	}
	signed, e := client.PresignedGetObject(ctx, "openim38", strings.TrimPrefix(decoded, "/openim38/"), 15*time.Minute, nil)
	if e != nil {
		return "", e
	}
	return signed.String(), nil
}
