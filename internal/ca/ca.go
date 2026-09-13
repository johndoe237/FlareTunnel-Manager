// Package ca validates and materializes the persistent FlareTunnel CA key.
package ca

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const KeyEnv = "FLARETUNNEL_CA_KEY_B64"

const certificateName = "Flaretunnel-CA.crt"

// ResolveCertificate finds the packaged public CA without requiring a
// deployment-specific path. The Docker image uses its internal application
// share directory; local runs can use the repository's certs directory or a
// certs directory next to the executable.
func ResolveCertificate() (string, error) {
	var candidates []string
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, "certs", certificateName))
	}
	if executable, err := os.Executable(); err == nil {
		dir := filepath.Dir(executable)
		candidates = append(candidates,
			filepath.Join(dir, "certs", certificateName),
			filepath.Join(dir, "..", "share", "flaretunnel-manager", certificateName),
		)
	}
	for _, path := range candidates {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, nil
		}
	}
	return "", fmt.Errorf("packaged public CA certificate %s not found", certificateName)
}

// MaterializeKey decodes and validates the private key, verifies that it
// matches certPath, and writes it to keyPath with owner-only permissions.
// Secret bytes are never included in returned errors.
func MaterializeKey(certPath, keyB64, keyPath string) error {
	if keyB64 == "" {
		return fmt.Errorf("%s is required", KeyEnv)
	}
	keyPEM, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil {
		return fmt.Errorf("%s is not valid Base64", KeyEnv)
	}
	block, _ := pem.Decode(keyPEM)
	if block == nil {
		return fmt.Errorf("%s does not contain a PEM private key", KeyEnv)
	}

	var privateKey *rsa.PrivateKey
	if parsed, parseErr := x509.ParsePKCS1PrivateKey(block.Bytes); parseErr == nil {
		privateKey = parsed
	} else if parsedAny, parseErr := x509.ParsePKCS8PrivateKey(block.Bytes); parseErr == nil {
		var ok bool
		privateKey, ok = parsedAny.(*rsa.PrivateKey)
		if !ok {
			return fmt.Errorf("%s does not contain an RSA private key", KeyEnv)
		}
	} else {
		return fmt.Errorf("%s does not contain a valid RSA private key", KeyEnv)
	}

	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return fmt.Errorf("cannot read FlareTunnel CA certificate: %w", err)
	}
	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil {
		return fmt.Errorf("FlareTunnel CA certificate is not valid PEM")
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return fmt.Errorf("FlareTunnel CA certificate is invalid: %w", err)
	}
	if !cert.IsCA {
		return fmt.Errorf("FlareTunnel CA certificate is not a CA")
	}
	now := time.Now()
	if now.Before(cert.NotBefore) || now.After(cert.NotAfter) {
		return fmt.Errorf("FlareTunnel CA certificate is not currently valid")
	}
	publicKey, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok || publicKey.N.Cmp(privateKey.N) != 0 || publicKey.E != privateKey.E {
		return fmt.Errorf("%s does not match the FlareTunnel CA certificate", KeyEnv)
	}

	file, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("cannot materialize FlareTunnel CA key: %w", err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return fmt.Errorf("cannot protect FlareTunnel CA key: %w", err)
	}
	if _, err := file.Write(keyPEM); err != nil {
		_ = file.Close()
		return fmt.Errorf("cannot materialize FlareTunnel CA key: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("cannot close FlareTunnel CA key: %w", err)
	}
	return nil
}
