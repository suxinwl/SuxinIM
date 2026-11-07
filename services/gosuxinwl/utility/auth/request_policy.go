package auth

import (
	"sync/atomic"

	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
)

type tokenRequestPolicy struct {
	check    func(*ghttp.Request, *CustomClaims) error
	accepted func(*ghttp.Request, *CustomClaims)
}

var requestPolicy atomic.Pointer[tokenRequestPolicy]

// SetTokenRequestPolicy installs an optional deployment policy. It runs after
// verifying the cached token but before refreshing it or issuing plugin cookies.
// The returned cleanup restores the prior policy only while this one is current.
func SetTokenRequestPolicy(check func(*ghttp.Request, *CustomClaims) error, accepted ...func(*ghttp.Request, *CustomClaims)) func() {
	policy := &tokenRequestPolicy{check: check}
	if len(accepted) != 0 {
		policy.accepted = accepted[0]
	}
	previous := requestPolicy.Swap(policy)
	return func() { requestPolicy.CompareAndSwap(policy, previous) }
}
