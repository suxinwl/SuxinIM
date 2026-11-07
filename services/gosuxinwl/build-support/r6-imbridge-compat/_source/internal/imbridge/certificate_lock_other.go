//go:build !linux

package imbridge

func lockCertificateInbox(string) (func(), error) { return nil, errCertificateUnavailable }
