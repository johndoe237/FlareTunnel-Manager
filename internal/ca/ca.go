// Package ca validates CA keys and creates ephemeral transport certificates.
package ca

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const KeyEnv = "FLARETUNNEL_MITM_CA_KEY_B64"
const TransportKeyEnv = "FLARETUNNEL_TRANSPORT_CA_KEY_B64"
const certificateName = "Flaretunnel-MITM-CA.crt"
const transportCertificateName = "Flaretunnel-TRANSPORT-CA.crt"

func resolve(name string) (string, error) {
	var candidates []string
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, "certs", name))
	}
	if executable, err := os.Executable(); err == nil {
		dir := filepath.Dir(executable)
		candidates = append(candidates, filepath.Join(dir, "certs", name), filepath.Join(dir, "..", "share", "flaretunnel-manager", name))
	}
	for _, path := range candidates {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, nil
		}
	}
	return "", fmt.Errorf("packaged public CA certificate %s not found", name)
}
func ResolveCertificate() (string, error)          { return resolve(certificateName) }
func ResolveTransportCertificate() (string, error) { return resolve(transportCertificateName) }

func parseRSAKey(keyB64, envName string) (*rsa.PrivateKey, []byte, error) {
	if strings.TrimSpace(keyB64) == "" {
		return nil, nil, fmt.Errorf("%s is required", envName)
	}
	keyPEM, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil {
		return nil, nil, fmt.Errorf("%s is not valid Base64", envName)
	}
	block, _ := pem.Decode(keyPEM)
	if block == nil {
		return nil, nil, fmt.Errorf("%s does not contain a PEM private key", envName)
	}
	var key *rsa.PrivateKey
	if parsed, e := x509.ParsePKCS1PrivateKey(block.Bytes); e == nil {
		key = parsed
	} else if parsedAny, e := x509.ParsePKCS8PrivateKey(block.Bytes); e == nil {
		var ok bool
		key, ok = parsedAny.(*rsa.PrivateKey)
		if !ok {
			return nil, nil, fmt.Errorf("%s does not contain an RSA private key", envName)
		}
	} else {
		return nil, nil, fmt.Errorf("%s does not contain a valid RSA private key", envName)
	}
	return key, keyPEM, nil
}

func MaterializeKeyFor(certPath, keyB64, keyPath, envName, label string) error {
	key, keyPEM, err := parseRSAKey(keyB64, envName)
	if err != nil {
		return err
	}
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return fmt.Errorf("cannot read %s certificate: %w", label, err)
	}
	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil {
		return fmt.Errorf("%s certificate is not valid PEM", label)
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return fmt.Errorf("%s certificate is invalid: %w", label, err)
	}
	if !cert.IsCA {
		return fmt.Errorf("%s certificate is not a CA", label)
	}
	now := time.Now()
	if now.Before(cert.NotBefore) || now.After(cert.NotAfter) {
		return fmt.Errorf("%s certificate is not currently valid", label)
	}
	publicKey, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok || publicKey.N.Cmp(key.N) != 0 || publicKey.E != key.E {
		return fmt.Errorf("%s does not match the %s certificate", envName, label)
	}
	file, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("cannot materialize %s key: %w", label, err)
	}
	defer file.Close()
	if err := file.Chmod(0o600); err != nil {
		return fmt.Errorf("cannot protect %s key: %w", label, err)
	}
	if _, err := file.Write(keyPEM); err != nil {
		return fmt.Errorf("cannot materialize %s key: %w", label, err)
	}
	return file.Close()
}
func MaterializeKey(certPath, keyB64, keyPath string) error {
	return MaterializeKeyFor(certPath, keyB64, keyPath, KeyEnv, "FlareTunnel MITM CA")
}

func ParseSANs(raw string) ([]string, []net.IP, error) {
	var dns []string
	var ips []net.IP
	for _, value := range strings.Fields(raw) {
		if ip := net.ParseIP(value); ip != nil {
			if ip.IsUnspecified() {
				return nil, nil, fmt.Errorf("FLARETUNNEL_TLS_SAN contains unspecified IP %q", value)
			}
			ips = append(ips, ip)
			continue
		}
		if value == "" || strings.ContainsAny(value, "/,:[]") {
			return nil, nil, fmt.Errorf("FLARETUNNEL_TLS_SAN contains invalid identity %q", value)
		}
		dns = append(dns, value)
	}
	if len(dns) == 0 && len(ips) == 0 {
		return nil, nil, fmt.Errorf("FLARETUNNEL_TLS_SAN must contain at least one DNS name or IP address")
	}
	return dns, ips, nil
}

func GenerateTransportCertificate(caCertPath, caKeyPath, outputCert, outputKey, sanRaw string) error {
	dnsNames, ips, err := ParseSANs(sanRaw)
	if err != nil {
		return err
	}
	caPEM, err := os.ReadFile(caCertPath)
	if err != nil {
		return fmt.Errorf("cannot read transport CA: %w", err)
	}
	cb, _ := pem.Decode(caPEM)
	if cb == nil {
		return fmt.Errorf("transport CA is not PEM")
	}
	caCert, err := x509.ParseCertificate(cb.Bytes)
	if err != nil {
		return err
	}
	keyPEM, err := os.ReadFile(caKeyPath)
	if err != nil {
		return fmt.Errorf("cannot read transport CA key: %w", err)
	}
	kb, _ := pem.Decode(keyPEM)
	if kb == nil {
		return fmt.Errorf("transport CA key is not PEM")
	}
	caKey, err := x509.ParsePKCS1PrivateKey(kb.Bytes)
	if err != nil {
		parsed, e := x509.ParsePKCS8PrivateKey(kb.Bytes)
		if e != nil {
			return fmt.Errorf("transport CA key is invalid")
		}
		var ok bool
		caKey, ok = parsed.(*rsa.PrivateKey)
		if !ok {
			return fmt.Errorf("transport CA key is not RSA")
		}
	}
	serverKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	if err != nil {
		return err
	}
	commonName := "FlareTunnel transport"
	if len(dnsNames) > 0 {
		commonName = dnsNames[0]
	} else if len(ips) > 0 {
		commonName = ips[0].String()
	}
	tmpl := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: commonName}, DNSNames: dnsNames, IPAddresses: ips, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(24 * time.Hour), KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, BasicConstraintsValid: true}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &serverKey.PublicKey, caKey)
	if err != nil {
		return err
	}
	if err := os.WriteFile(outputCert, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644); err != nil {
		return err
	}
	return os.WriteFile(outputKey, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(serverKey)}), 0o600)
}
