package auth

import (
	"context"
	"github.com/golang-jwt/jwt/v5"
	"github.com/suxinwl/GoSuxin/framework/container/gvar"
	"github.com/suxinwl/GoSuxin/framework/crypto/gaes"
	"github.com/suxinwl/GoSuxin/framework/encoding/gbase64"
	"testing"
	"time"
)

// Security checks use synthetic keys so they do not depend on deployment secrets.
func useTestAuthKeys(t *testing.T) {
	t.Helper()
	previousSecret, previousEncryption := SecretKey, EncryptKey
	SecretKey = gvar.New("unit-test-jwt-signing-secret")
	EncryptKey = gvar.New("0123456789abcdef0123456789abcdef")
	t.Cleanup(func() { SecretKey, EncryptKey = previousSecret, previousEncryption })
}

func TestJWTRejectsOtherAlgorithmsAndInvalidSignatures(t *testing.T) {
	useTestAuthKeys(t)
	claims := CustomClaims{Data: map[string]interface{}{"uid": 1}, RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}}
	for _, method := range []*jwt.SigningMethodHMAC{jwt.SigningMethodHS384, jwt.SigningMethodHS512} {
		encoded, err := jwt.NewWithClaims(method, claims).SignedString([]byte(SecretKey.String()))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = JwtParseToken(encoded); err == nil {
			t.Fatal("accepted alternate algorithm")
		}
		if _, err = JwtOnlyParseToken(encoded); err == nil {
			t.Fatal("accepted alternate algorithm in expiry-free parser")
		}
	}
	bad, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("different-signing-key"))
	if _, err := JwtOnlyParseToken(bad); err == nil {
		t.Fatal("accepted invalid signature")
	}
	expired := claims
	expired.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Hour))
	encoded, err := createToken(expired)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = JwtParseToken(encoded); err == nil {
		t.Fatal("accepted expired token")
	}
	if _, err = JwtOnlyParseToken(encoded); err != nil {
		t.Fatalf("expiry-free compatibility: %v", err)
	}
}

func TestDecryptTokenRejectsTruncatedPlaintext(t *testing.T) {
	useTestAuthKeys(t)
	for _, plain := range []string{"", "short", "01234567890123456789012345678901"} {
		encrypted, err := gaes.Encrypt([]byte(plain), []byte(EncryptKey.String()))
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err = DecryptToken(context.Background(), string(gbase64.Encode(encrypted))); err == nil {
			t.Fatal("accepted truncated token")
		}
	}
}
