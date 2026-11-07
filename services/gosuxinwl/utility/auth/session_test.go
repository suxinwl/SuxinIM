package auth

import (
	"context"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/suxinwl/GoSuxin/framework/container/gvar"
	"github.com/suxinwl/GoSuxin/framework/util/guid"
)

func testSession(t *testing.T) (string, string, *TokenData) {
	t.Helper()
	useTestAuthKeys(t)
	timeout, refresh, multi, prefix := TokenTimeout, MaxRefresh, MultiLogin, TokenCacheKey
	TokenTimeout, MaxRefresh, MultiLogin, TokenCacheKey = gvar.New(13800), gvar.New(600), gvar.New(false), gvar.New("auth-session-test-"+guid.S())
	t.Cleanup(func() { TokenTimeout, MaxRefresh, MultiLogin, TokenCacheKey = timeout, refresh, multi, prefix })
	key := guid.S()
	token, err := GenerateToken(context.Background(), key, map[string]interface{}{"uid": 71})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = RemoveToken(context.Background(), key) })
	data, _, err := GetTokenData(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	return key, token, data
}

func changeTestExpiry(t *testing.T, key string, data *TokenData, expiry time.Time) {
	t.Helper()
	claims, err := JwtOnlyParseToken(data.JwtToken)
	if err != nil {
		t.Fatal(err)
	}
	claims.ExpiresAt = jwt.NewNumericDate(expiry)
	data.JwtToken, err = createToken(*claims)
	if err != nil {
		t.Fatal(err)
	}
	if err = setCache(TokenCacheKey.String()+key, data); err != nil {
		t.Fatal(err)
	}
}

func TestFourHourSessionAndRefreshWindow(t *testing.T) {
	key, opaque, data := testSession(t)
	claims, err := JwtParseToken(data.JwtToken)
	if err != nil {
		t.Fatal(err)
	}
	if remaining := time.Until(claims.ExpiresAt.Time); remaining > 4*time.Hour || remaining < 4*time.Hour-2*time.Second {
		t.Fatalf("session lifetime = %s, want 4 hours", remaining)
	}
	original := data.JwtToken
	if IsRefresh(original) || !IsEffective(key, data) || data.JwtToken != original {
		t.Fatal("a fresh session must not refresh outside the configured window")
	}
	changeTestExpiry(t, key, data, time.Now().Add(5*time.Minute))
	if !IsRefresh(data.JwtToken) || !IsEffective(key, data) {
		t.Fatal("valid session in final 10 minutes was not refreshed")
	}
	current, _, err := GetTokenData(context.Background(), opaque)
	if err != nil {
		t.Fatal(err)
	}
	claims, err = JwtParseToken(current.JwtToken)
	if err != nil || time.Until(claims.ExpiresAt.Time) < 4*time.Hour-2*time.Second || current.UuId != data.UuId {
		t.Fatal("refresh must preserve the opaque session identity and extend to 4 hours")
	}
}

func TestMultiLoginRetainsOnlyIdenticalSecurityClaims(t *testing.T) {
	key, _, _ := testSession(t)
	MultiLogin = gvar.New(true)
	ctx := context.Background()
	firstClaims := map[string]interface{}{"uid": 71, "hrmAssetEpoch": 1, "hrmAssetCredential": "first"}
	first, err := GenerateToken(ctx, key, firstClaims)
	if err != nil {
		t.Fatal(err)
	}
	again, err := GenerateToken(ctx, key, firstClaims)
	if err != nil || again != first {
		t.Fatal("unchanged authenticated claims must retain the live opaque session")
	}
	for _, changed := range []map[string]interface{}{
		{"uid": 71, "hrmAssetEpoch": 1, "hrmAssetCredential": "changed-password"},
		{"uid": 71, "hrmAssetEpoch": 2, "hrmAssetCredential": "changed-password"},
	} {
		next, err := GenerateToken(ctx, key, changed)
		if err != nil || next == first {
			t.Fatal("changed security state must rotate the opaque UUID")
		}
		if _, _, err = GetTokenData(ctx, first); err == nil {
			t.Fatal("the previous security snapshot remained authenticated")
		}
		first = next
	}
}

func TestRefreshRejectsExpiredRevokedAndReplacedSnapshots(t *testing.T) {
	for _, reason := range []string{"expired", "logout", "new login"} {
		t.Run(reason, func(t *testing.T) {
			key, opaque, snapshot := testSession(t)
			changeTestExpiry(t, key, snapshot, time.Now().Add(5*time.Minute))
			var replacement string
			switch reason {
			case "expired":
				changeTestExpiry(t, key, snapshot, time.Now().Add(-time.Minute))
			case "logout":
				if err := RemoveToken(context.Background(), key); err != nil {
					t.Fatal(err)
				}
			case "new login":
				var err error
				replacement, err = GenerateToken(context.Background(), key, map[string]interface{}{"uid": 72})
				if err != nil {
					t.Fatal(err)
				}
			}
			if doRefresh(key, snapshot) {
				t.Fatal("stale or expired session was refreshed")
			}
			if reason != "expired" {
				if _, _, err := GetTokenData(context.Background(), opaque); err == nil {
					t.Fatal("revoked snapshot restored the old token")
				}
			}
			if replacement != "" {
				data, _, err := GetTokenData(context.Background(), replacement)
				if err != nil || data.UuId == snapshot.UuId {
					t.Fatal("new login was overwritten by an old request")
				}
			}
		})
	}
}
