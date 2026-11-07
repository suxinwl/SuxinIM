package imbridge

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func certificateFixture(t *testing.T, domain string, until time.Time) certificateSubmission {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	must(t, err)
	leaf := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: domain}, DNSNames: []string{domain}, NotBefore: time.Now().Add(-time.Hour), NotAfter: until, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, leaf, leaf, &key.PublicKey, key)
	must(t, err)
	private, err := x509.MarshalPKCS8PrivateKey(key)
	must(t, err)
	revision := int64(0)
	return certificateSubmission{CertPEM: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})), KeyPEM: string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private})), ExpectedRevision: &revision}
}

func TestCertificateSubmissionChecksHostnameValidityAndKeyWithoutExposingMaterial(t *testing.T) {
	now := time.Now()
	valid := certificateFixture(t, certificateDomain, now.Add(24*time.Hour))
	if _, e := validateCertificateSubmission(valid, now); e != nil {
		t.Fatal("valid keypair rejected")
	}
	for _, bad := range []certificateSubmission{
		certificateFixture(t, "other.example", now.Add(time.Hour)),
		certificateFixture(t, certificateDomain, now.Add(-time.Minute)),
		{CertPEM: valid.CertPEM, KeyPEM: certificateFixture(t, certificateDomain, now.Add(time.Hour)).KeyPEM},
		{CertPEM: valid.CertPEM, KeyPEM: "do-not-print-this-private-data"},
	} {
		if _, e := validateCertificateSubmission(bad, now); !errors.Is(e, ErrInvalid) {
			t.Fatal("unsafe keypair accepted")
		}
	}
	raw, err := json.Marshal(CertificateStatus{Available: true, Domain: certificateDomain, Status: "pending"})
	must(t, err)
	if strings.Contains(string(raw), "certPEM") || strings.Contains(string(raw), "keyPEM") || strings.Contains(string(raw), "PRIVATE KEY") {
		t.Fatal("certificate metadata discloses PEM")
	}
}

func TestCertificateBodyLimitAndStrictRequestShape(t *testing.T) {
	for _, body := range []string{
		`{"certPEM":"x","keyPEM":"y"}`,
		`{"certPEM":"x","keyPEM":"y","expectedRevision":0,"command":"anything"}`,
		`{"certPEM":"x","keyPEM":"y","expectedRevision":0} {}`,
		`{"certPEM":"` + strings.Repeat("x", 256<<10) + `","keyPEM":"y","expectedRevision":0}`,
	} {
		r := httptest.NewRequest("POST", "/certificate", strings.NewReader(body))
		var in certificateSubmission
		if e := decodeCertificate(r, &in); !errors.Is(e, ErrInvalid) {
			t.Fatal("invalid request accepted")
		}
	}
}

func TestCertificateQueueLinuxPrivateAtomicAndRevisioned(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("private systemd certificate inbox is Linux only")
	}
	dir := t.TempDir()
	must(t, os.Chmod(dir, 0700))
	t.Setenv("GOSUXIN_CERTIFICATE_INBOX", dir)
	status, err := certificateStatus()
	must(t, err)
	if !status.Available || status.Revision != 0 || status.Status != "not_configured" {
		t.Fatal("bad empty inbox status")
	}
	in := certificateFixture(t, certificateDomain, time.Now().Add(time.Hour))
	status, err = submitCertificate(in, 99)
	must(t, err)
	if status.Status != "pending" || status.Revision != 1 {
		t.Fatal("submission not queued")
	}
	for _, name := range []string{"request.json", "status.json", ".submit.lock"} {
		info, e := os.Stat(filepath.Join(dir, name))
		must(t, e)
		if info.Mode().Perm() != 0600 {
			t.Fatal("nonprivate certificate queue file", name)
		}
	}
	requestRaw, err := os.ReadFile(filepath.Join(dir, "request.json"))
	must(t, err)
	var request certificateRequest
	must(t, json.Unmarshal(requestRaw, &request))
	if request.Revision != 1 || request.RequestedBy != 99 || request.CertPEM != in.CertPEM || request.KeyPEM != in.KeyPEM {
		t.Fatal("private request did not preserve authorized keypair")
	}
	publicRaw, err := os.ReadFile(filepath.Join(dir, "status.json"))
	must(t, err)
	if strings.Contains(string(publicRaw), "PRIVATE KEY") || strings.Contains(string(publicRaw), "certPEM") || strings.Contains(string(publicRaw), "keyPEM") {
		t.Fatal("status contains private key or PEM")
	}
	if _, err = submitCertificate(in, 99); !errors.Is(err, ErrConflict) {
		t.Fatal("pending submission overwritten")
	}
	// Simulate the fixed root watcher consuming revision 1 with a rejection.
	must(t, os.Remove(filepath.Join(dir, "request.json")))
	status.Status = "rejected"
	status.Message = "证书链验证失败"
	must(t, writeCertificateJSON(filepath.Join(dir, "status.json"), status))
	if _, err = submitCertificate(in, 99); !errors.Is(err, ErrConflict) {
		t.Fatal("consumed revision replayed")
	}
	revision := int64(1)
	in.ExpectedRevision = &revision
	status, err = submitCertificate(in, 99)
	must(t, err)
	if status.Revision != 2 {
		t.Fatal("rejected request did not consume revision")
	}
}

func TestAttachmentCookieUsesIndependentSecureHttpOnlyGrant(t *testing.T) {
	w := httptest.NewRecorder()
	setAttachmentCookie(w, LoginResult{AttachmentToken: "independent-grant", ExpiresAt: time.Now().Add(time.Hour).Unix()}, false)
	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatal("missing attachment cookie")
	}
	c := cookies[0]
	if c.Name != "GoSuxinAttachment" || c.Value != "independent-grant" || !c.Secure || !c.HttpOnly || c.SameSite != 4 || c.Path != "/" || c.MaxAge <= 0 {
		t.Fatal("unsafe attachment cookie")
	}
	w = httptest.NewRecorder()
	setAttachmentCookie(w, LoginResult{}, true)
	if c = w.Result().Cookies()[0]; c.Value != "" || c.MaxAge >= 0 {
		t.Fatal("logout did not clear attachment grant")
	}
}
