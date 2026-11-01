package tokenverify

import (
	"github.com/golang-jwt/jwt/v4"
	"testing"
	"time"
)

func TestRevocationAcceptsExpiredSignedIdentityButNotAuthentication(t *testing.T) {
	verifier := &Token{Secret: "synthetic-exact-chat", Expires: time.Hour}
	expired := verifier.buildClaims("alice", TokenUser)
	expired.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Hour))
	value, err := jwt.NewWithClaims(jwt.SigningMethodHS256, expired).SignedString([]byte(verifier.Secret))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := verifier.GetToken(value); err == nil {
		t.Fatal("expired token authenticated")
	}
	if user, err := verifier.GetTokenForRevocation(value); err != nil || user != "alice" {
		t.Fatal("expired exact deletion cannot finish")
	}
	forged, err := jwt.NewWithClaims(jwt.SigningMethodHS256, expired).SignedString([]byte("wrong-secret"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := verifier.GetTokenForRevocation(forged); err == nil {
		t.Fatal("forged revocation identity accepted")
	}
}
