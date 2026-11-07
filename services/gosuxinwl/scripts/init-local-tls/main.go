// init-local-tls creates a short-lived HTTPS certificate for domain file validation.
// It never installs certificates into an operating system trust store.
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const certificateLifetime = 14 * 24 * time.Hour

type certificateResult struct {
	CertificatePath string
	PrivateKeyPath  string
	NotBefore       time.Time
	NotAfter        time.Time
}

func main() {
	domain := flag.String("domain", "kf.szputy.com", "DNS hostname for the temporary certificate")
	dir := flag.String("dir", "", "output directory (default: runtime/tls/<domain>)")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "init-local-tls: unexpected positional arguments")
		os.Exit(1)
	}
	result, err := initializeCertificate(*domain, *dir, time.Now())
	if err != nil {
		fmt.Fprintln(os.Stderr, "init-local-tls:", err)
		os.Exit(1)
	}
	fmt.Printf("Certificate: %s\nPrivate key: %s\nValid from: %s\nValid until: %s\n",
		result.CertificatePath, result.PrivateKeyPath,
		result.NotBefore.UTC().Format(time.RFC3339), result.NotAfter.UTC().Format(time.RFC3339))
	fmt.Println("Temporary self-signed certificate for validation only; replace it with the issued certificate. Browser trust is not installed.")
}

func initializeCertificate(domain, dir string, now time.Time) (certificateResult, error) {
	var result certificateResult
	domain = strings.ToLower(strings.TrimSpace(domain))
	if !validDNSName(domain) {
		return result, errors.New("domain must be an ASCII DNS hostname without a wildcard, port, or path")
	}
	if dir == "" {
		dir = filepath.Join("runtime", "tls", domain)
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return result, fmt.Errorf("resolve certificate directory: %w", err)
	}
	result.CertificatePath = filepath.Join(dir, "bootstrap-cert.pem")
	result.PrivateKeyPath = filepath.Join(dir, "bootstrap-key.pem")
	certExists, err := regularFileExists(result.CertificatePath)
	if err != nil {
		return result, err
	}
	keyExists, err := regularFileExists(result.PrivateKeyPath)
	if err != nil {
		return result, err
	}
	if certExists != keyExists {
		return result, errors.New("only one certificate pair file exists; refusing to overwrite or repair a partial pair")
	}
	if certExists {
		certificate, err := readValidPair(result.CertificatePath, result.PrivateKeyPath, domain, now)
		if err != nil {
			return result, err
		}
		result.NotBefore, result.NotAfter = certificate.NotBefore, certificate.NotAfter
		return result, nil
	}

	certPEM, keyPEM, certificate, err := generatePair(domain, now)
	if err != nil {
		return result, err
	}
	// On Windows, the deployment script sets the directory ACL before invoking us.
	// The mode below restricts newly created directories on systems with POSIX modes.
	if err := os.MkdirAll(dir, 0700); err != nil {
		return result, fmt.Errorf("create certificate directory: %w", err)
	}
	if err := writePairExclusive(result.CertificatePath, result.PrivateKeyPath, certPEM, keyPEM); err != nil {
		return result, err
	}
	result.NotBefore, result.NotAfter = certificate.NotBefore, certificate.NotAfter
	return result, nil
}

func validDNSName(domain string) bool {
	if len(domain) == 0 || len(domain) > 253 || !strings.Contains(domain, ".") {
		return false
	}
	for _, label := range strings.Split(domain, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return false
			}
		}
	}
	// A numeric address needs an IP SAN; this tool intentionally issues DNS SANs only.
	lastLabel := domain[strings.LastIndexByte(domain, '.')+1:]
	return strings.IndexFunc(lastLabel, func(c rune) bool { return c < '0' || c > '9' }) >= 0
}

func regularFileExists(path string) (bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect certificate pair path: %w", err)
	}
	if !info.Mode().IsRegular() {
		return false, errors.New("certificate pair paths must be regular files, not directories or links")
	}
	return true, nil
}

func readValidPair(certPath, keyPath, domain string, now time.Time) (*x509.Certificate, error) {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, fmt.Errorf("read certificate file: %w", err)
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("read private key file: %w", err)
	}
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil || len(pair.Certificate) == 0 {
		return nil, errors.New("existing certificate and private key do not form a valid matching pair; files left unchanged")
	}
	certificate, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return nil, errors.New("existing certificate cannot be parsed; files left unchanged")
	}
	if len(certificate.DNSNames) == 0 || certificate.VerifyHostname(domain) != nil {
		return nil, errors.New("existing certificate DNS SAN does not match the requested domain; files left unchanged")
	}
	if now.Before(certificate.NotBefore) || !now.Before(certificate.NotAfter) {
		return nil, errors.New("existing certificate is expired or not yet valid; files left unchanged")
	}
	if certificate.IsCA || len(certificate.ExtKeyUsage) != 1 || certificate.ExtKeyUsage[0] != x509.ExtKeyUsageServerAuth {
		return nil, errors.New("existing certificate is not restricted to TLS server authentication; files left unchanged")
	}
	return certificate, nil
}

func generatePair(domain string, now time.Time) ([]byte, []byte, *x509.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, nil, errors.New("generate temporary certificate private key failed")
	}
	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return nil, nil, nil, errors.New("generate certificate serial number failed")
	}
	serial.Add(serial, big.NewInt(1))
	start := now.UTC().Truncate(time.Second)
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: domain},
		DNSNames:              []string{domain},
		NotBefore:             start,
		NotAfter:              start.Add(certificateLifetime),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, nil, errors.New("create temporary certificate failed")
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, nil, errors.New("encode temporary certificate private key failed")
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, nil, errors.New("verify generated certificate failed")
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), certificate, nil
}

func writePairExclusive(certPath, keyPath string, certPEM, keyPEM []byte) error {
	// Acquire both paths exclusively before writing either one. Never truncate files.
	certFile, err := os.OpenFile(certPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("create certificate file exclusively: %w", err)
	}
	keyFile, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		_ = certFile.Close()
		_ = os.Remove(certPath) // Only the empty file created by this invocation.
		return fmt.Errorf("create private key file exclusively: %w", err)
	}
	complete := false
	defer func() {
		_ = certFile.Close()
		_ = keyFile.Close()
		if !complete {
			_ = os.Remove(certPath)
			_ = os.Remove(keyPath)
		}
	}()
	if _, err := keyFile.Write(keyPEM); err != nil {
		return errors.New("write private key file failed")
	}
	if _, err := certFile.Write(certPEM); err != nil {
		return errors.New("write certificate file failed")
	}
	if err := keyFile.Sync(); err != nil {
		return errors.New("flush private key file failed")
	}
	if err := certFile.Sync(); err != nil {
		return errors.New("flush certificate file failed")
	}
	if err := keyFile.Close(); err != nil {
		return errors.New("close private key file failed")
	}
	if err := certFile.Close(); err != nil {
		return errors.New("close certificate file failed")
	}
	complete = true
	return nil
}
