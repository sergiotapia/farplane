package githubapp

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strings"
)

// ParsePrivateKey parses a PKCS#1 or PKCS#8 RSA private key PEM.
func ParsePrivateKey(pemBytes string) (*rsa.PrivateKey, error) {
	pemBytes = strings.TrimSpace(pemBytes)
	if pemBytes == "" {
		return nil, fmt.Errorf("github app private key is empty")
	}
	block, _ := pem.Decode([]byte(pemBytes))
	if block == nil {
		return nil, fmt.Errorf("github app private key: no PEM block")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("github app private key: %w", err)
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("github app private key: not RSA")
	}
	return key, nil
}
