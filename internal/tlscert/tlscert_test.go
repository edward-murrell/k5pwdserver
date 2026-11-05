package tlscert

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writePair writes a self-signed certificate with the given common name, and
// sets both files' modification time to mod.
func writePair(t *testing.T, dir, cn string, mod time.Time) (string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certFile := filepath.Join(dir, "cert.pem")
	keyFile := filepath.Join(dir, "key.pem")
	write(t, certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), mod)
	write(t, keyFile, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), mod)
	return certFile, keyFile
}

func write(t *testing.T, path string, data []byte, mod time.Time) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mod, mod); err != nil {
		t.Fatal(err)
	}
}

func commonName(t *testing.T, r *Reloader) string {
	t.Helper()
	cert, err := r.GetCertificate(nil)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	return leaf.Subject.CommonName
}

func TestReload(t *testing.T) {
	dir := t.TempDir()
	start := time.Now().Add(-time.Minute)
	certFile, keyFile := writePair(t, dir, "first", start)

	r, err := New(certFile, keyFile)
	if err != nil {
		t.Fatal(err)
	}
	if cn := commonName(t, r); cn != "first" {
		t.Fatalf("common name = %q, want first", cn)
	}

	writePair(t, dir, "second", start.Add(time.Second))
	if cn := commonName(t, r); cn != "second" {
		t.Errorf("after renewal common name = %q, want second", cn)
	}

	write(t, certFile, []byte("half written"), start.Add(2*time.Second))
	if cn := commonName(t, r); cn != "second" {
		t.Errorf("after bad write common name = %q, want second", cn)
	}
}

func TestNewFailsOnMissingFiles(t *testing.T) {
	dir := t.TempDir()
	if _, err := New(filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")); err == nil {
		t.Error("New should fail when the files don't exist")
	}
}
