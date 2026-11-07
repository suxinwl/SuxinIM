package accountsecurity

import "context"

type entryZoneKey struct{}

// WithEntryZone carries a zone established by the trusted ingress, or by an
// authenticated internal request that included the zone in its signed body.
// Never call it with a public query parameter or a client-supplied header.
// Missing values from older internal producers default to the public policy.
func WithEntryZone(ctx context.Context, zone string) context.Context {
	if zone == "" {
		zone = "internet"
	} else if zone != "lan" && zone != "internet" {
		zone = "invalid"
	}
	return context.WithValue(ctx, entryZoneKey{}, zone)
}

// EntryZone distinguishes an explicit request zone from older non-HTTP jobs
// whose trusted caller still supplies only an address.
func EntryZone(ctx context.Context) (string, bool) {
	zone, present := ctx.Value(entryZoneKey{}).(string)
	return zone, present
}
