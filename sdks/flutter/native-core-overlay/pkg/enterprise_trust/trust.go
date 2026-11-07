// Package enterprise_trust augments platform roots with the constrained PUTY
// enterprise root. Verification, hostname checks and TLS 1.2+ remain enabled.
package enterprise_trust

import (
 "crypto/tls"
 "crypto/x509"
 _ "embed"
 "net/http"
 "github.com/gorilla/websocket"
)

//go:embed puty_enterprise_root_ca.crt
var rootPEM []byte

func init() {
 roots, err := x509.SystemCertPool()
 if err != nil || roots == nil { roots = x509.NewCertPool() }
 if !roots.AppendCertsFromPEM(rootPEM) { panic("invalid PUTY enterprise root") }
 transport := http.DefaultTransport.(*http.Transport).Clone()
 transport.TLSClientConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
 http.DefaultTransport = transport
 websocket.DefaultDialer.TLSClientConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
}
