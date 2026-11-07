package api

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/openimsdk/tools/mcontext"
	"net/url"
	"openim38.local/identity"
	"strings"
	"time"
)

// Upload handles belong to the initiating SID; refreshing that SID is allowed,
// while a replacement login must initiate its own upload authorization.
func hrmUpload(c *gin.Context, uploadID string, create bool) error {
	if !identity.Required() || !identity.Managed(mcontext.GetOpUserID(c)) || uploadID == "" {
		return nil
	}
	state, err := identity.Global()
	if err != nil {
		return err
	}
	v, err := state.Validate(c.Request.Context(), c.GetHeader("token"), mcontext.GetOpUserID(c), "im", 0)
	if err != nil {
		return err
	}
	key := "HRM38:UPLOAD:" + identity.Hash(uploadID+"\x00"+v.SessionID)
	if create {
		return state.Write(c.Request.Context(), key, v.SessionID, time.Until(time.Unix(v.ExpiresAt, 0)))
	}
	var sid string
	if err = state.Read(c.Request.Context(), key, &sid); err != nil || sid != v.SessionID {
		return identity.ErrSession
	}
	return nil
}

// Wrap only presigned storage URLs. Stable /object/name references deliberately
// remain stable and are reauthorized by the client for its current entry.
func hrmObjectResponse(c *gin.Context, response any, write bool) error {
	if !identity.Required() || !identity.Managed(mcontext.GetOpUserID(c)) {
		return nil
	}
	state, e := identity.Global()
	if e != nil {
		return e
	}
	v, e := state.Validate(c.Request.Context(), c.GetHeader("token"), mcontext.GetOpUserID(c), "im", 0)
	if e != nil {
		return e
	}
	origin := identity.EntryOrigin(c.Request)
	if origin == "" {
		return identity.ErrSession
	}
	data, e := json.Marshal(response)
	if e != nil {
		return e
	}
	var value any
	if e = json.Unmarshal(data, &value); e != nil {
		return e
	}
	var walk func(any) error
	walk = func(item any) error {
		switch item := item.(type) {
		case map[string]any:
			for key, child := range item {
				if key == "url" || key == "URL" {
					if raw, ok := child.(string); ok && strings.Contains(raw, "/openim38") {
						parsed, err := url.Parse(raw)
						if err != nil || parsed.Path != "/openim38" && !strings.HasPrefix(parsed.Path, "/openim38/") {
							continue
						}
						wrapped, e := state.WrapAttachment(c.Request.Context(), v, origin, raw, write)
						if e != nil {
							return e
						}
						item[key] = wrapped
						continue
					}
				}
				if e := walk(child); e != nil {
					return e
				}
			}
		case []any:
			for _, child := range item {
				if e := walk(child); e != nil {
					return e
				}
			}
		}
		return nil
	}
	if e = walk(value); e != nil {
		return e
	}
	data, e = json.Marshal(value)
	if e != nil {
		return e
	}
	return json.Unmarshal(data, response)
}
