package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/tls"
	"crypto/x509"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestCertificateSANMatchingAndReuse(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	dir := t.TempDir()
	result, err := initializeCertificate("kf.szputy.com", dir, now)
	if err != nil {
		t.Fatal(err)
	}
	certBefore := mustRead(t, result.CertificatePath)
	keyBefore := mustRead(t, result.PrivateKeyPath)
	pair, err := tls.X509KeyPair(certBefore, keyBefore)
	if err != nil {
		t.Fatal("generated certificate and key do not match")
	}
	cert, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cert.DNSNames, []string{"kf.szputy.com"}) || len(cert.IPAddresses) != 0 || cert.VerifyHostname("kf.szputy.com") != nil {
		t.Fatal("certificate must contain the requested DNS SAN only")
	}
	if cert.IsCA || !reflect.DeepEqual(cert.ExtKeyUsage, []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}) || cert.KeyUsage != x509.KeyUsageDigitalSignature {
		t.Fatal("certificate must be restricted to TLS server authentication")
	}
	key, ok := pair.PrivateKey.(*ecdsa.PrivateKey)
	if !ok || key.Curve != elliptic.P256() {
		t.Fatal("certificate key must be ECDSA P256")
	}
	if cert.NotAfter.Sub(cert.NotBefore) != certificateLifetime || cert.CheckSignature(cert.SignatureAlgorithm, cert.RawTBSCertificate, cert.Signature) != nil {
		t.Fatal("certificate must be self-signed and valid for fourteen days")
	}
	reused, err := initializeCertificate("kf.szputy.com", dir, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if reused != result || !bytes.Equal(certBefore, mustRead(t, result.CertificatePath)) || !bytes.Equal(keyBefore, mustRead(t, result.PrivateKeyPath)) {
		t.Fatal("reuse must not rewrite the existing certificate pair")
	}
}

func TestPartialPairNeverOverwritten(t *testing.T) {
	for _, existing := range []string{"bootstrap-cert.pem", "bootstrap-key.pem"} {
		t.Run(existing, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, existing)
			marker := []byte("preexisting file must remain unchanged")
			if err := os.WriteFile(path, marker, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := initializeCertificate("kf.szputy.com", dir, time.Now()); err == nil {
				t.Fatal("partial pair must be rejected")
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 1 || !bytes.Equal(marker, mustRead(t, path)) {
				t.Fatal("partial pair rejection must not change either path")
			}
		})
	}
}

func TestInvalidExistingPairRejectedWithoutChanges(t *testing.T) {
	for _, mode := range []string{"wrong-domain", "expired", "not-yet-valid", "mismatched-key"} {
		t.Run(mode, func(t *testing.T) {
			now := time.Now().UTC().Truncate(time.Second)
			dir := t.TempDir()
			result, err := initializeCertificate("kf.szputy.com", dir, now)
			if err != nil {
				t.Fatal(err)
			}
			domain, checkTime := "kf.szputy.com", now
			switch mode {
			case "wrong-domain":
				domain = "another.example.com"
			case "expired":
				checkTime = now.Add(certificateLifetime)
			case "not-yet-valid":
				checkTime = now.Add(-time.Hour)
			case "mismatched-key":
				_, differentKey, _, err := generatePair(domain, now)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(result.PrivateKeyPath, differentKey, 0600); err != nil {
					t.Fatal("prepare mismatched key fixture failed")
				}
			}
			certBefore, keyBefore := mustRead(t, result.CertificatePath), mustRead(t, result.PrivateKeyPath)
			if _, err := initializeCertificate(domain, dir, checkTime); err == nil {
				t.Fatal("invalid existing certificate pair must be rejected")
			}
			if !bytes.Equal(certBefore, mustRead(t, result.CertificatePath)) || !bytes.Equal(keyBefore, mustRead(t, result.PrivateKeyPath)) {
				t.Fatal("validation must not modify existing files")
			}
		})
	}
}

func TestExclusiveWritePreservesExistingKey(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath := filepath.Join(dir, "bootstrap-cert.pem"), filepath.Join(dir, "bootstrap-key.pem")
	marker := []byte("key created between the initial check and exclusive write")
	if err := os.WriteFile(keyPath, marker, 0600); err != nil {
		t.Fatal(err)
	}
	if err := writePairExclusive(certPath, keyPath, []byte("certificate"), []byte("private key")); err == nil {
		t.Fatal("exclusive write must reject a concurrently created key")
	}
	if !bytes.Equal(marker, mustRead(t, keyPath)) {
		t.Fatal("exclusive write must not overwrite the existing key")
	}
	if _, err := os.Stat(certPath); !os.IsNotExist(err) {
		t.Fatal("new empty certificate must be removed after exclusive key creation fails")
	}
}

func TestUnsafeDomainRejectedBeforeDirectoryCreation(t *testing.T) {
	for _, domain := range []string{"../example.com", "*.example.com", "example.com:443", "127.0.0.1", "example.com/path", ""} {
		dir := filepath.Join(t.TempDir(), "not-created")
		if _, err := initializeCertificate(domain, dir, time.Now()); err == nil {
			t.Fatalf("unsafe domain accepted: %q", domain)
		}
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatal("domain validation must precede filesystem writes")
		}
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
