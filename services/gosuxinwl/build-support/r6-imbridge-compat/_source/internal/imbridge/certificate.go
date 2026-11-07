package imbridge

import (
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const certificateDomain = "kf.szputy.com"

var errCertificateUnavailable = errors.New("证书管理尚未配置，请联系服务器管理员")

type CertificateStatus struct {
	Available         bool   `json:"available"`
	Revision          int64  `json:"revision"`
	Status            string `json:"status"`
	Domain            string `json:"domain"`
	NotBefore         string `json:"notBefore,omitempty"`
	NotAfter          string `json:"notAfter,omitempty"`
	Issuer            string `json:"issuer,omitempty"`
	Subject           string `json:"subject,omitempty"`
	FingerprintSHA256 string `json:"fingerprintSHA256,omitempty"`
	UpdatedAt         int64  `json:"updatedAt"`
	Message           string `json:"message,omitempty"`
}

type certificateSubmission struct {
	CertPEM          string `json:"certPEM"`
	KeyPEM           string `json:"keyPEM"`
	ExpectedRevision *int64 `json:"expectedRevision"`
}

type certificateRequest struct {
	Revision    int64  `json:"revision"`
	CertPEM     string `json:"certPEM"`
	KeyPEM      string `json:"keyPEM"`
	RequestedAt int64  `json:"requestedAt"`
	RequestedBy int64  `json:"requestedBy"`
}

func decodeCertificate(r *http.Request, out *certificateSubmission) error {
	if r.Method != http.MethodPost {
		return ErrInvalid
	}
	r.Body = http.MaxBytesReader(nil, r.Body, 256<<10)
	defer r.Body.Close()
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(out) != nil || d.Decode(new(any)) != io.EOF || out.ExpectedRevision == nil || *out.ExpectedRevision < 0 {
		return ErrInvalid
	}
	return nil
}

func certificateInbox() (string, error) {
	value := os.Getenv("GOSUXIN_CERTIFICATE_INBOX")
	if runtime.GOOS != "linux" || value == "" || !filepath.IsAbs(value) {
		return "", errCertificateUnavailable
	}
	value = filepath.Clean(value)
	info, err := os.Lstat(value)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0700 {
		return "", errCertificateUnavailable
	}
	return value, nil
}

func readCertificateStatus(dir string) (CertificateStatus, error) {
	result := CertificateStatus{Available: true, Revision: 0, Status: "not_configured", Domain: certificateDomain}
	path := filepath.Join(dir, "status.json")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() > 16<<10 {
		return result, errCertificateUnavailable
	}
	raw, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(raw, &result) != nil || result.Revision < 0 {
		return CertificateStatus{}, errCertificateUnavailable
	}
	switch result.Status {
	case "not_configured", "pending", "applied", "rejected":
	default:
		return CertificateStatus{}, errCertificateUnavailable
	}
	result.Available, result.Domain = true, certificateDomain
	// Only public metadata crosses the API boundary, even if an unexpected
	// field was accidentally written into the private watcher status file.
	for _, value := range []string{result.Message, result.Issuer, result.Subject} {
		if len(value) > 2048 || strings.Contains(value, "-----BEGIN") {
			return CertificateStatus{}, errCertificateUnavailable
		}
	}
	return result, nil
}

func certificateStatus() (CertificateStatus, error) {
	dir, err := certificateInbox()
	if err != nil {
		return CertificateStatus{Available: false, Status: "not_configured", Domain: certificateDomain, Message: errCertificateUnavailable.Error()}, nil
	}
	return readCertificateStatus(dir)
}

func validateCertificateSubmission(in certificateSubmission, now time.Time) (*x509.Certificate, error) {
	if len(in.CertPEM) == 0 || len(in.KeyPEM) == 0 || len(in.CertPEM)+len(in.KeyPEM) > 240<<10 {
		return nil, ErrInvalid
	}
	pair, err := tls.X509KeyPair([]byte(in.CertPEM), []byte(in.KeyPEM))
	if err != nil || len(pair.Certificate) == 0 || len(pair.Certificate) > 8 {
		return nil, ErrInvalid
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil || leaf.IsCA || leaf.VerifyHostname(certificateDomain) != nil || now.Before(leaf.NotBefore) || !now.Before(leaf.NotAfter) {
		return nil, ErrInvalid
	}
	server := len(leaf.ExtKeyUsage) == 0
	for _, usage := range leaf.ExtKeyUsage {
		if usage == x509.ExtKeyUsageAny || usage == x509.ExtKeyUsageServerAuth {
			server = true
		}
	}
	if !server {
		return nil, ErrInvalid
	}
	switch key := leaf.PublicKey.(type) {
	case *rsa.PublicKey:
		if key.N.BitLen() < 2048 {
			return nil, ErrInvalid
		}
	case *ecdsa.PublicKey:
		if key.Curve.Params().BitSize < 256 {
			return nil, ErrInvalid
		}
	default:
		return nil, ErrInvalid
	}
	for _, raw := range pair.Certificate[1:] {
		cert, err := x509.ParseCertificate(raw)
		if err != nil || now.Before(cert.NotBefore) || !now.Before(cert.NotAfter) {
			return nil, ErrInvalid
		}
	}
	return leaf, nil
}

func writeCertificateJSON(path string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".certificate-*")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if err = file.Chmod(0600); err == nil {
		_, err = file.Write(raw)
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(name, path)
}

func submitCertificate(in certificateSubmission, actor int64) (CertificateStatus, error) {
	dir, err := certificateInbox()
	if err != nil {
		return CertificateStatus{}, errCertificateUnavailable
	}
	if actor <= 0 || in.ExpectedRevision == nil || *in.ExpectedRevision < 0 {
		return CertificateStatus{}, ErrInvalid
	}
	leaf, err := validateCertificateSubmission(in, time.Now())
	if err != nil {
		return CertificateStatus{}, err
	}
	unlock, err := lockCertificateInbox(dir)
	if err != nil {
		return CertificateStatus{}, errCertificateUnavailable
	}
	defer unlock()
	old, err := readCertificateStatus(dir)
	if err != nil {
		return old, err
	}
	if old.Revision != *in.ExpectedRevision || old.Status == "pending" {
		return old, ErrConflict
	}
	if _, err = os.Lstat(filepath.Join(dir, "request.json")); !errors.Is(err, os.ErrNotExist) {
		return old, ErrConflict
	}
	sum := sha256.Sum256(leaf.Raw)
	status := CertificateStatus{Available: true, Revision: old.Revision + 1, Status: "pending", Domain: certificateDomain, NotBefore: leaf.NotBefore.UTC().Format(time.RFC3339), NotAfter: leaf.NotAfter.UTC().Format(time.RFC3339), Issuer: leaf.Issuer.String(), Subject: leaf.Subject.String(), FingerprintSHA256: hex.EncodeToString(sum[:]), UpdatedAt: time.Now().Unix(), Message: "已提交，等待服务器验证并装载"}
	if err = writeCertificateJSON(filepath.Join(dir, "status.json"), status); err != nil {
		return CertificateStatus{}, errCertificateUnavailable
	}
	request := certificateRequest{Revision: status.Revision, CertPEM: in.CertPEM, KeyPEM: in.KeyPEM, RequestedAt: status.UpdatedAt, RequestedBy: actor}
	if err = writeCertificateJSON(filepath.Join(dir, "request.json"), request); err != nil {
		// Do not leave a permanently pending queue when submission failed before
		// systemd.path could observe the atomic request file.
		_ = writeCertificateJSON(filepath.Join(dir, "status.json"), old)
		return CertificateStatus{}, errCertificateUnavailable
	}
	return status, nil
}
