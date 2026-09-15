package ca

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testPair(t *testing.T, dir string) (string, string, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Flaretunnel-CA"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPath := filepath.Join(dir, "Flaretunnel-MITM-CA.crt")
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644); err != nil {
		t.Fatal(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	return certPath, base64.StdEncoding.EncodeToString(keyPEM), filepath.Join(dir, "Flaretunnel-MITM-CA.key")
}

func TestMaterializeKeyValidatesAndProtectsMatchingKey(t *testing.T) {
	dir := t.TempDir()
	certPath, keyB64, keyPath := testPair(t, dir)
	if err := MaterializeKey(certPath, keyB64, keyPath); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("key permissions = %o, want 600", info.Mode().Perm())
	}
}

func TestMaterializeKeyRejectsMissingAndMismatchedKey(t *testing.T) {
	dir := t.TempDir()
	certPath, _, keyPath := testPair(t, dir)
	if err := MaterializeKey(certPath, "", keyPath); err == nil {
		t.Fatal("missing key should fail")
	}
	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	otherPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(otherKey)})
	if err := MaterializeKey(certPath, base64.StdEncoding.EncodeToString(otherPEM), keyPath); err == nil {
		t.Fatal("mismatched key should fail")
	}
}
