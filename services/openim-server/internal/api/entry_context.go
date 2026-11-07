package api

import (
	"github.com/gin-gonic/gin"
	"openim38.local/identity"
)

// Gin's RPC context reads its own keys rather than Request.Context by default.
// Bind the server-derived fact to both; never promote an HTTP customHeader.
func bindRequestEntry(c *gin.Context) {
	ctx := identity.WithRequestEntry(c.Request.Context(), c.Request)
	c.Request = c.Request.WithContext(ctx)
	c.Set(identity.EntryZoneMetadataKey, []string{identity.EntryZoneFromContext(ctx)})
	value, _ := c.Get("customHeader")
	keys, _ := value.([]string)
	keys = append([]string(nil), keys...)
	for _, key := range keys {
		if key == identity.EntryZoneMetadataKey {
			c.Set("customHeader", keys)
			return
		}
	}
	c.Set("customHeader", append(keys, identity.EntryZoneMetadataKey))
}
